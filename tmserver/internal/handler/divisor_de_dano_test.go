package handler

import (
	"encoding/binary"
	"io"
	"log/slog"
	"sync/atomic"
	"testing"

	"github.com/jeanluca/w2pp-openwyd/internal/npctemplate"
	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/world"
)

// comDivisor monta uma entidade nua com um item no slot da fada.
func comDivisor(item int16, valor uint8) *world.Entity {
	e := &world.Entity{}
	if item != 0 {
		e.Equip[world.DividerEquipSlot] = world.Item{Index: item}
		e.Equip[world.DividerEquipSlot].Effects[0] = world.Effect{Effect: 43, Value: valor}
	}
	return e
}

func TestDivisorDoSlotDaFada(t *testing.T) {
	casos := []struct {
		nome         string
		item         int16
		valor        uint8
		golpe, tique int32
	}{
		{"sem item", 0, 0, 1, 1},
		{"786 sem refino: o mínimo 2 manda", 786, 0, 2, 4},
		{"786 +1 ainda cai no mínimo", 786, 1, 2, 4},
		{"786 +15", 786, 15, 15, 30},
		{"1936 sem refino", 1936, 0, 20, 20},
		{"1936 +2 como o Lich Crunt", 1936, 2, 20, 20},
		{"1936 +15 como o Verid", 1936, 15, 150, 150},
		{"1937 sem refino como o Kefra", 1937, 0, 2000, 2000},
		{"1937 +15 como o Cristal", 1937, 15, 15000, 15000},
		{"item qualquer no slot não divide", 2367, 15, 1, 1},
	}
	for _, c := range casos {
		e := comDivisor(c.item, c.valor)
		if g := divisorDeGolpe(e); g != c.golpe {
			t.Errorf("%s: divisorDeGolpe = %d, quero %d", c.nome, g, c.golpe)
		}
		if g := divisorDeTique(e); g != c.tique {
			t.Errorf("%s: divisorDeTique = %d, quero %d", c.nome, g, c.tique)
		}
	}
}

// O golpe menor que o divisor tira ZERO — é o que faz o chefe do legado ser
// intocável para quem bate fraco, e é divisão inteira, não arredondamento.
func TestDanoNoPortadorTruncaParaBaixo(t *testing.T) {
	kefra := comDivisor(1937, 0) // ÷2000
	casos := []struct{ dano, quero int }{
		{1, 0}, {1999, 0}, {2000, 1}, {21371, 10}, {4_000_000, 2000},
	}
	for _, c := range casos {
		if got := danoNoPortador(kefra, c.dano); got != c.quero {
			t.Errorf("dano %d no Kefra = %d, quero %d", c.dano, got, c.quero)
		}
	}
	nu := comDivisor(0, 0)
	if got := danoNoPortador(nu, 21371); got != 21371 {
		t.Errorf("sem divisor o dano tem de passar inteiro: %d", got)
	}
	// O veneno do 786 é dividido pelo dobro (ProcessSecMinTimer.cpp:2339).
	if got := tiqueNoPortador(comDivisor(786, 15), 1000); got != 33 {
		t.Errorf("veneno de 1000 no 786 +15 = %d, quero 33", got)
	}
}

// O spawn carrega o item do slot 13 do template, e só ele: o resto do
// equipamento do monstro continua sendo aparência.
func TestSpawnCarregaODivisorDoTemplate(t *testing.T) {
	root := releaseDir(t)
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	casos := []struct {
		arquivo string
		item    int16
		valor   uint8
		divisor int32
	}{
		{"Cristal", 1937, 15, 15000},
		{"Verid", 1936, 15, 150},
		{"Templario_Amald", 1936, 2, 20},
		{"Lugefer", 786, 6, 6},
		{"Rainha_Rubra", 0, 0, 1}, // nenhum item no slot 13
	}
	for _, c := range casos {
		tmpl, _, err := npctemplate.Load(root, c.arquivo)
		if err != nil {
			t.Fatalf("%s: %v", c.arquivo, err)
		}
		w := world.New(world.Config{GridDim: 64}, log, nil, nil)
		id := w.SpawnMob(tmpl, 30, 30)
		if id < 0 {
			t.Fatalf("%s: não nasceu", c.arquivo)
		}
		e := w.Entity(id)
		if got := e.Equip[world.DividerEquipSlot].Index; got != c.item {
			t.Errorf("%s: item no slot 13 = %d, quero %d", c.arquivo, got, c.item)
		}
		if got := e.Equip[world.DividerEquipSlot].Effects[0].Value; got != c.valor {
			t.Errorf("%s: valor do divisor = %d, quero %d", c.arquivo, got, c.valor)
		}
		if got := divisorDeGolpe(e); got != c.divisor {
			t.Errorf("%s: divisor = %d, quero %d", c.arquivo, got, c.divisor)
		}
		// Os outros quinze slots continuam vazios: ligar o equipamento inteiro é
		// outra obra (equipamento-de-mob-nao-conta).
		for i := range e.Equip {
			if i != world.DividerEquipSlot && e.Equip[i].Index != 0 {
				t.Errorf("%s: slot %d veio preenchido (%d)", c.arquivo, i, e.Equip[i].Index)
			}
		}
	}
}

