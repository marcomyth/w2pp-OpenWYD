package accounts

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
)

// Esconder uma conta do ranking, sem dar cargo a ela.
//
// O ranking do site e do bot já não mostra quem tem cargo (admin ou moderator). Isso
// resolve a equipe de direito e NÃO resolve quem testa o jogo com conta comum — e dar
// cargo de moderador a alguém só para sumir de uma lista entregaria junto os comandos de
// GM, que é caro demais para o problema.
//
// Por isso a marca é separada do cargo, e não dá poder nenhum: ela esconde, e só.

// AcaoForaDoRanking é a ação gravada na auditoria.
const AcaoForaDoRanking = "FORA_DO_RANKING"

// MaxNotaForaDoRanking limita a observação. Ela é opcional — diferente da nota do
// pagamento à mão, aqui não há dinheiro em jogo e exigir texto para marcar três contas
// viraria "teste" digitado três vezes, que não informa nada a ninguém.
const MaxNotaForaDoRanking = 300

// ErrNotaLonga é a observação acima do limite.
var ErrNotaLonga = errors.New("accounts: observacao longa demais")

// ForaDoRanking marca ou desmarca uma conta, e devolve se algo mudou.
//
// DEVOLVE `mudou` EM VEZ DE ERRO quando o estado já era o pedido: marcar o que já está
// marcado não é falha de ninguém — é o segundo clique, ou duas pessoas resolvendo a
// mesma coisa. A tela diz "nada mudou" e ninguém vai procurar defeito.
//
// A AUDITORIA VAI NA MESMA TRANSAÇÃO da escrita. Gravar depois abre a janela em que a
// conta some do ranking e não há linha dizendo quem a escondeu — e "sumi do ranking e
// ninguém sabe por quê" é exatamente o tipo de reclamação que não se resolve sem
// registro.
//
// O FOR UPDATE existe pelo mesmo motivo de sempre: duas marcações ao mesmo tempo
// gravariam duas auditorias, uma delas dizendo que mudou algo que já estava mudado.
func (s *Store) ForaDoRanking(ctx context.Context, atorConta, atorPainel int64, papel string,
	targetID int64, fora bool, nota string,
) (mudou bool, err error) {
	if len(nota) > MaxNotaForaDoRanking {
		return false, ErrNotaLonga
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return false, fmt.Errorf("accounts: begin fora do ranking: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var atual bool
	err = tx.QueryRow(ctx,
		`SELECT fora_do_ranking FROM account WHERE id = $1 FOR UPDATE`, targetID).Scan(&atual)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, ErrNotFound
	}
	if err != nil {
		return false, fmt.Errorf("accounts: lendo fora do ranking: %w", err)
	}
	if atual == fora {
		return false, nil
	}

	if _, err := tx.Exec(ctx,
		`UPDATE account SET fora_do_ranking = $2 WHERE id = $1`, targetID, fora); err != nil {
		return false, fmt.Errorf("accounts: gravando fora do ranking: %w", err)
	}

	antes, _ := json.Marshal(map[string]any{"fora_do_ranking": atual})
	depois, _ := json.Marshal(map[string]any{"fora_do_ranking": fora, "observacao": nota})
	var ator, painel *int64
	if atorConta != 0 {
		ator = &atorConta
	}
	if atorPainel != 0 {
		painel = &atorPainel
	}
	// Um ator e só um, como em toda escrita do painel: conta do jogo OU usuário do
	// painel. Os dois, ou nenhum, é bug de quem chamou, e uma auditoria sem dono é
	// pior que nenhuma porque parece completa.
	if (ator == nil) == (painel == nil) {
		return false, fmt.Errorf("accounts: fora do ranking sem ator ou com dois: conta=%d painel=%d",
			atorConta, atorPainel)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO admin_audit_log
		    (actor_account_id, actor_painel_usuario_id, actor_role, action,
		     target_account_id, old_value, new_value)
		VALUES ($1, $2, $3, $4, $5, $6, $7)`,
		ator, painel, papel, AcaoForaDoRanking, targetID, antes, depois); err != nil {
		return false, fmt.Errorf("accounts: auditoria do fora do ranking: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return false, fmt.Errorf("accounts: commit fora do ranking: %w", err)
	}
	return true, nil
}
