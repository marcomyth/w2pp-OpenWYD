package handler

import (
	"log/slog"
	"testing"

	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/world"
)

// TestFadaJuntaPilhas pins which fairies merge what comes in: the Vermelha and
// the two drop fairies (Azul and do Vale). The Verde is deliberately out — it
// carries the Água run instead (fada_leva_agua.go) — and the Vermelha is in
// both lists because it pays both bonuses.
func TestFadaJuntaPilhas(t *testing.T) {
	casos := []struct {
		nome  string
		fada  int16
		quero bool
	}{
		{"sem fada", 0, false},
		{"Vermelha 3 dias", 3902, true},
		{"Vermelha 5 dias", 3905, true},
		{"Vermelha 7 dias", 3908, true},
		{"Azul 3 dias", 3901, true},
		{"Azul 5 dias", 3904, true},
		{"Azul 7 dias", 3907, true},
		{"do Vale", 3916, true},
		{"Verde", 3900, false},
		{"Suprema", 3913, false},
		{"Prateada", 3914, false},
		{"Dourada", 3915, false},
	}
	for _, c := range casos {
		t.Run(c.nome, func(t *testing.T) {
			e := &world.Entity{}
			e.Equip[fairyEquipSlot] = world.Item{Index: c.fada}
			if got := fadaJuntaPilhas(e); got != c.quero {
				t.Errorf("fadaJuntaPilhas(%d) = %v, quero %v", c.fada, got, c.quero)
			}
		})
	}
	if fadaJuntaPilhas(nil) {
		t.Error("fadaJuntaPilhas(nil) ligou o merge")
	}
}

// TestItensQueEmpilham is the list as it was asked for. Each family is here
// because a farm run produces it by the dozen; each exclusion is here because
// putting it in would be a decision nobody took.
func TestItensQueEmpilham(t *testing.T) {
	empilham := map[string][]int16{
		"Poeira de Ori/Lac":      {412, 413},
		"Resto de Ori/Lac":       {419, 420},
		"Pedra do Sábio":         {1774},
		"Âmagos (ponta a ponta)": {2390, 2405, 2419},
		"Gemas":                  {2441, 2442, 2443, 2444},
		"Água M (LV1..Neses)":    {777, 780, 784, 785},
		"Água N (LV1..Neses)":    {3173, 3176, 3180, 3181},
		"Água A (LV1..Neses)":    {3182, 3186, 3189, 3190},
		"Barras de gold":         {4010, 4011, 4028, 4029},
		"Classes A-E e (P)":      {4016, 4019, 4020, 4021, 4024, 4025},
		"Moedas de Prata":        {4026, 4027},
		"Moeda de cash":          {3393, 3394},
	}
	for nome, idxs := range empilham {
		for _, idx := range idxs {
			if !isSplittable(idx) {
				t.Errorf("%s: item %d não empilha", nome, idx)
			}
		}
	}

	naoEmpilham := map[string]int16{
		"Barra de Mithril é material de máquina": 3027,
		"logo antes dos Âmagos":                  2389,
		"logo depois dos Âmagos":                 2420,
		"logo antes das gemas":                   2440,
		"logo depois das gemas":                  2445,
		"logo antes da Água M":                   776,
		"logo depois da Água M":                  786,
		"logo antes da Água N":                   3172,
		"logo depois da Água A":                  3191,
		"logo antes das Classes (Capa)":          4015,
		"logo depois das Moedas de Prata":        4030,
		"logo antes da moeda de cash":            3392,
		// A Barra de Ouro (3000Cash) é da mesma família das duas acima; fica de
		// fora porque ninguém pediu, e o teste avisa se ela entrar sem decisão.
		"Barra de Ouro (3000Cash)": 3395,
		"uma espada":               30,
	}
	for nome, idx := range naoEmpilham {
		if isSplittable(idx) {
			t.Errorf("%s: item %d passou a empilhar sem ninguém decidir", nome, idx)
		}
	}
}

func fixturaPilha(t *testing.T) (*Dispatcher, *world.World, *world.Entity) {
	t.Helper()
	log := slog.New(slog.DiscardHandler)
	d := New(Config{Log: log})
	w := world.New(world.Config{GridDim: 16}, log, nil, nil)
	e := &world.Entity{ID: 0, Mode: world.MobUser, Name: "Dono"}
	return d, w, e
}

