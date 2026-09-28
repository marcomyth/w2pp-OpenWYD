package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/jeanluca/w2pp-openwyd/internal/pilha"
)

// A cobrança pelo lado de QUEM PAGA.
//
// Tudo o mais neste sistema olha a cobrança pelo lado do vendedor ou do dinheiro.
// Esta é a única leitura feita para o COMPRADOR, e ela existe porque a decisão de
// 23/09/2026 põe o pagamento no site: ele clica em comprar no jogo, recebe um
// aviso no jogo, e paga o copia-e-cola na conta dele, no site.
//
// SÓ LEITURA. Não cria, não cancela, não expira. Quem cria é o jogo, e quem fecha
// é o pagamento, a saída do comprador do jogo, ou o prazo.

// JanelaCobrancaRecente é por quanto tempo uma cobrança JÁ FECHADA continua sendo
// devolvida ao comprador.
//
// Sem ela, a página esvazia no instante em que a cobrança fecha — e o pior
// instante possível para isso é o segundo seguinte ao pagamento. Quem acabou de
// mandar dinheiro e vê a tela ficar em branco abre chamado, e está certo em abrir.
//
// Dez minutos é ponto de partida, como os cinco da janela de pagamento: é número
// para medir, não para defender. O que ele precisa ser é maior do que o tempo que
// alguém leva para voltar à página depois de pagar.
const JanelaCobrancaRecente = 10 * time.Minute

// EstadoCobrancaComprador é o que a página do comprador tem de conseguir dizer.
type EstadoCobrancaComprador int

const (
	// EstadoCobrancaDesconhecido nunca sai daqui; existe para o zero não ser um
	// estado com significado.
	EstadoCobrancaDesconhecido EstadoCobrancaComprador = iota
	// EstadoCobrancaAberta: o código vale, o prazo não passou. Pague.
	EstadoCobrancaAberta
	// EstadoCobrancaExpirada: o prazo passou. A varredura pode ainda não ter
	// fechado a linha, e isso não muda o que a página diz.
	EstadoCobrancaExpirada
	// EstadoCobrancaPaga: o dinheiro entrou e o item está a caminho.
	EstadoCobrancaPaga
	// EstadoCobrancaCancelada: fechou sem pagamento, e NADA FOI COBRADO.
	//
	// O SIGNIFICADO MUDOU em 24/09/2026, e o motivo é que ele tinha ficado sem
	// produtor: a razão original era "o comprador saiu do jogo", e o cancelamento por
	// logout saiu no PR 92 — ninguém mais o produzia. Agora o único produtor é a
	// RECUSA DEFINITIVA da processadora (FecharCobrancaPorRecusaDefinitiva, em
	// pix_da_cobranca.go): a cobrança não pôde ser gerada, nenhum código existiu, e
	// por isso é certo dizer que nada foi cobrado.
	//
	// O que a pessoa pode fazer é o mesmo nos dois significados — tentar de novo —,
	// e é por isso que o estado serve sem número novo no contrato. O texto do
	// web.proto ainda descreve a razão antiga: ele muda no próximo handshake com o
	// site, porque qualquer byte no .proto troca o sha e trava o build deles até o
	// sync. Esta é a fonte da verdade até lá.
	EstadoCobrancaCancelada
	// EstadoCobrancaPagaSemItem: o dinheiro chegou depois de a cobrança fechar e
	// o item não foi entregue. O valor está em análise.
	EstadoCobrancaPagaSemItem
	// EstadoCobrancaValorDivergente: entrou um valor diferente do cobrado, e uma
	// pessoa vai olhar.
	//
	// A cobrança continua ABERTA no banco — ela não se resolveu —, mas para a
	// página ela NÃO é "pague agora": o código sairia ao lado de um pagamento que
	// já chegou, e convidaria a pagar duas vezes.
	EstadoCobrancaValorDivergente
)

// CobrancaDoComprador é a cobrança do jeito que a página precisa dela.
//
// A descrição do item vem da FOTOGRAFIA do anúncio e não do baú vivo do vendedor:
// a fotografia é a resposta a "o que exatamente eu paguei" depois de o item sair
// do baú, e é a única fonte que funciona com o vendedor fora do jogo.
type CobrancaDoComprador struct {
	CobrancaID    int64
	CodigoPix     string
	ValorCentavos int64
	ExpiraEm      time.Time
	Estado        EstadoCobrancaComprador
	ItemIndex     int16
	Refino        int
	Quantidade    int
	VendedorNome  string
	// ReembolsoPedidoEm é quando o reembolso foi PEDIDO, zero quando não foi.
	// A partir desta data a página conta os até dois dias úteis da análise.
	ReembolsoPedidoEm time.Time
	Reembolso         EstadoReembolso
	// Entrega é onde está o item depois de pago: a caminho, ou segurado porque o
	// baú está cheio. Ver EstadoEntrega.
	Entrega EstadoEntrega
}

