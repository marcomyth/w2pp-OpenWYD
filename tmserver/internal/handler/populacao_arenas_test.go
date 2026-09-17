package handler

import (
	"path/filepath"
	"slices"
	"testing"

	"github.com/jeanluca/w2pp-openwyd/internal/npcgener"
	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/world"
)

// A população das arenas (populacao_arenas.go). Os testes sobem o servidor do
// relógio com o tique parado e rodam as passadas DENTRO do laço, com os blocos
// postos à mão: dois no Cemitério (um da fila de 15 s, um de relógio com grupo) e
// dois fora, em Armia, ao lado do Mestre Grifo. Os blocos da arena espalham o
// ponto de nascimento (SegRange), como o conteúdo: o servidor só procura célula
// livre num 7x7 em volta do ponto, e 162 monstros parados não cabem num ponto só.
// Os monstros são de clã hostil: um de clã neutro dentro da cidade é NPC
// (world.nonCombatNPC) e não volta pela fila.

// Os índices ficam longe de 0-7, que são do evento do Coliseu
// (world.IsEventOwnedGenerator) e não entram nem na fila nem nas arenas.
const (
	arenaFila    = 40 // Cemitério, MinuteGenerate -1, teto 2 no conteúdo
	arenaRelogio = 41 // Cemitério, relógio de 8 passadas, grupo de 3, teto 3
	foraFila     = 42 // Armia, MinuteGenerate -1, teto 2
	foraRelogio  = 43 // Armia, relógio de 1 passada, teto 2
)

// Com o mínimo do Cemitério (90 por jogador), os tetos 2 e 3 do conteúdo viram
// 36 e 54.
const (
	baseFila    = 36
	baseRelogio = 54
)

// passadaDoRelogio é a vez do bloco de relógio de 8 (índice 41) pelo período do
// conteúdo (1, 9, 17...); passadaForaDaVez não é, e dentro da arena ele enche
// assim mesmo.
const (
	passadaDoRelogio = 9
	passadaForaDaVez = 10
)

