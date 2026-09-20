package handler

import (
	"testing"

	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/rng"
)

// As três tabelas têm de somar exatamente 10.000. Uma que não fecha não falha:
// o último prêmio come a sobra calado, e a chance que o painel anuncia deixa de
// ser a que o jogo sorteia.
func TestBauApoiadorTabelasFecham(t *testing.T) {
	t.Parallel()
	casos := []struct {
		nome   string
		tabela []chestPrize
	}{
		{"bronze", bauBronzeTable},
		{"apoiador", bauApoiadorTable},
		{"supremo", bauSupremoTable},
	}
	for _, c := range casos {
		t.Run(c.nome, func(t *testing.T) {
			if got := chestDenominator(c.tabela); got != bauApoiadorTotal {
				t.Errorf("denominador = %d, quer %d", got, bauApoiadorTotal)
			}
			// Os limiares são acumulados: cada um tem de ser maior que o anterior,
			// ou o prêmio do meio nunca sai.
			anterior := 0
			for i, p := range c.tabela {
				if p.upTo <= anterior {
					t.Errorf("linha %d: upTo %d não passa do anterior %d", i, p.upTo, anterior)
				}
				anterior = p.upTo
			}
		})
	}
}

// chancesPorItem devolve, por índice de item, quantas unidades daquele item um
// baú paga em média — a chance da linha vezes o tamanho da pilha.
func chancesPorItem(tabela []chestPrize) map[int16]float64 {
	out := make(map[int16]float64)
	anterior := 0
	for _, p := range tabela {
		chance := float64(p.upTo-anterior) / float64(bauApoiadorTotal)
		anterior = p.upTo
		qtd := 1
		if n := itemAmount(p.item); n > 1 {
			qtd = n
		}
		out[p.item.Index] += chance * float64(qtd)
	}
	return out
}

// O invariante que mais importa: o Supremo paga >= o Apoiador em todo prêmio.
// A soma tem de fechar 10.000, então subir uma linha baixa outra — é fácil
// melhorar o baú de topo num prêmio e piorá-lo noutro sem perceber.
func TestBauSupremoNuncaPerdeParaOApoiador(t *testing.T) {
	t.Parallel()
	apoiador := chancesPorItem(bauApoiadorTable)
	supremo := chancesPorItem(bauSupremoTable)
	for item, quanto := range apoiador {
		if supremo[item]+1e-9 < quanto {
			t.Errorf("item %d: Supremo paga %.4f por baú, Apoiador paga %.4f", item, supremo[item], quanto)
		}
	}
}

// O Bronze não dá ovo nem âmago de Equipado: é a escada inteira do brinde, e um
// ovo escapando para o baú de R$ 29,90 desfaz o argumento de venda do Prata.
func TestBauBronzeNaoDaOvoNemEquipado(t *testing.T) {
	t.Parallel()
	proibidos := map[int16]string{
		itemOvoEquipN:   "ovo de Equipado N",
		itemOvoEquipB:   "ovo de Equipado B",
		itemOvoLeveN:    "ovo de Leve N",
		itemOvoLeveB:    "ovo de Leve B",
		itemAmagoEquipN: "âmago de Equipado N",
		itemAmagoEquipB: "âmago de Equipado B",
	}
	for _, p := range bauBronzeTable {
		if nome, proibido := proibidos[p.item.Index]; proibido {
			t.Errorf("Baú Bronze paga %s (%d)", nome, p.item.Index)
		}
	}
}

// Os três índices definitivos e os três emprestados abrem, e abrem a MESMA
// tabela — um teste em jogo que rodasse noutra tabela não testaria nada.
func TestBauApoiadorRegistradoNosDoisIndices(t *testing.T) {
	t.Parallel()
	pares := []struct {
		nome              string
		definitivo, teste int16
		esperada          []chestPrize
	}{
		{"bronze", itemBauBronze, itemBauBronzeTeste, bauBronzeTable},
		{"apoiador", itemBauApoiador, itemBauApoiadorTeste, bauApoiadorTable},
		{"supremo", itemBauSupremo, itemBauSupremoTeste, bauSupremoTable},
	}
	for _, par := range pares {
		t.Run(par.nome, func(t *testing.T) {
			for _, idx := range []int16{par.definitivo, par.teste} {
				got, ok := chestTables[idx]
				if !ok {
					t.Fatalf("índice %d não abre", idx)
				}
				if len(got) != len(par.esperada) || chestDenominator(got) != chestDenominator(par.esperada) {
					t.Errorf("índice %d abre outra tabela", idx)
				}
			}
		})
	}
}

// As tabelas do legado continuam em base 100 — a mudança do denominador não
// podia reescalar o que já estava em produção.
func TestBausDoLegadoSeguemEmCem(t *testing.T) {
	t.Parallel()
	for _, idx := range []int16{itemBauPedraSecreta, 3213, 3217, 3218, itemBauRunas} {
		if got := chestDenominator(chestTables[idx]); got != 100 {
			t.Errorf("baú %d: denominador = %d, quer 100", idx, got)
		}
	}
}