// EstadoEntrega diz onde está o item de uma compra já paga.
//
// CALCULADO NA LEITURA, e não guardado numa coluna. Quem descobre que o item não
// cabe é o servidor de jogo, no laço dele, e ele só fala com o banco por gRPC —
// marcar uma coluna exigiria um RPC novo, e quando NADA é entregue o dreno nem
// chega a chamar o banco. O calculado também é mais honesto: uma coluna diria "não
// cabia às 02h10" e envelheceria, enquanto isto diz "não cabe agora".
//
// "SEM ESPAÇO LIVRE" NÃO É "NÃO CABE": item que empilha entra num baú lotado. Por
// isso o estado afirma o FATO, e a página diz "o baú está sem espaço livre" em vez
// de prever que o item não vai entrar — assim a frase não fica falsa quando ele
// empilhar.
//
// E O ESPAÇO É LIDO NO BANCO, não na memória do jogo: o webserver não fala com o
// laço. Com o jogador online, o baú do banco é o do último save, então isto pode
// estar um save atrasado. É a melhor resposta disponível deste lado, e é melhor que
// a anterior, que era dizer "a caminho" para quem nunca receberia.
type EstadoEntrega int

const (
	// EntregaNenhuma: não há linha de entrega. Ainda não pago, ou sem entrega.
	EntregaNenhuma EstadoEntrega = iota
	// EntregaNaFila: pago e na fila. Chega no próximo dreno, que é o login ou a
	// entrega imediata.
	EntregaNaFila
	// EntregaPresa: está na fila E o baú da conta não tem espaço livre.
	EntregaPresa
	// EntregaFeita: o item entrou no baú.
	EntregaFeita
	// EntregaPerdida: a linha foi encerrada sem entregar. Não acontece sozinha.
	EntregaPerdida
)

// EstadoReembolso é o caminho de volta do dinheiro que chegou tarde.
//
// Campo próprio e não mais um estado da cobrança porque os dois andam em relógios
// diferentes: a cobrança fecha em segundos, e o reembolso pode levar dias. Juntar
// os dois seria precisar de um estado para cada par.
type EstadoReembolso int

const (
	// ReembolsoNenhum: não há reembolso a fazer. É o caso de quase toda cobrança.
	ReembolsoNenhum EstadoReembolso = iota
	// ReembolsoPendente: devemos o reembolso e ainda não pedimos à processadora.
	ReembolsoPendente
	// ReembolsoPedido: a processadora aceitou e está analisando. É o estado que
	// dura até dois dias úteis.
	ReembolsoPedido
	// ReembolsoConcluido: o dinheiro voltou.
	ReembolsoConcluido
	// ReembolsoRecusado: a processadora recusou. NÃO se tenta de novo em laço e
	// NÃO sai da página sozinho — espera uma pessoa.
	ReembolsoRecusado
	// ReembolsoIncerto: o pedido saiu e a resposta não voltou. PODE TER SIDO
	// CRIADO (0126).
	//
	// Nunca se pede de novo daqui, e é por isso que ele é um estado e não um erro:
	// dois pedidos sobre o mesmo dinheiro devolveriam o dobro ao comprador, e o
	// dobro sai da conta da Hanna. Não há consulta de reembolso na ponte para
	// desempatar — só uma pessoa olhando o painel da processadora.
	ReembolsoIncerto
)

