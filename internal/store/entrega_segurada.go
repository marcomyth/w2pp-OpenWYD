package store

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/jeanluca/w2pp-openwyd/internal/savefmt"
)

// EstadoEntrega é o que responder a quem pergunta "onde está o meu item".
//
// O vocabulário é o do contrato combinado com o par do site, e existe porque a página
// mostrava "o item está a caminho" para duas situações muito diferentes: a entrega que
// vai acontecer no próximo login, e a que não vai acontecer nenhuma vez até a pessoa
// esvaziar o baú. Quem está no segundo caso esperava um dia que não chegava.
type EstadoEntrega string

const (
	// EntregaIndefinida: não há linha de entrega. Nem prometida, nem negada.
	EntregaIndefinida EstadoEntrega = ""
	// EntregaEsperando: pago e na fila. Chega no próximo dreno, que é o login ou a
	// entrega imediata.
	EntregaEsperando EstadoEntrega = "WAITING"
	// EntregaSegurada: está na fila E o baú da conta não tem espaço livre.
	EntregaSegurada EstadoEntrega = "HELD"
	// EntregaEntregue: o item entrou no baú.
	EntregaEntregue EstadoEntrega = "DELIVERED"
	// EntregaPerdida: a linha foi encerrada sem entregar. Não acontece sozinha.
	EntregaPerdida EstadoEntrega = "LOST"
)

// BauSemEspaco responde se a conta está com o baú lotado.
//
// É O DENOMINADOR DA REGRA DO SEGURADO, e mora aqui porque DOIS leitores precisam da
// mesma resposta — o web-api, na compra do mercado, e o painel, na lista de itens a
// caminho. Duas consultas iguais escritas em dois lugares é como uma delas fica para
// trás no dia em que o número 128 mudar.
//
// O MÁXIMO VEM DO savefmt, que é o formato do save, e não de uma constante nova aqui: o
// 128 é o MAX_CARGO do jogo, e copiá-lo criaria um segundo lugar para ele estar errado.
//
// O QUE ISTO NÃO RESPONDE, e a diferença importa para o texto da tela: "sem espaço
// livre" não é "não cabe". Item que empilha entra num espaço já ocupado, então um baú
// lotado ainda recebe mais de algo que já esteja lá. Quem mostra isto ao jogador tem de
// dizer o fato — "o baú está sem espaço livre" — e não a previsão. Foi o acerto feito com
// o par do site justamente para a tela não mentir quando o item empilhar e entrar.
//
// FUNÇÃO E NÃO SÓ MÉTODO, e o motivo é concreto: os dois leitores têm pools DIFERENTES.
// O web-api usa o *store.Store, e o painel usa o entrega.Store, com pool próprio. Um
// método no *store.Store obrigaria o painel a escrever a mesma consulta de novo, que é
// exatamente o que esta função existe para impedir. Recebe quem consulta e serve aos dois.
func BauSemEspaco(ctx context.Context, q QuemConsulta, accountID int64) (bool, error) {
	var ocupados int
	if err := q.QueryRow(ctx, `
		SELECT count(*) FROM item
		 WHERE account_id = $1 AND owner_kind = 'account_cargo'`, accountID).Scan(&ocupados); err != nil {
		return false, fmt.Errorf("store: contando o bau da conta %d: %w", accountID, err)
	}
	return ocupados >= savefmt.MaxCargo, nil
}

// QuemConsulta é o mínimo que BauSemEspaco precisa: um pool, uma transação, tanto faz.
type QuemConsulta interface {
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

// BauSemEspaco no *Store, para quem já tem um.
func (s *Store) BauSemEspaco(ctx context.Context, accountID int64) (bool, error) {
	return BauSemEspaco(ctx, s.pool, accountID)
}

// EstadoDaEntrega traduz o status da linha, mais o baú, no que a tela mostra.
//
// A regra numa função só porque ela vai ser aplicada em dois lugares, e uma regra
// duplicada é a que sai de sincronia. Não toca no banco de propósito: quem chama já leu o
// status na própria consulta, e uma segunda leitura aqui daria duas fotos de instantes
// diferentes da mesma linha.
//
// SEGURADO SÓ VALE PARA PENDENTE. Um baú cheio não muda nada sobre uma entrega que já
// aconteceu, e dizer "segurada" sobre uma linha entregue seria afirmar que o jogador não
// recebeu o que ele tem no baú.
func EstadoDaEntrega(status string, bauSemEspaco bool) EstadoEntrega {
	switch status {
	case "pending":
		if bauSemEspaco {
			return EntregaSegurada
		}
		return EntregaEsperando
	case "delivered":
		return EntregaEntregue
	case "lost":
		return EntregaPerdida
	default:
		// Status que este código não conhece NÃO vira "a caminho". Prometer entrega por
		// causa de uma palavra que ninguém previu é o erro mais caro dos dois: o
		// indefinido faz a tela calar, e calar não mente.
		return EntregaIndefinida
	}
}
