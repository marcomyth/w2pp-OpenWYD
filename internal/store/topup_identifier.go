package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

// O ID DA PROCESSADORA PARA A COBRANÇA DE DOAÇÃO (0127), e a fila que o usa.
//
// Ele existe porque quem cria a cobrança da doação é o SITE, direto na
// processadora: o servidor nunca vê o id, e sem o id não dá para perguntar sobre o
// pagamento. Até esta coluna, um aviso perdido era dinheiro cobrado e crédito nunca
// dado, sem nada do nosso lado capaz de perceber.

// ResultadoAnexo é o que o Attach fez, e cada recusa tem nome.
type ResultadoAnexo int

const (
	// AnexoGravado: o id foi gravado agora.
	AnexoGravado ResultadoAnexo = iota
	// AnexoRepetido: este pedido já tinha exatamente este id. É a repetição
	// funcionando, e não erro.
	AnexoRepetido
	// AnexoConflitoDePedido: este pedido já tem OUTRO id. Nada muda — o primeiro
	// fica, porque foi com ele que a cobrança nasceu.
	AnexoConflitoDePedido
	// AnexoConflitoDeIdentifier: este id já está preso a OUTRO pedido.
	//
	// É A RECUSA QUE PROTEGE DINHEIRO. Deixada passar, um pagamento satisfaria dois
	// pedidos pendentes e creditaria os dois — e o segundo crédito sai do bolso da
	// dona do servidor. Não deveria acontecer nunca, e é por isso que merece alarme
	// e não encolher de ombros.
	AnexoConflitoDeIdentifier
	// AnexoPedidoInexistente: não há pedido com essa referência.
	AnexoPedidoInexistente
	// AnexoJaPago: o pedido já foi creditado. Não há o que anexar: a varredura só
	// olha pendentes, e apontar um pedido fechado para um pagamento não quer dizer
	// nada de honesto.
	AnexoJaPago
)

// AnexarIdentifierDoTopup grava o id da processadora num pedido de doação.
//
// A LINHA É TRAVADA ANTES DE DECIDIR, e não é zelo: o site chama isto no meio da
// compra, e uma repetição por timeout chega junto com a primeira. Sem o FOR UPDATE,
// as duas leriam "sem id" e as duas gravariam — e a segunda gravação, se viesse com
// id diferente, passaria por cima daquele com que a cobrança nasceu.
//
// O CONFLITO DE IDENTIFIER É PEGO PELO BANCO, e não por uma consulta antes: entre a
// consulta e o INSERT cabe a outra transação. O índice único parcial da 0127 é quem
// garante, e aqui a violação é só traduzida.
func (s *Store) AnexarIdentifierDoTopup(ctx context.Context, externalRef, identifier string) (ResultadoAnexo, error) {
	var res ResultadoAnexo
	err := s.inTx(ctx, func(tx pgx.Tx) error {
		var id int64
		var status int16
		var atual *string
		err := tx.QueryRow(ctx, `
			SELECT id, status, gateway_identifier FROM donate_topup_order
			 WHERE external_reference = $1 FOR UPDATE`, externalRef).Scan(&id, &status, &atual)
		if errors.Is(err, pgx.ErrNoRows) {
			res = AnexoPedidoInexistente
			return nil
		}
		if err != nil {
			return fmt.Errorf("store: anexar identifier ref=%q: %w", externalRef, err)
		}
		if status == TopupStatusPaid {
			res = AnexoJaPago
			return nil
		}
		if atual != nil {
			if *atual == identifier {
				res = AnexoRepetido
			} else {
				res = AnexoConflitoDePedido
			}
			return nil
		}
		if _, err := tx.Exec(ctx, `
			UPDATE donate_topup_order SET gateway_identifier = $2 WHERE id = $1`,
			id, identifier); err != nil {
			if ehConflitoDeIndice(err, "donate_topup_order_identifier") {
				res = AnexoConflitoDeIdentifier
				return nil
			}
			return fmt.Errorf("store: anexar identifier ref=%q: %w", externalRef, err)
		}
		res = AnexoGravado
		return nil
	})
	if err != nil {
		return AnexoPedidoInexistente, err
	}
	return res, nil
}

// TopupParaConferir é um pedido de doação pendente que dá para perguntar.
type TopupParaConferir struct {
	ExternalReference string
	Identifier        string
	CentavosPedidos   int64
	CriadoEm          time.Time
}

// TopupsParaConferir lista os pedidos pendentes com id, dentro da janela.
//
// A JANELA EXISTE PORQUE O PEDIDO DE DOAÇÃO NÃO EXPIRA. Não há coluna de prazo, e um
// carrinho abandonado fica pendente para sempre. Sem corte, a varredura perguntaria à
// processadora sobre todo carrinho abandonado da história do servidor, todos os dias.
//
// O corte é a mesma JanelaDaCobrancaMorta do RMT de propósito: os dois são o mesmo
// chute — quanto tempo um código Pix continua pagável — e têm de mudar juntos no dia
// em que alguém medir.
//
// OS MAIS NOVOS PRIMEIRO. Quem acabou de pagar é quem está olhando a tela.
func (s *Store) TopupsParaConferir(ctx context.Context, janela time.Duration, limite int) ([]TopupParaConferir, error) {
	if janela <= 0 {
		janela = JanelaDaCobrancaMorta
	}
	if limite <= 0 {
		limite = 100
	}
	rows, err := s.pool.Query(ctx, `
		SELECT external_reference, gateway_identifier, amount_cents, created_at
		  FROM donate_topup_order
		 WHERE status = $1 AND gateway_identifier IS NOT NULL
		   AND created_at > now() - $2::interval
		 ORDER BY created_at DESC
		 LIMIT $3`, TopupStatusPending, janela, limite)
	if err != nil {
		return nil, fmt.Errorf("store: topups para conferir: %w", err)
	}
	defer rows.Close()
	var lista []TopupParaConferir
	for rows.Next() {
		var t TopupParaConferir
		if err := rows.Scan(&t.ExternalReference, &t.Identifier, &t.CentavosPedidos, &t.CriadoEm); err != nil {
			return nil, fmt.Errorf("store: topups para conferir: %w", err)
		}
		lista = append(lista, t)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: topups para conferir: %w", err)
	}
	return lista, nil
}

// TopupsSemIdentifier conta os pedidos pendentes RECENTES que a varredura não
// alcança, porque ninguém anexou o id.
//
// É O TERMÔMETRO DO ATTACH. Com o site chamando, este número fica perto de zero.
// Subindo, ele diz que a chamada parou de acontecer — e o efeito é invisível de outro
// jeito, porque o webhook continua funcionando e ninguém repara que a rede embaixo
// sumiu.
//
// RECENTES, com a mesma janela, e essa parte é o que impede o número de virar ruído:
// carrinho abandonado é o NORMAL da doação, ao contrário do RMT. Sem o corte, o
// contador cresceria para sempre contando gente que só desistiu de comprar, e um
// número que só sobe não avisa nada.
func (s *Store) TopupsSemIdentifier(ctx context.Context, janela time.Duration) (int, error) {
	if janela <= 0 {
		janela = JanelaDaCobrancaMorta
	}
	var n int
	err := s.pool.QueryRow(ctx, `
		SELECT count(*) FROM donate_topup_order
		 WHERE status = $1 AND gateway_identifier IS NULL
		   AND created_at > now() - $2::interval`, TopupStatusPending, janela).Scan(&n)
	if err != nil {
		return 0, fmt.Errorf("store: topups sem identifier: %w", err)
	}
	return n, nil
}
