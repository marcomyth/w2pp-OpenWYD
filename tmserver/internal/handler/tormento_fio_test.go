package handler

import (
	"context"
	"io"
	"log/slog"
	"net"
	"testing"
	"time"

	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/content"
	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/protocol"
	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/world"
)

// A REFLEXÃO DO ESCUDO DO TORMENTO, NOS DOIS CAMINHOS DE GOLPE.
//
// A função certa existindo não prova nada: ela tem de estar LIGADA no golpe de
// jogador (combat.go) e no de monstro (mobai.go). Testar só a função deixa
// passar exatamente o erro mais comum — e passou: as duas sabotagens que
// desligavam as chamadas não quebraram nenhum teste unitário.

// bmComTormento veste um BM de escudo, com a passiva e o Éden.
func bmComTormento(e *world.Entity) *world.Entity {
	e.Class = 2
	e.LearnedSkill |= learnedEscudoDoTormento | learnedEden
	e.Special[naturezaKind] = naturezaMaestriaCheia
	e.BaseSpecial = e.Special
	e.Str, e.Dex = 2800, 700
	e.Equip[weaponSlotR] = world.Item{Index: testHermai}
	e.Equip[weaponSlotL] = world.Item{Index: testEscudo}
	return e
}

// efeitosDoTormento é o catálogo mínimo que o teste precisa: o Hermai com o seu
// wtype e o escudo SEM wtype, que é como o servidor o reconhece.
func efeitosDoTormento() map[int][]content.BaseEffect {
	return map[int][]content.BaseEffect{
		testHermai: {{Eff: efWType, Val: wtypeArremesso}},
		testEscudo: {{Eff: efAc, Val: testEscudoAC}},
	}
}

// O GOLPE DE MONSTRO: o mob que bate no BM perde vida por isso.
func TestTormentoRefleteNoGolpeDeMonstro(t *testing.T) {
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	d := New(Config{Log: log, CombatRules: regraSemEscala(), ItemEffects: efeitosDoTormento()})
	w := world.New(world.Config{GridDim: 64}, log, nil, nil)

	alvo := bmComTormento(&world.Entity{ID: 1, Level: 399, HP: 500_000, MaxHP: 500_000, AC: 100})
	mob := &world.Entity{ID: world.MaxUser + 1, Level: 399, HP: 100_000, MaxHP: 100_000, Damage: 5_000}
	vidaDoMob := mob.HP

	// Vários golpes, porque um só pode ser aparado (o parry é um sorteio).
	for range 40 {
		alvo.HP = alvo.MaxHP
		d.danoDoGolpeDeMonstro(w, mob, alvo)
	}
	if mob.HP >= vidaDoMob {
		t.Errorf("o monstro bateu 40 vezes e não perdeu vida: %d de %d — a reflexão não está ligada em mobai.go",
			mob.HP, vidaDoMob)
	}

	// E o contraprova: sem a passiva, o monstro não perde nada.
	semPassiva := bmComTormento(&world.Entity{ID: 1, Level: 399, HP: 500_000, MaxHP: 500_000, AC: 100})
	semPassiva.LearnedSkill = 0
	mob2 := &world.Entity{ID: world.MaxUser + 2, Level: 399, HP: 100_000, MaxHP: 100_000, Damage: 5_000}
	for range 40 {
		semPassiva.HP = semPassiva.MaxHP
		d.danoDoGolpeDeMonstro(w, mob2, semPassiva)
	}
	if mob2.HP != 100_000 {
		t.Errorf("sem a passiva o monstro não podia perder vida, tem %d", mob2.HP)
	}
}

