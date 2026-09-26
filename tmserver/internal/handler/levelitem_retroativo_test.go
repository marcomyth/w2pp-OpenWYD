package handler

import (
	"log/slog"
	"path/filepath"
	"testing"

	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/content"
	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/world"
)

// retroFixture monta um mundo com o LevelItem.txt de verdade, um armazém vazio
// e um TK sem pontos distribuídos (12/12/12/12): o personagem que o legado
// deixava sem peça nenhuma do 29 ao 254.
type retroFixture struct {
	d   *Dispatcher
	w   *world.World
	s   *world.Session
	e   *world.Entity
	tab *content.LevelItems
}

func novoRetro(t *testing.T, nivel int32, classMaster uint8) *retroFixture {
	t.Helper()
	tab, _, err := content.LoadLevelItems(filepath.Join("..", "..", "..", "Release", "TMsrv", "run", "LevelItem.txt"))
	if err != nil {
		t.Skipf("LevelItem.txt indisponível: %v", err)
	}
	d := New(Config{Log: slog.New(slog.DiscardHandler), LevelItems: tab})
	w := world.New(world.Config{GridDim: 16}, slog.New(slog.DiscardHandler), nil, nil)
	const conta = 77
	w.SetCargo(conta, &world.CargoState{})
	return &retroFixture{
		d: d, w: w, tab: tab,
		s: &world.Session{Conn: 1, Mode: world.UserPlay, AccountID: conta},
		e: &world.Entity{ID: 1, Name: "Upado", Class: 0, Level: nivel, ClassMaster: classMaster,
			BaseStr: 12, BaseInt: 12, BaseDex: 12, BaseCon: 12},
	}
}

func (f *retroFixture) armazem() []world.Item {
	var out []world.Item
	for _, it := range f.w.Cargo(f.s.AccountID).Items {
		if !it.Empty() {
			out = append(out, it)
		}
	}
	return out
}

// esperado é a peça de cada nível até o do personagem, na ordem.
func (f *retroFixture) esperado() []int16 {
	var out []int16
	for nivel := int32(1); nivel <= f.e.Level; nivel++ {
		if item, _ := pecaDoNivelEm(f.tab, f.e, nivel); !item.Empty() {
			out = append(out, item.Index)
		}
	}
	return out
}

// Um TK sem pontos no 160 recebe a peça de cada nível que passou, montaria do
// 149 inclusive, e a marca fecha: o segundo login não entrega de novo.
func TestRetroativoEntregaTudoUmaVez(t *testing.T) {
	f := novoRetro(t, 160, classMasterMortal)
	quer := f.esperado()
	if len(quer) < 10 {
		t.Fatalf("o TK sem pontos no 160 devia ter pelo menos 10 peças, a tabela dá %d", len(quer))
	}
	montaria, _ := pecaDoNivelEm(f.tab, f.e, 149)
	if montaria.Empty() {
		t.Fatal("o nível 149 devia entregar a montaria")
	}

	f.d.entregaItensDeNivelRetroativos(f.w, f.s, f.e)

	got := f.armazem()
	if len(got) != len(quer) {
		t.Fatalf("armazém com %d peças, queria %d", len(got), len(quer))
	}
	temMontaria := false
	for i, it := range got {
		if it.Index != quer[i] {
			t.Errorf("vaga %d = %d, queria %d", i, it.Index, quer[i])
		}
		if it.Index == montaria.Index {
			temMontaria = true
		}
	}
	if !temMontaria {
		t.Errorf("a montaria do 149 (%d) não chegou", montaria.Index)
	}
	if f.e.NivelRetroativo != nivelRetroativoConcluido {
		t.Errorf("marca = %d, queria %d", f.e.NivelRetroativo, nivelRetroativoConcluido)
	}

	f.d.entregaItensDeNivelRetroativos(f.w, f.s, f.e)
	if n := len(f.armazem()); n != len(quer) {
		t.Errorf("o segundo login entregou de novo: %d peças, queria %d", n, len(quer))
	}
}

// Só o Mortal recebe, como na entrega de cada nível; a marca de quem não é
// Mortal não muda.
func TestRetroativoSoMortal(t *testing.T) {
	for _, cm := range []uint8{classMasterArch, classMasterCelestial} {
		f := novoRetro(t, 160, cm)
		f.d.entregaItensDeNivelRetroativos(f.w, f.s, f.e)
		if n := len(f.armazem()); n != 0 {
			t.Errorf("class_master %d recebeu %d peças", cm, n)
		}
		if f.e.NivelRetroativo != 0 {
			t.Errorf("class_master %d ficou com a marca %d", cm, f.e.NivelRetroativo)
		}
	}
}

// Abaixo do primeiro nível que entrega não há o que dar, e a marca fecha: a
// partir daqui a entrega de cada nível cuida dele.
func TestRetroativoAbaixoDo29Fecha(t *testing.T) {
	f := novoRetro(t, 20, classMasterMortal)
	f.d.entregaItensDeNivelRetroativos(f.w, f.s, f.e)
	if n := len(f.armazem()); n != 0 {
		t.Errorf("nível 20 recebeu %d peças", n)
	}
	if f.e.NivelRetroativo != nivelRetroativoConcluido {
		t.Errorf("marca = %d, queria %d", f.e.NivelRetroativo, nivelRetroativoConcluido)
	}
}

// Armazém cheio não perde nada: a entrega para na primeira peça que não cabe,
// guarda até onde foi, e o próximo login entrega o resto — nem uma peça a mais,
// nem uma a menos que a entrega de uma vez só.
func TestRetroativoArmazemCheioContinuaNoProximoLogin(t *testing.T) {
	f := novoRetro(t, 160, classMasterMortal)
	quer := f.esperado()
	cargo := f.w.Cargo(f.s.AccountID)
	const livres = 3
	ocupado := world.Item{Index: 1}
	for i := livres; i < world.MaxCargo-2; i++ {
		cargo.Items[i] = ocupado
	}

	f.d.entregaItensDeNivelRetroativos(f.w, f.s, f.e)

	if f.e.NivelRetroativo == 0 || f.e.NivelRetroativo >= nivelRetroativoConcluido {
		t.Fatalf("marca = %d, queria um nível parcial", f.e.NivelRetroativo)
	}
	primeira := []int16{cargo.Items[0].Index, cargo.Items[1].Index, cargo.Items[2].Index}
	for i, idx := range primeira {
		if idx != quer[i] {
			t.Errorf("vaga %d = %d, queria %d", i, idx, quer[i])
		}
	}

	// O jogador tira as três peças e o resto do entulho, e entra de novo.
	for i := range cargo.Items {
		cargo.Items[i] = world.Item{}
	}
	f.d.entregaItensDeNivelRetroativos(f.w, f.s, f.e)

	resto := f.armazem()
	if len(resto) != len(quer)-livres {
		t.Fatalf("segundo login entregou %d, queria %d", len(resto), len(quer)-livres)
	}
	for i, it := range resto {
		if it.Index != quer[livres+i] {
			t.Errorf("segundo login, peça %d = %d, queria %d", i, it.Index, quer[livres+i])
		}
	}
	if f.e.NivelRetroativo != nivelRetroativoConcluido {
		t.Errorf("marca = %d, queria %d", f.e.NivelRetroativo, nivelRetroativoConcluido)
	}
}
