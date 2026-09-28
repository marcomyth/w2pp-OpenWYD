package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

// ErrCobrancaInexistente é pedir o Pix de uma cobrança que não existe.
var ErrCobrancaInexistente = errors.New("store: cobranca inexistente")

// ErrPonteSemCodigo é a ponte responder sem o código do Pix.
//
// Erro e não "sem código ainda": um código vazio gravado é indistinguível de
// nunca ter chamado, e a linha ficaria para sempre num estado que a trava
// considera resolvido — a página mostraria "gerando" até o prazo vencer.
var ErrPonteSemCodigo = errors.New("store: a ponte respondeu sem codigo pix")

// ErrIdentifierDeOutraCobranca é a processadora devolver, para esta cobrança, um
// identifier que JÁ ESTÁ GRAVADO em outra.
//
// Recusa e não sobrescrita. Duas linhas nossas apontando para o mesmo pagamento
// significa pedir dois reembolsos do mesmo dinheiro, e é isso que o índice único
// da 0115 existe para impedir. Se isto aparecer, alguém tem de olhar: ou a
// processadora reaproveitou um id, ou duas cobranças nossas nasceram com a mesma
// referência.
var ErrIdentifierDeOutraCobranca = errors.New("store: o identifier ja pertence a outra cobranca")

// ErrPonteSemIdentifier é a ponte devolver o código do Pix e NÃO o identifier.
//
// Tratado igual ao código vazio, e a razão é o que se perde sem ele: o identifier é
// o id DELES, e é por ele que o aviso de pagamento acha a cobrança, que a consulta
// do fim da janela consulta, e que o reembolso é pedido. Um código gravado sem
// identifier daria um Pix pagável cujo pagamento ninguém conseguiria ligar de volta
// à venda — e devolver o dinheiro também ficaria sem referência.
//
// Então é melhor NÃO mostrar código nenhum: a leitura seguinte tenta de novo com a
// mesma referência, e a ponte devolve a cobrança que já existe lá, agora com o id.
var ErrPonteSemIdentifier = errors.New("store: a ponte respondeu sem identifier")

// PixDaCobranca é o código que o comprador vê, e como ele chegou aqui.
type PixDaCobranca struct {
	CodigoPix  string
	Identifier string
	// SemPrazo é verdadeiro quando NADA foi criado porque não havia tempo útil
	// sobrando, ou porque a cobrança não está mais aberta.
	SemPrazo bool
	// Criado diz se a ponte foi chamada NESTA chamada. Só para o log e para o
	// teste: quem mostra a página trata igual os dois casos.
	Criado bool
}

// CriadorDePix é a chamada à ponte, injetada.
//
// Injetada e não chamada direto porque o pacote store não conhece rede nem
// processadora, e porque é este ponto que o teste precisa segurar para provar que
// duas leituras ao mesmo tempo produzem UMA chamada só.
type CriadorDePix func(ctx context.Context, referencia string, centavos int64) (codigoPix, identifier string, err error)

