package content

import "testing"

// TestKitDoNovatoNoCatalogo: as duas variantes do kit (/novato) têm de chegar ao
// servidor exatamente como foram escritas — mesmo efeito do item original, preço
// zero e EF_NOTRADE.
//
// Cada uma dessas três coisas falha em silêncio se estiver errada, e é por isso
// que este teste existe em vez de uma conferida a olho no CSV:
//
//   - sem EF_VOLATILE o item vira equipamento no useItem e some da bolsa sem
//     fazer nada;
//   - com preço, o kit inteiro vira gold no primeiro NPC, que é o oposto de ser
//     intransferível;
//   - sem EF_NOTRADE ele passa para outra conta, e a trava de uma vez por conta
//     deixa de valer qualquer coisa.
func TestKitDoNovatoNoCatalogo(t *testing.T) {
	lista, err := LoadItemList(release(t, "Common", "ItemList.csv"))
	if err != nil {
		t.Skipf("ItemList.csv indisponível: %v", err)
	}
	// EF_VOLATILE não é um efeito de score: o parser o guarda à parte, em
	// Volatiles(), e por isso ele não aparece em BaseEffects().
	efeitos, precos, volateis := lista.BaseEffects(), lista.Prices(), lista.Volatiles()

	const efNoTrade = 127
	for _, tc := range []struct {
		idx      int
		nome     string
		volatile int // o mesmo EF_VOLATILE do item de origem
		origem   int
	}{
		{5760, "Frango Assado (Novato)", 63, 3314},
		{5761, "Baú de Experiência (Novato)", 198, 4140},
	} {
		var noTrade int16
		for _, e := range efeitos[tc.idx] {
			if e.Eff == efNoTrade {
				noTrade = e.Val
			}
		}
		if vol := volateis[tc.idx]; vol != tc.volatile {
			t.Errorf("%s: EF_VOLATILE = %d, queria %d (o mesmo do item %d)", tc.nome, vol, tc.volatile, tc.origem)
		}
		if noTrade == 0 {
			t.Errorf("%s: sem EF_NOTRADE — o kit passaria para outra conta", tc.nome)
		}
		if p := precos[tc.idx]; p != 0 {
			t.Errorf("%s: preço = %d, queria 0 — com preço o kit vira gold no NPC", tc.nome, p)
		}
	}
}
