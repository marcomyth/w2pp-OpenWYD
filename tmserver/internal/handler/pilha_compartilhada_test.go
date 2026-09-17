package handler

import (
	"testing"

	"github.com/jeanluca/w2pp-openwyd/internal/pilha"
	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/world"
)

// TestAListaDePilhaBateComAsConstantesDoJogo: internal/pilha escreve os índices
// em número; os nomes que o jogo usa têm de continuar dentro dela, e os vizinhos
// de fora, fora. O baú do painel também tem de ser o do mundo.
func TestAListaDePilhaBateComAsConstantesDoJogo(t *testing.T) {
	dentro := []int16{
		itemPedraDoSabio, itemBarraPrata10Mi, itemBarraPrata50Mi, itemBarraPrata100Mi, itemBarraPrata1Bi,
		itemGemaBase, itemGemaLast, itemAguaMBase, itemAguaMLast, itemAguaNBase, itemAguaALast,
		classeALo, classeLast, itemQuestRewardBase, itemQuestRewardLast,
	}
	for _, idx := range dentro {
		if !pilha.Empilha(idx) {
			t.Errorf("o jogo empilha %d, mas internal/pilha não", idx)
		}
	}
	fora := []int16{itemGemaBase - 1, itemGemaLast + 1, itemAguaMBase - 1, itemAguaMLast + 1,
		itemAguaNBase - 1, itemAguaALast + 1, classeALo - 1, classeLast + 1, itemQuestRewardBase - 1, itemQuestRewardLast + 1}
	for _, idx := range fora {
		if pilha.Empilha(idx) {
			t.Errorf("internal/pilha empilha %d, fora das faixas do jogo", idx)
		}
	}
	if pilha.EspacosDoBau != world.MaxCargo {
		t.Errorf("pilha.EspacosDoBau = %d, world.MaxCargo = %d", pilha.EspacosDoBau, world.MaxCargo)
	}
}
