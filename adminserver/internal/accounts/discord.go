package accounts

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
)

// MaxNotaDoDiscord é o teto da observação obrigatória, no mesmo espírito do motivo do
// bloqueio: cabe uma frase, e não um relatório.
const MaxNotaDoDiscord = 500

// ErrNotaDoDiscord é a recusa de desvincular sem dizer por quê.
var ErrNotaDoDiscord = errors.New("accounts: a soltura do discord precisa de observacao")

// ErrSemDiscord é a recusa de desvincular o que não está vinculado.
var ErrSemDiscord = errors.New("accounts: a conta nao tem discord vinculado")

// DesvincularDiscord tira o Discord de uma conta, à mão.
//
// POR QUE ELA EXISTE. O jogador NÃO troca o próprio vínculo: trocar é recusado, senão
// quem tomasse uma conta trocaria o Discord em silêncio e levaria junto o cargo que ele
// dá. Essa recusa só é sustentável porque existe esta saída — sem ela, "só a staff
// desfaz" seria uma frase sem ninguém atrás, e quem vinculou errado ficaria preso para
// sempre.
//
// A OBSERVAÇÃO É OBRIGATÓRIA e a auditoria vai na MESMA transação. Desvincular é abrir
// a porta para outra pessoa pegar aquele Discord, e se isso for feito por engano — ou a
// pedido de quem não era o dono — a única coisa que sobra para reconstruir a história é
// esta linha. Auditoria gravada fora da transação é auditoria que pode faltar
// justamente na vez em que a decisão deu errado.
//
// O DISCORD ANTIGO VAI PARA A AUDITORIA, e é EXCEÇÃO DELIBERADA à regra do 0141 de
// que o id não entra em registro nosso. Não é descuido: a auditoria só a staff lê, e
// ali o número É o objeto da decisão — sem ele a linha diria "alguém desvinculou algo",
// que daqui a um mês não responde nada.
//
// O log de aplicação continua dizendo só a CONTA, como manda o 0141. A diferença entre
// os dois é quem lê: o log vai para a plataforma, a auditoria fica no banco e tem dono.
func (s *Store) DesvincularDiscord(ctx context.Context, atorConta, atorPainel int64, papel string,
	accountID int64, nota string,
) error {
	nota = strings.TrimSpace(nota)
	if nota == "" || len(nota) > MaxNotaDoDiscord {
		return ErrNotaDoDiscord
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("accounts: begin desvincular discord: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var atual *string
	err = tx.QueryRow(ctx,
		`SELECT discord_id FROM account WHERE id = $1 FOR UPDATE`, accountID).Scan(&atual)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return fmt.Errorf("accounts: ler discord para desvincular: %w", err)
	}
	if atual == nil {
		// Dizer isso é melhor que fingir que desvinculou: quem clicou está
		// investigando alguém que não consegue vincular, e um "pronto" o faria
		// procurar a causa no lugar errado.
		return ErrSemDiscord
	}

	if _, err := tx.Exec(ctx,
		`UPDATE account SET discord_id = NULL WHERE id = $1`, accountID); err != nil {
		return fmt.Errorf("accounts: desvincular discord: %w", err)
	}

	var ator, painel any
	if atorConta != 0 {
		ator = atorConta
	}
	if atorPainel != 0 {
		painel = atorPainel
	}
	if (ator == nil) == (painel == nil) {
		return fmt.Errorf("accounts: desvincular discord sem ator ou com dois: conta=%d painel=%d",
			atorConta, atorPainel)
	}
	antes := fmt.Sprintf(`{"discord_id":%q}`, *atual)
	depois := fmt.Sprintf(`{"discord_id":null,"observacao":%q}`, nota)
	if _, err := tx.Exec(ctx, `
		INSERT INTO admin_audit_log
		    (actor_account_id, actor_painel_usuario_id, actor_role, action,
		     target_account_id, old_value, new_value)
		VALUES ($1, $2, $3, 'DESVINCULAR_DISCORD', $4, $5, $6)`,
		ator, painel, papel, accountID, antes, depois); err != nil {
		return fmt.Errorf("accounts: auditoria da soltura do discord: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("accounts: commit desvincular discord: %w", err)
	}
	return nil
}
