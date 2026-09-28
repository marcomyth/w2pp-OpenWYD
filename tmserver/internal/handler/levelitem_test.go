package handler

import (
	"path/filepath"
	"testing"

	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/content"
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

// TestSemPontosRecebePeloLadoMago: o personagem nasce 12/12/12/12 e subir de
// nível não mexe nesses quatro, então quem é upado sem distribuir fica sem
// construção — e o LevelItem.txt não tem peça para ela. Pedido do Marco em
// 26/09: "precisa receber, podemos optar nesse caso pelo lado mago". Lido do
// arquivo de verdade, para o teste quebrar se o conteúdo mudar a coluna MAG.
func TestSemPontosRecebePeloLadoMago(t *testing.T) {
	tab, _, err := content.LoadLevelItems(filepath.Join("..", "..", "..", "Release", "TMsrv", "run", "LevelItem.txt"))
	if err != nil {
		t.Skipf("LevelItem.txt indisponível: %v", err)
	}
	semPontos := func(classe uint8, nivel int32) *world.Entity {
		return &world.Entity{Class: classe, Level: nivel, BaseStr: 12, BaseInt: 12, BaseDex: 12, BaseCon: 12}
	}
	casos := []struct {
		nome       string
		e          *world.Entity
		item       int16
		construcao int
	}{
		// "#TK MAG SET MALHA": a luva mágica, com Mágico 6 em vez de Dano 20.
		{"TK sem pontos no 29", semPontos(0, 29), 1147, construcaoInt},
		{"FM sem pontos na arma do 154", semPontos(1, 154), 899, construcaoInt},
		{"BM sem pontos na arma do 254", semPontos(2, 254), 3566, construcaoInt},
		// A HT não tem coluna de Int; o "MAG" dela é o TYPE 3 do arquivo.
		{"HT sem pontos na arma do 154", semPontos(3, 154), 839, construcaoDestreza},
		// Quem tem construção fica com a dela.
		{"TK de força continua no DN", &world.Entity{Class: 0, Level: 29, BaseStr: 60, BaseInt: 12, BaseDex: 12, BaseCon: 12}, 1147, 0},
	}
	for _, c := range casos {
		t.Run(c.nome, func(t *testing.T) {
			item, construcao := pecaDoNivel(tab, c.e)
			if item.Index != c.item || construcao != c.construcao {
				t.Errorf("peça %d pela construção %d, queria %d pela %d", item.Index, construcao, c.item, c.construcao)
			}
		})
	}
	// O set MAG e o DN da luva do TK são o mesmo item com adições diferentes: o
	// lado mago é o do Mágico (60), não o do Dano (2).
	if item, _ := pecaDoNivel(tab, semPontos(0, 29)); item.Effects[1][0] != 60 {
		t.Errorf("a luva do TK sem pontos veio com o efeito %d, queria o Mágico (60)", item.Effects[1][0])
	}
}

// TestDestrezaSemColunaRecebePeloDN: só a Huntress tem coluna de Destreza no
// LevelItem.txt, então o TK, a FM e o BM de Destreza não recebiam set nem arma.
// Decisão do Marco em 26/09: recebem pelo lado DN. A Huntress de Destreza fica
// com a coluna dela.
func TestDestrezaSemColunaRecebePeloDN(t *testing.T) {
	tab, _, err := content.LoadLevelItems(filepath.Join("..", "..", "..", "Release", "TMsrv", "run", "LevelItem.txt"))
	if err != nil {
		t.Skipf("LevelItem.txt indisponível: %v", err)
	}
	destreza := func(classe uint8, nivel int32) *world.Entity {
		return &world.Entity{Class: classe, Level: nivel, BaseStr: 12, BaseInt: 12, BaseDex: 60, BaseCon: 12}
	}
	casos := []struct {
		nome       string
		e          *world.Entity
		item       int16
		construcao int
	}{
		{"TK de destreza no 29", destreza(0, 29), 1147, construcaoForca},
		{"BM de destreza na arma do 154", destreza(2, 154), 884, construcaoForca},
		{"FM de destreza na arma do 254", destreza(1, 254), 3556, construcaoForca},
		{"HT de destreza fica com a coluna dela", destreza(3, 154), 839, construcaoDestreza},
	}
	for _, c := range casos {
		t.Run(c.nome, func(t *testing.T) {
			item, construcao := pecaDoNivel(tab, c.e)
			if item.Index != c.item || construcao != c.construcao {
				t.Errorf("peça %d pela construção %d, queria %d pela %d", item.Index, construcao, c.item, c.construcao)
			}
		})
	}
	if item, _ := pecaDoNivel(tab, destreza(0, 29)); item.Effects[1][0] != 2 {
		t.Errorf("a luva do TK de destreza veio com o efeito %d, queria o Dano (2)", item.Effects[1][0])
	}
}

// TestLuvaDaHTSemCritico: a luva dos quatro sets da Huntress vinha com o
// adicional da luva DN de todas as classes (Dano 20 + Crítico 4%). Decisão do
// Marco em 26/09: Dano 20 + Defesa 20.
func TestLuvaDaHTSemCritico(t *testing.T) {
	tab, _, err := content.LoadLevelItems(filepath.Join("..", "..", "..", "Release", "TMsrv", "run", "LevelItem.txt"))
	if err != nil {
		t.Skipf("LevelItem.txt indisponível: %v", err)
	}
	const refino, dano, defesa = 43, 2, 3
	quer := [3][2]uint8{{refino, 3}, {dano, 20}, {defesa, 20}}
	for nivel, luva := range map[int32]int16{29: 1606, 84: 1621, 119: 1636, 159: 1651} {
		for construcao := 0; construcao < content.LevelItemBuilds; construcao++ {
			item := tab.Para(3, construcao, nivel)
			if item.Index != luva || item.Effects != quer {
				t.Errorf("nível %d, construção %d: %d %v, queria %d %v", nivel, construcao, item.Index, item.Effects, luva, quer)
			}
		}
	}
}

// TestLuvaFisicaSemCritico: a luva do lado físico de TODAS as classes sai sem
// crítico (Marco, 26/09: "todas as luvas sem crítico em lado físico, TODAS").
// A do TK, da FM e do BM de Força vinha com Dano 20 + Crítico 4%; passa a Dano
// 20 + Defesa 20, como a da Huntress. Vale também para a entrega retroativa,
// que entrega essas mesmas luvas de novo.
func TestLuvaFisicaSemCritico(t *testing.T) {
	tab, _, err := content.LoadLevelItems(filepath.Join("..", "..", "..", "Release", "TMsrv", "run", "LevelItem.txt"))
	if err != nil {
		t.Skipf("LevelItem.txt indisponível: %v", err)
	}
	const refino, dano, defesa = 43, 2, 3
	quer := [3][2]uint8{{refino, 3}, {dano, 20}, {defesa, 20}}
	luvas := map[uint8]map[int32]int16{
		0: {29: 1147, 84: 1186, 119: 1201, 159: 1216}, // TK
		1: {29: 1297, 84: 1321, 119: 1336, 159: 1351}, // FM
		2: {29: 1456, 84: 1471, 119: 1486, 159: 1501}, // BM
	}
	for classe, porNivel := range luvas {
		for nivel, luva := range porNivel {
			e := &world.Entity{Class: classe, Level: nivel, BaseStr: 60, BaseInt: 12, BaseDex: 12, BaseCon: 12}
			item, _ := pecaDoNivel(tab, e)
			if item.Index != luva || item.Effects != quer {
				t.Errorf("classe %d nível %d: %d %v, queria %d %v", classe, nivel, item.Index, item.Effects, luva, quer)
			}
		}
	}
	// Nenhuma peça do arquivo, em classe, construção ou nível algum, sai com
	// crítico numa luva.
	for classe := 0; classe < 4; classe++ {
		for construcao := 0; construcao < content.LevelItemBuilds; construcao++ {
			for nivel := int32(1); nivel < 400; nivel++ {
				item := tab.Para(classe, construcao, nivel)
				for _, ef := range item.Effects {
					if (ef[0] == 42 || ef[0] == 71) && ehLuvaDeNivel(item.Index) {
						t.Errorf("classe %d construção %d nível %d: luva %d com crítico %v", classe, construcao, nivel, item.Index, item.Effects)
					}
				}
			}
		}
	}
}

// ehLuvaDeNivel são as dezesseis luvas que o LevelItem.txt entrega.
func ehLuvaDeNivel(idx int16) bool {
	switch idx {
	case 1147, 1186, 1201, 1216, 1297, 1321, 1336, 1351, 1456, 1471, 1486, 1501, 1606, 1621, 1636, 1651:
		return true
	}
	return false
}