// TestPutCarryItemJuntaComFada is the feature: the drop lands on the pile it
// belongs to instead of taking the next free slot.
func TestPutCarryItemJuntaComFada(t *testing.T) {
	d, w, e := fixturaPilha(t)
	e.Equip[fairyEquipSlot] = world.Item{Index: 3902} // Vermelha
	e.Carry[0] = world.Item{Index: 419, Effects: [3]world.Effect{{Effect: efAmount, Value: 61}}}

	slot := d.putCarryItem(w, e, world.Item{Index: 419})

	if slot != 0 {
		t.Fatalf("o Resto foi para o slot %d, quero o 0 (a pilha)", slot)
	}
	if got := itemAmount(e.Carry[0]); got != 62 {
		t.Errorf("pilha = %d, quero 62", got)
	}
	if !e.Carry[1].Empty() {
		t.Errorf("o slot 1 foi gasto com %d", e.Carry[1].Index)
	}
}

// Sem fada nada muda: o item toma o próximo slot livre, que é o comportamento
// que todo mundo tem hoje.
func TestPutCarryItemSemFadaNaoJunta(t *testing.T) {
	d, w, e := fixturaPilha(t)
	e.Carry[0] = world.Item{Index: 419, Effects: [3]world.Effect{{Effect: efAmount, Value: 61}}}

	if slot := d.putCarryItem(w, e, world.Item{Index: 419}); slot != 1 {
		t.Errorf("sem fada o item foi para o slot %d, quero o 1", slot)
	}
	if got := itemAmount(e.Carry[0]); got != 61 {
		t.Errorf("a pilha mudou para %d sem fada nenhuma", got)
	}
}

// O teto de 120 é o mesmo do merge manual: o que não cabe fica num slot novo,
// e o que cabe entra. É o caso que decide se a fada some com item.
func TestPutCarryItemRespeitaOTetoDe120(t *testing.T) {
	d, w, e := fixturaPilha(t)
	e.Equip[fairyEquipSlot] = world.Item{Index: 3901} // Azul
	e.Carry[0] = world.Item{Index: 412, Effects: [3]world.Effect{{Effect: efAmount, Value: 119}}}

	entrando := world.Item{Index: 412, Effects: [3]world.Effect{{Effect: efAmount, Value: 5}}}
	d.putCarryItem(w, e, entrando)

	if got := itemAmount(e.Carry[0]); got != maxStackAmount {
		t.Errorf("pilha = %d, quero o teto %d", got, maxStackAmount)
	}
	if e.Carry[1].Index != 412 {
		t.Fatalf("o resto da pilha não foi para o slot 1 (index %d)", e.Carry[1].Index)
	}
	if got := itemAmount(e.Carry[1]); got != 4 {
		t.Errorf("sobra = %d, quero 4 (119+5 = 120 e sobram 4)", got)
	}
}

// Bolsa cheia sem pilha compatível é a única perda possível, e ela tem de ser
// relatada: quem chama avisa o jogador.
func TestPutCarryItemSemEspacoRelataFalha(t *testing.T) {
	d, w, e := fixturaPilha(t)
	e.Equip[fairyEquipSlot] = world.Item{Index: 3902}
	for i := 0; i < baseCarrySlots; i++ {
		e.Carry[i] = world.Item{Index: 30} // espadas: não empilham
	}

	if slot := d.putCarryItem(w, e, world.Item{Index: 419}); slot >= 0 {
		t.Errorf("bolsa cheia devolveu o slot %d em vez de -1", slot)
	}
}

// Item que não empilha nunca é mesclado, mesmo com a fada: uma capa +9 não pode
// virar "duas capas" num slot.
func TestPutCarryItemNaoJuntaOQueNaoEmpilha(t *testing.T) {
	d, w, e := fixturaPilha(t)
	e.Equip[fairyEquipSlot] = world.Item{Index: 3916} // do Vale
	e.Carry[0] = world.Item{Index: 30}

	if slot := d.putCarryItem(w, e, world.Item{Index: 30}); slot != 1 {
		t.Errorf("a espada foi para o slot %d, quero o 1", slot)
	}
}