// CriarPixSeFaltar devolve o código Pix da cobrança, criando-o na processadora só
// se ele ainda não existir.
//
// A CORRIDA QUE ESTA FUNÇÃO EXISTE PARA IMPEDIR, porque ela custa dinheiro de
// verdade: a página do comprador consulta a cada cinco segundos enquanto a
// cobrança está aberta, e duas abas, ou um refresh, fazem duas leituras chegarem
// juntas. Sem trava, as duas veem codigo_pix nulo, as duas chamam a processadora,
// e nascem DUAS cobranças lá. A aba que perdeu mostraria o código que ela mesma
// criou — um identifier que não está gravado em lugar nenhum. O comprador paga
// esse, o aviso chega com um identifier que não acha cobrança, e o dinheiro dele
// fica sem venda.
//
// Três coisas juntas fecham isso, e nenhuma das três basta sozinha:
//
//  1. FOR UPDATE na linha da cobrança, mantido DURANTE a chamada à ponte. É o que
//     faz a segunda leitura esperar em vez de chamar. Sim, isso segura uma
//     transação aberta por alguns segundos de rede, e é aceito: o volume é de
//     poucas cobranças por dia, e a alternativa custa o dinheiro acima.
//
//     E é FOR UPDATE na linha, e não um advisory lock, por um motivo que não é de
//     gosto: a varredura do prazo pode expirar esta cobrança enquanto a ponte
//     responde. Com a linha travada ela espera; com advisory lock ela passaria, e
//     a gente gravaria um código novo numa cobrança já vencida.
//
//  2. A condição "codigo_pix IS NULL" na escrita. Redundante sob a trava, de
//     propósito: é a rede de proteção para o dia em que alguém trocar a trava por
//     algo mais frouxo sem notar. Barato.
//
//  3. DEVOLVE SEMPRE O QUE FICOU GRAVADO, nunca o que acabou de voltar da ponte.
//     É a regra que garante que as duas abas mostram o MESMO código.
//
// Sobre a resposta INCERTA da ponte — a chamada saiu e a resposta não voltou: a
// função devolve erro e NÃO grava nada. A leitura seguinte tenta de novo com a
// MESMA referencia_externa, e é para isso que essa referência nasce antes da
// chamada: a ponte reconhece a repetição e devolve o identifier da cobrança que já
// existe lá, em vez de criar uma segunda.
func (s *Store) CriarPixSeFaltar(ctx context.Context, cobrancaID int64,
	minimoRestante time.Duration, criar CriadorDePix,
) (PixDaCobranca, error) {
	var out PixDaCobranca
	err := s.inTx(ctx, func(tx pgx.Tx) error {
		var referencia string
		var centavos int64
		var status int16
		var expiraEm time.Time
		var codigo, identifier *string

		err := tx.QueryRow(ctx, `
			SELECT referencia_externa, valor_centavos, status, expira_em,
			       codigo_pix, identifier_syncpay
			  FROM rmt_cobranca
			 WHERE id = $1
			   FOR UPDATE`, cobrancaID).
			Scan(&referencia, &centavos, &status, &expiraEm, &codigo, &identifier)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrCobrancaInexistente
		}
		if err != nil {
			return fmt.Errorf("store: lendo a cobranca %d: %w", cobrancaID, err)
		}

		// Já tem código. É o caminho mais comum de longe: a página relê a cada
		// cinco segundos e só a PRIMEIRA leitura chama a ponte.
		if codigo != nil && *codigo != "" {
			out = PixDaCobranca{CodigoPix: *codigo, Identifier: texto(identifier)}
			return nil
		}

		// NÃO CRIA EM COBRANÇA QUE NÃO ESTÁ ABERTA. Sem isto, uma leitura da página
		// de uma cobrança já cancelada ou vencida geraria um Pix pagável por um item
		// que não está mais preso a ninguém.
		if status != cobrancaAberta {
			out.SemPrazo = true
			return nil
		}

		// NEM COM O PRAZO NO FIM, e este é o guarda que fabrica menos reembolso.
		// Criar um Pix com dez segundos de vida é quase garantir um pagamento fora
		// do prazo: o comprador abre o aplicativo do banco, paga, e o dinheiro cai
		// numa cobrança que já venceu. O prejuízo é dos dois lados — ele não recebe
		// o item na hora e a gente devolve pagando taxa.
		if time.Until(expiraEm) < minimoRestante {
			out.SemPrazo = true
			return nil
		}

		cod, ident, err := criar(ctx, referencia, centavos)
		if err != nil {
			return fmt.Errorf("store: criando o pix da cobranca %d: %w", cobrancaID, err)
		}
		if cod == "" {
			return fmt.Errorf("store: cobranca %d: %w", cobrancaID, ErrPonteSemCodigo)
		}
		if ident == "" {
			return fmt.Errorf("store: cobranca %d: %w", cobrancaID, ErrPonteSemIdentifier)
		}

		err = tx.QueryRow(ctx, `
			UPDATE rmt_cobranca
			   SET codigo_pix = $2, identifier_syncpay = $3
			 WHERE id = $1 AND codigo_pix IS NULL
			RETURNING codigo_pix, coalesce(identifier_syncpay, '')`,
			cobrancaID, cod, ident).Scan(&out.CodigoPix, &out.Identifier)
		if ehConflitoDeIndice(err, "rmt_cobranca_identifier") {
			return fmt.Errorf("store: cobranca %d, identifier %q: %w",
				cobrancaID, ident, ErrIdentifierDeOutraCobranca)
		}
		if errors.Is(err, pgx.ErrNoRows) {
			// Sob o FOR UPDATE isto não deveria acontecer: ninguém mais podia ter
			// gravado entre a leitura e a escrita. Se acontecer, a trava foi
			// afrouxada em algum momento — e a resposta certa continua sendo
			// DEVOLVER O QUE ESTÁ GRAVADO, porque é ele que o comprador da outra aba
			// está vendo e é ele que a confirmação vai encontrar.
			return tx.QueryRow(ctx, `
				SELECT coalesce(codigo_pix, ''), coalesce(identifier_syncpay, '')
				  FROM rmt_cobranca WHERE id = $1`, cobrancaID).
				Scan(&out.CodigoPix, &out.Identifier)
		}
		if err != nil {
			return fmt.Errorf("store: gravando o pix da cobranca %d: %w", cobrancaID, err)
		}
		out.Criado = true
		return nil
	})
	if err != nil {
		return PixDaCobranca{}, err
	}
	return out, nil
}

