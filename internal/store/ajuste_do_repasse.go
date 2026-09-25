package store

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
)

// ErrAjusteInvalido é um valor novo que não pode valer para aquela dívida.
var ErrAjusteInvalido = errors.New("store: valor de ajuste invalido")

// ErrAjusteSemNota é o ajuste sem explicação.
var ErrAjusteSemNota = errors.New("store: ajuste de valor exige nota")

// AcaoAjusteDeRepasse é o nome desta ação no log de auditoria.
//
// Mora aqui e não no pacote do painel porque é ESTA função que escreve a linha, dentro
// da transação. Um nome definido longe de quem o grava é um nome que muda num lado só.
const AcaoAjusteDeRepasse = "REPASSE_VALOR_AJUSTADO"

// AtorDoAjuste é quem mexeu no valor.
//
// Traz conta e papel para a linha da auditoria, e o nome para a coluna da linha do
// repasse. São três porque a auditoria pede id e a tela mostra nome, e resolver isso
// buscando um a partir do outro faria a escrita de dinheiro depender de outra consulta.
type AtorDoAjuste struct {
	ContaID int64
	Papel   string
	Nome    string
}

// AjustarValorDoRepasse corrige quanto se deve a um vendedor, e NÃO devolve a dívida
// para a fila.
//
// Existe por um caso real: a venda de R$ 1,00 de 25/09/2026 abriu um repasse de R$ 1,00
// cheio, porque o desconto da taxa ainda não estava no ar. A processadora ficou com
// R$ 0,80, e pagar o cheio faria a casa cobrir a diferença. Corrigir isso com um UPDATE
// solto no banco resolveria esta linha e deixaria o próximo caso sem caminho — e sem
// ninguém sabendo quem mudou, de quanto para quanto, e por quê.
//
// SÓ MEXE NO RECUSADO. Não é conservadorismo: um repasse PENDENTE pode ser lido pela
// fila de pagar entre a leitura e a escrita, e a ponte receberia uma ordem com o valor
// velho; um ENVIADO já foi mandado; um PAGO viraria um registro que contradiz o extrato.
// O recusado é o único estado em que se tem CERTEZA de que nada saiu, e é por isso que
// ele é o único em que mexer no número é seguro.
//
// E CONTINUA RECUSADO depois do ajuste, de propósito. Devolver à fila aqui juntaria duas
// decisões diferentes numa tecla só: "o valor está errado" e "já dá para tentar de
// novo". No caso que originou isto, a segunda é falsa — a recusa foi de credencial, e
// voltar à fila só faria a varredura tomar o mesmo 403 de dois em dois minutos. Quem
// devolve à fila é o botão que já existe, quando a credencial estiver certa.
//
// O TETO É O VALOR DA COBRANÇA, e o piso é um centavo. Acima da cobrança seria pagar
// mais do que entrou, que é dinheiro saindo do nada; zero ou negativo não é pagamento.
// O teto é lido da cobrança na mesma transação, e não recebido de fora: um limite que
// quem chama informa não é limite.
//
// A AUDITORIA VAI NA MESMA TRANSAÇÃO, como no transicaoDaStaff do reembolso. Duas
// escritas não servem aqui: esta ação muda QUANTO uma pessoa recebe, e uma mudança de
// dinheiro aplicada sem registro é a que ninguém consegue explicar depois. Se a auditoria
// falhar, o ajuste não acontece.
//
// É por isso que o ator traz conta e papel, e não só o nome: a linha da auditoria precisa
// de quem, e "quem" num sistema de dinheiro é um id, não um texto que se digita.
//
// Devolve o valor ANTIGO, para quem chama poder mostrar de quanto para quanto.
func (s *Store) AjustarValorDoRepasse(ctx context.Context, id int64, novoCentavos int64,
	ator AtorDoAjuste, nota string,
) (int64, error) {
	if nota == "" {
		// A NOTA É OBRIGATÓRIA, e a regra mora aqui e não só na tela. Um número de
		// dinheiro sobrescrito sem motivo escrito é exatamente o que ninguém consegue
		// explicar seis meses depois, e outra tela amanhã chamaria esta função sem
		// saber da regra.
		return 0, ErrAjusteSemNota
	}
	if novoCentavos <= 0 {
		return 0, fmt.Errorf("%w: %d nao e um pagamento", ErrAjusteInvalido, novoCentavos)
	}

	var antigo int64
	err := s.inTx(ctx, func(tx pgx.Tx) error {
		var teto, vendedor int64
		// O VENDEDOR SAI DAQUI para ser o alvo da auditoria: o dinheiro é dele, e uma
		// linha de auditoria que não diz de quem era o dinheiro não responde a pergunta
		// que se faz numa disputa.
		err := tx.QueryRow(ctx, `
			SELECT r.valor_centavos, c.valor_centavos, r.vendedor_conta
			  FROM rmt_repasse r
			  JOIN rmt_cobranca c ON c.id = r.cobranca_id
			 WHERE r.id = $1 AND r.status = $2
			 FOR UPDATE OF r`, id, repasseRecusado).Scan(&antigo, &teto, &vendedor)
		if errors.Is(err, pgx.ErrNoRows) {
			// Não existe, ou não está recusado. É a mesma resposta de propósito: a
			// tela não deve poder descobrir o estado de uma linha tentando mexer nela.
			return ErrRepasseInexistente
		}
		if err != nil {
			return fmt.Errorf("store: lendo o repasse %d para ajuste: %w", id, err)
		}
		if novoCentavos > teto {
			return fmt.Errorf("%w: %d passa do valor da cobranca, %d",
				ErrAjusteInvalido, novoCentavos, teto)
		}
		// O VALOR ANTIGO VAI PARA A LINHA na mesma escrita, e não só para a auditoria.
		// A auditoria deste sistema é gravada DEPOIS da mudança, e o painel já admite
		// que a mudança pode acontecer e o registro não. Para um número sobrescrito
		// isso não basta: sem o antigo em algum lugar que esta transação garanta,
		// ninguém responde "quanto era" no dia em que a auditoria falhar.
		//
		// E o ajuste_de_centavos guarda o PRIMEIRO valor, não o da vez anterior:
		// COALESCE mantém o que já estava lá, para dois ajustes seguidos não apagarem
		// de onde a linha partiu.
		if _, err := tx.Exec(ctx, `
			UPDATE rmt_repasse
			   SET valor_centavos = $2,
			       ajuste_de_centavos = coalesce(ajuste_de_centavos, $3),
			       ajuste_nota = $4, ajustado_por = $5, ajustado_em = now()
			 WHERE id = $1 AND status = $6`,
			id, novoCentavos, antigo, nota, ator.Nome, repasseRecusado); err != nil {
			return fmt.Errorf("store: ajustando o valor do repasse %d: %w", id, err)
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO admin_audit_log
			    (actor_account_id, actor_role, action, target_account_id, old_value, new_value)
			VALUES ($1, $2, $3, $4, $5, $6)`,
			ator.ContaID, ator.Papel, AcaoAjusteDeRepasse, vendedor,
			fmt.Sprintf(`{"repasse":%d,"centavos":%d}`, id, antigo),
			fmt.Sprintf(`{"repasse":%d,"centavos":%d,"nota":%q}`, id, novoCentavos, nota)); err != nil {
			return fmt.Errorf("store: ajuste do repasse %d: registrando na auditoria: %w", id, err)
		}
		return nil
	})
	if err != nil {
		return 0, err
	}
	return antigo, nil
}
