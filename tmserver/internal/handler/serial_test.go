package handler

import (
	"testing"

	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/refine"
	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/world"
)

// refinado builds an item at a real refine level.
//
// It goes through refine.Set rather than writing the byte directly: from +10 up
// the level is packed into a gem range, so an item with EF_SANC = 11 is refine
// ONE, not eleven. A test that wrote the raw byte would be asserting against a
// level the game never sees.
func refinado(index int16, nivel int) world.Item {
	it := world.Item{Index: index}
	// Bootstrap plants the EF_SANC pair; Set never allocates one, the same way
	// the anvil does not.
	if !refine.Bootstrap(&it) || !refine.Set(&it, nivel, 0) {
		panic("não consegui montar um item refinado para o teste")
	}
	return it
}

// comEfeito builds an item carrying one effect pair.
func comEfeito(index int16, ef, val uint8) world.Item {
	it := world.Item{Index: index}
	it.Effects[0] = world.Effect{Effect: ef, Value: val}
	return it
}

func TestMarcavelSegueARegraDoJogo(t *testing.T) {
	d := New(Config{ItemPos: map[int]int{
		1100: 8,              // espada comum
		1500: serialPosAlto1, // classe de slot com barra mais alta
		1600: serialPosAlto2, //
	}})

	casos := []struct {
		nome  string
		it    world.Item
		quer  bool
		porqu string
	}{
		{"espada crua", world.Item{Index: 1100}, false,
			"item sem refino e sem bônus não interessa a ninguém"},
		{"refino 3", refinado(1100, serialSanc), true,
			"é o corte que o próprio jogo usava para decidir o que registrar"},
		{"refino 2", refinado(1100, serialSanc-1), false,
			"abaixo do corte, e sem bônus que salve"},
		{"refino 11", refinado(1100, 11), true,
			"o alvo principal: equipamento de topo"},
		{"dano alto", comEfeito(1100, efDamage, serialDam), true,
			"bônus grande vale tanto quanto refino"},
		{"dano baixo", comEfeito(1100, efDamage, serialDam-1), false, ""},
		{"magia alta", comEfeito(1100, efMagic, serialMagic), true, ""},
		{"item de guilda", comEfeito(1100, efHWordGuild, 4), true,
			"pertence a uma guilda, não a uma pessoa"},
		{"outra metade da guilda", comEfeito(1100, efLWordGuild, 9), true, ""},
		{"índice escolhido a dedo", world.Item{Index: 753}, true,
			"da lista que o BASE_NeedLog trazia"},
		{"slot vazio", world.Item{}, false, ""},
	}
	for _, c := range casos {
		if got := d.Marcavel(c.it); got != c.quer {
			t.Errorf("%s: marcável = %v, want %v%s", c.nome, got, c.quer,
				func() string {
					if c.porqu == "" {
						return ""
					}
					return " — " + c.porqu
				}())
		}
	}
}

// A barra mais alta para nPos 64 e 192 é do original: essas peças já carregam
// números maiores, então o limiar comum acusaria todas.
func TestMarcavelUsaABarraAltaOndeOOriginalUsava(t *testing.T) {
	d := New(Config{ItemPos: map[int]int{1500: serialPosAlto1, 1100: 8}})

	meio := uint8(serialDam) // passa no limiar comum, não no alto
	if !d.Marcavel(comEfeito(1100, efDamage, meio)) {
		t.Error("item de slot comum devia passar no limiar comum")
	}
	if d.Marcavel(comEfeito(1500, efDamage, meio)) {
		t.Error("item de slot alto passou com dano de item comum")
	}
	if !d.Marcavel(comEfeito(1500, efDamage, uint8(serialDamAlto))) {
		t.Error("item de slot alto não passou nem no limiar dele")
	}
}

// TestMarcavelNaoMarcaEmpilhavel: uma pilha junta e divide, e identidade que
// some numa junção não é identidade. Vale mesmo para os índices que a lista do
// original trazia.
func TestMarcavelNaoMarcaEmpilhavel(t *testing.T) {
	d := New(Config{})
	for _, idx := range []int16{412, 413, 419, 420} {
		if !serialIndices[idx] {
			t.Fatalf("o teste está errado: %d devia estar na lista do original", idx)
		}
		if !isSplittable(idx) {
			t.Fatalf("o teste está errado: %d devia ser empilhável", idx)
		}
		if d.Marcavel(world.Item{Index: idx}) {
			t.Errorf("índice %d foi marcado apesar de empilhável", idx)
		}
	}
	// Refino não muda isso: uma pilha refinada continua sendo uma pilha.
	if d.Marcavel(refinado(412, 11)) {
		t.Error("pilha refinada foi marcada")
	}
}

// A regressão que justifica todo o comentário no topo de serial.go. O
// BASE_NeedLog original devolve TRUE para QUALQUER item, por causa de dois
// testes que são sempre verdadeiros (`idx >= 551 || idx <= 562`, e
// `cEffect != 0 || cEffect != 59`). Portado ao pé da letra, cada poção do mundo
// ganharia um número e a tela de repetidos viraria ruído.
func TestMarcavelNaoMarcaPocao(t *testing.T) {
	d := New(Config{ItemPos: map[int]int{2: 0, 30: 0}})
	for _, idx := range []int16{2, 30, 551, 555, 562} {
		if d.Marcavel(world.Item{Index: idx}) {
			t.Errorf("índice %d ganhou número; a régua voltou a ser o bug do original", idx)
		}
	}
}

// Item de crescimento guarda um contador nos bytes de efeito, não um bônus, e
// lê-los como dano dá número grande sem significado nenhum.
func TestMarcavelIgnoraBonusDeItemDeCrescimento(t *testing.T) {
	d := New(Config{})
	if v := serialBonus(comEfeito(serialGrowthLo, efDamage, 99), efDamage); v != 0 {
		t.Errorf("bônus de item de crescimento = %d, want 0", v)
	}
	if d.Marcavel(comEfeito(serialGrowthLo+1, efDamage, 99)) {
		t.Error("item de crescimento foi marcado por um bônus que não existe")
	}
}

// O multiplicador de refino do BASE_GetBonusItemAbility, na única faixa em que
// ele é alcançável: refino 1 e 2 (de 3 para cima a regra já disse sim antes).
func TestSerialBonusAplicaOMultiplicadorDeRefino(t *testing.T) {
	it := world.Item{Index: 1100}
	it.Effects[0] = world.Effect{Effect: efDamage, Value: 10}
	if !refine.Bootstrap(&it) || !refine.Set(&it, 2, 0) {
		t.Fatal("não consegui montar o item refinado")
	}

	// 10 * (2+10) / 10 = 12
	if got := serialBonus(it, efDamage); got != 12 {
		t.Errorf("bônus com refino 2 = %d, want 12", got)
	}
	semRefino := comEfeito(1100, efDamage, 10)
	if got := serialBonus(semRefino, efDamage); got != 10 {
		t.Errorf("bônus sem refino = %d, want 10", got)
	}
}