// O GOLPE DE JOGADOR: quem bate no BM pelo protocolo perde vida por isso.
func TestTormentoRefleteNoGolpeDeJogador(t *testing.T) {
	db := newDB()
	db.loadResult = world.CharacterState{
		Slot: 0, Name: "Lutador", Class: 0, X: 40, Y: 40,
		HP: 60_000, MaxHP: 60_000, MP: 30_000, MaxMP: 30_000, Level: 399, Str: 2000,
	}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	d := New(Config{Log: log, CombatRules: regraSemEscala(), ItemEffects: efeitosDoTormento()})
	w := world.New(world.Config{GridDim: 64, Now: relogioEmServerTime()}, log, db, d.Handle)
	// Sem tick handler o GoDetached do noLaco nunca roda.
	w.SetTickHandler(10*time.Millisecond, d.Tick)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { _ = w.Serve(ctx, ln); close(done) }()
	defer func() { cancel(); <-done }()

	atacante := enterWorldAs(t, ln.Addr().String(), "tester")
	defer atacante.Close()
	vitima := enterWorldAs(t, ln.Addr().String(), "tradeb")
	defer vitima.Close()
	esvaziar(atacante, vitima)

	const atacanteID, vitimaID = 1, 2
	send(t, atacante, protocol.MsgPKMode, protocol.EncodeStandardParm(1))

	// A vítima vira um BM de escudo com a passiva e o Éden; os dois ficam lado a
	// lado em campo aberto, fora de cidade.
	// A vítima vira um BM de escudo com a passiva e o Éden; os dois ficam lado a
	// lado em campo aberto, fora de cidade.
	//
	// A ORDEM importa: refreshScore RECALCULA Damage, AC e MaxHP a partir dos
	// atributos e do equipamento, então tudo que este teste força tem de vir
	// DEPOIS dele. Forçar antes deixava o atacante batendo 1 de dano, e 10% de 1
	// é zero — o teste falhava com o código certo.
	noLaco(t, w, func(w *world.World) {
		a, v := w.Entity(atacanteID), w.Entity(vitimaID)
		if a == nil || v == nil {
			t.Fatal("os dois jogadores tinham de estar no mundo")
		}
		a.X, a.Y, v.X, v.Y = 40, 40, 41, 40
		a.PKMode = true
		a.PKPoint, v.PKPoint = 50, 50
		bmComTormento(v)
		d.refreshScore(v)
		d.refreshScore(a)
		// Agora sim: um atacante que machuca e uma vítima que aguenta os 40 golpes.
		a.Damage, a.AffDamage, a.HP, a.MaxHP = 20_000, 0, 60_000, 60_000
		v.AC, v.AffAC = 0, 0
		v.HP, v.MaxHP = 2_000_000, 2_000_000
	})

	var vidaInicial int32
	noLaco(t, w, func(w *world.World) { vidaInicial = w.Entity(atacanteID).HP })

	for i := range 40 {
		attackFrame(t, atacante, serverTime+uint32(i)*1000, vitimaID, -1)
	}

	// O golpe TEM de ter entrado: um teste em que ninguém apanha passa com a
	// reflexão desligada e com ela ligada, e não prova nada.
	vidaDaVitima, vidaFinal := esperarReflexao(t, w, vitimaID, atacanteID, vidaInicial)
	if vidaDaVitima >= 2_000_000 {
		t.Fatalf("a vítima não apanhou (%d de 2000000): o teste não chegou a exercitar a reflexão", vidaDaVitima)
	}
	if vidaFinal >= vidaInicial {
		t.Errorf("o atacante bateu 40 vezes num BM de escudo e não perdeu vida (%d de %d) — "+
			"a reflexão não está ligada em combat.go", vidaFinal, vidaInicial)
	}
}

// esperarReflexao espera os golpes escritos no socket chegarem ao laço, e
// devolve as duas vidas lidas na MESMA passagem.
//
// attackFrame só escreve no socket e volta; noLaco entra no laço por outro
// caminho (GoDetached) e não espera a goroutine da conexão consumir o que foi
// escrito. Ler a vida uma vez só, logo depois dos 40 golpes, passa na máquina
// rápida e falha na carregada: na CI de 21/09/2026 este teste morreu no próprio
// guard, "a vítima não apanhou (2000000 de 2000000)", com o pacote levando 330 s
// contra 200 s aqui. O prazo é generoso de propósito — quem falha por tempo é
// só o caso em que a reflexão realmente não acontece.
func esperarReflexao(t *testing.T, w *world.World, vitimaID, atacanteID int, vidaInicial int32) (int32, int32) {
	t.Helper()
	prazo := time.Now().Add(10 * time.Second)
	for {
		var vidaDaVitima, vidaFinal int32
		noLaco(t, w, func(w *world.World) {
			vidaDaVitima = w.Entity(vitimaID).HP
			vidaFinal = w.Entity(atacanteID).HP
		})
		if vidaFinal < vidaInicial || time.Now().After(prazo) {
			return vidaDaVitima, vidaFinal
		}
		time.Sleep(10 * time.Millisecond)
	}
}
