//go:build simulacao

package handler

// Recalibração do dano dos monstros das quests do Acampamento Troll e do Castelo
// Orc (21/09/2026): quanto cada template tira de cada classe por segundo, e
// quantos deles em cima de um jogador passam a parede da poção (2.000/s).
//
//	go test -tags simulacao -run TestSimulacaoQuests -v ./tmserver/internal/handler/

import (
	"fmt"
	"path/filepath"
	"testing"

	"github.com/jeanluca/w2pp-openwyd/internal/npctemplate"
	"github.com/jeanluca/w2pp-openwyd/internal/savefmt"
	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/world"
)

// monstroDaQuest é um template da quest e quantos deles a corrida põe em campo.
type monstroDaQuest struct {
	arquivo string
	papel   string
	emCampo int
}

var questTroll = []monstroDaQuest{
	{"ATroll_Enigma", "boss", 1},
	{"ATroll_Caos", "guardião", 4},
	{"ATroll_Mago", "seguidor", 4},
	{"ATroll_Insano", "tropa", 20},
	{"ATroll_Cacador", "tropa", 20},
}

var questOrc = []monstroDaQuest{
	{"COrc_GraoLorde", "boss", 1},
	{"COrc_Sentinela", "guardião", 1},
	{"COrc_Capitao", "guardião", 1},
	{"COrc_Chefe", "guardião", 1},
	{"COrc_Guarda", "seguidor", 4},
	{"COrc_Cavaleiro", "tropa", 16},
	{"COrc_Arqueiro", "tropa", 16},
	{"COrc_MeioOrc", "tropa", 14},
	{"COrc_Mago", "tropa", 14},
}

// TestSimulacaoQuestsFicha imprime os números crus de cada template.
func TestSimulacaoQuestsFicha(t *testing.T) {
	root := filepath.Join("..", "..", "..", "Release")
	for _, q := range [][]monstroDaQuest{questTroll, questOrc} {
		for _, m := range q {
			b, _, err := npctemplate.Load(root, m.arquivo)
			if err != nil {
				t.Skipf("%s: %v", m.arquivo, err)
			}
			mob, _, err := savefmt.DecodeMobAny(b)
			if err != nil {
				t.Fatalf("%s: %v", m.arquivo, err)
			}
			t.Logf("%-16s %-9s nv %3d  HP %9d  def %5d  dano %5d  dex %4d  resist %v  barra %v",
				m.arquivo, m.papel, mob.BaseScore.Level, mob.BaseScore.MaxHp, mob.BaseScore.AC, mob.BaseScore.Damage,
				mob.BaseScore.Dex, mob.Resist, mob.SkillBar)
		}
	}
}

// golpeDoMonstro mede o dano médio de um template contra um personagem, com a
// esquiva e o crítico rolando como em jogo.
func (sm *simulador) golpeDoMonstro(mob *world.Entity, alvo *world.Entity, n int) float64 {
	vida := alvo.HP
	total := 0
	for i := 0; i < n; i++ {
		alvo.HP = vida
		if d := sm.d.danoDoGolpeDeMonstro(sm.w, mob, alvo); d > 0 {
			total += d
		}
	}
	alvo.HP = vida
	return float64(total) / float64(n)
}

// TestSimulacaoQuestsDano: dano por segundo de cada monstro contra cada classe,
// e quantos deles bastam para passar a poção.
func TestSimulacaoQuestsDano(t *testing.T) {
	root := filepath.Join("..", "..", "..", "Release")
	sm := novoSimulador(t, root)
	elenco := sm.elenco()

	t.Log("=== fichas dos personagens (com os 7.000 de vida dos acessórios) ===")
	tipos := make([]*lutador, 0, len(elenco))
	for _, c := range elenco {
		l := comVidaExtra(c.novo(1), simVidaExtra)
		tipos = append(tipos, l)
		t.Logf("%-28s defesa %5d  HP %6d", c.nome, effectiveAC(l.e), l.e.HP)
	}

	for nome, q := range map[string][]monstroDaQuest{"Acampamento Troll": questTroll, "Castelo Orc": questOrc} {
		t.Logf("=== %s ===", nome)
		for _, m := range q {
			mob := sm.chefe(alvoDeRaide{arquivo: m.arquivo})
			linha := fmt.Sprintf("%-16s %-9s", m.arquivo, m.papel)
			for i, c := range elenco {
				alvo := tipos[i]
				media := sm.golpeDoMonstro(mob, alvo.e, 300)
				dps := media * 1000 / float64(mobAttackCadence)
				segundos := 0.0
				if dps > 0 {
					segundos = float64(alvo.e.HP) / dps
				}
				linha += fmt.Sprintf("\n    %-28s %6.0f/golpe  %6.0f/s  morre em %5.1f s  (%d em cima = poção estourada)",
					c.nome, media, dps, segundos, int(applyCasting/max(dps, 1))+1)
			}
			t.Log(linha)
			sm.w.DespawnMob(mob.ID, 1)
		}
	}
}

// bonecoDeDefesa é um jogador de teste com a defesa pedida: o que muda o golpe
// de monstro é só a AC (e a esquiva, que vem de Dex).
func (sm *simulador) bonecoDeDefesa(id int, defesa int32, dex int16, vida int32) *world.Entity {
	return &world.Entity{ID: id, Class: 0, ClassMaster: classMasterMortal, Level: 380,
		Str: 1000, BaseStr: 1000, Dex: dex, BaseDex: dex, Con: 300, BaseCon: 300,
		AC: defesa, BaseAC: defesa, HP: vida, MaxHP: vida, BaseMaxHP: vida}
}