// O item do slot 13 não pode mexer no score do monstro: os três divisores só
// têm EF_CLASS no catálogo, e o refino gravado neles não vira atributo.
func TestDivisorNaoMexeNoScoreDoMonstro(t *testing.T) {
	root := releaseDir(t)
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	d := New(Config{Log: log})
	for _, arquivo := range []string{"Kefra", "Sombra_Negra", "Verid", "Templario_Amald"} {
		tmpl, _, err := npctemplate.Load(root, arquivo)
		if err != nil {
			t.Fatalf("%s: %v", arquivo, err)
		}
		w := world.New(world.Config{GridDim: 64}, log, nil, nil)
		e := w.Entity(w.SpawnMob(tmpl, 30, 30))
		antes := *e
		d.refreshScore(e)
		if e.MaxHP != antes.MaxHP || e.Damage != antes.Damage || e.AC != antes.AC || e.HpAddPct != 0 {
			t.Errorf("%s: o slot 13 mexeu no score — vida %d→%d, dano %d→%d, defesa %d→%d, HpAddPct %d",
				arquivo, antes.MaxHP, e.MaxHP, antes.Damage, e.Damage, antes.AC, e.AC, e.HpAddPct)
		}
	}
}

// mobComDivisor é o mob de campo dos testes de combate com um item no slot da
// fada: índice em Equip[13] e o refino no primeiro par de efeitos, que é de onde
// o legado tira o valor do divisor.
func mobComDivisor(nome string, hp uint32, item int16, valor uint8) []byte {
	b := targetMob(nome, 0, hp)
	const eq = 140 + world.DividerEquipSlot*8 // Equip[16] @140, STRUCT_ITEM de 8 bytes
	binary.LittleEndian.PutUint16(b[eq:], uint16(item))
	b[eq+2], b[eq+3] = 43, valor // stEffect[0] = EF_SANC, valor
	return b
}

// O golpe de um jogador em cima de um monstro com divisor: o cliente vê o número
// inteiro e o monstro perde só a parte dividida. É o caminho de verdade, do
// socket até a vida do bicho (_MSG_Attack.cpp:1570).
func TestGolpeEmChefeComDivisor(t *testing.T) {
	casos := []struct {
		nome    string
		item    int16
		valor   uint8
		divisor int32
	}{
		{"sem divisor", 0, 0, 1},
		{"786 +4", 786, 4, 4},
		{"1936 +2, como o Lich Crunt", 1936, 2, 20},
		{"1937 sem refino, como o Kefra", 1937, 0, 2000},
	}
	for _, c := range casos {
		t.Run(c.nome, func(t *testing.T) {
			addr, stop, w := startServerSkillsTargetMob(t, skillCombatDB(0), mobComDivisor("Chefe", 5000, c.item, c.valor))
			defer stop()
			conn := enterWorld(t, addr)
			defer conn.Close()

			antes := vidaDoMob(w)
			golpeCorpoACorpo(t, conn, serverTime, world.MaxUser, 6, 5, 6, 5)
			eco, ok := ecoDoGolpe(t, conn)
			if !ok {
				t.Fatal("o golpe foi recusado")
			}
			golpe := eco.Dam[0].Damage
			if golpe <= 0 {
				t.Fatalf("golpe sem dano: %d", golpe)
			}
			perdeu := antes - vidaDoMob(w)
			if quero := golpe / c.divisor; perdeu != quero {
				t.Errorf("golpe de %d com divisor %d: o chefe perdeu %d de vida, quero %d",
					golpe, c.divisor, perdeu, quero)
			}
			// O divisor não pode encolher o número que o cliente desenha: quem bate
			// continua vendo o golpe inteiro, e é isso que o legado faz.
			if c.divisor > 1 && golpe < c.divisor && perdeu != 0 {
				t.Errorf("golpe de %d menor que o divisor %d tirou %d; tinha de tirar zero", golpe, c.divisor, perdeu)
			}
		})
	}
}

