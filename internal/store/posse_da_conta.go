package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

// PrazoDaPosse é quanto tempo uma posse sobrevive sem sinal de vida.
//
// Três vezes o intervalo do batimento: uma pausa de coletor de lixo ou um soluço
// de rede não podem tirar a conta de quem está jogando. Do outro lado, é o tempo
// máximo que alguém espera para voltar depois de o servidor cair sem soltar nada.
const PrazoDaPosse = 90 * time.Second

// IntervaloDoBatimento é de quanto em quanto tempo o dono diz que está vivo.
const IntervaloDoBatimento = 30 * time.Second

// ErrContaEmUso é a recusa de tomar uma conta que outra execução ainda tem.
var ErrContaEmUso = errors.New("store: a conta esta em jogo em outra execucao")

// TomarPosseDaConta marca esta execução como dona da conta, ou recusa.
//
// A TOMADA É UMA INSTRUÇÃO SÓ, e tem de ser: ler antes e escrever depois deixaria
// justamente a fresta que a trava existe para fechar. Ela passa em três casos, e
// nenhum outro:
//
//   - a conta não tem dono;
//   - o dono sou eu (relogin no mesmo processo, ou um retry);
//   - o dono não dá sinal de vida há mais que o prazo.
//
// Devolve ErrContaEmUso quando outra execução viva está com ela.
func (s *Store) TomarPosseDaConta(ctx context.Context, accountID, epoca int64) error {
	if epoca <= 0 {
		// Sem época não há posse: é o servidor sem banco de ordem, e aí a trava de
		// dentro do processo é tudo o que existe — o mesmo de antes deste código.
		return nil
	}
	tag, err := s.pool.Exec(ctx, `
		UPDATE account
		   SET dono_epoca = $2, dono_desde = now(), dono_batimento = now()
		 WHERE id = $1
		   AND (dono_epoca IS NULL
		        OR dono_epoca = $2
		        OR dono_batimento IS NULL
		        OR dono_batimento < now() - $3::interval)`,
		accountID, epoca, PrazoDaPosse)
	if err != nil {
		return fmt.Errorf("store: tomar posse da conta a=%d: %w", accountID, err)
	}
	if tag.RowsAffected() == 0 {
		// Nenhuma linha: ou a conta não existe, ou é de outro. São coisas
		// diferentes e o login trata cada uma de um jeito.
		var existe bool
		if err := s.pool.QueryRow(ctx, `SELECT true FROM account WHERE id = $1`, accountID).Scan(&existe); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return ErrNotFound
			}
			return fmt.Errorf("store: conferindo a conta da posse a=%d: %w", accountID, err)
		}
		return ErrContaEmUso
	}
	return nil
}

// SoltarPosseDaConta devolve a conta, e SÓ se ela for minha.
//
// O `dono_epoca = $2` não é zelo: sem ele, um processo soltaria a posse de outro —
// e o caso em que isso aconteceria é justamente aquele em que a posse acabou de
// mudar de mão, que é quando ela mais importa.
func (s *Store) SoltarPosseDaConta(ctx context.Context, accountID, epoca int64) error {
	if epoca <= 0 {
		return nil
	}
	return s.inTx(ctx, func(tx pgx.Tx) error {
		return soltarPosseTx(ctx, tx, accountID, epoca)
	})
}

// soltarPosseTx é a soltura dentro de uma transação que já existe.
//
// O save final chama por aqui: a posse sai na MESMA transação em que o personagem e
// a carga entram. Deixa de ser uma ordem entre duas chamadas e passa a ser uma coisa
// só — e save que falha mantém a posse, que é o certo: conta cujo último save não
// caiu é justamente a que ninguém deve carregar.
func soltarPosseTx(ctx context.Context, tx pgx.Tx, accountID, epoca int64) error {
	if epoca <= 0 {
		return nil
	}
	if _, err := tx.Exec(ctx, `
		UPDATE account SET dono_epoca = NULL, dono_desde = NULL, dono_batimento = NULL
		 WHERE id = $1 AND dono_epoca = $2`, accountID, epoca); err != nil {
		return fmt.Errorf("store: soltar posse da conta a=%d: %w", accountID, err)
	}
	return nil
}

// BaterPelasContas diz "ainda estou vivo" pelas contas desta execução e devolve
// QUAIS continuam sendo dela.
//
// O RETORNO É O PONTO. Quem não voltou na lista deixou de ser meu — o banco ficou
// lento, o prazo venceu, e outra execução tomou a conta. Continuar jogando nela
// seria a divergência que esta trava existe para impedir, então quem chama grava o
// que dá e derruba a sessão.
func (s *Store) BaterPelasContas(ctx context.Context, epoca int64, contas []int64) ([]int64, error) {
	if epoca <= 0 || len(contas) == 0 {
		return nil, nil
	}
	rows, err := s.pool.Query(ctx, `
		UPDATE account SET dono_batimento = now()
		 WHERE id = ANY($1) AND dono_epoca = $2
		 RETURNING id`, contas, epoca)
	if err != nil {
		return nil, fmt.Errorf("store: batimento da posse: %w", err)
	}
	defer rows.Close()
	var minhas []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("store: lendo o batimento: %w", err)
		}
		minhas = append(minhas, id)
	}
	return minhas, rows.Err()
}
