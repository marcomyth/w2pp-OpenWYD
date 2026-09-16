package handler

import (
	"context"
	"io"
	"log/slog"
	"net"
	"testing"
	"time"

	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/protocol"
	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/world"
)

// startServerHallDoKefra sobe um mundo com a grade inteira (o piso do Hall fica
// em 2364,3892, longe das grades pequenas dos outros testes) e o jogador já em
// cima do piso, na posição que a ficha manda. Nível 330 porque ficha sem nível é
// lida como personagem recém-criado.
func startServerHallDoKefra(t *testing.T, db world.Persistence) (string, func(), *world.World) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	d := New(Config{Log: log})
	w := world.New(world.Config{GridDim: 4096}, log, db, d.Handle)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { _ = w.Serve(ctx, ln); close(done) }()
	stop := func() {
		cancel()
		select {
		case <-done:
		case <-time.After(2 * time.Second):
			t.Error("o servidor não parou")
		}
	}
	return ln.Addr().String(), stop, w
}

// fichaNoPisoDoHall é a ficha de quem está em cima do piso do Hall com as
// entradas que o teste quiser.
func fichaNoPisoDoHall(entradas int32) world.CharacterState {
	return world.CharacterState{
		Slot: 0, Name: "Heroi", Level: 330,
		X: hallDoKefraPisoX, Y: hallDoKefraPisoY,
		HP: 1000, MaxHP: 1000,
		KefraTicket: entradas,
	}
}

// jogadorNoLaco devolve a posição e as entradas do único jogador em jogo, lidas
// dentro do laço do mundo.
func jogadorNoLaco(t *testing.T, w *world.World) (x, y int16, entradas int32) {
	t.Helper()
	noLacoDoMundo(t, w, func(w *world.World) {
		w.ForEachPlaying(-1, func(_ *world.Session, e *world.Entity) {
			x, y, entradas = e.X, e.Y, e.KefraTicket
		})
	})
	return x, y, entradas
}

// O piso do Hall é um BLOCO DE 4x4, não uma casa. O legado arredonda a posição
// para baixo até o múltiplo de 4 antes de comparar (`xv = (*x) & 0xFFFC`,
// GetFunc.cpp:784-785), e só então testa `xv == 2364 && yv == 3892`
// (GetFunc.cpp:994). Quem tratar isso como uma casa só deixa o jogador pisando
// no portal sem nada acontecer em 15 das 16 casas.
func TestPisoDoHallDoKefraEUmBlocoDeQuatro(t *testing.T) {
	for x := int16(hallDoKefraPisoX); x < hallDoKefraPisoX+4; x++ {
		for y := int16(hallDoKefraPisoY); y < hallDoKefraPisoY+4; y++ {
			if !noPisoDoHallDoKefra(x, y) {
				t.Errorf("(%d,%d) ficou fora do piso e está dentro do bloco", x, y)
			}
		}
	}
	fora := [][2]int16{
		{hallDoKefraPisoX - 1, hallDoKefraPisoY},
		{hallDoKefraPisoX + 4, hallDoKefraPisoY},
		{hallDoKefraPisoX, hallDoKefraPisoY - 1},
		{hallDoKefraPisoX, hallDoKefraPisoY + 4},
	}
	for _, p := range fora {
		if noPisoDoHallDoKefra(p[0], p[1]) {
			t.Errorf("(%d,%d) entrou no piso e está fora do bloco", p[0], p[1])
		}
	}
}

// O destino espalha três casas em cada eixo, como toda rota do legado
// (`2364 + rand() % 3`, `3906 + rand() % 3`).
func TestDestinoDoHallDoKefraEspalhaTresCasas(t *testing.T) {
	x, y := destinoDoHallDoKefra(func(int) int { return 0 })
	if x != hallDoKefraDestX || y != hallDoKefraDestY {
		t.Errorf("sorteio zerado = (%d,%d), want (%d,%d)", x, y, hallDoKefraDestX, hallDoKefraDestY)
	}
	x, y = destinoDoHallDoKefra(func(int) int { return 2 })
	if x != hallDoKefraDestX+2 || y != hallDoKefraDestY+2 {
		t.Errorf("sorteio no topo = (%d,%d), want (%d,%d)", x, y, hallDoKefraDestX+2, hallDoKefraDestY+2)
	}
}

// Pisar no piso com entrada no bolso gasta UMA, diz quantas restam e leva para
// dentro do Hall (GetFunc.cpp:994-1004).
func TestHallDoKefraGastaUmaEntradaELeva(t *testing.T) {
	db := newDB()
	db.loadResult = fichaNoPisoDoHall(3)
	addr, stop, w := startServerHallDoKefra(t, db)
	defer stop()
	c := enterWorld(t, addr)
	defer c.Close()

	send(t, c, protocol.MsgReqTeleport, nil)

	if got, want := esperarMensagem(t, c, "vezes"), "Você pode usar isto 2 vezes."; got != want {
		t.Errorf("aviso = %q, want %q", got, want)
	}
	x, y, entradas := jogadorNoLaco(t, w)
	if entradas != 2 {
		t.Errorf("entradas = %d, want 2 (gastou uma das três)", entradas)
	}
	if x < hallDoKefraDestX || x > hallDoKefraDestX+2 || y < hallDoKefraDestY || y > hallDoKefraDestY+2 {
		t.Errorf("caiu em (%d,%d), want dentro de %d..%d × %d..%d",
			x, y, hallDoKefraDestX, hallDoKefraDestX+2, hallDoKefraDestY, hallDoKefraDestY+2)
	}
}

// Sem entrada, nada acontece: no legado a condição do ticket faz parte do teste
// do piso, então a posição não casa com rota nenhuma e o jogador fica onde está.
// Nada de aviso, nada de teleporte, nada de entrada negativa.
func TestHallDoKefraSemEntradaNaoLeva(t *testing.T) {
	db := newDB()
	db.loadResult = fichaNoPisoDoHall(0)
	addr, stop, w := startServerHallDoKefra(t, db)
	defer stop()
	c := enterWorld(t, addr)
	defer c.Close()

	send(t, c, protocol.MsgReqTeleport, nil)

	if ty, _, ok := readMaybe(t, c); ok {
		t.Fatalf("o servidor respondeu %#x a quem não tem entrada; o legado fica calado", ty)
	}
	x, y, entradas := jogadorNoLaco(t, w)
	if entradas != 0 {
		t.Errorf("entradas = %d, want 0", entradas)
	}
	if x != hallDoKefraPisoX || y != hallDoKefraPisoY {
		t.Errorf("saiu para (%d,%d) sem entrada, want ficar em (%d,%d)",
			x, y, hallDoKefraPisoX, hallDoKefraPisoY)
	}
}
