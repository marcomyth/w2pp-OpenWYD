package handler

import (
	"github.com/jeanluca/w2pp-openwyd/internal/droprule"
	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/world"
)

// O FrenzyDemonLord é o chefe do Submundo (0090). Pedido da equipe em
// 25/09/2026: volta de 4 em 4 horas, como a Sombra Negra e o Verid do Gelo, e
// fica 4 horas de fora depois de o servidor subir.
//
// O 240 que o commit do Submundo gravou no NPCGener para dar as 4 horas dava 48
// minutos: MinuteGenerate conta passadas de 12 s (spawnrate.MinTimerPass), não
// minutos. O bloco passou a -1 e volta pela fila da morte, com a espera daqui.
// O segundo bloco dele (3135), no mesmo ponto, voltava em 15 s: com Exp 2.000
// ele não é chefe sozinho (chefes.go corta em 1 milhão). A 0139 o desliga.
const (
	frenzyTemplate = "FrenzyDemonLord"
	// frenzyHoras é a espera entre a morte e a volta (esperaDoRenascimento), e a
	// espera depois do boot (ApplyFrenzyBoot).
	frenzyHoras = 4
)

// geradorDoFrenzy diz se o bloco idx é o do FrenzyDemonLord.
func geradorDoFrenzy(w *world.World, idx int) bool {
	g := w.GeneratorAt(idx)
	return g != nil && droprule.Canonical(g.LeaderName) == droprule.Canonical(frenzyTemplate)
}

// ApplyFrenzyBoot segura o FrenzyDemonLord por frenzyHoras depois que o servidor
// sobe, pelo mesmo motivo do Gelo (ApplyGeloChefesBoot): o boot povoa todo bloco
// de uma vez, e sem isto um reinício valeria um chefe. Roda depois dos blocos
// desligados, para adiar só o que ficou de pé.
func (d *Dispatcher) ApplyFrenzyBoot(w *world.World) {
	var blocos []int
	for idx := 0; idx < w.GeneratorCount(); idx++ {
		if geradorDoFrenzy(w, idx) && w.DeferGenerator(idx, frenzyHoras*msPorHora) > 0 {
			blocos = append(blocos, idx)
		}
	}
	d.log.Info("FrenzyDemonLord nasce horas depois do boot", "horas", frenzyHoras, "blocos", blocos)
}
