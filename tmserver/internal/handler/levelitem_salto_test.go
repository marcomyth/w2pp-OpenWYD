package handler

import (
	"context"
	"io"
	"log/slog"
	"net"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jeanluca/w2pp-openwyd/internal/level"
	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/content"
	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/world"
)

// tabelaDeTeste é um LevelItem.txt de duas linhas com as peças que a Foema de
// construção Int recebe no 29 e no 34 — os mesmos itens e efeitos do arquivo de
// verdade (TYPE 2 = mais Int, pelo cabeçalho).
func tabelaDeTeste(t *testing.T) *content.LevelItems {
	t.Helper()
	arq := filepath.Join(t.TempDir(), "LevelItem.txt")
	texto := "0 29 1 2 1297 43 3 60 6 3 20\n1 34 1 2 1309 43 3 60 6 74 12\n"
	if err := os.WriteFile(arq, []byte(texto), 0o600); err != nil {
		t.Fatal(err)
	}
	tab, avisos, err := content.LoadLevelItems(arq)
	if err != nil || len(avisos) > 0 {
		t.Fatalf("a tabela de teste não carregou: err=%v avisos=%v", err, avisos)
	}
	return tab
}

var (
	pecaDo29 = world.Item{Index: 1297, Effects: [3]world.Effect{{Effect: 43, Value: 3}, {Effect: 60, Value: 6}, {Effect: 3, Value: 20}}}
	pecaDo34 = world.Item{Index: 1309, Effects: [3]world.Effect{{Effect: 43, Value: 3}, {Effect: 60, Value: 6}, {Effect: 74, Value: 12}}}
)

// startServerItemDeNivel sobe um mundo com a tabela de peças ligada e um jogador
// em jogo, e devolve o Dispatcher: a subida de nível é chamada DENTRO do laço do
// mundo (noLacoDoMundo), que é o único dono do estado.
func startServerItemDeNivel(t *testing.T) (*Dispatcher, *world.World, func()) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	clock := &atomic.Uint32{}
	clock.Store(serverTime)
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	d := New(Config{Log: log, Now: func() time.Time { return time.Unix(0, 0) }, LevelItems: tabelaDeTeste(t)})
	w := world.New(world.Config{GridDim: 16, Now: clock.Load}, log, novatoDB(), d.Handle)
	w.SetSessionEndHandler(d.SessionEnd)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { _ = w.Serve(ctx, ln); close(done) }()
	c := enterWorldAs(t, ln.Addr().String(), "tester")
	return d, w, func() {
		_ = c.Close()
		cancel()
		select {
		case <-done:
		case <-time.After(2 * time.Second):
			t.Error("o servidor não parou")
		}
	}
}

// comoFoemaInt deixa o jogador como Foema Mortal de construção Int, no nível nv
// com exp de XP total. Só pode ser chamada de dentro do laço do mundo.
func comoFoemaInt(e *world.Entity, nv int32, exp int64) {
	e.Class = 1
	e.ClassMaster = classMasterMortal
	e.BaseStr, e.BaseInt, e.BaseDex, e.BaseCon = 5, 200, 5, 5
	e.Level, e.Exp = nv, exp
}

// noJogador roda fn com a sessão e a entidade do único jogador em jogo, dentro do
// laço, e falha alto se não houver jogador — um teste que não achou ninguém não
// pode passar calado.
func noJogador(t *testing.T, w *world.World, fn func(*world.World, *world.Session, *world.Entity)) {
	t.Helper()
	achou := 0
	noLacoDoMundo(t, w, func(w *world.World) {
		w.ForEachPlaying(-1, func(s *world.Session, e *world.Entity) {
			achou++
			fn(w, s, e)
		})
	})
	if achou != 1 {
		t.Fatalf("jogadores em jogo = %d, queria 1", achou)
	}
}

func mesmaPeca(a, b world.Item) bool {
	return a.Index == b.Index && a.Effects == b.Effects
}

