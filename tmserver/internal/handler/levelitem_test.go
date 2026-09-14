package handler

import (
	"testing"

	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/world"
)

// TestConstrucaoDoPersonagem prende a regra do legado, que não é simétrica: só
// vence quem for ESTRITAMENTE maior que os outros três. Dois atributos empatados
// no topo não caem no primeiro deles — caem no 3, o balde do resto. É onde um
// porte descuidado erra, porque a leitura natural seria "o maior ganha".
func TestConstrucaoDoPersonagem(t *testing.T) {
	casos := []struct {
		nome                        string
		str, inteligencia, dex, con int16
		quer                        int
	}{
		{"força sozinha no topo", 100, 50, 50, 50, 0},
		{"int sozinha no topo", 50, 100, 50, 50, 1},
		{"destreza sozinha no topo", 50, 50, 100, 50, 2},
		{"constituição no topo cai no resto", 50, 50, 50, 100, 3},
		{"força e int empatadas caem no resto", 100, 100, 50, 50, 3},
		{"força e constituição empatadas caem no resto", 100, 50, 50, 100, 3},
		{"tudo igual cai no resto", 60, 60, 60, 60, 3},
		{"ficha zerada cai no resto", 0, 0, 0, 0, 3},
		{"força por um ponto ainda é força", 51, 50, 50, 50, 0},
	}
	for _, c := range casos {
		t.Run(c.nome, func(t *testing.T) {
			e := &world.Entity{BaseStr: c.str, BaseInt: c.inteligencia, BaseDex: c.dex, BaseCon: c.con}
			if got := construcaoDoPersonagem(e); got != c.quer {
				t.Errorf("construção = %d, queria %d", got, c.quer)
			}
		})
	}
}

// TestEntregaSemTabelaNaoFazNada: um servidor sem LevelItem.txt carregado não
// pode explodir no level-up de ninguém. É o caminho de todo teste que não se
// importa com a entrega, e do servidor rodando sem o conteúdo montado.
func TestEntregaSemTabelaNaoFazNada(t *testing.T) {
	d := New(Config{})
	if d.levelItems != nil {
		t.Fatal("sem Config.LevelItems a tabela devia ficar nil")
	}
	// não deve entrar em pânico nem tocar em nada
	d.entregaItemDeNivel(nil, nil, nil)
}
