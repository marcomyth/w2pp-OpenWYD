package mapaevento

import (
	"path/filepath"
	"testing"

	"github.com/jeanluca/w2pp-openwyd/internal/npcgener"
	"github.com/jeanluca/w2pp-openwyd/internal/regions"
)

const conteudo = "../../Release/TMsrv/run"

// Os retângulos são cópia do Regions.txt: se alguém mexer lá, a lista daqui
// precisa acompanhar, senão o painel mostra um lugar e o jogo limpa outro.
func TestRetangulosBatemComORegions(t *testing.T) {
	tab, err := regions.Load(filepath.Join(conteudo, "Regions.txt"))
	if err != nil {
		t.Fatal(err)
	}
	for _, m := range Todos {
		achou := false
		for _, r := range tab {
			if r.Name != m.Regiao {
				continue
			}
			achou = true
			if int(r.X1) != m.X1 || int(r.Y1) != m.Y1 || int(r.X2) != m.X2 || int(r.Y2) != m.Y2 {
				t.Errorf("%s: Regions.txt tem %d,%d-%d,%d, a lista tem %d,%d-%d,%d",
					m.Regiao, r.X1, r.Y1, r.X2, r.Y2, m.X1, m.Y1, m.X2, m.Y2)
			}
		}
		if !achou {
			t.Errorf("%s não está no Regions.txt", m.Regiao)
		}
		if !Contem(m.EntradaX, m.EntradaY) {
			t.Errorf("%s: a entrada %d,%d fica fora do mapa", m.Nome, m.EntradaX, m.EntradaY)
		}
	}
}

// Os blocos que motivaram a lista precisam cair dentro dela: é o que prova que a
// limpeza pega a população que estava de pé.
func TestBlocosConhecidosCaemNosMapas(t *testing.T) {
	gens, err := npcgener.Load(filepath.Join(conteudo, "NPCGener.txt"))
	if err != nil {
		t.Fatal(err)
	}
	casos := []struct {
		bloco int
		mapa  string
	}{
		{4241, "Nova_Guerra_Noatun"}, {4787, "Nova_Guerra_Noatun"}, {4848, "Nova_Guerra_Noatun"},
		{4875, "Monster_City"}, {6051, "Monster_City"},
		{4767, "Pistas"},
		{195, "Cubo_N"}, {220, "Cubo_M"}, {3776, "Big_Cubo"},
	}
	for _, c := range casos {
		g := gens[c.bloco]
		m, ok := Em(int(g.SegX[0]), int(g.SegY[0]))
		if !ok || m.Regiao != c.mapa {
			t.Errorf("bloco %d (%s em %d,%d): mapa %q, quer %q", c.bloco, g.Leader, g.SegX[0], g.SegY[0], m.Regiao, c.mapa)
		}
	}
}

// Nenhum mapa de evento pode pegar cidade, campo de caça ou masmorra vizinha.
func TestNaoPegaLugaresVizinhos(t *testing.T) {
	fora := []struct {
		nome string
		x, y int
	}{
		{"Noatun", 1050, 1706},
		{"Armia", 2086, 2093},
		{"Kefra City", 3250, 1720},
		{"Portão Infernal", 1790, 3650},
		{"Kefra Esquerda", 2200, 3900},
		{"Submundo 1", 1400, 3780},
	}
	for _, f := range fora {
		if m, ok := Em(f.x, f.y); ok {
			t.Errorf("%s (%d,%d) caiu em %s", f.nome, f.x, f.y, m.Nome)
		}
	}
}
