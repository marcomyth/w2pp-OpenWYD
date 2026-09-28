package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/jeanluca/w2pp-openwyd/internal/emblema"
)

// IntervaloEntreTrocasDeEmblema é o limite de uma troca por guilda.
//
// POR GUILDA, e não por conta: o custo da troca é a imagem mudando para todos os
// membros de uma vez. E o REMOVER conta como troca — senão alternar pôr-e-tirar
// burlaria o limite sem esforço nenhum.
const IntervaloEntreTrocasDeEmblema = 5 * time.Minute

// Erros da troca de emblema. Cada um é uma resposta prevista, e a diferença entre
// eles é o que a página diz ao líder: "você não lidera", "essa imagem não serve",
// "espere um pouco".
var (
	ErrNaoLideraAGuilda = errors.New("store: nenhum personagem desta conta lidera esta guilda")
	ErrEmblemaInvalido  = errors.New("store: a imagem nao e o emblema que o jogo desenha")
	ErrEmblemaMuitoCedo = errors.New("store: o emblema desta guilda foi trocado ha pouco")
)

// GuildaQueLidero é uma guilda liderada por algum personagem da conta.
type GuildaQueLidero struct {
	ID        uint16
	Nome      string
	TemBrasao bool
}

// GuildasQueLidero lista as guildas lideradas por QUALQUER personagem da conta.
//
// PLURAL de propósito: uma conta tem até quatro personagens, e nada impede dois
// deles liderarem guildas diferentes. Uma tela que assuma uma só erra no dia em que
// isso acontecer.
func (s *Store) GuildasQueLidero(ctx context.Context, accountID int64) ([]GuildaQueLidero, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT g.id, g.name, g.emblema IS NOT NULL
		  FROM guild_member m
		  JOIN guild g ON g.id = m.guild_id
		 WHERE m.account_id = $1 AND m.guild_level = 9
		 ORDER BY g.name`, accountID)
	if err != nil {
		return nil, fmt.Errorf("store: listar guildas que lidero a=%d: %w", accountID, err)
	}
	defer rows.Close()
	var out []GuildaQueLidero
	for rows.Next() {
		var g GuildaQueLidero
		var id int
		if err := rows.Scan(&id, &g.Nome, &g.TemBrasao); err != nil {
			return nil, fmt.Errorf("store: lendo guilda que lidero: %w", err)
		}
		g.ID = uint16(id)
		out = append(out, g)
	}
	return out, rows.Err()
}

// SalvarEmblemaDaGuilda grava o emblema, conferindo liderança, formato e prazo.
//
// TUDO NA MESMA TRANSAÇÃO, com a linha da guilda trancada: a conferência do prazo e
// a gravação são a mesma decisão. Separadas, duas requisições que chegassem juntas
// passariam as duas pela conferência antes de qualquer uma gravar — e o limite que
// existe para poupar os membros de uma imagem piscando não existiria.
//
// A liderança é conferida CONTRA O BANCO e não contra o que o site mandou: a sessão
// do site diz de quem é a conta, e só o banco sabe quem lidera o quê.
func (s *Store) SalvarEmblemaDaGuilda(ctx context.Context, accountID int64, guildID uint16, imagem []byte) error {
	if !emblema.Valido(imagem) {
		return ErrEmblemaInvalido
	}
	return s.inTx(ctx, func(tx pgx.Tx) error {
		if err := podeTrocarEmblema(ctx, tx, accountID, guildID); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx,
			`UPDATE guild SET emblema = $2, emblema_trocado_em = now() WHERE id = $1`,
			guildID, imagem); err != nil {
			return fmt.Errorf("store: gravar emblema g=%d: %w", guildID, err)
		}
		return nil
	})
}

// LimparEmblemaDaGuilda tira o emblema, com as mesmas conferências.
//
// O PRAZO VALE AQUI TAMBÉM. Sem isso, pôr e tirar alternadamente passaria por cima
// do limite, e o limite existe justamente para os membros não verem a imagem
// piscando.
func (s *Store) LimparEmblemaDaGuilda(ctx context.Context, accountID int64, guildID uint16) error {
	return s.inTx(ctx, func(tx pgx.Tx) error {
		if err := podeTrocarEmblema(ctx, tx, accountID, guildID); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx,
			`UPDATE guild SET emblema = NULL, emblema_trocado_em = now() WHERE id = $1`,
			guildID); err != nil {
			return fmt.Errorf("store: limpar emblema g=%d: %w", guildID, err)
		}
		return nil
	})
}

// podeTrocarEmblema tranca a guilda e responde as três perguntas: ela existe, esta
// conta a lidera, e já passou o prazo.
func podeTrocarEmblema(ctx context.Context, tx pgx.Tx, accountID int64, guildID uint16) error {
	var trocadoEm *time.Time
	err := tx.QueryRow(ctx,
		`SELECT emblema_trocado_em FROM guild WHERE id = $1 FOR UPDATE`, guildID).Scan(&trocadoEm)
	if errors.Is(err, pgx.ErrNoRows) {
		// Guilda que não existe e guilda que não é sua dão a MESMA resposta de
		// propósito: um site que distinguisse as duas contaria, para quem tentasse
		// números na mão, quais guildas existem.
		return ErrNaoLideraAGuilda
	}
	if err != nil {
		return fmt.Errorf("store: trancar guilda g=%d: %w", guildID, err)
	}
	var lidera bool
	if err := tx.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM guild_member
			 WHERE guild_id = $1 AND account_id = $2 AND guild_level = 9
		)`, guildID, accountID).Scan(&lidera); err != nil {
		return fmt.Errorf("store: conferir lideranca g=%d a=%d: %w", guildID, accountID, err)
	}
	if !lidera {
		return ErrNaoLideraAGuilda
	}
	if trocadoEm != nil && time.Since(*trocadoEm) < IntervaloEntreTrocasDeEmblema {
		return ErrEmblemaMuitoCedo
	}
	return nil
}

// EmblemaDaGuilda devolve a imagem e quando ela foi posta.
//
// O BOOLEANO É SEPARADO DA IMAGEM porque, sem ele, "não tem emblema" e "falhei ao
// ler" viram o mesmo slice vazio — e a página não saberia se mostra o padrão ou um
// aviso. Guilda que não existe devolve tem=false, e não erro: para quem lê a imagem,
// as duas coisas pedem a mesma resposta.
func (s *Store) EmblemaDaGuilda(ctx context.Context, guildID uint16) (imagem []byte, trocadoEm time.Time, tem bool, err error) {
	var img []byte
	var em *time.Time
	e := s.pool.QueryRow(ctx,
		`SELECT emblema, emblema_trocado_em FROM guild WHERE id = $1`, guildID).Scan(&img, &em)
	if errors.Is(e, pgx.ErrNoRows) {
		return nil, time.Time{}, false, nil
	}
	if e != nil {
		return nil, time.Time{}, false, fmt.Errorf("store: ler emblema g=%d: %w", guildID, e)
	}
	if len(img) == 0 {
		return nil, time.Time{}, false, nil
	}
	if em != nil {
		trocadoEm = *em
	}
	return img, trocadoEm, true, nil
}
