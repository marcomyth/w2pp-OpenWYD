package handler

import (
	"bytes"
	"log/slog"
	"testing"

	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/world"
)

// A SAFIRA PASSA A VALER POR UNIDADE NAS DUAS PONTAS, e estes testes guardam as duas.
//
// Decisão da Hanna em 29/09/2026: "npc aceita pilha da safira, mas paga por cada". Ela
// empilha desde o PR 210, e sem isto vender uma pilha de 120 pagava os mesmos 125.000
// de uma só — o jogador perdia 119 Safiras.
//
// POR QUE OS TESTES DE COMPRA E DE VENDA MORAM JUNTOS: as duas metades são uma decisão
// só. Vender por unidade sem comprar por unidade é ouro sem fim (comprar a pilha por
// 1.000.000 e revender por N × 125.000), e comprar por unidade sem vender por unidade é
// só um item caro. Separar os testes deixaria alguém remover uma metade achando que
// mexe numa coisa.

// TestSafiraVendeEComproraPorUnidade mede as contas em números, nas duas pontas.
//
// EM NÚMEROS E NÃO EM FÓRMULA: a fórmula do preço de venda (um quarto, e metade disso
// acima de 10.000) já tem teste próprio. Aqui o que se guarda é o VALOR que o jogador
// recebe e paga, porque é isso que ele reclama quando está errado.
func TestSafiraVendeEComproraPorUnidade(t *testing.T) {
	t.Parallel()
	const (
		precoDeCatalogo = 1_000_000
		porUnidade      = 125_000 // 1.000.000/4 = 250.000, e acima de 10.000 cai à metade
	)

	if got := precoDeVendaNoNPC(precoDeCatalogo); got != porUnidade {
		t.Fatalf("o NPC paga %d por uma Safira, esperava %d", got, porUnidade)
	}

	casos := []struct {
		pilha        int
		querVendidas int
		querCobradas int
	}{
		{1, 1, 1},
		{3, 3, 3},
		{5, 5, 5},
		{120, 120, 120},
	}
	for _, c := range casos {
		it := world.Item{Index: safira}
		if c.pilha > 1 {
			setItemAmount(&it, c.pilha)
		}
		if got := unidadesVendidas(it); got != c.querVendidas {
			t.Errorf("pilha de %d: unidadesVendidas = %d, queria %d", c.pilha, got, c.querVendidas)
		}
		if got := unidadesCobradas(it); got != c.querCobradas {
			t.Errorf("pilha de %d: unidadesCobradas = %d, queria %d", c.pilha, got, c.querCobradas)
		}
		// Os valores que a Hanna vai conferir no jogo.
		if pago := int(porUnidade) * c.querVendidas; pago != int(porUnidade)*c.pilha {
			t.Errorf("pilha de %d rende %d", c.pilha, pago)
		}
	}

	// Os três números pedidos, escritos como números.
	if pago := int(porUnidade) * unidadesVendidas(safiraEmPilha(5)); pago != 625_000 {
		t.Errorf("vender 5 Safiras paga %d, esperava 625.000", pago)
	}
	if pago := int(porUnidade) * unidadesVendidas(world.Item{Index: safira}); pago != 125_000 {
		t.Errorf("vender 1 Safira paga %d, esperava 125.000", pago)
	}
	if cobrado := precoDeCatalogo * unidadesCobradas(safiraEmPilha(3)); cobrado != 3_000_000 {
		t.Errorf("comprar 3 Safiras cobra %d, esperava 3.000.000", cobrado)
	}
}

// TestAsDuasMetadesDaSafiraAndamJuntas é a trava contra o ouro sem fim.
//
// SE ALGUÉM TIRAR UMA DAS DUAS, este teste falha e diz por quê. Não é zelo: a Safira É
// vendida em loja (o Bardes e o Redmiron), e a regra escrita em vendidasPorUnidade diz
// que um item só entra lá depois de conferir que nenhuma loja o vende em pilha. A Safira
// entra porque a COMPRA também é por unidade — as duas linhas, juntas, são a condição.
func TestAsDuasMetadesDaSafiraAndamJuntas(t *testing.T) {
	t.Parallel()
	if !vendidasPorUnidade[safira] {
		t.Error("a Safira saiu de vendidasPorUnidade: vender a pilha volta a pagar por uma só")
	}
	if !cobradasPorUnidade[safira] {
		t.Error("a Safira saiu de cobradasPorUnidade: comprar a pilha por 1.000.000 e " +
			"revender por N x 125.000 e OURO SEM FIM a partir de N=8")
	}
}

// TestOAlarmeDoEmpilhavelSemEspacoParaQuantidade guarda a linha que faltava.
//
// O QUE ACONTECIA EM SILÊNCIO: o setItemAmount procura um espaço de efeito livre e, se
// os três estiverem ocupados, NÃO FAZ NADA. O item saía do itemToSel empilhável e sem
// EF_AMOUNT, que é o pacote que mata o cliente — o servidor lê a falta do 61 como "um"
// e segue, o cliente não lê e cai no login. O jogador via "o jogo fecha quando eu
// entro" e ninguém tinha por onde começar, porque o servidor não reclamava de nada.
func TestOAlarmeDoEmpilhavelSemEspacoParaQuantidade(t *testing.T) {
	var registro bytes.Buffer
	antes := logDoAlarmeDoItem
	logDoAlarmeDoItem = slog.New(slog.NewTextHandler(&registro, &slog.HandlerOptions{Level: slog.LevelDebug}))
	t.Cleanup(func() { logDoAlarmeDoItem = antes })

	// Um empilhável com os TRÊS espaços de efeito ocupados por outra coisa: não sobra
	// onde pôr a quantidade.
	it := world.Item{Index: safira, Effects: [3]world.Effect{
		{Effect: 3, Value: 10}, {Effect: 4, Value: 20}, {Effect: 5, Value: 30},
	}}
	_ = itemToSel(it)

	escrito := registro.String()
	if !bytesContem(escrito, "item empilhavel sem espaco para quantidade") {
		t.Fatalf("o alarme nao saiu. Log: %s", escrito)
	}
	// O ÍNDICE TEM DE ESTAR NA LINHA: sem ele o log diz que há um item torto e não diz
	// qual, e quem for procurar não tem por onde começar.
	if !bytesContem(escrito, "697") {
		t.Errorf("o alarme nao diz qual item e: %s", escrito)
	}

	// E O CAMINHO NORMAL NÃO ALARMA, senão o log viraria ruído e ninguém o leria: um
	// empilhável com espaço recebe a quantidade e sai calado.
	registro.Reset()
	_ = itemToSel(world.Item{Index: safira})
	if registro.Len() != 0 {
		t.Errorf("o alarme saiu num item normal: %s", registro.String())
	}
}

// safiraEmPilha monta uma Safira com a quantidade pedida.
func safiraEmPilha(n int) world.Item {
	it := world.Item{Index: safira}
	setItemAmount(&it, n)
	return it
}

func bytesContem(s, sub string) bool {
	return bytes.Contains([]byte(s), []byte(sub))
}
