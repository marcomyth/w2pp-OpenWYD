package handler

import (
	"path/filepath"
	"testing"

	"github.com/jeanluca/w2pp-openwyd/internal/npcgener"
	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/world"
)

// A população das arenas (populacao_arenas.go). Os testes sobem o servidor do
// relógio com o tique parado e rodam as passadas DENTRO do laço, com os blocos
// postos à mão: dois no Cemitério (um da fila de 15 s, um de relógio com grupo) e
// dois fora, em Armia, ao lado do Mestre Grifo. Os monstros são de clã hostil: um
// de clã neutro dentro da cidade é NPC (world.nonCombatNPC) e não volta pela fila.

// Os índices ficam longe de 0-7, que são do evento do Coliseu
// (world.IsEventOwnedGenerator) e não entram nem na fila nem nas arenas.
const (
	arenaFila    = 40 // Cemitério, MinuteGenerate -1, até 2
	arenaRelogio = 41 // Cemitério, relógio de 8 passadas, grupo de 3, até 3
	foraFila     = 42 // Armia, MinuteGenerate -1, até 2
	foraRelogio  = 43 // Armia, relógio de 1 passada, até 2
)

// passadaDoRelogio é a passada 9: o bloco de relógio de 8 (índice 41) só roda em
// 1, 9, 17...
const passadaDoRelogio = 9

func montaBlocosDaArena(w *world.World, d *Dispatcher) []*world.Generator {
	mob := func(nome string) []byte { b := expMobTemplate(80, 1000, 5); copy(b[0:16], nome); return b }
	gens := make([]*world.Generator, foraRelogio+1)
	for idx, g := range map[int]*world.Generator{
		arenaFila:    {Name: "Esqueleto", MinuteGenerate: -1, MaxNumMob: 2, LeaderTmpl: mob("Esqueleto"), SegX: [5]int16{2400}, SegY: [5]int16{2100}},
		arenaRelogio: {Name: "Aparicao", MinuteGenerate: 8, MinGroup: 2, MaxGroup: 2, MaxNumMob: 3, LeaderTmpl: mob("Aparicao"), FollowerTmpl: mob("Esqueleto"), SegX: [5]int16{2412}, SegY: [5]int16{2115}},
		foraFila:     {Name: "Lobo", MinuteGenerate: -1, MaxNumMob: 2, LeaderTmpl: mob("Lobo"), SegX: [5]int16{2130}, SegY: [5]int16{2090}},
		foraRelogio:  {Name: "Urso", MinuteGenerate: 1, MaxNumMob: 2, LeaderTmpl: mob("Urso"), SegX: [5]int16{2140}, SegY: [5]int16{2095}},
	} {
		gens[idx] = g
	}
	w.RegisterGenerators(gens)
	d.resolverBlocosDasArenas(w)
	return gens
}

// passada roda o relógio de 12 s na passada p, na ordem do Tick.
func passada(w *world.World, d *Dispatcher, p int) {
	antes := d.tickCount
	d.tickCount = p * minTimerTicks
	d.generateMobs(w)
	d.reporArenas(w)
	d.tickCount = antes
}

// poeNaArena leva todo jogador em jogo para dentro do Cemitério, com a bandeira.
func poeNaArena(w *world.World, d *Dispatcher) {
	i := int16(0)
	w.ForEachPlayer(func(s *world.Session, e *world.Entity) {
		e.QuestFlag = quest256Steps[0].flag
		d.doTeleport(w, s, 2395+i, 2090)
		i += 2
	})
}

// mataDoBloco mata n monstros vivos do bloco, como morte em combate.
func mataDoBloco(w *world.World, gen, n int) int {
	var ids []int
	w.ForEachMob(func(id int, e *world.Entity) {
		if int(e.GenIndex) == gen && len(ids) < n {
			ids = append(ids, id)
		}
	})
	for _, id := range ids {
		w.DespawnMob(id, 1)
	}
	return len(ids)
}