// TestMaquinaSeparaAPilha is the loss the merging fairy would otherwise cause:
// a machine used to wipe the input slot, so a pile of 120 paid for one attempt.
func TestMaquinaSeparaAPilha(t *testing.T) {
	d, w, e := fixturaPilha(t)
	s := &world.Session{Conn: 0, Mode: world.UserPlay}
	e.Carry[3] = world.Item{Index: 2390, Effects: [3]world.Effect{{Effect: efAmount, Value: 120}}}

	if !d.separarUnidadesParaMaquina(w, s, e, []int{3}, umaUnidade) {
		t.Fatal("a máquina recusou uma pilha com a bolsa vazia")
	}
	if got := itemAmount(e.Carry[3]); got != 1 {
		t.Errorf("o slot da máquina ficou com %d, quero 1", got)
	}
	resto := -1
	for i := 0; i < baseCarrySlots; i++ {
		if i != 3 && e.Carry[i].Index == 2390 {
			resto = i
		}
	}
	if resto < 0 {
		t.Fatal("o resto da pilha não foi para nenhum slot: 119 Âmagos sumiram")
	}
	if got := itemAmount(e.Carry[resto]); got != 119 {
		t.Errorf("resto = %d, quero 119", got)
	}
}

// Sem espaço para o resto, a máquina recusa ANTES de consumir. O contrário é
// cobrar a pilha inteira por uma tentativa.
func TestMaquinaRecusaSemEspacoParaOResto(t *testing.T) {
	d, w, e := fixturaPilha(t)
	s := &world.Session{Conn: 0, Mode: world.UserPlay}
	for i := 0; i < baseCarrySlots; i++ {
		e.Carry[i] = world.Item{Index: 30}
	}
	e.Carry[3] = world.Item{Index: 2390, Effects: [3]world.Effect{{Effect: efAmount, Value: 120}}}

	if d.separarUnidadesParaMaquina(w, s, e, []int{3}, umaUnidade) {
		t.Error("a máquina aceitou a pilha sem ter onde devolver o resto")
	}
	if got := itemAmount(e.Carry[3]); got != 120 {
		t.Errorf("a pilha foi mexida (%d) numa recusa: nada pode ser gasto", got)
	}
}

// Um slot com uma unidade só é o caso normal, e ele não pode ganhar tratamento
// especial nenhum: a máquina segue consumindo o slot como sempre fez.
func TestMaquinaNaoMexeEmSlotDeUmaUnidade(t *testing.T) {
	d, w, e := fixturaPilha(t)
	s := &world.Session{Conn: 0, Mode: world.UserPlay}
	e.Carry[0] = world.Item{Index: 2390}

	if !d.separarUnidadesParaMaquina(w, s, e, []int{0}, umaUnidade) {
		t.Fatal("a máquina recusou um item sem pilha")
	}
	if e.Carry[0].Index != 2390 || !e.Carry[1].Empty() {
		t.Error("a máquina mexeu na bolsa para um item que não era pilha")
	}
}

// TestEntregaEstampaOAmount is the crash guard: a stackable stored with no
// EF_AMOUNT kills the client when the login blob arrives, and the Água reward is
// minted bare (world.Item{Index: …}).
func TestEntregaEstampaOAmount(t *testing.T) {
	d, w, e := fixturaPilha(t)

	slot := d.putCarryItem(w, e, world.Item{Index: 3174}) // Pergaminho da Água (N) LV2
	if slot < 0 {
		t.Fatal("o pergaminho não entrou na bolsa")
	}
	if !hasAmountEffect(e.Carry[slot]) {
		t.Errorf("o pergaminho ficou sem EF_AMOUNT: %v", e.Carry[slot].Effects)
	}
	if got := itemAmount(e.Carry[slot]); got != 1 {
		t.Errorf("amount = %d, quero 1", got)
	}
	if n := countStacksMissingAmount(e.Carry[:]); n != 0 {
		t.Errorf("%d itens empilháveis sem amount na bolsa", n)
	}
}

// O que não empilha continua sem amount: escrever um lá gastaria um slot de
// efeito de uma espada por nada.
func TestEntregaNaoEstampaOQueNaoEmpilha(t *testing.T) {
	d, w, e := fixturaPilha(t)

	slot := d.putCarryItem(w, e, world.Item{Index: 30})
	if slot < 0 {
		t.Fatal("a espada não entrou na bolsa")
	}
	if hasAmountEffect(e.Carry[slot]) {
		t.Errorf("a espada ganhou EF_AMOUNT: %v", e.Carry[slot].Effects)
	}
}