// O veneno também é dividido, e no tique do relógio o legado dobra o divisor do
// item 786 (ProcessSecMinTimer.cpp:2339). Aqui o caminho é o processMobAffect do
// servidor, o mesmo que roda em jogo.
func TestVenenoEmChefeComDivisor(t *testing.T) {
	casos := []struct {
		nome    string
		item    int16
		valor   uint8
		divisor int32
	}{
		{"sem divisor", 0, 0, 1},
		{"786 +4 dobra no veneno", 786, 4, 8},
		{"1936 +2 não dobra", 1936, 2, 20},
	}
	for _, c := range casos {
		log := slog.New(slog.NewTextHandler(io.Discard, nil))
		d := New(Config{Log: log})
		w := world.New(world.Config{GridDim: 32}, log, nil, nil)
		id := w.SpawnMob(mobComDivisor("Chefe", 5000, c.item, c.valor), 10, 10)
		e := w.Entity(id)
		e.Affect[0] = world.Affect{Type: affectPoison, Time: 10}
		antes := e.HP
		d.processMobAffect(w, id, e)
		if perdeu, quero := antes-e.HP, int32(poisonTickDamage)/c.divisor; perdeu != quero {
			t.Errorf("%s: o veneno tirou %d, quero %d", c.nome, perdeu, quero)
		}
	}
}

// O golpe de MONSTRO em cima de quem carrega divisor passa pelo mesmo corte
// (Server.cpp:10066) — é o caminho dos pets e das evocações batendo num chefe.
func TestGolpeDeMonstroEmChefeComDivisor(t *testing.T) {
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	for _, com := range []bool{false, true} {
		d := New(Config{Log: log})
		clock := &atomic.Uint32{}
		clock.Store(10_000)
		w := world.New(world.Config{GridDim: 32, Now: clock.Load}, log, nil, nil)
		item, valor := int16(0), uint8(0)
		if com {
			item, valor = 1937, 0 // ÷2000
		}
		chefe := w.Entity(w.SpawnMob(mobComDivisor("Chefe", 5000, item, valor), 10, 10))
		bicho := w.Entity(w.SpawnMob(aggressiveMob(), 11, 10))
		antes := chefe.HP
		// O relógio anda entre os golpes: mobAttack tem cadência, e o primeiro golpe
		// de cada bicho ainda é adiado de propósito para não sair em salva.
		for i := 0; i < 8; i++ {
			d.mobAttack(w, bicho.ID, bicho, chefe)
			clock.Add(3000)
		}
		perdeu := antes - chefe.HP
		// O bicho bate 500 por golpe: com ÷2000 não arranha, sem divisor derruba.
		if com && perdeu != 0 {
			t.Errorf("com o divisor do Kefra, oito golpes de 500 tiraram %d de vida", perdeu)
		}
		if !com && perdeu <= 0 {
			t.Errorf("sem divisor, oito golpes não tiraram vida nenhuma (%d)", perdeu)
		}
	}
}

// O desenho dos quatro chefes (18/09/2026), em vida do template × divisor do
// slot 13. Os tempos vêm da simulação de raide (simulacao_chefes_test.go, tag
// simulacao), com Huntress iguais ao Xorimpas e 60 s para quem morre voltar da
// cidade.
func TestDesenhoDosChefes(t *testing.T) {
	root := releaseDir(t)
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	casos := []struct {
		arquivo  string
		vida     int32
		item     int16
		valor    uint8
		divisor  int32
		efetiva  int64
		proposta string
	}{
		{"Kefra", 56_000_000, 1936, 2, 20, 1_120_000_000, "2 h com 20 jogadores; com 16 ou menos a guilda não derruba"},
		{"Cav._Lugefer", 32_000, 1936, 25, 250, 8_000_000, "15 min com uma party de 6"},
		{"Sombra_Negra", 28_000, 1936, 30, 300, 8_400_000, "48 min com uma party de 6"},
		{"Sombra_Negra_", 28_000, 1936, 30, 300, 8_400_000, "o gêmeo do Sombra Negra, mesmo desenho"},
		{"Lich_Crunt", 800_000, 1936, 2, 20, 16_000_000, "51 min com uma party de 6"},
	}
	for _, c := range casos {
		tmpl, _, err := npctemplate.Load(root, c.arquivo)
		if err != nil {
			t.Fatalf("%s: %v", c.arquivo, err)
		}
		w := world.New(world.Config{GridDim: 64}, log, nil, nil)
		e := w.Entity(w.SpawnMob(tmpl, 30, 30))
		it := e.Equip[world.DividerEquipSlot]
		if it.Index != c.item || it.Effects[0].Value != c.valor {
			t.Errorf("%s: slot 13 com item %d +%d, quero %d +%d (%s)",
				c.arquivo, it.Index, it.Effects[0].Value, c.item, c.valor, c.proposta)
		}
		if got := divisorDeGolpe(e); got != c.divisor {
			t.Errorf("%s: divisor %d, quero %d", c.arquivo, got, c.divisor)
		}
		if e.MaxHP != c.vida {
			t.Errorf("%s: vida do template %d, quero %d", c.arquivo, e.MaxHP, c.vida)
		}
		if got := int64(e.MaxHP) * int64(divisorDeGolpe(e)); got != c.efetiva {
			t.Errorf("%s: %d de vida efetiva, quero %d — %s", c.arquivo, got, c.efetiva, c.proposta)
		}
	}
}
