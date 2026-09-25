package handler

import (
	"io"
	"log/slog"
	"path/filepath"
	"testing"

	"github.com/jeanluca/w2pp-openwyd/internal/droprule"
	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/content"
	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/world"
)

// O FrenzyDemonLord volta 4 h depois da morte. Com Exp 2.000 ele não é chefe
// sozinho, e sem a regra daqui voltaria em 15 s.
func TestFrenzyRenasceEm4Horas(t *testing.T) {
	frenzy := geradorSozinho(2_000, 10)
	frenzy.LeaderName = frenzyTemplate
	w := world.New(world.Config{GridDim: 64}, slog.New(slog.NewTextHandler(io.Discard, nil)), world.NopPersistence{}, nil)
	w.RegisterGenerators([]*world.Generator{30: geradorSozinho(2_000, 12), 31: frenzy})
	d := dispatcherQuieto()
	quatro := uint32(4 * msPorHora)
	if got := d.esperaDoRenascimento(w, 31); got != quatro {
		t.Errorf("FrenzyDemonLord volta em %d ms, want %d", got, quatro)
	}
	if got := d.esperaDoRenascimento(w, 30); got == quatro {
		t.Error("um monstro qualquer de Exp baixa também ganhou as 4 h")
	}
}

// Depois do boot o FrenzyDemonLord só aparece 4 h depois, pela fila da morte, e
// o resto do mundo fica de pé.
func TestFrenzyNasceHorasDepoisDoBoot(t *testing.T) {
	var agora uint32 = 5_000
	bloco := func(nome string) *world.Generator {
		g := geradorSozinho(2_000, 10)
		g.LeaderName = nome
		g.LeaderTmpl = moldeDeMonstro(nome, 2_000, 0)
		return g
	}
	w := world.New(world.Config{GridDim: 4096, Now: func() uint32 { return agora }},
		slog.New(slog.NewTextHandler(io.Discard, nil)), world.NopPersistence{}, nil)
	w.RegisterGenerators([]*world.Generator{30: bloco(frenzyTemplate), 31: bloco("Morlock")})
	for _, idx := range []int{30, 31} {
		if len(w.GenerateMob(idx)) != 1 {
			t.Fatalf("o boot não levantou o bloco %d", idx)
		}
	}
	vivos := func() map[int]bool {
		out := map[int]bool{}
		for id := world.MaxUser; id < world.MaxMob; id++ {
			if e := w.Entity(id); e != nil {
				out[int(e.GenIndex)] = true
			}
		}
		return out
	}

	dispatcherQuieto().ApplyFrenzyBoot(w)

	if v := vivos(); v[30] || !v[31] {
		t.Fatalf("depois do boot: %v, want só o 31 de pé", v)
	}
	quatro := uint32(frenzyHoras * msPorHora)
	if got := w.SpawnDueRespawns(agora + quatro - 1); len(got) != 0 {
		t.Fatalf("o FrenzyDemonLord voltou antes das 4 h: %v", got)
	}
	agora += quatro
	if got := w.SpawnDueRespawns(agora); len(got) != 1 {
		t.Fatalf("4 h depois do boot voltaram %d, want 1", len(got))
	}
	if v := vivos(); !v[30] {
		t.Fatalf("o FrenzyDemonLord não está de pé 4 h depois do boot: %v", v)
	}
}

// Os blocos do FrenzyDemonLord no NPCGener não têm período de minuto: um período
// positivo o traria pelo relógio do gerador e ignoraria as 4 h — que é o que o
// 240 fazia, 48 minutos em passadas de 12 s.
func TestFrenzyBlocosSemPeriodo(t *testing.T) {
	gens, err := content.LoadNPCGenerators(filepath.Join(releaseDir(t), "TMsrv", "run", "NPCGener.txt"))
	if err != nil {
		t.Fatal(err)
	}
	var blocos []int
	for i, g := range gens {
		if droprule.Canonical(g.Leader) != droprule.Canonical(frenzyTemplate) {
			continue
		}
		blocos = append(blocos, i)
		if g.MinuteGenerate > 0 {
			t.Errorf("bloco %d tem MinuteGenerate %d: voltaria pelo relógio, não em 4 h", i, g.MinuteGenerate)
		}
	}
	// 3134 é o chefe; 3135, no mesmo ponto, é desligado pela 0139
	// (content.TestBlocosDesligadosPorIndice prende o índice).
	if len(blocos) != 2 || blocos[0] != 3134 || blocos[1] != 3135 {
		t.Errorf("blocos do FrenzyDemonLord = %v, want [3134 3135]", blocos)
	}
}
