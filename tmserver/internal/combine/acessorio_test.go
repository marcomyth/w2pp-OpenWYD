package combine

import (
	"testing"

	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/refine"
	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/world"
)

const efSancTeste = 43

// refinado devolve o item no nível pedido, gravado como o jogo grava.
func refinado(t *testing.T, index int16, level int) world.Item {
	t.Helper()
	it := world.Item{Index: index, Effects: [3]world.Effect{{Effect: efSancTeste}}}
	if level > 0 && !refine.Set(&it, level, 0) {
		t.Fatalf("não deu para gravar +%d no item %d", level, index)
	}
	return it
}

func receita(itens ...world.Item) []world.Item {
	out := make([]world.Item, 8)
	copy(out, itens)
	return out
}

func joias(idx int16) []world.Item {
	return []world.Item{{Index: idx}, {Index: idx}, {Index: idx}, {Index: idx}}
}

func TestAcessorioMais10Recipe(t *testing.T) {
	brinco := func(level int) world.Item { return refinado(t, 595, level) }
	casos := []struct {
		nome  string
		itens []world.Item
		quer  bool
	}{
		{"brinco com Diamantes", receita(append([]world.Item{brinco(9), brinco(9), {Index: 1774}}, joias(2441)...)...), true},
		{"brinco com Corais", receita(append([]world.Item{brinco(9), brinco(9), {Index: 1774}}, joias(2443)...)...), true},
		// As quatro joias valem desde 17/09.
		{"brinco com Esmeraldas", receita(append([]world.Item{brinco(9), brinco(9), {Index: 1774}}, joias(2442)...)...), true},
		{"brinco com Garnets", receita(append([]world.Item{brinco(9), brinco(9), {Index: 1774}}, joias(2444)...)...), true},
		{"bracelete com Garnets", receita(append([]world.Item{refinado(t, 514, 9), refinado(t, 514, 9), {Index: 1774}}, joias(2444)...)...), true},
		{"joia que não é das quatro", receita(append([]world.Item{brinco(9), brinco(9), {Index: 1774}}, joias(2445)...)...), false},
		{"joias misturadas", receita(brinco(9), brinco(9), world.Item{Index: 1774}, world.Item{Index: 2441}, world.Item{Index: 2441}, world.Item{Index: 2443}, world.Item{Index: 2441}), false},
		{"brinco +8", receita(append([]world.Item{brinco(8), brinco(9), {Index: 1774}}, joias(2441)...)...), false},
		{"sacrifício +8", receita(append([]world.Item{brinco(9), brinco(8), {Index: 1774}}, joias(2441)...)...), false},
		{"sem Pedra do Sábio", receita(append([]world.Item{brinco(9), brinco(9), {Index: 413}}, joias(2441)...)...), false},
		// A liberação é por lista: um orb no mesmo espaço de acessório não entra.
		{"orb fora da lista", receita(append([]world.Item{refinado(t, 612, 9), refinado(t, 612, 9), {Index: 1774}}, joias(2441)...)...), false},
		// A Pedra Espiritual é a casa futura da perfuração e da absorção.
		{"Pedra Espiritual fora da lista", receita(append([]world.Item{refinado(t, 633, 9), refinado(t, 633, 9), {Index: 1774}}, joias(2441)...)...), false},
	}
	for _, tc := range casos {
		t.Run(tc.nome, func(t *testing.T) {
			if got := AcessorioMais10Recipe(tc.itens); got != tc.quer {
				t.Errorf("AcessorioMais10Recipe = %v, esperado %v", got, tc.quer)
			}
		})
	}
}

func TestListaDaReforma(t *testing.T) {
	dentro := []int16{591, 595, 507, 510, 514, 551, 554, 555, 558, 559, 562, 567, 570, 661, 663, 762, 768, 1738}
	fora := []int16{501, 508, 509, 515, 516, 523, 563, 612, 633, 640, 3464, 1760}
	for _, idx := range dentro {
		if !AcessorioAteMais15(idx) {
			t.Errorf("%d devia subir até +15", idx)
		}
	}
	for _, idx := range fora {
		if AcessorioAteMais15(idx) {
			t.Errorf("%d não devia estar na lista", idx)
		}
	}
}