// TestSaltoDeNiveisEntregaAPecaDeCadaNivel: quem ganha XP para vários níveis de
// uma vez recebe a peça de CADA nível cruzado.
//
// O legado sobe um nível por chamada (CMob::CheckGetLevel, CMob.cpp:1113) e todo
// ponto que chama CheckGetLevel chama DoItemLevel logo depois, então no original
// quem tem XP para sete níveis sobe um por golpe e recebe cada peça. Aqui a subida
// é num laço, e a entrega vinha uma vez só, com o nível final: do 28 ao 35 não
// chegava nem a do 29 nem a do 34, e nenhuma linha de log dizia isso. Medido na
// cópia em 14/09/2026 antes deste teste existir.
func TestSaltoDeNiveisEntregaAPecaDeCadaNivel(t *testing.T) {
	d, w, stop := startServerItemDeNivel(t)
	defer stop()

	var nivel int32
	var armazem [world.MaxCargo]world.Item
	noJogador(t, w, func(w *world.World, s *world.Session, e *world.Entity) {
		comoFoemaInt(e, 28, level.NextLevelExp(34)) // XP exata para chegar ao 35
		d.applyLevelUps(w, s, e)
		nivel = e.Level
		if c := w.Cargo(s.AccountID); c != nil {
			armazem = c.Items
		}
	})

	if nivel != 35 {
		t.Fatalf("nível = %d, queria 35", nivel)
	}
	if !mesmaPeca(armazem[0], pecaDo29) {
		t.Errorf("vaga 0 = %+v, queria a peça do 29 (%+v): a peça do nível cruzado no meio do salto sumiu", armazem[0], pecaDo29)
	}
	if !mesmaPeca(armazem[1], pecaDo34) {
		t.Errorf("vaga 1 = %+v, queria a peça do 34 (%+v): a peça do nível cruzado no meio do salto sumiu", armazem[1], pecaDo34)
	}
	for vaga := 2; vaga < world.MaxCargo; vaga++ {
		if !armazem[vaga].Empty() {
			t.Errorf("vaga %d recebeu %+v; só o 29 e o 34 entregam", vaga, armazem[vaga])
		}
	}
}

// TestItemDeNivelNaoUsaAsDuasUltimasVagas: o DoItemLevel do legado procura vaga em
// i < MAX_CARGO - 2 (Server.cpp:9762). Com as vagas 0 a 125 ocupadas, o original
// não entrega — e aqui a peça ia para a 126.
func TestItemDeNivelNaoUsaAsDuasUltimasVagas(t *testing.T) {
	d, w, stop := startServerItemDeNivel(t)
	defer stop()

	var nivel int32
	var armazem [world.MaxCargo]world.Item
	noJogador(t, w, func(w *world.World, s *world.Session, e *world.Entity) {
		c := w.Cargo(s.AccountID)
		if c == nil {
			t.Error("a conta não tem armazém carregado")
			return
		}
		for i := 0; i < world.MaxCargo-2; i++ {
			c.Items[i] = world.Item{Index: 3173}
		}
		comoFoemaInt(e, 28, level.NextLevelExp(28)) // um nível só: 28 -> 29
		d.applyLevelUps(w, s, e)
		nivel = e.Level
		armazem = c.Items
	})

	if nivel != 29 {
		t.Fatalf("nível = %d, queria 29", nivel)
	}
	for _, vaga := range []int{world.MaxCargo - 2, world.MaxCargo - 1} {
		if !armazem[vaga].Empty() {
			t.Errorf("vaga %d recebeu %+v; o legado nunca entrega a peça nas duas últimas vagas", vaga, armazem[vaga])
		}
	}
}

// TestSetlevelDeGMNaoEntregaPeca: o /gm setlevel equivale ao "set exp" do legado
// (imple.cpp:154-159), que chama CheckGetLevel e não chama DoItemLevel. Subir um
// boneco de teste não pode encher o armazém dele de peças.
func TestSetlevelDeGMNaoEntregaPeca(t *testing.T) {
	d, w, stop := startServerItemDeNivel(t)
	defer stop()

	var nivel int32
	var armazem [world.MaxCargo]world.Item
	noJogador(t, w, func(w *world.World, s *world.Session, e *world.Entity) {
		comoFoemaInt(e, 28, level.NextLevelExp(27))
		d.gmSetLevel(w, s, "34")
		nivel = e.Level
		if c := w.Cargo(s.AccountID); c != nil {
			armazem = c.Items
		}
	})

	if nivel != 34 {
		t.Fatalf("nível = %d, queria 34", nivel)
	}
	for vaga, it := range armazem {
		if !it.Empty() {
			t.Errorf("vaga %d recebeu %+v pelo /gm setlevel; o legado não entrega peça por comando", vaga, it)
		}
	}
}
