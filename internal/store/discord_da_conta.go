package store

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// Erros do vínculo do Discord. Cada um é uma resposta prevista, e a diferença entre
// eles é o que a página diz à pessoa.
var (
	// ErrDiscordInvalido: não é um snowflake numérico.
	ErrDiscordInvalido = errors.New("store: o id do discord nao e um snowflake")
	// ErrDiscordEmOutraConta: este Discord já está vinculado a OUTRA conta.
	ErrDiscordEmOutraConta = errors.New("store: este discord ja esta em outra conta")
	// ErrContaTemOutroDiscord: esta conta já tem OUTRO Discord.
	//
	// TROCAR É RECUSADO, e só a staff desfaz. Sem isso, quem tomasse uma conta
	// trocaria o vínculo em silêncio e levaria junto o cargo que o Discord dá.
	ErrContaTemOutroDiscord = errors.New("store: esta conta ja tem outro discord")
)

// maxSnowflake é o comprimento máximo que um snowflake do Discord alcança.
//
// Ele é um inteiro de 64 bits sem sinal impresso em decimal, então 20 dígitos é o
// teto aritmético. O corte existe para uma string absurda não virar uma linha no
// banco, e não para adivinhar o formato: o que vale é ser só dígitos.
const maxSnowflake = 20

// snowflakeValido confere o formato sem tentar saber se a conta existe no Discord.
//
// SÓ DÍGITOS, e pelo menos um. Não dá para validar de verdade sem falar com o
// Discord, e não é o servidor de jogo que fala — quem faz o OAuth é o site, e é ele
// que garante que o id veio de lá. Esta conferência é a rede contra um campo
// digitado à mão ou um cliente remendado.
func snowflakeValido(id string) bool {
	if id == "" || len(id) > maxSnowflake {
		return false
	}
	for _, r := range id {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

// VincularDiscord guarda o Discord da conta, ou recusa dizendo por quê.
//
// TUDO NUMA TRANSAÇÃO, com a linha da conta trancada: ler "esta conta já tem outro"
// e depois gravar seriam duas decisões, e duas requisições que chegassem juntas
// passariam as duas pela leitura antes de qualquer uma gravar.
//
// GRAVAR O MESMO DISCORD DE NOVO PASSA. É o caso de quem refaz o OAuth, e recusar
// ali mandaria a pessoa pedir ajuda para uma coisa que já está do jeito que ela quer.
func (s *Store) VincularDiscord(ctx context.Context, accountID int64, discordID string) error {
	if !snowflakeValido(discordID) {
		return ErrDiscordInvalido
	}
	return s.inTx(ctx, func(tx pgx.Tx) error {
		var atual *string
		err := tx.QueryRow(ctx,
			`SELECT discord_id FROM account WHERE id = $1 FOR UPDATE`, accountID).Scan(&atual)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound
		}
		if err != nil {
			return fmt.Errorf("store: lendo o discord da conta a=%d: %w", accountID, err)
		}
		if atual != nil {
			if *atual == discordID {
				return nil // o mesmo de novo: nada a fazer, e não é erro
			}
			return ErrContaTemOutroDiscord
		}

		// O ÍNDICE ÚNICO É QUEM DECIDE, e não uma consulta antes dele. Uma leitura
		// "já existe em outra conta?" seguida do INSERT deixaria a fresta entre as
		// duas; o banco fecha essa fresta sozinho, e aqui só se traduz a recusa dele.
		_, err = tx.Exec(ctx, `UPDATE account SET discord_id = $2 WHERE id = $1`, accountID, discordID)
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return ErrDiscordEmOutraConta
		}
		if err != nil {
			return fmt.Errorf("store: gravando o discord a=%d: %w", accountID, err)
		}
		return nil
	})
}

// DiscordDaConta devolve o Discord vinculado, ou vazio quando não há.
func (s *Store) DiscordDaConta(ctx context.Context, accountID int64) (string, error) {
	var id *string
	err := s.pool.QueryRow(ctx,
		`SELECT discord_id FROM account WHERE id = $1`, accountID).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", ErrNotFound
	}
	if err != nil {
		return "", fmt.Errorf("store: lendo o discord a=%d: %w", accountID, err)
	}
	if id == nil {
		return "", nil
	}
	return *id, nil
}

// DesvincularDiscord tira o vínculo. É a saída da staff para a conta que trocou de
// dono ou vinculou errado — o próprio jogador não desfaz, senão a trava de "esta
// conta já tem outro" não valeria nada.
func (s *Store) DesvincularDiscord(ctx context.Context, accountID int64) error {
	tag, err := s.pool.Exec(ctx,
		`UPDATE account SET discord_id = NULL WHERE id = $1`, accountID)
	if err != nil {
		return fmt.Errorf("store: desvinculando o discord a=%d: %w", accountID, err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}