// TestArenaUmJogadorMantemABase: sem ninguém e com um jogador dentro, os blocos da
// arena ficam no teto do conteúdo (base 1,0 do Coveiro), e só os da arena são
// marcados para a passada nova.
func TestArenaUmJogadorMantemABase(t *testing.T) {
	srv := startServerRelogioDasArenas(t, mortalDoCemiterio(), inicioDaVolta)
	c := enterWorldAs(t, srv.addr, "tester")
	defer c.Close()
	drena(t, c)

	srv.noLaco(t, func(w *world.World, d *Dispatcher) {
		gens := montaBlocosDaArena(w, d)
		for i, quer := range map[int]bool{arenaFila: true, arenaRelogio: true, foraFila: false, foraRelogio: false} {
			if gens[i].ArenaRefill != quer {
				t.Errorf("bloco %d: da arena = %v, quero %v", i, gens[i].ArenaRefill, quer)
			}
		}
		passada(w, d, passadaDoRelogio)
		if gens[arenaFila].CurrentNumMob != 2 || gens[arenaRelogio].CurrentNumMob != 3 {
			t.Errorf("sem ninguém dentro: %d e %d, quero a base 2 e 3", gens[arenaFila].CurrentNumMob, gens[arenaRelogio].CurrentNumMob)
		}
		poeNaArena(w, d)
		if n := jogadoresNasArenas(w)[0]; n != 1 {
			t.Errorf("jogadores no Cemitério = %d, quero 1", n)
			return
		}
		mataDoBloco(w, arenaFila, 2)
		mataDoBloco(w, arenaRelogio, 3)
		passada(w, d, passadaDoRelogio+8)
		if gens[arenaFila].CurrentNumMob != 2 || gens[arenaRelogio].CurrentNumMob != 3 {
			t.Errorf("um jogador dentro: %d e %d, quero a base 2 e 3", gens[arenaFila].CurrentNumMob, gens[arenaRelogio].CurrentNumMob)
		}
	})
}

// TestArenaTresJogadoresTriplicamESaidaBaixaSemMatar: três dentro, três vezes a
// base, em grupos que não passam do teto. Dois saem: a passada seguinte não mata
// ninguém; conforme os monstros morrem, a reposição para na base de um. A fila de
// 15 s não repõe os mortos da arena.
func TestArenaTresJogadoresTriplicamESaidaBaixaSemMatar(t *testing.T) {
	b, c3 := mortalDoCemiterio(), mortalDoCemiterio()
	b.Name, c3.Name = "HeroB", "HeroC"
	srv := startServerRelogioDasArenasCom(t, mortalDoCemiterio(), inicioDaVolta, map[int64]world.CharacterState{11: b, 12: c3})
	for _, conta := range []string{"tester", "tradeb", "conta12"} {
		c := enterWorldAs(t, srv.addr, conta)
		t.Cleanup(func() { _ = c.Close() })
		drena(t, c)
	}

	srv.noLaco(t, func(w *world.World, d *Dispatcher) {
		gens := montaBlocosDaArena(w, d)
		poeNaArena(w, d)
		if n := jogadoresNasArenas(w)[0]; n != 3 {
			t.Errorf("jogadores no Cemitério = %d, quero 3", n)
			return
		}
		passada(w, d, passadaDoRelogio)
		if gens[arenaFila].CurrentNumMob != 6 || gens[arenaRelogio].CurrentNumMob != 9 {
			t.Errorf("três dentro: %d e %d, quero 6 e 9", gens[arenaFila].CurrentNumMob, gens[arenaRelogio].CurrentNumMob)
			return
		}

		// Dois saem pela cidade, sem a bandeira.
		saiu := 0
		w.ForEachPlayer(func(s *world.Session, e *world.Entity) {
			if s.AccountID != 7 {
				e.QuestFlag = 0
				d.doTeleport(w, s, 2113+int16(saiu), 2079)
				saiu++
			}
		})
		if n := jogadoresNasArenas(w)[0]; n != 1 {
			t.Errorf("depois da saída, jogadores no Cemitério = %d, quero 1", n)
			return
		}
		passada(w, d, passadaDoRelogio+8)
		if gens[arenaFila].CurrentNumMob != 6 || gens[arenaRelogio].CurrentNumMob != 9 {
			t.Errorf("a passada depois da saída mexeu nos vivos: %d e %d, quero 6 e 9", gens[arenaFila].CurrentNumMob, gens[arenaRelogio].CurrentNumMob)
		}

		// Morrem cinco da fila e sete do relógio; a fila de 15 s não traz nenhum.
		if mataDoBloco(w, arenaFila, 5) != 5 || mataDoBloco(w, arenaRelogio, 7) != 7 {
			t.Error("não achei os monstros para matar")
			return
		}
		if ids := w.SpawnDueRespawns(w.Now() + 60_000); len(ids) != 0 {
			t.Errorf("a fila de 15 s repôs %d monstros da arena", len(ids))
		}
		passada(w, d, passadaDoRelogio+16)
		if gens[arenaFila].CurrentNumMob != 2 || gens[arenaRelogio].CurrentNumMob != 3 {
			t.Errorf("reposição com um jogador: %d e %d, quero a base 2 e 3", gens[arenaFila].CurrentNumMob, gens[arenaRelogio].CurrentNumMob)
		}
	})
}