// O prêmio numerado de evento usa os três slots de efeito, e o terceiro é o
// filler que setItemAmount sobrescreveria. A numeração vence: nada é estampado
// quando não há slot livre.
func TestEntregaNaoRoubaOSlotDoSerialDeEvento(t *testing.T) {
	d, w, e := fixturaPilha(t)
	numerado := world.Item{Index: 3174, Effects: [3]world.Effect{
		{Effect: eventSerialHi, Value: 0},
		{Effect: eventSerialLo, Value: 100},
		{Effect: eventSerialRand, Value: 77},
	}}

	slot := d.putCarryItem(w, e, numerado)
	if slot < 0 {
		t.Fatal("o prêmio não entrou na bolsa")
	}
	if got := e.Carry[slot].Effects; got != numerado.Effects {
		t.Errorf("efeitos = %v, quero o serial intacto %v", got, numerado.Effects)
	}
}

// TestMaquinaCobraDezQuandoAReceitaPedeDez is the exploit the split opened and
// this closes. Three recipes are priced in poeira — Ehre "Misteriosa", Lindy and
// the Odin +12 — and they match on the AMOUNT in the slot (>= 10). Leaving a
// single unit there handed out the result for a tenth of its price.
func TestMaquinaCobraDezQuandoAReceitaPedeDez(t *testing.T) {
	d, w, e := fixturaPilha(t)
	s := &world.Session{Conn: 0, Mode: world.UserPlay}
	e.Carry[2] = world.Item{Index: 413, Effects: [3]world.Effect{{Effect: efAmount, Value: 120}}}

	if !d.separarUnidadesParaMaquina(w, s, e, []int{2}, precisaDeDez(2)) {
		t.Fatal("a máquina recusou com a bolsa vazia")
	}
	if got := itemAmount(e.Carry[2]); got != 10 {
		t.Errorf("o slot da máquina ficou com %d poeiras, quero 10", got)
	}
	resto := -1
	for i := 0; i < baseCarrySlots; i++ {
		if i != 2 && e.Carry[i].Index == 413 {
			resto = i
		}
	}
	if resto < 0 {
		t.Fatal("o resto da pilha não foi devolvido")
	}
	if got := itemAmount(e.Carry[resto]); got != 110 {
		t.Errorf("resto = %d, quero 110 (120 − 10)", got)
	}
}

// Dez exatos é o caso do jogador que montou o slot à mão: nada a separar, e o
// slot tem de ficar intocado para a receita continuar batendo.
func TestMaquinaNaoMexeEmDezExatos(t *testing.T) {
	d, w, e := fixturaPilha(t)
	s := &world.Session{Conn: 0, Mode: world.UserPlay}
	e.Carry[2] = world.Item{Index: 413, Effects: [3]world.Effect{{Effect: efAmount, Value: 10}}}

	if !d.separarUnidadesParaMaquina(w, s, e, []int{2}, precisaDeDez(2)) {
		t.Fatal("a máquina recusou dez poeiras exatas")
	}
	if got := itemAmount(e.Carry[2]); got != 10 {
		t.Errorf("o slot ficou com %d, quero os 10 intactos", got)
	}
	if !e.Carry[0].Empty() {
		t.Error("a máquina espalhou poeira pela bolsa sem precisar")
	}
}

// precisaDeDez cobra dez SÓ nos slots nomeados: as outras células da mesma
// receita seguem valendo uma unidade.
func TestPrecisaDeDezSoNosSlotsNomeados(t *testing.T) {
	precisa := precisaDeDez(0, 1)
	if got := precisa(0); got != 10 {
		t.Errorf("slot 0 = %d, quero 10", got)
	}
	if got := precisa(1); got != 10 {
		t.Errorf("slot 1 = %d, quero 10", got)
	}
	for _, sl := range []int{2, 3, 6} {
		if got := precisa(sl); got != 1 {
			t.Errorf("slot %d = %d, quero 1", sl, got)
		}
	}
	if got := umaUnidade(0); got != 1 {
		t.Errorf("umaUnidade = %d, quero 1", got)
	}
}