// CobrancaDoIdentifier acha a cobrança pelo id DA PROCESSADORA.
//
// Existe para o caminho do aviso poder conferir a referência que a processadora
// devolveu contra a que está gravada aqui. Sem essa conferência, uma resposta com
// a referência de OUTRA cobrança confirmaria a venda errada: item para o comprador
// errado e marca de vendido no item de outro vendedor.
//
// achou=false é normal e tem dois significados que não se separam daqui: o
// pagamento não é nosso, ou é nosso e o identifier ainda não estava gravado quando
// o aviso chegou. Quem chama trata os dois como "não confirma agora".
func (s *Store) CobrancaDoIdentifier(ctx context.Context, identifier string) (referencia string, achou bool, err error) {
	if identifier == "" {
		return "", false, nil
	}
	err = s.pool.QueryRow(ctx, `
		SELECT referencia_externa FROM rmt_cobranca
		 WHERE identifier_syncpay = $1`, identifier).Scan(&referencia)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", false, nil
	}
	if err != nil {
		return "", false, fmt.Errorf("store: cobranca do identifier: %w", err)
	}
	return referencia, true, nil
}

// NomeDaConta resolve o nome a partir do id, porque as chamadas de controle do
// servidor de jogo falam por nome de conta e o banco fala por id.
func (s *Store) NomeDaConta(ctx context.Context, accountID int64) (string, error) {
	var nome string
	err := s.pool.QueryRow(ctx,
		`SELECT name FROM account WHERE id = $1`, accountID).Scan(&nome)
	if err != nil {
		return "", fmt.Errorf("store: nome da conta %d: %w", accountID, err)
	}
	return nome, nil
}

// texto desreferencia um TEXT nulo do banco.
func texto(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

// GravarIdentifierSeFaltar completa a cobrança com o id DELES, quando ele ficou
// para trás.
//
// EXISTE POR CAUSA DA CORRIDA NO NASCIMENTO: o webhook da processadora pode chegar
// antes de o nosso UPDATE do identifier ter commitado. Nesse caso a confirmação
// acontece pela referência que a processadora devolveu, e a cobrança fica PAGA com
// identifier nulo — o que só dói depois, quando alguém precisa pedir o reembolso
// dela e não tem por onde.
//
// SÓ PREENCHE O NULO. Nunca sobrescreve um identifier já gravado: se os dois
// existirem e forem diferentes, isso é um problema para uma pessoa olhar, e não algo
// para esta função resolver sozinha em silêncio.
func (s *Store) GravarIdentifierSeFaltar(ctx context.Context, referenciaExterna, identifier string) error {
	if identifier == "" {
		return nil
	}
	_, err := s.pool.Exec(ctx, `
		UPDATE rmt_cobranca SET identifier_syncpay = $2
		 WHERE referencia_externa = $1 AND identifier_syncpay IS NULL`,
		referenciaExterna, identifier)
	if err != nil {
		return fmt.Errorf("store: gravando o identifier de %q: %w", referenciaExterna, err)
	}
	return nil
}

// FecharCobrancaPorRecusaDefinitiva fecha uma cobrança que a processadora RECUSOU de
// um jeito que não muda tentando de novo.
//
// POR QUE ELA EXISTE, e é um defeito de produção de 24/09/2026: a criação do Pix
// tratava toda falha como TROPEÇO DE REDE, de propósito — a página relê a cada cinco
// segundos, e um erro passageiro se resolve na tentativa seguinte. O raciocínio está
// certo para o que é passageiro.
//
// Só que uma recusa de FORMATO não é passageira. A referência saía com um prefixo que
// a ponte não aceita, e o servidor repetiu a mesma chamada recusada a cada cinco
// segundos, indefinidamente. Do lado do jogador: "gerando o código" para sempre, sem
// explicação e sem o item liberado.
//
// É SEGURO FECHAR, e a segurança vem de um fato e não de uma aposta: NENHUM CÓDIGO
// FOI CRIADO. Sem código não há o que pagar, então não existe dinheiro a caminho que
// este fechamento possa perder. É diferente de cancelar uma cobrança com código vivo,
// onde a pessoa pode ter pagado no segundo anterior.
//
// E o item NÃO é solto aqui: ele continua marcado para o anúncio, que segue na
// prateleira. O que este fechamento libera é o COMPRADOR, que só pode ter uma
// cobrança aberta — sem isso ele ficaria preso a uma cobrança morta e não conseguiria
// tentar de novo nem comprar outra coisa.
func (s *Store) FecharCobrancaPorRecusaDefinitiva(ctx context.Context, cobrancaID int64) (bool, error) {
	tag, err := s.pool.Exec(ctx, `
		UPDATE rmt_cobranca SET status = $2, encerrada_em = now()
		 WHERE id = $1 AND status = $3
		   -- SÓ SEM CÓDIGO. O WHERE é o guarda, e não a ordem das chamadas: se por
		   -- qualquer caminho já houver código gravado, esta cobrança é pagável e
		   -- fechá-la aqui poderia descartar um pagamento em curso.
		   AND codigo_pix IS NULL`,
		cobrancaID, cobrancaCancelada, cobrancaAberta)
	if err != nil {
		return false, fmt.Errorf("store: fechar cobranca %d por recusa definitiva: %w", cobrancaID, err)
	}
	return tag.RowsAffected() > 0, nil
}
