package handler

import (
	"github.com/jeanluca/w2pp-openwyd/internal/droprule"
	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/world"
)

// As arenas da Quest 256 do Coração do Kaizen (passo 3), das Hidras (passo 4) e
// dos Elfos (passo 5) usam templates do legado que só nascem nelas (NPCGener
// 3476-3515). Pagam em Restos e Âmagos pela Mesa de Drops: Kaizen e Hidras desde
// 14/09/2026 (migração 0065), os Elfos com os números das Hidras desde 17/09
// (0075).
// A Mesa diz se o item cai e com que chance, mas não guarda quantidade: o pacote
// que o líder de cada grupo solta mora aqui, como o dos guardiões do Castelo Orc.
//
// A chave é o nome do ARQUIVO do template, como na Mesa, para que um "/gm npc
// criar" do mesmo template siga a regra e nada mais no mundo mude.
var arenaQuest256Pacotes = map[string]map[int16]int{
	droprule.Canonical("Cav._Kaizen"):   {419: 3, 420: 2}, // Resto de Oriharucon, Resto de Lactolerium
	droprule.Canonical("Hidra_Dourada"): {419: 3, 420: 2},
	droprule.Canonical("Mestre_Elfo"):   {419: 3, 420: 2},
}

// arenaQuest256Finish dá ao item que o líder de uma arena soltou o tamanho do
// pacote. Só empilhável ganha quantidade: EF_AMOUNT em outro item é uma pilha
// que o cliente não divide e gasta inteira.
func arenaQuest256Finish(mob *world.Entity, it *world.Item) {
	if mob == nil || mob.TemplateName == "" {
		return
	}
	if n := arenaQuest256Pacotes[droprule.Canonical(mob.TemplateName)][it.Index]; n > 1 && isSplittable(it.Index) {
		setItemAmount(it, n)
	}
}
