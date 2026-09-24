package handler

import (
	"context"
	"io"
	"log/slog"
	"net"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/world"
	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/worldevents"
)

// coliseuGeradores são os seis blocos das ondas no ponto do legado (2635,1726),
// com grupos pequenos para o teste contar.
func coliseuGeradores() []*world.Generator {
	gens := make([]*world.Generator, 8)
	for _, b := range []int{0, 1, 2, 5, 6, 7} {
		gens[b] = &world.Generator{
			Name: "Onda", MinuteGenerate: -1, MaxNumMob: 100, MinGroup: 1, MaxGroup: 2,
			SegX: [5]int16{2635}, SegY: [5]int16{1726}, SegRange: [5]int16{5}, // StartRange 5, como no NPCGener
			LeaderTmpl: plainMobTemplate("Ciclope"), FollowerTmpl: plainMobTemplate("Ciclope"),
		}
	}
	return gens
}

func semeiaPortoesDoColiseu(t *testing.T, w *world.World) {
	t.Helper()
	for _, p := range coliseuPortoes {
		if w.SeedWorldItem(world.Item{Index: p.item}, p.x, p.y, world.StateOpen) < 0 {
			t.Fatalf("não semeei o portão %d em (%d,%d)", p.item, p.x, p.y)
		}
	}
}

// coliseuMundo é o mundo sem socket, com o relógio na mão do teste.
func coliseuMundo(t *testing.T, agora *time.Time) (*Dispatcher, *world.World) {
	t.Helper()
	log := slog.New(slog.DiscardHandler)
	d := New(Config{Log: log, Now: func() time.Time { return *agora }})
	w := world.New(world.Config{GridDim: world.DefaultGridDim}, log, nil, d.Handle)
	semeiaPortoesDoColiseu(t, w)
	w.RegisterGenerators(coliseuGeradores())
	return d, w
}

// passaAte anda o relógio de segundo em segundo até ate, um tique por segundo.
func passaAte(d *Dispatcher, w *world.World, agora *time.Time, ate time.Time) {
	for agora.Before(ate) {
		*agora = agora.Add(time.Second)
		d.tickCount++
		d.tickColiseu(w)
	}
}

// estadoDosPortoes devolve o estado da entrada e dos de dentro, exigindo que
// cada grupo esteja num estado só.
func estadoDosPortoes(t *testing.T, d *Dispatcher, w *world.World) (entrada, internos int16) {
	t.Helper()
	entrada, internos = -1, -1
	for i, id := range d.portoesDoColiseuIDs(w) {
		g := w.GroundItem(id)
		if g == nil {
			t.Fatalf("portão %d não achado", i)
		}
		alvo := &entrada
		if coliseuPortoes[i].interno {
			alvo = &internos
		}
		if *alvo != -1 && *alvo != g.State {
			t.Fatalf("portões do mesmo grupo em estados diferentes: %d e %d", *alvo, g.State)
		}
		*alvo = g.State
	}
	return entrada, internos
}

func vivosDasOndas(w *world.World) map[int16]int {
	n := map[int16]int{}
	w.ForEachMob(func(_ int, e *world.Entity) {
		if e.HP > 0 && blocoDasOndas(int(e.GenIndex)) {
			n[e.GenIndex]++
		}
	})
	return n
}

// Desligado, o Coliseu não toca a arena nem às 20h: portões abertos, nenhuma
// onda, nenhum anonimato, nenhuma regra de passo.
func TestColiseuDesligadoNaoTocaAArena(t *testing.T) {
	agora := time.Date(2026, time.September, 24, 19, 59, 0, 0, time.Local)
	d, w := coliseuMundo(t, &agora)
	d.coliseu.itemBatalha = 413
	passaAte(d, w, &agora, agora.Add(20*time.Minute))

	if e, i := estadoDosPortoes(t, d, w); e != world.StateOpen || i != world.StateOpen {
		t.Errorf("portões %d/%d desligado, want abertos (%d)", e, i, world.StateOpen)
	}
	if n := vivosDasOndas(w); len(n) != 0 {
		t.Errorf("ondas desligado: %v", n)
	}
	if w.Anonimo(2615, 1725) || d.batalhaAtiva() {
		t.Error("anonimato ou Batalha Real com o Coliseu desligado")
	}
	if d.coliseu.ondas.Fase() != worldevents.ColiseuParado {
		t.Errorf("fase %d desligado, want parado", d.coliseu.ondas.Fase())
	}
}

