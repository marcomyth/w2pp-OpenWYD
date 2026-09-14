package content

import (
	"path/filepath"
	"testing"
)

// TestKeyIDsDoCatalogoReal: o EF_KEYID dos portões e das chaves sai do
// ItemList.csv de verdade, e continua FORA do BaseEffects, que é da conta de
// atributos.
func TestKeyIDsDoCatalogoReal(t *testing.T) {
	items, err := LoadItemList(filepath.Join("..", "..", "..", "Release", "Common", "ItemList.csv"))
	if err != nil {
		t.Skipf("ItemList.csv indisponível: %v", err)
	}
	keys := items.KeyIDs()
	for idx, quer := range map[int]int{
		451: 2, 458: 2, // Chave_da_Primeira_Porta e Primeira_Porta
		452: 3, 459: 3, // Segunda
		453: 4, 461: 4, // Última
		751: 10, 757: 10, // Emblema do Dragão e Portão dos 2 Castelos
	} {
		if got := keys[idx]; got != quer {
			t.Errorf("EF_KEYID do item %d = %d, queria %d", idx, got, quer)
		}
	}
	for _, idx := range []int{451, 458} {
		for _, e := range items.BaseEffects()[idx] {
			if e.Eff == 39 {
				t.Errorf("o item %d levou o EF_KEYID para o BaseEffects, que é da conta de atributos", idx)
			}
		}
	}
}