func montaBlocosDaArena(w *world.World, d *Dispatcher) []*world.Generator {
	mob := func(nome string) []byte {
		b := expMobTemplate(80, 1000, 5)
		copy(b[0:16], nome)
		return b
	}
	gens := make([]*world.Generator, foraRelogio+1)
	gens[arenaFila] = &world.Generator{Name: "Esqueleto", MinuteGenerate: -1, MaxNumMob: 2, LeaderTmpl: mob("Esqueleto"),
		SegX: [5]int16{2400}, SegY: [5]int16{2100}, SegRange: [5]int16{12}}
	gens[arenaRelogio] = &world.Generator{Name: "Aparicao", MinuteGenerate: 8, MinGroup: 2, MaxGroup: 2, MaxNumMob: 3,
		LeaderTmpl: mob("Aparicao"), FollowerTmpl: mob("Esqueleto"), SegX: [5]int16{2412}, SegY: [5]int16{2115}, SegRange: [5]int16{12}}
	gens[foraFila] = &world.Generator{Name: "Lobo", MinuteGenerate: -1, MaxNumMob: 2, LeaderTmpl: mob("Lobo"),
		SegX: [5]int16{2130}, SegY: [5]int16{2090}}
	gens[foraRelogio] = &world.Generator{Name: "Urso", MinuteGenerate: 1, MaxNumMob: 2, LeaderTmpl: mob("Urso"),
		SegX: [5]int16{2140}, SegY: [5]int16{2095}}
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
// arena ficam na base (90 repartidos: 36 e 54), o de relógio inclusive fora da
// vez dele, e só os da arena são marcados para a passada nova.
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
		if d.popBaseDasArenas[0] != densidadeMinimaPorArena[0] {
			t.Errorf("população de um jogador no Cemitério = %d, quero o mínimo %d", d.popBaseDasArenas[0], densidadeMinimaPorArena[0])
		}
		passada(w, d, passadaForaDaVez)
		if gens[arenaFila].CurrentNumMob != baseFila || gens[arenaRelogio].CurrentNumMob != baseRelogio {
			t.Errorf("sem ninguém dentro: %d e %d, quero a base %d e %d",
				gens[arenaFila].CurrentNumMob, gens[arenaRelogio].CurrentNumMob, baseFila, baseRelogio)
		}
		poeNaArena(w, d)
		if n := jogadoresNasArenas(w)[0]; n != 1 {
			t.Errorf("jogadores no Cemitério = %d, quero 1", n)
			return
		}
		mataDoBloco(w, arenaFila, baseFila)
		mataDoBloco(w, arenaRelogio, baseRelogio)
		passada(w, d, passadaForaDaVez+1)
		if gens[arenaFila].CurrentNumMob != baseFila || gens[arenaRelogio].CurrentNumMob != baseRelogio {
			t.Errorf("um jogador dentro: %d e %d, quero a base %d e %d",
				gens[arenaFila].CurrentNumMob, gens[arenaRelogio].CurrentNumMob, baseFila, baseRelogio)
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
		passada(w, d, passadaForaDaVez)
		if gens[arenaFila].CurrentNumMob != 3*baseFila || gens[arenaRelogio].CurrentNumMob != 3*baseRelogio {
			t.Errorf("três dentro: %d e %d, quero %d e %d",
				gens[arenaFila].CurrentNumMob, gens[arenaRelogio].CurrentNumMob, 3*baseFila, 3*baseRelogio)
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
		passada(w, d, passadaForaDaVez+1)
		if gens[arenaFila].CurrentNumMob != 3*baseFila || gens[arenaRelogio].CurrentNumMob != 3*baseRelogio {
			t.Errorf("a passada depois da saída mexeu nos vivos: %d e %d, quero %d e %d",
				gens[arenaFila].CurrentNumMob, gens[arenaRelogio].CurrentNumMob, 3*baseFila, 3*baseRelogio)
		}

		// Morre quase tudo — o que sobra fica abaixo da base de um jogador; a fila de
		// 15 s não traz nenhum.
		mortesFila, mortesRelogio := 3*baseFila-baseFila/2, 3*baseRelogio-baseRelogio/2
		if mataDoBloco(w, arenaFila, mortesFila) != mortesFila || mataDoBloco(w, arenaRelogio, mortesRelogio) != mortesRelogio {
			t.Error("não achei os monstros para matar")
			return
		}
		if ids := w.SpawnDueRespawns(w.Now() + 60_000); len(ids) != 0 {
			t.Errorf("a fila de 15 s repôs %d monstros da arena", len(ids))
		}
		passada(w, d, passadaForaDaVez+2)
		if gens[arenaFila].CurrentNumMob != baseFila || gens[arenaRelogio].CurrentNumMob != baseRelogio {
			t.Errorf("reposição com um jogador: %d e %d, quero a base %d e %d",
				gens[arenaFila].CurrentNumMob, gens[arenaRelogio].CurrentNumMob, baseFila, baseRelogio)
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
			t.Errorf("fora da arena: relógio %d e fila %d, quero o teto do conteúdo 2 e 2",
				gens[foraRelogio].CurrentNumMob, gens[foraFila].CurrentNumMob)
		}
		if gens[arenaFila].CurrentNumMob != 2*baseFila {
			t.Errorf("na arena com dois: %d, quero %d (a conta do teste está certa?)", gens[arenaFila].CurrentNumMob, 2*baseFila)
		}

		mataDoBloco(w, foraFila, 1)
		if ids := w.SpawnDueRespawns(w.Now() + 60_000); len(ids) != 1 || gens[foraFila].CurrentNumMob != 2 {
			t.Errorf("o bloco de fora não voltou pela fila: %d renascidos, contagem %d", len(ids), gens[foraFila].CurrentNumMob)
		}

		// O relógio comum não repõe o bloco de relógio da arena, nem na vez dele.
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
	if cargaMaxArena != 270 {
		t.Fatalf("carga máxima = %d, quero 270", cargaMaxArena)
	}
	d := New(Config{})
	d.popBaseDasArenas = [len(baseDaArenaDecimos)]int{90, 90, 45, 51, 45}
	casos := []struct {
		passo, jogadores, quero int
	}{
		{0, 0, 1}, {0, 1, 1}, {0, 3, 3}, {0, 10, 3}, // Coveiro: 270/90 = 3
		{1, 4, 3}, {1, 6, 3}, // Jardim: 270/90 = 3
		{2, 6, 6}, {2, 7, 6}, // Kaizen: 270/45 = 6
		{3, 5, 5}, {3, 6, 5}, // Hidras: 270/51 = 5
	}
	for _, cs := range casos {
		if got := d.multiplicadorDaArena(cs.passo, cs.jogadores); got != cs.quero {
			t.Errorf("arena %d com %d jogadores: %dx, quero %dx", cs.passo, cs.jogadores, got, cs.quero)
		}
	}
}

// TestDistribuiPopulacao: o alvo repartido pelos blocos como o modelo reparte, em
// proporção ao teto de hoje, pelos maiores restos.
func TestDistribuiPopulacao(t *testing.T) {
	casos := []struct {
		nome  string
		tetos []int
		alvo  int
		quero []int
	}{
		{"Jardim com 45", []int{3, 3, 3, 3, 3, 2, 2, 2, 2, 2}, 45, []int{5, 5, 5, 5, 5, 4, 4, 4, 4, 4}},
		{"Jardim com 90", []int{3, 3, 3, 3, 3, 2, 2, 2, 2, 2}, 90, []int{11, 11, 11, 11, 11, 7, 7, 7, 7, 7}},
		{"Elfos", []int{4, 4, 4, 4, 2, 2, 2}, 45, []int{9, 8, 8, 8, 4, 4, 4}},
		{"Kaizen", []int{2, 3, 3, 3, 3, 3, 3, 2, 2, 2, 2, 2, 2}, 45, []int{3, 4, 4, 4, 4, 4, 4, 3, 3, 3, 3, 3, 3}},
		{"alvo igual a hoje", []int{3, 2, 2}, 7, []int{3, 2, 2}},
		{"sem blocos", nil, 45, []int{}},
	}
	for _, cs := range casos {
		if got := distribuiPopulacao(cs.tetos, cs.alvo); !slices.Equal(got, cs.quero) {
			t.Errorf("%s: %v, quero %v", cs.nome, got, cs.quero)
		}
	}
}

// TestBlocosDasArenasNoConteudo: com o NPCGener de verdade, o boot resolve os
// mesmos blocos que o modelo mediu, e a população de um jogador fica no mínimo de
// cada arena (90 no Coveiro e no Jardim, 45 nas outras), ou acima quando o
// conteúdo já pede mais (Hidras 51).
func TestBlocosDasArenasNoConteudo(t *testing.T) {
	root := releaseDir(t)
	gens, err := npcgener.Load(filepath.Join(root, "TMsrv", "run", "NPCGener.txt"))
	if err != nil {
		t.Fatal(err)
	}
	d := New(Config{})
	w := world.New(world.Config{GridDim: 32}, d.log, nil, d.Handle)
	blocos := make([]*world.Generator, len(gens))
	for idx, g := range gens {
		if g.Leader == "" {
			continue
		}
		blocos[idx] = &world.Generator{Name: g.Leader, MinuteGenerate: g.MinuteGenerate, MaxNumMob: g.MaxNumMob,
			SegX: g.SegX, SegY: g.SegY, LeaderTmpl: []byte{0}}
	}
	w.RegisterGenerators(blocos)
	d.resolverBlocosDasArenas(w)

	var porArena [len(baseDaArenaDecimos)]int
	for _, b := range d.blocosDasArenas {
		porArena[b.passo]++
	}
	if quero := [...]int{18, 10, 13, 20, 7}; porArena != quero {
		t.Errorf("blocos por arena = %v, quero %v", porArena, quero)
	}
	if quero := [...]int{90, 90, 45, 51, 45}; d.popBaseDasArenas != quero {
		t.Errorf("população de um jogador por arena = %v, quero %v", d.popBaseDasArenas, quero)
	}
}