// CobrancaAtualDoComprador devolve a cobrança desta conta como COMPRADORA: a
// aberta, ou a que fechou há pouco.
//
// Devolve found=false quando não há nenhuma, e isso NÃO é erro: é o estado normal
// de quase toda conta em quase todo momento, e quem pergunta é a página da conta,
// que as pessoas abrem para ver outras coisas.
//
// NO MÁXIMO UMA, e a garantia não é desta consulta: é do índice único da 0105, um
// aberta por anúncio. O ORDER BY existe para o caso de haver duas FECHADAS dentro
// da janela recente — aí ganha a mais nova, que é a que a pessoa está olhando.
//
// O CÓDIGO PIX SÓ SAI NO ESTADO ABERTO. Cobrança morta ao lado de um código vivo
// convida alguém a pagar uma coisa que já acabou — e o código continua pagável na
// processadora, que é justamente o caso que não se quer fabricar.
func (s *Store) CobrancaAtualDoComprador(ctx context.Context, compradorConta int64,
	janelaRecente time.Duration,
) (bool, CobrancaDoComprador, error) {
	if janelaRecente <= 0 {
		janelaRecente = JanelaCobrancaRecente
	}
	var cob CobrancaDoComprador
	var status int16
	var codigo *string
	var nomeVendedor *string
	var eff [6]int16
	var reembolsoStatus *int16
	var reembolsoEm *time.Time
	var divergente *int64
	var entregaStatus *string
	var itensNoBau int

	// O nome do vendedor vem da FOTOGRAFIA (0113) e não de uma busca por
	// personagem da conta. Buscar erraria de duas formas ao mesmo tempo: mostraria
	// um personagem que pode não ser o da barraca, e exporia o nome de outro
	// personagem da conta, que não tem nada a ver com a venda.
	// A PRECEDÊNCIA, e ela é o desenho:
	//
	//  0. A ABERTA ganha sempre. É a única em que a pessoa pode fazer algo.
	//  1. Senão, um pagamento atrasado cujo reembolso não terminou — e este NÃO
	//     envelhece. O dinheiro de alguém está parado; uma página que o esquece
	//     em dez minutos deixa a pessoa sem nada para olhar e sem a quem
	//     perguntar.
	//  2. Senão, a mais recente que fechou dentro da janela.
	//
	// Um atrasado que está atrás de uma aberta volta sozinho quando a aberta
	// fechar. Nada se perde, só entra na fila.
	err := s.pool.QueryRow(ctx, `
		SELECT c.id, c.codigo_pix, c.valor_centavos, c.expira_em, c.status,
		       c.reembolso_status, c.reembolso_pedido_em, c.valor_divergente_centavos,
		       a.item_index, a.eff1, a.effv1, a.eff2, a.effv2, a.eff3, a.effv3,
		       a.vendedor_personagem,
		       e.status,
		       -- O espaço livre do baú, contado aqui e não numa segunda consulta:
		       -- assim ele é lido no MESMO instante do resto, e a página não mistura
		       -- uma entrega de agora com um baú de um segundo atrás.
		       (SELECT count(*) FROM item
		         WHERE account_id = c.comprador_conta AND owner_kind = 'account_cargo')
		  FROM rmt_cobranca c
		  JOIN rmt_anuncio a ON a.id = c.anuncio_id
		  LEFT JOIN delivery_queue e ON e.id = c.entrega_id
		 WHERE c.comprador_conta = $1
		   AND (c.status = $2
		        OR (c.status = $4 AND (c.reembolso_status IS NULL OR c.reembolso_status <> $5))
		        OR c.encerrada_em > now() - $3::interval)
		 ORDER BY CASE
		            WHEN c.status = $2 THEN 0
		            WHEN c.status = $4 AND (c.reembolso_status IS NULL
		                                    OR c.reembolso_status <> $5) THEN 1
		            ELSE 2
		          END,
		          c.criada_em DESC
		 LIMIT 1`,
		compradorConta, cobrancaAberta, janelaRecente.String(),
		cobrancaPagaSemItem, reembolsoConcluido).
		Scan(&cob.CobrancaID, &codigo, &cob.ValorCentavos, &cob.ExpiraEm, &status,
			&reembolsoStatus, &reembolsoEm, &divergente,
			&cob.ItemIndex, &eff[0], &eff[1], &eff[2], &eff[3], &eff[4], &eff[5],
			&nomeVendedor, &entregaStatus, &itensNoBau)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, CobrancaDoComprador{}, nil
	}
	if err != nil {
		return false, CobrancaDoComprador{}, fmt.Errorf(
			"store: cobranca atual do comprador %d: %w", compradorConta, err)
	}

	cob.Estado = estadoParaOComprador(status, cob.ExpiraEm)
	// A divergência ganha do status: a linha está aberta no banco, mas para a
	// pessoa ela não é "pague agora" — o dinheiro dela já chegou.
	if divergente != nil {
		cob.Estado = EstadoCobrancaValorDivergente
	}
	if cob.Estado == EstadoCobrancaAberta && codigo != nil {
		cob.CodigoPix = *codigo
	}
	if nomeVendedor != nil {
		cob.VendedorNome = *nomeVendedor
	}
	if reembolsoStatus != nil {
		cob.Reembolso = EstadoReembolso(*reembolsoStatus)
	}
	if reembolsoEm != nil {
		cob.ReembolsoPedidoEm = *reembolsoEm
	}
	cob.Refino, cob.Quantidade = refinoEQuantidade(eff)
	cob.Entrega = estadoDaEntrega(entregaStatus, itensNoBau)
	return true, cob, nil
}