// A noite das 20h inteira, de segundo em segundo, com as horas do legado.
func TestColiseuNoiteDasVinteNoMundo(t *testing.T) {
	agora := time.Date(2026, time.September, 24, 19, 59, 30, 0, time.Local)
	d, w := coliseuMundo(t, &agora)
	d.ligarColiseu(w)
	if e, i := estadoDosPortoes(t, d, w); e != world.StateOpen || i != world.StateLocked {
		t.Fatalf("ao ligar: entrada %d, internos %d; want aberta e trancados", e, i)
	}

	base := time.Date(2026, time.September, 24, 20, 0, 0, 0, time.Local)
	passaAte(d, w, &agora, base.Add(3*time.Minute+12*time.Second))
	if e, i := estadoDosPortoes(t, d, w); e != world.StateLocked || i != world.StateLocked {
		t.Errorf("20:03: entrada %d, internos %d; want os dois trancados", e, i)
	}
	if !d.coliseu.ondas.Limite150() {
		t.Error("20:03: a hora de novato é a mesma e o limite 150 não ligou")
	}

	passaAte(d, w, &agora, base.Add(4*time.Minute+12*time.Second))
	onda1 := vivosDasOndas(w)
	// De 4 a 7 grupos de 2 a 3 (líder + 1 a 2) no bloco 0, e só nele.
	if n := onda1[0]; n < 8 || n > 21 || len(onda1) != 1 {
		t.Errorf("20:04: %v; want só o bloco 0, com 8 a 21", onda1)
	}

	passaAte(d, w, &agora, base.Add(5*time.Minute+12*time.Second))
	if _, i := estadoDosPortoes(t, d, w); i != world.StateOpen {
		t.Errorf("20:05: internos %d, want abertos", i)
	}

	passaAte(d, w, &agora, base.Add(13*time.Minute+12*time.Second))
	ondas := vivosDasOndas(w)
	if ondas[0] == 0 || ondas[1] == 0 || ondas[2] == 0 || ondas[5]+ondas[6]+ondas[7] != 0 {
		t.Errorf("20:13: %v; want Ciclopes dos três blocos e nenhum Orc", ondas)
	}

	passaAte(d, w, &agora, base.Add(15*time.Minute+12*time.Second))
	if n := vivosDasOndas(w); len(n) != 0 {
		t.Errorf("20:15: sobraram %v", n)
	}
	if e, i := estadoDosPortoes(t, d, w); e != world.StateOpen || i != world.StateLocked {
		t.Errorf("20:15: entrada %d, internos %d; want aberta e trancados", e, i)
	}
	if d.coliseu.ondas.Limite150() {
		t.Error("20:15: o limite 150 ficou ligado")
	}
}

// Desligar no meio do evento devolve a arena ao que ela era.
func TestColiseuDesligarNoMeio(t *testing.T) {
	agora := time.Date(2026, time.September, 24, 20, 0, 0, 0, time.Local)
	d, w := coliseuMundo(t, &agora)
	d.ligarColiseu(w)
	passaAte(d, w, &agora, agora.Add(6*time.Minute))
	if len(vivosDasOndas(w)) == 0 {
		t.Fatal("nenhuma onda até 20:06")
	}
	d.desligarColiseu(w)
	if n := vivosDasOndas(w); len(n) != 0 {
		t.Errorf("desligado: sobraram %v", n)
	}
	if e, i := estadoDosPortoes(t, d, w); e != world.StateOpen || i != world.StateOpen {
		t.Errorf("desligado: portões %d/%d, want abertos", e, i)
	}
	if d.coliseu.ondas.Fase() != worldevents.ColiseuParado || d.coliseu.ondas.Limite150() {
		t.Error("desligado com fase ou limite de pé")
	}
}

