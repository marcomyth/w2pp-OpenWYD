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
	// EstadoCobrancaCancelada: fechou sem pagamento, porque o comprador saiu do
	// jogo.
	EstadoCobrancaCancelada
	// EstadoCobrancaPagaSemItem: o dinheiro chegou depois de a cobrança fechar e
	// o item não foi entregue. O valor está em análise.
	EstadoCobrancaPagaSemItem
)

// CobrancaDoComprador é a cobrança do jeito que a página precisa dela.
//
// A descrição do item vem da FOTOGRAFIA do anúncio e não do baú vivo do vendedor:
// a fotografia é a resposta a "o que exatamente eu paguei" depois de o item sair
// do baú, e é a única fonte que funciona com o vendedor fora do jogo.
type CobrancaDoComprador struct {
	CodigoPix     string
	ValorCentavos int64
	ExpiraEm      time.Time
	Estado        EstadoCobrancaComprador
	ItemIndex     int16
	Refino        int
	Quantidade    int
	VendedorNome  string
}

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

	// O nome do vendedor vem da FOTOGRAFIA (0113) e não de uma busca por
	// personagem da conta. Buscar erraria de duas formas ao mesmo tempo: mostraria
	// um personagem que pode não ser o da barraca, e exporia o nome de outro
	// personagem da conta, que não tem nada a ver com a venda.
	err := s.pool.QueryRow(ctx, `
		SELECT c.codigo_pix, c.valor_centavos, c.expira_em, c.status,
		       a.item_index, a.eff1, a.effv1, a.eff2, a.effv2, a.eff3, a.effv3,
		       a.vendedor_personagem
		  FROM rmt_cobranca c
		  JOIN rmt_anuncio a ON a.id = c.anuncio_id
		 WHERE c.comprador_conta = $1
		   AND (c.status = $2 OR c.encerrada_em > now() - $3::interval)
		 ORDER BY c.criada_em DESC
		 LIMIT 1`,
		compradorConta, cobrancaAberta, janelaRecente.String()).
		Scan(&codigo, &cob.ValorCentavos, &cob.ExpiraEm, &status,
			&cob.ItemIndex, &eff[0], &eff[1], &eff[2], &eff[3], &eff[4], &eff[5],
			&nomeVendedor)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, CobrancaDoComprador{}, nil
	}
	if err != nil {
		return false, CobrancaDoComprador{}, fmt.Errorf(
			"store: cobranca atual do comprador %d: %w", compradorConta, err)
	}

	cob.Estado = estadoParaOComprador(status, cob.ExpiraEm)
	if cob.Estado == EstadoCobrancaAberta && codigo != nil {
		cob.CodigoPix = *codigo
	}
	if nomeVendedor != nil {
		cob.VendedorNome = *nomeVendedor
	}
	cob.Refino, cob.Quantidade = refinoEQuantidade(eff)
	return true, cob, nil
}

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
