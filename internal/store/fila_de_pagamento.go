package store

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

// PrazoDoPagamento é o que a regra promete ao vendedor: 48 horas.
//
// CORRIDAS, e não horas úteis. A janela de atendimento — das 15h às 21h, em dias úteis —
// é promessa ao jogador, e NÃO regra de código (decisão da Hanna, 25/09/2026): contar
// só hora útil aqui faria a tela discordar do que o jogo diz, e obrigaria este servidor
// a saber feriado brasileiro para responder "vence quando".
const PrazoDoPagamento = 48 * time.Hour

// AcaoChavePixRevelada é o nome, na auditoria, de "alguém leu a chave inteira".
const AcaoChavePixRevelada = "REPASSE_CHAVE_REVELADA"

// PagamentoNaFila é uma dívida esperando a staff pagar, como a tela mostra.
//
// SÓ MÁSCARA AQUI. A chave e o CPF inteiros são dado pessoal, e uma lista os põe na tela
// de uma vez, para todas as linhas, em toda visita — inclusive quando a staff só passou
// para ver quantas faltavam. Quem vai pagar pede a linha dela, e essa leitura fica
// registrada (ChaveParaPagar).
type PagamentoNaFila struct {
	ID             int64
	VendedorConta  int64
	ValorCentavos  int64
	ChaveMascarada string
	TipoChave      TipoChavePix
	DocMascarado   string
	CriadoEm       time.Time
	// VenceEm é CriadoEm + PrazoDoPagamento, calculado aqui e não no template: é a
	// promessa feita ao vendedor, e a tela que decide a ordem de pagar não deve ter de
	// fazer conta de data para saber o que está atrasado.
	VenceEm time.Time
}

// FilaDePagamentoAMao lista o que a staff tem para pagar, do mais antigo para o mais
// novo — a ordem em que a promessa vence.
//
// O FILTRO É O MESMO da antiga fila do saque automático: pendente e com documento
// cadastrado. Quem ainda não cadastrou chave ou CPF não entra aqui, porque não há onde
// pagar; esse caso tem contador próprio e a tela dele já existe.
func (s *Store) FilaDePagamentoAMao(ctx context.Context, limite int) ([]PagamentoNaFila, error) {
	if limite <= 0 {
		limite = 200
	}
	rows, err := s.pool.Query(ctx, `
		SELECT r.id, r.vendedor_conta, r.valor_centavos, d.chave, d.tipo, d.documento,
		       r.criado_em
		  FROM rmt_repasse r
		  JOIN rmt_recebedor d ON d.account_id = r.vendedor_conta
		 WHERE r.status = $1 AND d.documento IS NOT NULL
		 ORDER BY r.criado_em, r.id
		 LIMIT $2`, repassePendente, limite)
	if err != nil {
		return nil, fmt.Errorf("store: lendo a fila de pagamento: %w", err)
	}
	defer rows.Close()

	var fila []PagamentoNaFila
	for rows.Next() {
		var p PagamentoNaFila
		var chave, doc string
		var tipo int16
		if err := rows.Scan(&p.ID, &p.VendedorConta, &p.ValorCentavos,
			&chave, &tipo, &doc, &p.CriadoEm); err != nil {
			return nil, fmt.Errorf("store: lendo linha da fila de pagamento: %w", err)
		}
		// A MÁSCARA É FEITA AQUI, e o valor inteiro nem sai desta função. Mascarar na
		// camada de cima significaria o inteiro viajar até lá e depender de ninguém
		// esquecer de mascarar numa tela nova.
		p.TipoChave = TipoChavePix(tipo)
		p.ChaveMascarada = MascaraChavePix(chave, p.TipoChave)
		p.DocMascarado = MascaraDocumento(doc)
		p.VenceEm = p.CriadoEm.Add(PrazoDoPagamento)
		fila = append(fila, p)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: lendo a fila de pagamento: %w", err)
	}
	return fila, nil
}

// ChaveInteira é o que a staff precisa para pagar à mão, uma linha por vez.
type ChaveInteira struct {
	ChavePix      string
	TipoChave     TipoChavePix
	Documento     string
	ValorCentavos int64
}

// ChaveParaPagar devolve a chave e o CPF INTEIROS de um repasse, e REGISTRA a leitura.
//
// POR QUE A LEITURA É UMA AÇÃO, e não um campo da lista: pagar à mão exige o dado
// inteiro, então esconder não é opção. O que dá para escolher é se a leitura deixa
// rastro — e aqui ela deixa, na mesma transação, com quem leu e quando.
//
// O QUE ISTO NÃO RESOLVE, e é honesto dizer: registrar não impede. Quem abriu a linha
// pode fotografar a tela, e nenhuma auditoria alcança isso. O controle real é a página
// ser da staff. O registro serve para depois — quando se pergunta quem teve acesso à
// chave de alguém, existe resposta em vez de "qualquer um da equipe, em qualquer dia".
//
// SÓ DE PENDENTE. Não há motivo para revelar a chave de uma dívida já paga, e a recusa
// aqui é o que impede a tela de virar consulta de dado pessoal por id.
func (s *Store) ChaveParaPagar(ctx context.Context, repasseID int64, ator AtorDoAjuste,
) (ChaveInteira, error) {
	var c ChaveInteira
	var tipo int16
	var vendedor int64
	err := s.inTx(ctx, func(tx pgx.Tx) error {
		err := tx.QueryRow(ctx, `
			SELECT d.chave, d.tipo, d.documento, r.valor_centavos, r.vendedor_conta
			  FROM rmt_repasse r
			  JOIN rmt_recebedor d ON d.account_id = r.vendedor_conta
			 WHERE r.id = $1 AND r.status = $2`, repasseID, repassePendente).
			Scan(&c.ChavePix, &tipo, &c.Documento, &c.ValorCentavos, &vendedor)
		if errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("repasse %d: %w", repasseID, ErrRepasseInexistente)
		}
		if err != nil {
			return fmt.Errorf("store: lendo a chave do repasse %d: %w", repasseID, err)
		}
		// A AUDITORIA GUARDA A MÁSCARA, e não a chave. Ela diz QUEM leu O QUE, e para
		// isso a máscara basta — guardar o inteiro faria a trilha de acesso ser, ela
		// mesma, uma segunda cópia do dado pessoal, num lugar que ninguém apaga.
		if _, err := tx.Exec(ctx, `
			INSERT INTO admin_audit_log
			    (actor_account_id, actor_role, action, target_account_id, old_value, new_value)
			VALUES ($1, $2, $3, $4, $5, $6)`,
			ator.ContaID, ator.Papel, AcaoChavePixRevelada, vendedor,
			"{}", fmt.Sprintf(`{"repasse":%d,"chave":%q}`,
				repasseID, MascaraChavePix(c.ChavePix, TipoChavePix(tipo)))); err != nil {
			return fmt.Errorf("store: registrando a leitura da chave do repasse %d: %w", repasseID, err)
		}
		return nil
	})
	if err != nil {
		return ChaveInteira{}, err
	}
	c.TipoChave = TipoChavePix(tipo)
	c.Documento = strings.TrimSpace(c.Documento)
	return c, nil
}