// Na arena da Batalha Real todo mundo é "??????", sem capa e sem guilda, e
// ninguém protege a guilda com skill; fora da caixa nada muda.
func TestBatalhaRealEscondeQuemEstaNaArena(t *testing.T) {
	log := slog.New(slog.DiscardHandler)
	w := world.New(world.Config{GridDim: world.DefaultGridDim}, log, nil, nil)
	dentro := &world.Entity{ID: 3, Name: "Fulano", Guild: 77, X: 2615, Y: 1725}
	dentro.EquipVisual[capeEquipSlot], dentro.EquipAnct[capeEquipSlot] = 543, 9
	fora := &world.Entity{ID: 4, Name: "Beltrano", Guild: 77, X: 2600, Y: 1725}

	b := coliseuArena
	w.SetAnonimato(true, b.x1, b.y1, b.x2, b.y2)
	got := createMobFrom(w, dentro, 0)
	if got.Name != nomeAnonimo || got.Guild != 0 || got.Equip[capeEquipSlot] != 0 || got.AnctCode[capeEquipSlot] != 0 {
		t.Errorf("na arena: nome %q guilda %d capa %d/%d", got.Name, got.Guild, got.Equip[capeEquipSlot], got.AnctCode[capeEquipSlot])
	}
	if got := createMobFrom(w, fora, 0); got.Name != "Beltrano" || got.Guild != 77 {
		t.Errorf("fora da arena: nome %q guilda %d", got.Name, got.Guild)
	}
	aliado := &world.Entity{ID: 5, Guild: 77, X: 2616, Y: 1725}
	if skillSameLeaderOrGuild(w, dentro, aliado) {
		t.Error("na Batalha Real a guilda ainda protege")
	}
	w.SetAnonimato(false, 0, 0, 0, 0)
	if !skillSameLeaderOrGuild(w, dentro, aliado) {
		t.Error("sem Batalha Real a guilda deixou de proteger")
	}
}

// O drop do Coliseu {N}: um em 14 para cada item, só dos Fantasmas 4623-4634 e
// só com o Coliseu ligado.
func TestColiseuNDropDosFantasmas(t *testing.T) {
	d, w, killer := mobKilledWorld(t)
	contar := func(bloco int16) map[int16]int {
		n := map[int16]int{}
		for range 700 {
			killer.Carry = [world.MaxCarry]world.Item{}
			d.coliseuNMorto(w, killer, &world.Entity{GenIndex: bloco})
			for _, it := range killer.Carry {
				if !it.Empty() {
					n[it.Index]++
				}
			}
		}
		return n
	}
	if n := contar(4623); len(n) != 0 {
		t.Fatalf("desligado: %v", n)
	}
	d.coliseu.ligado = true
	n := contar(4630)
	total := 0
	for idx, q := range n {
		if idx != 419 && idx != 420 && idx != 4026 {
			t.Errorf("item %d fora do drop do Coliseu {N}", idx)
		}
		total += q
	}
	// 700 × 3/14 = 150; a faixa larga só pega um sorteio quebrado.
	if total < 100 || total > 200 {
		t.Errorf("%d drops em 700 abates, want perto de 150 (%v)", total, n)
	}
	if n := contar(4635); len(n) != 0 {
		t.Errorf("bloco 4635 deu drop: %v", n)
	}
}

// servidorDoColiseu é o servidor por socket com a arena e o relógio de parede
// na mão do teste. Não tem tique: o teste anda o Coliseu pelo noLaco.
type servidorDoColiseu struct {
	addr  string
	d     *Dispatcher
	w     *world.World
	agora atomic.Int64
}

func startServerColiseu(t *testing.T, loads map[int64]world.CharacterState) *servidorDoColiseu {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	srv := &servidorDoColiseu{addr: ln.Addr().String()}
	srv.agora.Store(time.Date(2026, time.September, 24, 15, 0, 0, 0, time.Local).UnixNano())
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	srv.d = New(Config{Log: log, Now: func() time.Time { return time.Unix(0, srv.agora.Load()) }})
	db := gmDB()
	for id, st := range loads {
		db.loads[id] = st
	}
	srv.w = world.New(world.Config{GridDim: world.DefaultGridDim}, log, db, srv.d.Handle)
	semeiaPortoesDoColiseu(t, srv.w)
	srv.w.RegisterGenerators(coliseuGeradores())
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { _ = srv.w.Serve(ctx, ln); close(done) }()
	t.Cleanup(func() {
		cancel()
		select {
		case <-done:
		case <-time.After(2 * time.Second):
			t.Error("o servidor não parou")
		}
	})
	return srv
}

