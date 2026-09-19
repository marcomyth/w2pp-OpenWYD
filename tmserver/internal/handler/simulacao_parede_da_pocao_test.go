//go:build simulacao

package handler

// A PAREDE DA POÇÃO (18/09/2026). A poção cura 2.000 por segundo e não se mexe:
// quem não tira mais que isso por segundo não mata ninguém, por mais tempo que a
// luta dure. Este arquivo mede exatamente isso, personagem por personagem.
//
//	go test -tags simulacao -run TestDiagnostico -v ./tmserver/internal/handler/

import (
	"fmt"
	"path/filepath"
	"testing"

	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/world"
)

// danoPorSegundo roda n rodadas da ação de a contra b, repondo a vida do alvo, e
// devolve a média por golpe e o dano por segundo (um golpe a cada simPasso).
func danoPorSegundo(sm *simulador, a *lutador, b *lutador, n int) (float64, float64) {
	vida := b.e.HP
	total := 0
	var agora int64
	for i := 0; i < n; i++ {
		b.e.HP = vida
		g := a.acao(a.lado, b.e, agora)
		total += g.dano
		agora += simPasso
	}
	b.e.HP = vida
	media := float64(total) / float64(n)
	return media, media * 1000 / float64(simPasso)
}

func TestDiagnosticoFisicoCancel(t *testing.T) {
	sm := novoSimulador(t, filepath.Join("..", "..", "..", "Release"))
	sm.d.combatRules.PvPSkillPct, sm.d.combatRules.PvPMeleePct = 37, 37
	elenco := sm.elenco()

	t.Log("=== fichas ===")
	for _, c := range elenco {
		l := comVidaExtra(c.novo(1), simVidaExtra)
		t.Logf("%-28s ataque %6d  defesa %5d  HP %6d  crítico %3d",
			c.nome, sm.d.effectiveDamage(l.e), effectiveAC(l.e), l.e.HP, effectiveCritical(l.e))
	}

	t.Log("=== dano por segundo de cada um contra o Porradeiro Trans (poção = 2000/s) ===")
	for _, c := range elenco {
		a := comVidaExtra(c.novo(1), simVidaExtra)
		b := comVidaExtra(sm.transLutador(2, simEdenAnct), simVidaExtra)
		media, dps := danoPorSegundo(sm, a, b, 400)
		t.Logf("%-28s %7.0f por golpe  %7.0f por segundo  %s", c.nome, media, dps,
			map[bool]string{true: "PASSA da poção", false: ""}[dps > 2000])
	}

	t.Log("=== só o golpe FÍSICO da FM Cancel contra o Trans, por ataque dela ===")
	b := comVidaExtra(sm.transLutador(2, simEdenAnct), simVidaExtra)
	t.Logf("defesa do Trans = %d (o golpe enfrenta %d, e a conta tira metade disso)",
		effectiveAC(b.e), effectiveAC(b.e)*3)
	for _, atq := range []int32{3105, 6000, 9000, 12000, 15000, 20000, 25000, 30000} {
		a := comVidaExtra(sm.cancelLutador(1), simVidaExtra)
		ajustarAtaque(sm, a.e, atq)
		a.acao = func(ld *lado, alvo *world.Entity, _ int64) golpe { return sm.fisico(ld, alvo) }
		media, dps := danoPorSegundo(sm, a, b, 400)
		fmt.Printf("ataque %6d -> %7.0f por golpe, %7.0f por segundo\n", atq, media, dps)
	}
}

// ajustarAtaque acha o e.Damage que põe o Ataque da janela no valor pedido.
func ajustarAtaque(sm *simulador, e *world.Entity, alvo int32) {
	lo, hi := int32(0), int32(2_000_000)
	for lo < hi {
		mid := (lo + hi) / 2
		e.Damage = mid
		if sm.d.effectiveDamage(e) < alvo {
			lo = mid + 1
		} else {
			hi = mid
		}
	}
	e.Damage = lo
}

// Varre os dois botões do golpe físico dela: perfuração da armadura e bônus de
// dano com duas armas.
func TestDiagnosticoBotoesFisicos(t *testing.T) {
	sm := novoSimulador(t, filepath.Join("..", "..", "..", "Release"))
	sm.d.combatRules.PvPSkillPct, sm.d.combatRules.PvPMeleePct = 37, 37
	defer func(p, d int) { cancelPerfuracaoPct, cancelDanoDuasArmas = p, d }(cancelPerfuracaoPct, cancelDanoDuasArmas)

	alvos := []struct {
		nome string
		novo func(int) *lutador
	}{
		{"Trans", func(id int) *lutador { return sm.transLutador(id, simEdenAnct) }},
		{"Paladino", sm.paladinoLutador},
		{"Xorimpas Captura", func(id int) *lutador { return sm.htUmaOitava(id, learnedInvisibilidade) }},
	}
	fmt.Printf("%-10s %-8s", "perfuração", "dano")
	for _, a := range alvos {
		fmt.Printf(" | %-20s", a.nome)
	}
	fmt.Println()
	for _, perf := range []int{0, 20, 40, 60, 80} {
		for _, dano := range []int{0, 50, 100, 200, 400} {
			cancelPerfuracaoPct, cancelDanoDuasArmas = perf, dano
			fmt.Printf("%-10d %-8d", perf, dano)
			for _, alvo := range alvos {
				a := comVidaExtra(sm.cancelLutador(1), simVidaExtra)
				a.acao = func(ld *lado, e *world.Entity, _ int64) golpe { return sm.fisico(ld, e) }
				b := comVidaExtra(alvo.novo(2), simVidaExtra)
				media, dps := danoPorSegundo(sm, a, b, 400)
				fmt.Printf(" | %6.0f golpe %6.0f/s", media, dps)
			}
			fmt.Println()
		}
	}
	a := comVidaExtra(sm.cancelLutador(1), simVidaExtra)
	fmt.Printf("\nAtaque dela com a regra da INT valendo = %d\n", sm.d.effectiveDamage(a.e))
}

// Quanto cada personagem tira por segundo do Porradeiro Trans, por percentual de
// PvP — contra a poção, que cura 2000 por segundo e não se mexe.
func TestDiagnosticoParedeDaPocao(t *testing.T) {
	sm := novoSimulador(t, filepath.Join("..", "..", "..", "Release"))
	defer func(p, d int) { cancelPerfuracaoPct, cancelDanoDuasArmas = p, d }(cancelPerfuracaoPct, cancelDanoDuasArmas)
	cancelPerfuracaoPct, cancelDanoDuasArmas = 80, 200

	pcts := []int32{37, 50, 60, 70, 100}
	fmt.Printf("%-28s", "dano/s no Trans (poção 2000/s)")
	for _, p := range pcts {
		fmt.Printf(" | %d%%", p)
	}
	fmt.Println()
	for _, o := range sm.elenco() {
		fmt.Printf("%-28s", o.nome)
		for _, p := range pcts {
			sm.d.combatRules.PvPSkillPct, sm.d.combatRules.PvPMeleePct = p, p
			a := comVidaExtra(o.novo(1), simVidaExtra)
			b := comVidaExtra(sm.transLutador(2, simEdenAnct), simVidaExtra)
			hp := b.e.HP
			_, dps := danoPorSegundo(sm, a, b, 400)
			if dps <= applyCasting {
				fmt.Printf(" | %5.0f/s nunca", dps)
			} else {
				fmt.Printf(" | %5.0f/s %3.0f s", dps, float64(hp)/(dps-applyCasting))
			}
		}
		fmt.Println()
	}
}
