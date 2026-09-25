package accounts

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

// MaxNotaDaPosse é o teto da observação obrigatória, no mesmo espírito do motivo
// do bloqueio: cabe uma frase, e não um relatório.
const MaxNotaDaPosse = 500

// ErrNotaDaPosse é a recusa de soltar uma posse sem dizer por quê.
var ErrNotaDaPosse = errors.New("accounts: a soltura da posse precisa de observacao")

// ErrSemPosse é a recusa de soltar o que não está preso.
var ErrSemPosse = errors.New("accounts: a conta nao esta presa a execucao nenhuma")

// Posse é o que a tela mostra sobre a conta estar presa a uma execução do jogo.
type Posse struct {
	Presa bool
	// Epoca é a execução do tmServer que está com ela.
	Epoca int64
	// Desde e UltimoBatimento respondem a pergunta que decide o clique: o dono
	// está vivo? Batimento de segundos atrás é um servidor rodando; de minutos, é
	// um processo que se foi.
	Desde           *time.Time
	UltimoBatimento *time.Time
}

// PosseDaConta lê o estado da posse para a tela.
func (s *Store) PosseDaConta(ctx context.Context, accountID int64) (Posse, error) {
	var p Posse
	var epoca *int64
	err := s.pool.QueryRow(ctx,
		`SELECT dono_epoca, dono_desde, dono_batimento FROM account WHERE id = $1`, accountID).
		Scan(&epoca, &p.Desde, &p.UltimoBatimento)
	if errors.Is(err, pgx.ErrNoRows) {
		return Posse{}, ErrNotFound
	}
	if err != nil {
		return Posse{}, fmt.Errorf("accounts: ler posse: %w", err)
	}
	if epoca != nil {
		p.Presa, p.Epoca = true, *epoca
	}
	return p, nil
}

// SoltarPosse é a válvula: devolve à mão uma conta presa a uma execução do jogo.
//
// POR QUE ELA EXISTE. A posse é o que impede a mesma conta de estar em jogo em
// dois servidores, e ela se solta sozinha — no save de saída, ou pelo prazo do
// batimento. Mas se algum dia ela NÃO se soltar, o jogador fica de fora da própria
// conta esperando um conserto que só um deploy traria. Com o botão, a staff
// destrava agora.
//
// ELA NÃO É SEGURA SOZINHA, e isto tem de estar escrito. Soltar à mão a posse de um
// processo VIVO abre a porta para um segundo login na mesma conta — exatamente o que
// a posse existe para impedir. Quem fecha essa porta é o BATIMENTO do lado do jogo: o
// processo que perdeu a conta descobre no próximo batimento (ela não volta no
// RETURNING), grava o que dá e derruba a sessão. Sem esse par, esta função seria uma
// maneira de produzir o defeito à mão.
//
// É por isso que a tela mostra há quanto tempo foi o último batimento: o clique só é
// seguro quando o dono não está mais batendo.
//
// A OBSERVAÇÃO É OBRIGATÓRIA e a auditoria vai na MESMA transação. Soltar uma posse
// é dizer "o dono desta conta não existe mais", e essa afirmação pode estar errada:
// se estiver, alguém precisa poder ler depois quem disse e por quê. Auditoria
// gravada fora da transação é auditoria que pode faltar justamente na vez em que a
// soltura deu errado.
func (s *Store) SoltarPosse(ctx context.Context, atorConta, atorPainel int64, papel string,
	accountID int64, nota string,
) error {
	nota = strings.TrimSpace(nota)
	if nota == "" || len(nota) > MaxNotaDaPosse {
		return ErrNotaDaPosse
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("accounts: begin soltar posse: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var epoca *int64
	var batimento *time.Time
	err = tx.QueryRow(ctx,
		`SELECT dono_epoca, dono_batimento FROM account WHERE id = $1 FOR UPDATE`, accountID).
		Scan(&epoca, &batimento)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return fmt.Errorf("accounts: ler posse para soltar: %w", err)
	}
	if epoca == nil {
		// Nada a fazer, e dizer isso é melhor que fingir que soltou: quem clicou
		// está investigando um jogador que não consegue entrar, e "soltei" o faria
		// procurar a causa no lugar errado.
		return ErrSemPosse
	}

	if _, err := tx.Exec(ctx,
		`UPDATE account SET dono_epoca = NULL, dono_desde = NULL, dono_batimento = NULL
		  WHERE id = $1`, accountID); err != nil {
		return fmt.Errorf("accounts: soltar posse: %w", err)
	}
	// A auditoria aqui dentro, e não pelo caminho comum: ver a nota da função.
	var ator, painel any
	if atorConta != 0 {
		ator = atorConta
	}
	if atorPainel != 0 {
		painel = atorPainel
	}
	if (ator == nil) == (painel == nil) {
		return fmt.Errorf("accounts: soltar posse sem ator ou com dois: conta=%d painel=%d",
			atorConta, atorPainel)
	}
	antes := fmt.Sprintf(`{"epoca":%d,"batimento":%q}`, *epoca, horaOuVazio(batimento))
	depois := fmt.Sprintf(`{"epoca":null,"observacao":%q}`, nota)
	if _, err := tx.Exec(ctx, `
		INSERT INTO admin_audit_log
		    (actor_account_id, actor_painel_usuario_id, actor_role, action,
		     target_account_id, old_value, new_value)
		VALUES ($1, $2, $3, 'SOLTAR_POSSE_DA_CONTA', $4, $5, $6)`,
		ator, painel, papel, accountID, antes, depois); err != nil {
		return fmt.Errorf("accounts: auditoria da soltura da posse: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("accounts: commit soltar posse: %w", err)
	}
	return nil
}

func horaOuVazio(t *time.Time) string {
	if t == nil {
		return ""
	}
	return t.UTC().Format(time.RFC3339)
}