// entraEDrena entra com a conta e descarta tudo o que chegar: o teste olha o
// mundo pelo noLaco, e uma conexão não lida derruba a sessão quando a fila
// enche.
func entraEDrena(t *testing.T, addr, conta string) {
	t.Helper()
	c := enterWorldAs(t, addr, conta)
	t.Cleanup(func() { _ = c.Close() })
	go func() { _, _ = io.Copy(io.Discard, c) }()
}

// A hora de novato expulsa quem tem 150 ou mais e barra a volta, e deixa quem
// está abaixo (Server.cpp:7052-7057, _MSG_Action.cpp:326-337).
func TestColiseuLimiteDeNovato(t *testing.T) {
	srv := startServerColiseu(t, map[int64]world.CharacterState{
		22: {Slot: 0, Name: "Player", Level: 200, X: 2615, Y: 1725, HP: 1000, MaxHP: 1000},
		23: {Slot: 0, Name: "Victim", Level: 100, X: 2616, Y: 1726, HP: 1000, MaxHP: 1000},
	})
	entraEDrena(t, srv.addr, "player")
	entraEDrena(t, srv.addr, "victim")

	noLaco(t, srv.w, func(w *world.World) {
		d := srv.d
		d.ligarColiseu(w)
		d.coliseu.ondas.Forcar(d.now())
		d.passoDoColiseu(w, d.now())
	})
	var alto, baixo [2]int16
	noLaco(t, srv.w, func(w *world.World) {
		w.ForEachPlaying(-1, func(_ *world.Session, e *world.Entity) {
			switch e.Name {
			case "Player":
				alto = [2]int16{e.X, e.Y}
			case "Victim":
				baixo = [2]int16{e.X, e.Y}
			}
		})
	})
	if coliseuArena.contains(alto[0], alto[1]) {
		t.Errorf("nível 200 continua na arena em %v", alto)
	}
	if !coliseuArena.contains(baixo[0], baixo[1]) {
		t.Errorf("nível 100 foi tirado da arena para %v", baixo)
	}

	// Com o limite ligado, quem sobe para 150 dentro da arena sai no próximo
	// passo, e quem volta andando também.
	noLaco(t, srv.w, func(w *world.World) {
		w.ForEachPlaying(-1, func(s *world.Session, e *world.Entity) {
			switch e.Name {
			case "Victim":
				e.Level = 150
				srv.d.coliseuDepoisDoPasso(w, s, e)
				baixo = [2]int16{e.X, e.Y}
			case "Player":
				srv.d.doTeleport(w, s, 2620, 1725)
				srv.d.coliseuDepoisDoPasso(w, s, e)
				alto = [2]int16{e.X, e.Y}
			}
		})
	})
	if coliseuArena.contains(baixo[0], baixo[1]) || coliseuArena.contains(alto[0], alto[1]) {
		t.Errorf("com o limite ligado ficaram na arena: nível 150 em %v, nível 200 em %v", baixo, alto)
	}
}

