package mountbonus

import "testing"

// TestSvadilfariBateComOTooltip pins the compiled table to what the client draws:
// a level-120 Svadilfari's tooltip reads Aumento de Dano 840, Ataque Mágico 54,
// Evasão 6.0%, Imunidades 28. Those numbers come only out of the client's row.
func TestSvadilfariBateComOTooltip(t *testing.T) {
	b, ok := Default(2387)
	if !ok {
		t.Fatal("Svadilfari (2387) não está na tabela")
	}
	if atk, mag := b.AtLevel(120); atk != 840 || mag != 54 {
		t.Errorf("nível 120 = dano %d, magia %d; o tooltip mostra 840 e 54", atk, mag)
	}
	if b.Evasion != 60 || b.Resist != 28 {
		t.Errorf("evasão/imunidade = %d/%d, o tooltip mostra 6.0%% (60) e 28", b.Evasion, b.Resist)
	}
}

// TestEsferaEUmaLinhaSo pins this server's own product: the spheres
// (client/montarias) are told apart by how they look, not by what they lend, so
// every one of the seven carries the same numbers — 350/50 e os 12% de XP. A
// row that drifts here makes one sphere quietly better than its siblings, which
// nothing on the client's tooltip would ever reveal — that band has no stat
// line at all.
func TestEsferaEUmaLinhaSo(t *testing.T) {
	want := Bonus{Attack: 350, Magic: 50}
	for idx := int16(EsferaLo); idx <= EsferaHi; idx++ {
		b, ok := Default(idx)
		if !ok {
			t.Errorf("esfera %d não está na tabela", idx)
			continue
		}
		if b != want {
			t.Errorf("esfera %d = %+v, want %+v", idx, b, want)
		}
		if e, ok := TempExtra(idx); !ok || e.ExpPct != 12 {
			t.Errorf("esfera %d XP = %+v (achou=%v), want 12%%", idx, e, ok)
		}
	}
}

// TestVagaComTooltipFicaLivre: 3995 is the last slot whose stats the client can
// draw. Filling it is a decision, not an accident — if this starts failing,
// someone spent it.
func TestVagaComTooltipFicaLivre(t *testing.T) {
	if _, ok := Default(3995); ok {
		t.Error("3995 deixou de estar livre; era a única vaga com tooltip")
	}
}

func TestPadraoSoValeParaMontaria(t *testing.T) {
	for _, idx := range []int16{2359, 2390, 2968, 2976, 3979, 3995, 0} {
		if _, ok := Default(idx); ok {
			t.Errorf("Default(%d) = ok, e %d não é montaria", idx, idx)
		}
	}
	for _, idx := range []int16{AdultLo, AdultHi, TempLo, TempHi, EsferaLo, EsferaHi} {
		if _, ok := Default(idx); !ok {
			t.Errorf("Default(%d) = !ok, e %d é montaria", idx, idx)
		}
	}
}

// TestOverlaySoMexeNaAdulta: the panel configures adult lineages only, like the
// growth curve and the absorption. A row for anything else must not leak in.
func TestOverlaySoMexeNaAdulta(t *testing.T) {
	andaluzB := Bonus{Attack: 500, Magic: 85, Evasion: 20, Resist: 40}
	t0 := Table{2375: andaluzB, 3987: {Attack: 1}}
	if got, _ := t0.For(2375); got != andaluzB {
		t.Errorf("Andaluz B configurada = %+v, want %+v", got, andaluzB)
	}
	if got, _ := t0.For(2374); got != adult[14] {
		t.Errorf("linhagem sem linha = %+v, want o padrão %+v", got, adult[14])
	}
	if got, _ := t0.For(3987); got != temp[7] {
		t.Errorf("temporária com linha no overlay = %+v, want o padrão %+v", got, temp[7])
	}
}

func TestValid(t *testing.T) {
	if !(Bonus{MaxAttack, MaxMagic, MaxEvasion, MaxResist}).Valid() {
		t.Error("os tetos exatos precisam ser aceitos")
	}
	for _, b := range []Bonus{{-1, 0, 0, 0}, {0, MaxMagic + 1, 0, 0}, {0, 0, MaxEvasion + 1, 0}, {0, 0, 0, MaxResist + 1}} {
		if b.Valid() {
			t.Errorf("%+v aceito, want recusa", b)
		}
	}
	// Every compiled row must itself be valid: the defaults are what a
	// restore writes back to the screen.
	for i, b := range adult {
		if !b.Valid() {
			t.Errorf("linha adulta %d do padrão fora dos limites: %+v", i, b)
		}
	}
	for i, b := range temp {
		if !b.Valid() {
			t.Errorf("linha temporária %d do padrão fora dos limites: %+v", i, b)
		}
	}
}