// TestArenaContaSoVivoComBandeiraDentro: morto, sem bandeira ou com a bandeira de
// outra arena não conta.
func TestArenaContaSoVivoComBandeiraDentro(t *testing.T) {
	outro := mortalDoCemiterio()
	outro.Name = "HeroB"
	srv := startServerRelogioDasArenasCom(t, mortalDoCemiterio(), inicioDaVolta, map[int64]world.CharacterState{11: outro})
	for _, conta := range []string{"tester", "tradeb"} {
		c := enterWorldAs(t, srv.addr, conta)
		t.Cleanup(func() { _ = c.Close() })
		drena(t, c)
	}

	srv.noLaco(t, func(w *world.World, d *Dispatcher) {
		poeNaArena(w, d)
		var b *world.Entity
		w.ForEachPlayer(func(s *world.Session, e *world.Entity) {
			if s.AccountID == 11 {
				b = e
			}
		})
		casos := []struct {
			nome  string
			muda  func()
			quero int
		}{
			{"os dois vivos com bandeira", func() {}, 2},
			{"um morto", func() { b.HP = 0 }, 1},
			{"um sem bandeira", func() { b.HP = 1000; b.QuestFlag = 0 }, 1},
			{"um com a bandeira do Jardim", func() { b.QuestFlag = quest256Steps[1].flag }, 1},
		}
		for _, cs := range casos {
			cs.muda()
			if n := jogadoresNasArenas(w)[0]; n != cs.quero {
				t.Errorf("%s: contou %d, quero %d", cs.nome, n, cs.quero)
			}
		}
	})
}