// TestSimulacaoQuestsPorDefesa varre a defesa do jogador e mostra o dano por
// segundo de cada monstro das duas quests — a régua para recalibrar.
func TestSimulacaoQuestsPorDefesa(t *testing.T) {
	root := filepath.Join("..", "..", "..", "Release")
	sm := novoSimulador(t, root)
	defesas := []int32{800, 1000, 1200, 1400, 1600, 1800, 2000, 2400, 2800, 3200}

	for _, par := range []struct {
		nome string
		q    []monstroDaQuest
	}{{"Acampamento Troll", questTroll}, {"Castelo Orc", questOrc}} {
		t.Logf("=== %s — dano por golpe (1 golpe/s) por defesa do jogador ===", par.nome)
		cab := "monstro             dano"
		for _, d := range defesas {
			cab += fmt.Sprintf("  def%5d", d)
		}
		t.Log(cab)
		for _, m := range par.q {
			mob := sm.chefe(alvoDeRaide{arquivo: m.arquivo})
			linha := fmt.Sprintf("%-16s %-9s %5d", m.arquivo, m.papel, mob.Damage)
			for i, def := range defesas {
				alvo := sm.bonecoDeDefesa(100+i, def, 500, 20000)
				linha += fmt.Sprintf("  %8.0f", sm.golpeDoMonstro(mob, alvo, 400))
			}
			t.Log(linha)
			sm.w.DespawnMob(mob.ID, 1)
		}
	}
}

// TestSimulacaoQuestsTempoDeMatar: quanto uma tropa de Xorimpas leva para
// derrubar cada monstro das duas quests, e o divisor do slot 13 de cada um.
func TestSimulacaoQuestsTempoDeMatar(t *testing.T) {
	root := filepath.Join("..", "..", "..", "Release")
	for _, par := range []struct {
		nome string
		q    []monstroDaQuest
	}{{"Acampamento Troll", questTroll}, {"Castelo Orc", questOrc}} {
		t.Logf("=== %s ===", par.nome)
		for _, m := range par.q {
			for _, n := range []int{1, 4} {
				sm := novoSimulador(t, root)
				r := sm.raide(alvoDeRaide{arquivo: m.arquivo}, n, 30*60*1000, true)
				t.Logf("%-16s %-9s divisor %3d  vida efetiva %9d  %d jogador(es): %5.1f s",
					m.arquivo, m.papel, r.divisor, r.efetiva, n, float64(r.ms)/1000)
			}
		}
	}
}

// A régua de 21/09/2026: o jogador-alvo é um Mortal com set +6/+9, defesa 1.400,
// e a poção segura 2.000 por segundo. Oito monstros de tropa em cima dele é o
// limite: 250 de dano por golpe cada um.
const (
	defesaDaRegua  = int32(1400)
	paredeDaPocao  = 2000
	alvoTropa      = 250 // 8 em cima
	alvoSeguidor   = 350 // 5 em cima
	alvoGuardiao   = 550 // 3-4 em cima
	alvoBoss       = 800 // sozinho, com folga para a tropa em volta
	amostraDaRegua = 4000
)

// danoDeTemplate mede o dano médio de um template com o Damage trocado.
func (sm *simulador) danoDeTemplate(arquivo string, dano int32, defesa int32) float64 {
	mob := sm.chefe(alvoDeRaide{arquivo: arquivo, dano: dano})
	alvo := sm.bonecoDeDefesa(700, defesa, 500, 1<<28)
	m := sm.golpeDoMonstro(mob, alvo, amostraDaRegua)
	sm.w.DespawnMob(mob.ID, 1)
	return m
}

// TestSimulacaoQuestsCalibragem procura, para cada monstro das duas quests, o
// Damage de template que entrega o alvo do papel dele na régua.
func TestSimulacaoQuestsCalibragem(t *testing.T) {
	root := filepath.Join("..", "..", "..", "Release")
	alvos := map[string]int{"tropa": alvoTropa, "seguidor": alvoSeguidor, "guardião": alvoGuardiao, "boss": alvoBoss}

	for _, par := range []struct {
		nome string
		q    []monstroDaQuest
	}{{"Acampamento Troll", questTroll}, {"Castelo Orc", questOrc}} {
		t.Logf("=== %s (régua: defesa %d, poção %d/s) ===", par.nome, defesaDaRegua, paredeDaPocao)
		for _, m := range par.q {
			sm := novoSimulador(t, root)
			alvo := float64(alvos[m.papel])
			hoje := sm.danoDeTemplate(m.arquivo, 0, defesaDaRegua)
			lo, hi := int32(defesaDaRegua/2), int32(6000)
			for lo < hi {
				meio := (lo + hi) / 2
				if sm.danoDeTemplate(m.arquivo, meio, defesaDaRegua) < alvo {
					lo = meio + 1
				} else {
					hi = meio
				}
			}
			novo := sm.danoDeTemplate(m.arquivo, lo, defesaDaRegua)
			t.Logf("%-16s %-9s hoje %6.0f/golpe  →  dano de template %5d dá %5.0f/golpe (alvo %.0f; %d em cima = poção)",
				m.arquivo, m.papel, hoje, lo, novo, alvo, int(float64(paredeDaPocao)/novo))
		}
	}
}