// estadoDaEntrega decide onde está o item a partir da linha da caixa postal e do
// espaço do baú.
//
// SEM LINHA É "NENHUMA", e não "na fila": a cobrança que ainda não foi paga não tem
// entrega, e dizer "a caminho" ali prometeria uma coisa que ninguém pediu ao banco.
func estadoDaEntrega(status *string, itensNoBau int) EstadoEntrega {
	if status == nil {
		return EntregaNenhuma
	}
	switch *status {
	case "delivered":
		return EntregaFeita
	case "lost":
		return EntregaPerdida
	case "pending":
		// PRESA só quando o baú não tem espaço livre. É o fato que a página afirma;
		// se o item empilhar, ele entra mesmo assim, e a frase continua verdadeira
		// porque ela fala do baú e não do item.
		if itensNoBau >= maxCargoDoBau {
			return EntregaPresa
		}
		return EntregaNaFila
	}
	// Status que esta versão não conhece vira NENHUMA, e não um chute: a página tem
	// uma frase neutra para isso, e afirmar "entregue" por engano é o pior erro
	// possível aqui.
	return EntregaNenhuma
}

// maxCargoDoBau é MAX_CARGO, os espaços do baú da conta. O número mora aqui porque
// esta consulta é o único lugar do store que precisa saber quando o baú está cheio.
const maxCargoDoBau = 128

// estadoParaOComprador traduz o status da linha no que a página diz.
//
// A ABERTA COM PRAZO VENCIDO VIRA EXPIRADA aqui, sem esperar a varredura. A
// varredura roda de minuto em minuto e a página pode ser aberta no meio desse
// minuto; dizer "pague" para um código que já não vale seria mandar a pessoa
// mandar dinheiro para uma cobrança morta.
func estadoParaOComprador(status int16, expiraEm time.Time) EstadoCobrancaComprador {
	switch status {
	case cobrancaAberta:
		if time.Now().After(expiraEm) {
			return EstadoCobrancaExpirada
		}
		return EstadoCobrancaAberta
	case cobrancaPaga:
		return EstadoCobrancaPaga
	case cobrancaCancelada:
		return EstadoCobrancaCancelada
	case cobrancaExpirada:
		return EstadoCobrancaExpirada
	case cobrancaPagaSemItem:
		return EstadoCobrancaPagaSemItem
	}
	return EstadoCobrancaDesconhecido
}

// Os dois efeitos que a fotografia carrega e que a página do comprador mostra.
//
// Vêm dos mesmos números que o jogo usa — EF_SANC e EF_AMOUNT do ItemEffect.h —,
// e o `pilha.EfAmount` é a fonte do segundo justamente para não haver duas
// cópias do mesmo 61 no repositório. O refino não tem constante compartilhada
// ainda; quando tiver, esta linha vira o mesmo empréstimo.
const (
	efeitoRefino     int16 = 43 // EF_SANC: o nível de refino ("anc")
	efeitoQuantidade int16 = int16(pilha.EfAmount)
)

// refinoEQuantidade lê refino e quantidade dos efeitos da fotografia.
//
// São os dois números que o painel do jogo mostra ao lado de um item, e o site
// mostra os mesmos para a pessoa reconhecer o que comprou. Ler daqui e não de uma
// coluna própria é o que mantém UMA fonte: a fotografia.
func refinoEQuantidade(eff [6]int16) (refino, quantidade int) {
	quantidade = 1
	for i := 0; i < 6; i += 2 {
		switch eff[i] {
		case efeitoRefino:
			refino = int(eff[i+1])
		case efeitoQuantidade:
			if eff[i+1] > 0 {
				quantidade = int(eff[i+1])
			}
		}
	}
	return refino, quantidade
}

// Estados do reembolso como o banco os guarda (0114). Os números são os mesmos
// do EstadoReembolso, e a conversão continua explícita: eles vivem em lugares
// diferentes e podem mudar por motivos diferentes.
const (
	reembolsoPendente  int16 = 1
	reembolsoPedido    int16 = 2
	reembolsoConcluido int16 = 3
	reembolsoRecusado  int16 = 4
	reembolsoIncerto   int16 = 5
)