// TestBlocoForaDaArenaNaoMuda: com dois jogadores na arena, os blocos de fora
// continuam como sempre: o de relógio põe um grupo por passada até o teto do
// conteúdo, o da fila volta pela fila, e a passada das arenas não mexe em nenhum.
// O relógio comum também não mexe no bloco de relógio da arena.
func TestBlocoForaDaArenaNaoMuda(t *testing.T) {
	outro := mortalDoCemiterio()
	outro.Name = "HeroB"
	srv := startServerRelogioDasArenasCom(t, mortalDoCemiterio(), inicioDaVolta, map[int64]world.CharacterState{11: outro})
	for _, conta := range []string{"tester", "tradeb"} {
		c := enterWorldAs(t, srv.addr, conta)
		t.Cleanup(func() { _ = c.Close() })
		drena(t, c)
	}

	srv.noLaco(t, func(w *world.World, d *Dispatcher) {
		gens := montaBlocosDaArena(w, d)
		poeNaArena(w, d)
		w.GenerateMob(foraFila)
		w.GenerateMob(foraFila)
		for p := passadaDoRelogio; p < passadaDoRelogio+4; p++ {
			passada(w, d, p)
		}
		if gens[foraRelogio].CurrentNumMob != 2 || gens[foraFila].CurrentNumMob != 2 {
			t.Errorf("fora da arena: relógio %d e fila %d, quero o teto do conteúdo 2 e 2", gens[foraRelogio].CurrentNumMob, gens[foraFila].CurrentNumMob)
		}
		if gens[arenaFila].CurrentNumMob != 4 {
			t.Errorf("na arena com dois: %d, quero 4 (a conta do teste está certa?)", gens[arenaFila].CurrentNumMob)
		}

		mataDoBloco(w, foraFila, 1)
		if ids := w.SpawnDueRespawns(w.Now() + 60_000); len(ids) != 1 || gens[foraFila].CurrentNumMob != 2 {
			t.Errorf("o bloco de fora não voltou pela fila: %d renascidos, contagem %d", len(ids), gens[foraFila].CurrentNumMob)
		}

		// O relógio comum não repõe o bloco de relógio da arena.
		mataDoBloco(w, arenaRelogio, gens[arenaRelogio].CurrentNumMob)
		antes := d.tickCount
		d.tickCount = (passadaDoRelogio + 8) * minTimerTicks
		d.generateMobs(w)
		d.tickCount = antes
		if gens[arenaRelogio].CurrentNumMob != 0 {
			t.Errorf("o relógio comum repôs o bloco da arena: %d", gens[arenaRelogio].CurrentNumMob)
		}
	})
}

// TestLimiteDeCargaDaArena: o multiplicador para no limite de carga, e nunca fica
// abaixo de um.
func TestLimiteDeCargaDaArena(t *testing.T) {
	if cargaMaxArena != 164 {
		t.Fatalf("carga máxima = %d, quero 164 (8 salas de 20 e os 4 do chefe da Água)", cargaMaxArena)
	}
	d := New(Config{})
	d.popBaseDasArenas = [len(baseDaArenaDecimos)]int{46, 25, 45, 51, 29}
	casos := []struct {
		passo, jogadores, quero int
	}{
		{0, 0, 1}, {0, 1, 1}, {0, 3, 3}, {0, 10, 3}, // Coveiro: 164/46 = 3
		{1, 6, 6}, {1, 7, 6}, // Jardim: 164/25 = 6
		{4, 5, 5}, {4, 9, 5}, // Elfos: 164/29 = 5
	}
	for _, cs := range casos {
		if got := d.multiplicadorDaArena(cs.passo, cs.jogadores); got != cs.quero {
			t.Errorf("arena %d com %d jogadores: %dx, quero %dx", cs.passo, cs.jogadores, got, cs.quero)
		}
	}
}

// TestBlocosDasArenasNoConteudo: no NPCGener de verdade, os blocos e a população
// de um jogador por arena são os que o modelo mediu (zzplano, TROFEU=populacao).
func TestBlocosDasArenasNoConteudo(t *testing.T) {
	root := releaseDir(t)
	gens, err := npcgener.Load(filepath.Join(root, "TMsrv", "run", "NPCGener.txt"))
	if err != nil {
		t.Fatal(err)
	}
	var blocos, pop [len(baseDaArenaDecimos)]int
	for idx, g := range gens {
		if g.Leader == "" || g.MaxNumMob <= 0 ||
			world.IsWaterDungeonGenerator(idx) || world.IsEventOwnedGenerator(idx) || world.IsKefraGenerator(idx) {
			continue
		}
		passo, ok := passoDoBloco(g.SegX, g.SegY)
		if !ok {
			continue
		}
		blocos[passo]++
		pop[passo] += (g.MaxNumMob*baseDaArenaDecimos[passo] + 9) / 10
	}
	if quero := [...]int{18, 10, 13, 20, 7}; blocos != quero {
		t.Errorf("blocos por arena = %v, quero %v", blocos, quero)
	}
	if quero := [...]int{46, 25, 45, 51, 29}; pop != quero {
		t.Errorf("população de um jogador por arena = %v, quero %v", pop, quero)
	}
}
