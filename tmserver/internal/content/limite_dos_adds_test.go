package content

import (
	"testing"

	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/refine"
	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/world"
)

// As peças que o jogo entrega por nível (LevelItem.txt) cabem no teto dos adds de
// armadura (Marco, 27/09/2026): combinados param em Defesa 30, Crítico 7% e Magia
// 10%, e Defesa acima de 30 só sozinha. São peças fixas, sem sorteio, e o
// arquivo é editável: este teste é o que avisa se uma linha nova passar do teto.
func TestItemDeNivelCabeNoLimiteDosAdds(t *testing.T) {
	itens, err := LoadItemList(release(t, "Common", "ItemList.csv"))
	if err != nil {
		t.Skipf("Release content unavailable: %v", err)
	}
	pos := itens.Positions()
	tab, _, err := LoadLevelItems(release(t, "TMsrv", "run", "LevelItem.txt"))
	if err != nil {
		t.Skipf("Release content unavailable: %v", err)
	}
	vistos := 0
	for classe := 0; classe < LevelItemClasses; classe++ {
		for construcao := 0; construcao < LevelItemBuilds; construcao++ {
			for nivel := int32(0); nivel < 400; nivel++ {
				li := tab.Para(classe, construcao, nivel)
				if li.Empty() {
					continue
				}
				vistos++
				it := world.Item{Index: li.Index}
				for i, e := range li.Effects {
					it.Effects[i] = world.Effect{Effect: e[0], Value: e[1]}
				}
				if !refine.DentroDoLimite(it, pos[int(li.Index)]) {
					t.Errorf("classe %d, construção %d, nível %d: item %d %+v passa do teto dos adds",
						classe, construcao, nivel, li.Index, it.Effects)
				}
			}
		}
	}
	if vistos < 50 {
		t.Fatalf("só %d peças de nível lidas; o teste não olhou o arquivo", vistos)
	}
}
