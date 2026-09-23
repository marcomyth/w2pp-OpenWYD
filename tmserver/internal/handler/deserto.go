package handler

import (
	"github.com/jeanluca/w2pp-openwyd/internal/droprule"
	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/world"
)

// Os dois Agmo do Deserto (Tauron_Agmo e Verme_Agmo) são monstros de evento: os
// blocos deles (3451, 3452) nascem desligados desde a migração 0108, e a equipe
// os cria à mão num evento. Cada morte paga um pacote de âmagos N: 40 Sem Sela,
// 30 Fantasma, 20 Cavalo Leve e 10 Cavalo Equipado. A Mesa de Drops diz que
// cada um cai sempre; ela não guarda quantidade, então o tamanho do pacote mora
// aqui, como o das arenas da Quest 256.
//
// A chave é o nome do ARQUIVO do template, como na Mesa, para que o
// "/gm criar Tauron_Agmo" do evento siga a regra.
var desertoPacotes = map[string]map[int16]int{
	droprule.Canonical("Tauron_Agmo"): desertoPacoteAgmo,
	droprule.Canonical("Verme_Agmo"):  desertoPacoteAgmo,
}

var desertoPacoteAgmo = map[int16]int{
	2396: 40, // Âmago de Cav s/Sela N
	2397: 30, // Âmago de Cav Fantasm N
	2398: 20, // Âmago de Cavalo Leve N
	2399: 10, // Âmago de Cavalo Equip N
}

// desertoFinish dá ao item que um Agmo soltou o tamanho do pacote. Só empilhável
// ganha quantidade: EF_AMOUNT em outro item é uma pilha que o cliente não divide
// e gasta inteira. Os âmagos empilham até 120 (internal/pilha).
func desertoFinish(mob *world.Entity, it *world.Item) {
	if mob == nil || mob.TemplateName == "" {
		return
	}
	if n := desertoPacotes[droprule.Canonical(mob.TemplateName)][it.Index]; n > 1 && isSplittable(it.Index) {
		setItemAmount(it, n)
	}
}