// O troféu da Quest 256 junta SEM FADA (juntaNaPilhaDaMochila): ele cai de todo
// monstro e é gasto às centenas, e sem isto uma rodada de arena acaba com dez
// espaços do mesmo troféu — e o clique único da pilha inteira não teria pilha
// nenhuma para gastar.
func TestTrofeuJuntaSemFada(t *testing.T) {
	for _, idx := range []int16{4117, 4118, 4119, 4120, 4121} {
		d, w, e := fixturaPilha(t)
		e.Carry[0] = world.Item{Index: idx, Effects: [3]world.Effect{{Effect: efAmount, Value: 61}}}

		if slot := d.putCarryItem(w, e, world.Item{Index: idx}); slot != 0 {
			t.Errorf("troféu %d foi para o slot %d, quero o 0 (a pilha)", idx, slot)
		}
		if got := itemAmount(e.Carry[0]); got != 62 {
			t.Errorf("troféu %d: pilha = %d, quero 62", idx, got)
		}
		if !e.Carry[1].Empty() {
			t.Errorf("troféu %d gastou o slot 1", idx)
		}
	}
}

// O teto de 120 vale para o troféu como para qualquer pilha: sem fada nenhuma, o
// que passa do teto vai para um espaço novo em vez de sumir.
func TestTrofeuSemFadaRespeitaOTeto(t *testing.T) {
	d, w, e := fixturaPilha(t)
	e.Carry[0] = world.Item{Index: 4117, Effects: [3]world.Effect{{Effect: efAmount, Value: 119}}}

	d.putCarryItem(w, e, world.Item{Index: 4117, Effects: [3]world.Effect{{Effect: efAmount, Value: 5}}})

	if got := itemAmount(e.Carry[0]); got != maxStackAmount {
		t.Errorf("pilha = %d, quero o teto %d", got, maxStackAmount)
	}
	if e.Carry[1].Index != 4117 || itemAmount(e.Carry[1]) != 4 {
		t.Errorf("sobra = %v amount %d, quero 4 troféus no slot 1", e.Carry[1].Index, itemAmount(e.Carry[1]))
	}
}

// A porta livre é SÓ dos cinco troféus: o resto continua dependendo da fada, que é
// o que se paga por ela.
func TestSoOTrofeuJuntaSemFada(t *testing.T) {
	e := &world.Entity{}
	for _, idx := range []int16{4117, 4121} {
		if !juntaNaPilhaDaMochila(e, idx) {
			t.Errorf("troféu %d não junta sem fada", idx)
		}
	}
	for _, idx := range []int16{419, 412, 2390, 2441, 4116, 4122} {
		if juntaNaPilhaDaMochila(e, idx) {
			t.Errorf("item %d passou a juntar sem fada sem ninguém decidir", idx)
		}
	}
}

// TestMoedasDoBauJuntamNumaPilhaSo é o pedido de 20/09/2026, depois de o Marco
// abrir 128 baús: a Moeda de Prata (5Mi) e a moeda de cash saíam uma por slot e
// enchiam a bolsa. Com elas na lista de pilha, cada nova cópia cai na pilha que
// já está lá — inclusive sobre as avulsas de antes, que não têm EF_AMOUNT.
func TestMoedasDoBauJuntamNumaPilhaSo(t *testing.T) {
	for _, item := range []int16{itemMoeda5KK, itemMoeda1KK, itemMoedaWYD200} {
		d, w, e := fixturaPilha(t)
		e.Equip[fairyEquipSlot] = world.Item{Index: 3902} // Vermelha, a que junta
		e.Carry[0] = world.Item{Index: item}              // a avulsa que ele já tinha

		for i := 0; i < 9; i++ {
			if slot := d.putCarryItem(w, e, world.Item{Index: item}); slot != 0 {
				t.Fatalf("item %d: cópia %d foi para o slot %d, queria a pilha do 0", item, i+1, slot)
			}
		}
		if got := itemAmount(e.Carry[0]); got != 10 {
			t.Errorf("item %d: pilha ficou com %d, queria 10", item, got)
		}
		for i := 1; i < 5; i++ {
			if !e.Carry[i].Empty() {
				t.Errorf("item %d: o slot %d foi ocupado à toa", item, i)
			}
		}
	}
}