func TestEvolucaoRecipe(t *testing.T) {
	casos := []struct {
		nome   string
		itens  []world.Item
		quer   bool
		result int16
	}{
		{"Cristal +9 → Místico", receita(append([]world.Item{refinado(t, 563, 9), refinado(t, 563, 0), {Index: 1774}}, joias(2443)...)...), true, 559},
		{"Necromântica +9 → Siren", receita(append([]world.Item{refinado(t, 655, 9), refinado(t, 655, 3), {Index: 1774}}, joias(2441)...)...), true, 659},
		{"Siren +9 → Ankh", receita(append([]world.Item{refinado(t, 660, 9), refinado(t, 660, 9), {Index: 1774}}, joias(2441)...)...), true, 663},
		{"Cristal +8", receita(append([]world.Item{refinado(t, 563, 8), refinado(t, 563, 0), {Index: 1774}}, joias(2443)...)...), false, 0},
		{"cópia de outra árvore", receita(append([]world.Item{refinado(t, 563, 9), refinado(t, 564, 0), {Index: 1774}}, joias(2443)...)...), false, 0},
		{"Cristal com Esmeraldas", receita(append([]world.Item{refinado(t, 563, 9), refinado(t, 563, 0), {Index: 1774}}, joias(2442)...)...), true, 559},
		{"Ankh não evolui", receita(append([]world.Item{refinado(t, 661, 9), refinado(t, 661, 0), {Index: 1774}}, joias(2441)...)...), false, 0},
	}
	for _, tc := range casos {
		t.Run(tc.nome, func(t *testing.T) {
			if got := EvolucaoRecipe(tc.itens); got != tc.quer {
				t.Fatalf("EvolucaoRecipe = %v, esperado %v", got, tc.quer)
			}
			if !tc.quer {
				return
			}
			if next, _ := EvolucaoAcessorio(tc.itens[0].Index); next != tc.result {
				t.Errorf("evolui em %d, esperado %d", next, tc.result)
			}
		})
	}
}

func TestOdinArcanoRecipe(t *testing.T) {
	secretas := []world.Item{{Index: 5334}, {Index: 5335}, {Index: 5336}, {Index: 5337}}
	casos := []struct {
		nome  string
		itens []world.Item
		quer  int16
	}{
		{"Místico +15 da árvore 3", receita(append([]world.Item{refinado(t, 561, 15), {}, {}}, secretas...)...), 569},
		{"Místico +14", receita(append([]world.Item{refinado(t, 561, 14), {}, {}}, secretas...)...), 0},
		{"célula 1 ocupada", receita(append([]world.Item{refinado(t, 561, 15), {Index: 413}, {}}, secretas...)...), 0},
		{"pedras fora de ordem", receita(refinado(t, 561, 15), world.Item{}, world.Item{}, world.Item{Index: 5335}, world.Item{Index: 5334}, world.Item{Index: 5336}, world.Item{Index: 5337}), 0},
		{"Cristal +15", receita(append([]world.Item{refinado(t, 563, 15), {}, {}}, secretas...)...), 0},
	}
	for _, tc := range casos {
		t.Run(tc.nome, func(t *testing.T) {
			got, ok := OdinArcanoRecipe(tc.itens)
			if ok != (tc.quer != 0) || got != tc.quer {
				t.Errorf("OdinArcanoRecipe = (%d, %v), esperado %d", got, ok, tc.quer)
			}
			// Nenhuma receita numerada do Odin pode reconhecer essa entrada.
			if tc.quer != 0 {
				if id := MatchOdin(Catalog{}, tc.itens); id != OdinNoMatch {
					t.Errorf("MatchOdin também reconhece a receita do Arcano (id %d)", id)
				}
			}
		})
	}
}

// O +12 do Odin passa a aceitar os acessórios da lista, pelo índice — o nPos do
// brinco (256) continua fora da regra das armas.
func TestOdinMais12AceitaAcessorioDaLista(t *testing.T) {
	cat := Catalog{Pos: map[int]int{595: 256, 612: 1024}}
	secretas := []world.Item{{Index: 5334}, {Index: 5335}, {Index: 5336}, {Index: 5337}}
	brinco := receita(append([]world.Item{{Index: 4043}, {Index: 4043}, refinado(t, 595, 11)}, secretas...)...)
	if id := MatchOdin(cat, brinco); id != OdinPlus12 {
		t.Errorf("brinco +11 no Odin = receita %d, esperado +12 (%d)", id, OdinPlus12)
	}
	orb := receita(append([]world.Item{{Index: 4043}, {Index: 4043}, refinado(t, 612, 11)}, secretas...)...)
	if id := MatchOdin(cat, orb); id == OdinPlus12 {
		t.Error("um orb fora da lista passou no +12")
	}
}
