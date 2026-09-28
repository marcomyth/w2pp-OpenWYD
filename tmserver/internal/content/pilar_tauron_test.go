package content

import "testing"

// Os quatro grupos de Tauron do Pilar (1175-1201 × 1709-1735) ficam sem período
// de minuto: é a fila de renascimento que os traz de volta, em no máximo 10 s
// (handler/pilar_tauron.go). Um período no arquivo faria o relógio de minuto
// repor um grupo a cada 24 s, por cima da fila.
func TestTauronDoPilarSemPeriodoNoArquivo(t *testing.T) {
	gens, err := LoadNPCGenerators(release(t, "TMsrv", "run", "NPCGener.txt"))
	if err != nil {
		t.Skipf("Release content unavailable: %v", err)
	}
	achados := 0
	for i, g := range gens {
		if g.Leader != "Tauron" {
			continue
		}
		x, y := g.SegX[0], g.SegY[0]
		if x < 1175 || x > 1201 || y < 1709 || y > 1735 {
			continue
		}
		achados++
		if g.MinuteGenerate > 0 {
			t.Errorf("bloco %d do Pilar tem período %d; quer sem período (-1)", i, g.MinuteGenerate)
		}
		if g.MaxNumMob != 3 {
			t.Errorf("bloco %d do Pilar tem %d Tauron; quer grupo de 3", i, g.MaxNumMob)
		}
	}
	if achados != 4 {
		t.Errorf("achei %d grupos de Tauron no Pilar; quer 4 (Pilar 1, 2, 3 e o do lado)", achados)
	}
}