// drawChest tem de acertar as bordas: o primeiro número de cada faixa cai no
// prêmio daquela faixa, e o último número possível cai no último prêmio.
func TestDrawChestBordasDoBauSupremo(t *testing.T) {
	t.Parallel()
	casos := []struct {
		roll int
		quer int16
	}{
		{0, itemPergaAguaN1},
		{2399, itemPergaAguaN1},
		{2400, itemPoeiraLacto}, // a pilha de 5
		{4599, itemPoeiraLacto},
		{4600, itemAmagoLeveN},
		{7399, itemAmagoLeveB},
		{7400, itemBauExp},
		{8399, itemBauExp},
		{8400, itemMoeda5KK},
		{9199, itemMoeda5KK},
		{9200, itemMoedaWYD200},
		{9599, itemMoedaWYD200},
		{9600, itemAmagoEquipN},
		{9839, itemAmagoEquipN},
		{9840, itemAmagoEquipB},
		{9959, itemAmagoEquipB},
		{9960, itemOvoEquipN},
		{9989, itemOvoEquipN},
		{9990, itemOvoEquipB},
		{9999, itemOvoEquipB},
	}
	for _, c := range casos {
		if got := drawChest(bauSupremoTable, c.roll).Index; got != c.quer {
			t.Errorf("roll %d = item %d, quer %d", c.roll, got, c.quer)
		}
	}
}

// A pilha de entrega leva EF_AMOUNT, senão a fila manda 64 itens avulsos e o
// baú da conta enche.
func TestBauApoiadorItemLevaAQuantidade(t *testing.T) {
	t.Parallel()
	it := bauApoiadorItem(itemBauSupremo, 64)
	if it.Index != itemBauSupremo {
		t.Errorf("índice = %d, quer %d", it.Index, itemBauSupremo)
	}
	if got := itemAmount(it); got != 64 {
		t.Errorf("quantidade = %d, quer 64", got)
	}
}

// O sorteio fino NÃO pode ser rand()%10000: o rand() do MSVC vai só até 32767,
// então os valores abaixo de 2768 sairiam quatro vezes contra três de todos os
// outros, e um prêmio escrito como 24% pagaria 29,3%. Este teste mede as duas
// formas lado a lado e falha se chestRoll voltar a ser a ingênua.
func TestChestRollFinoNaoHerdaOViesDoLegado(t *testing.T) {
	t.Parallel()
	const rodadas = 400000
	// A faixa que o módulo ingênuo favorece: [0, 32768%10000) = [0, 2768).
	const favorecida = 32768 % bauApoiadorTotal

	contar := func(sorteia func() int) float64 {
		dentro := 0
		for i := 0; i < rodadas; i++ {
			if sorteia() < favorecida {
				dentro++
			}
		}
		return float64(dentro) / rodadas
	}

	esperado := float64(favorecida) / bauApoiadorTotal // 0,2768

	ingenuo := rng.NewSeeded(7)
	if got := contar(func() int { return ingenuo.Intn(bauApoiadorTotal) }); got < esperado*1.2 {
		t.Errorf("o módulo ingênuo deveria estar bem enviesado, mas deu %.4f (esperado ~%.4f)", got, esperado)
	}

	fino := rng.NewSeeded(7)
	got := contar(func() int { return chestRoll(fino, bauApoiadorTotal) })
	if got < esperado*0.98 || got > esperado*1.02 {
		t.Errorf("chestRoll enviesado: faixa favorecida saiu em %.4f, quer ~%.4f", got, esperado)
	}
}

// As tabelas do legado continuam gastando UMA chamada ao rand, com o viés que
// elas sempre tiveram — as chances delas vieram de um servidor que o tinha.
func TestChestRollDoLegadoGastaUmaChamadaSo(t *testing.T) {
	t.Parallel()
	a := rng.NewSeeded(123)
	b := rng.NewSeeded(123)
	_ = chestRoll(a, 100)
	esperado := b.Rand() // uma única chamada consumida
	if got := a.Rand(); got != b.Rand() {
		t.Errorf("a tabela de base 100 consumiu mais de um rand (a=%d, b esperava seguir de %d)", got, esperado)
	}
}

// Um sorteio longo tem de reproduzir as chances declaradas.
func TestBauSupremoDistribuicaoBate(t *testing.T) {
	t.Parallel()
	r := rng.NewSeeded(20260918)
	const rodadas = 200000
	saiu := make(map[int16]int)
	for i := 0; i < rodadas; i++ {
		saiu[drawChest(bauSupremoTable, chestRoll(r, bauApoiadorTotal)).Index]++
	}
	// A perga é 24%: com 200 mil rodadas o desvio esperado é de décimos de ponto.
	perga := float64(saiu[itemPergaAguaN1]) / rodadas
	if perga < 0.23 || perga > 0.25 {
		t.Errorf("pergaminho saiu em %.3f das vezes, quer ~0,240", perga)
	}
	// Os dois ovos somam 0,4%: o teste é frouxo de propósito, porque é o número
	// pequeno e o que importa é que ele NÃO seja da ordem de 1%.
	ovos := float64(saiu[itemOvoEquipN]+saiu[itemOvoEquipB]) / rodadas
	if ovos < 0.002 || ovos > 0.007 {
		t.Errorf("ovos saíram em %.4f das vezes, quer ~0,004", ovos)
	}
}
