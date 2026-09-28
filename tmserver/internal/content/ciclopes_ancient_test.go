package content

import "testing"

// Todo bloco liderado por um Ancião Ciclops fica sem período de minuto: é a fila
// de renascimento que os traz de volta, em no máximo 10 s
// (handler/ciclopes_ancient.go). Um período no arquivo faria o relógio de minuto
// repor o bloco a cada 12 ou 24 s, por cima da fila.
func TestAnciaoCiclopsSemPeriodoNoArquivo(t *testing.T) {
	gens, err := LoadNPCGenerators(release(t, "TMsrv", "run", "NPCGener.txt"))
	if err != nil {
		t.Skipf("Release content unavailable: %v", err)
	}
	achados := 0
	for i, g := range gens {
		if g.Leader != "Anciao_Ciclops" && g.Leader != "Anciao_Ciclops_" {
			continue
		}
		achados++
		if g.MinuteGenerate > 0 {
			t.Errorf("bloco %d (%s) tem período %d; quer sem período (-1)", i, g.Leader, g.MinuteGenerate)
		}
	}
	// 73 blocos em 28/09/2026. Se o número mudar, alguém mexeu nos Anciões no
	// arquivo e o teste precisa ser revisto junto.
	if achados != 73 {
		t.Errorf("achei %d blocos liderados pelo Ancião Ciclops; quer 73", achados)
	}
}