// Uma rodada inteira da Batalha Real, forçada na rodada 2 (qualquer nível).
func TestBatalhaRealRodadaInteira(t *testing.T) {
	srv := startServerColiseu(t, map[int64]world.CharacterState{
		// Na faixa estreita do veneno (2608-2622 × 1708-1711).
		22: {Slot: 0, Name: "Player", Level: 50, X: 2615, Y: 1710, HP: 5000, MaxHP: 5000},
		// No meio da arena, longe do veneno.
		23: {Slot: 0, Name: "Victim", Level: 50, X: 2630, Y: 1726, HP: 5000, MaxHP: 5000},
	})
	entraEDrena(t, srv.addr, "player")
	entraEDrena(t, srv.addr, "victim")
	inicio := time.Unix(0, srv.agora.Load())
	passo := func(minuto int) {
		noLaco(t, srv.w, func(w *world.World) {
			srv.d.passoDaBatalha(w, inicio.Add(time.Duration(minuto)*time.Minute))
		})
	}
	jogadores := func(fn func(*world.Session, *world.Entity)) {
		noLaco(t, srv.w, func(w *world.World) { w.ForEachPlaying(-1, fn) })
	}

	noLaco(t, srv.w, func(w *world.World) {
		d := srv.d
		d.ligarColiseu(w)
		d.coliseu.itemBatalha = 413
		if !d.coliseu.batalha.Forcar(inicio, 2) {
			t.Error("a rodada não abriu")
		}
		d.passoDaBatalha(w, inicio)
	})
	jogadores(func(_ *world.Session, e *world.Entity) {
		if e.Name != "Player" {
			return
		}
		if got := createMobFrom(srv.w, e, 0).Name; got != nomeAnonimo {
			t.Errorf("pronta: nome na arena %q, want %q", got, nomeAnonimo)
		}
		if !srv.d.batalhaTiraGrupo(e) {
			t.Error("pronta: o grupo continua valendo na arena")
		}
		if got := string(srv.d.falaDaBatalha(e, []byte("bom dia a todos"))); got != "??????a a todos" {
			t.Errorf("pronta: fala %q", got)
		}
	})

	passo(5)
	noLaco(t, srv.w, func(w *world.World) {
		if e, _ := estadoDosPortoes(t, srv.d, w); e != world.StateLocked {
			t.Errorf("luta: entrada %d, want trancada", e)
		}
	})

	passo(9)
	noLaco(t, srv.w, func(w *world.World) { srv.d.venenoDaBatalha(w) })
	jogadores(func(_ *world.Session, e *world.Entity) {
		want := int32(5000)
		if e.Name == "Player" {
			want = 3000
		}
		if e.HP != want {
			t.Errorf("veneno: %s com %d de vida, want %d", e.Name, e.HP, want)
		}
	})

	passo(13)
	noLaco(t, srv.w, func(w *world.World) {
		premio := false
		for dx := int16(-1); dx <= 1; dx++ {
			for dy := int16(-1); dy <= 1; dy++ {
				if g := w.GroundItemAt(batalhaPremioX+dx, batalhaPremioY+dy); g != nil && g.Item.Index == 413 {
					premio = true
				}
			}
		}
		if !premio {
			t.Error("prêmio: nada no altar")
		}
	})

	passo(14)
	noLaco(t, srv.w, func(w *world.World) {
		if e, _ := estadoDosPortoes(t, srv.d, w); e != world.StateOpen {
			t.Errorf("reabre: entrada %d, want aberta", e)
		}
		if w.Anonimo(2615, 1710) || srv.d.batalhaAtiva() {
			t.Error("reabre: a arena continua anônima")
		}
	})
}

// O comando de GM: estado, recusa desligado, liga e desliga.
func TestGMColiseu(t *testing.T) {
	srv := startServerColiseu(t, nil)
	mod := enterWorldAs(t, srv.addr, "mod")
	defer mod.Close()

	passos := []struct{ linha, quer string }{
		{"coliseu estado", "Coliseu: desligado."},
		{"coliseu iniciar", "O Coliseu está desligado; use /gm coliseu ligar."},
		{"coliseu ligar", "Coliseu: ligado; ondas às 20:00 (novato 20:00)"},
		{"coliseu batalha", "A Batalha Real precisa de um prêmio"},
		{"coliseu horas 18 21", "ondas às 18:00 (novato 21:00)"},
		{"coliseu premio 413", "Batalha Real às 19:00, prêmio 413"},
		{"coliseu iniciar", "(GM)"},
		{"coliseu fim", "fase 0"},
		{"coliseu desligar", "Coliseu: desligado."},
	}
	for _, p := range passos {
		gmFrame(t, mod, p.linha)
		if _, ok := panelWith(t, mod, p.quer); !ok {
			t.Fatalf("/gm %s: não veio %q", p.linha, p.quer)
		}
	}
}
