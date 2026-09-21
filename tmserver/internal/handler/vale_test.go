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

// startServerVale sobe um mundo com a grade inteira — o piso de Azran fica em
// 2548,1740 e o Vale em 2281,3688, longe das grades pequenas dos outros testes —
// com o jogador já em cima do piso. Nível 330 porque ficha sem nível é lida como
// personagem recém-criado.
func startServerVale(t *testing.T, db world.Persistence) (string, func(), *world.World) {
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

// fichaNoPisoDoVale é a ficha de quem está em cima do piso de Azran com a fada
// que o teste quiser no slot 13. fada 0 = slot vazio.
func fichaNoPisoDoVale(fada int16) world.CharacterState {
	st := world.CharacterState{
		Slot: 0, Name: "Heroi", Level: 330,
		X: valePisoX, Y: valePisoY,
		HP: 1000, MaxHP: 1000,
	}
	if fada != 0 {
		st.Equip[fairyEquipSlot] = world.Item{Index: fada}
	}
	return st
}

// O piso de Azran é um BLOCO DE 4x4, não uma casa: o legado arredonda a posição
// para baixo até o múltiplo de 4 antes de comparar (GetFunc.cpp:784-785). Tratar
// como casa única deixa o jogador pisando no portal sem nada acontecer em 15 das
// 16 casas.
func TestPisoDoValeEUmBlocoDeQuatro(t *testing.T) {
	for x := int16(valePisoX); x < valePisoX+4; x++ {
		for y := int16(valePisoY); y < valePisoY+4; y++ {
			if !noPisoDoVale(x, y) {
				t.Errorf("(%d,%d) ficou fora do piso e está dentro do bloco", x, y)
			}
		}
	}
	fora := [][2]int16{
		{valePisoX - 1, valePisoY},
		{valePisoX + 4, valePisoY},
		{valePisoX, valePisoY - 1},
		{valePisoX, valePisoY + 4},
	}
	for _, p := range fora {
		if noPisoDoVale(p[0], p[1]) {
			t.Errorf("(%d,%d) entrou no piso e está fora do bloco", p[0], p[1])
		}
	}
}

// O destino espalha três casas em cada eixo, como toda rota do legado
// (`2281 + rand() % 3`, `3688 + rand() % 3`, GetFunc.cpp:928-929).
func TestDestinoDoValeEspalhaTresCasas(t *testing.T) {
	x, y := destinoDoVale(func(int) int { return 0 })
	if x != valeDestX || y != valeDestY {
		t.Errorf("sorteio zerado = (%d,%d), want (%d,%d)", x, y, valeDestX, valeDestY)
	}
	x, y = destinoDoVale(func(int) int { return 2 })
	if x != valeDestX+2 || y != valeDestY+2 {
		t.Errorf("sorteio no topo = (%d,%d), want (%d,%d)", x, y, valeDestX+2, valeDestY+2)
	}
}

// A rota do Vale NÃO pode estar na teleportTable. Enquanto esteve, a consulta
// pura respondia antes de qualquer condição e levava ao Vale quem não tinha fada
// nenhuma — o bug que este arquivo existe para fechar.
func TestRotaDoValeNaoEstaNaTabela(t *testing.T) {
	if _, _, _, ok := world.TeleportDest(valePisoX, valePisoY); ok {
		t.Error("o piso de Azran voltou para a teleportTable; a rota do Vale é condicional")
	}
}

// Sem fada nenhuma o piso não leva a lugar nenhum — e diz por quê. No legado a
// condição faz parte do teste da rota e a recusa é o silêncio; aqui ela fala,
// porque esta é a única porta do Vale e um piso mudo é indistinguível de um
// teleporte quebrado.
func TestValeSemFadaNaoLevaEAvisa(t *testing.T) {
	db := newDB()
	db.loadResult = fichaNoPisoDoVale(0)
	addr, stop, w := startServerVale(t, db)
	defer stop()
	c := enterWorld(t, addr)
	defer c.Close()

	send(t, c, protocol.MsgReqTeleport, nil)

	if got, want := esperarMensagem(t, c, "Fada do Vale"), noticeText[NoticeValeSemFada]; got != want {
		t.Errorf("aviso = %q, want %q", got, want)
	}
	x, y := posicaoNoLaco(t, w)
	if x != valePisoX || y != valePisoY {
		t.Errorf("saiu para (%d,%d) sem fada, want ficar em (%d,%d)", x, y, valePisoX, valePisoY)
	}
}

// Outra fada não abre o Vale: o legado compara o sIndex com 3916, e só com ele
// (GetFunc.cpp:926). A Fada Verde de 30 dias é a mais forte das outras e também
// fica de fora.
func TestValeComOutraFadaNaoLeva(t *testing.T) {
	db := newDB()
	db.loadResult = fichaNoPisoDoVale(3913) // Fada_Verde(30dias)
	addr, stop, w := startServerVale(t, db)
	defer stop()
	c := enterWorld(t, addr)
	defer c.Close()

	send(t, c, protocol.MsgReqTeleport, nil)

	if got, want := esperarMensagem(t, c, "Fada do Vale"), noticeText[NoticeValeSemFada]; got != want {
		t.Errorf("aviso = %q, want %q", got, want)
	}
	x, y := posicaoNoLaco(t, w)
	if x != valePisoX || y != valePisoY {
		t.Errorf("a Fada Verde abriu o Vale: foi para (%d,%d)", x, y)
	}
}

// Com a Fada do Vale no slot 13 a rota funciona como no legado.
func TestValeComFadaDoValeLeva(t *testing.T) {
	db := newDB()
	db.loadResult = fichaNoPisoDoVale(itemFadaDoVale)
	addr, stop, w := startServerVale(t, db)
	defer stop()
	c := enterWorld(t, addr)
	defer c.Close()

	send(t, c, protocol.MsgReqTeleport, nil)

	x, y := esperarPosicao(t, w, func(x, y int16) bool { return x != valePisoX || y != valePisoY })
	if !dentroDe(x, y, valeDestX, valeDestY) {
		t.Errorf("caiu em (%d,%d), want dentro de %d..%d × %d..%d",
			x, y, valeDestX, valeDestX+2, valeDestY, valeDestY+2)
	}
}

// esperarPosicao lê a posição do jogador até ela satisfazer cond, porque o
// teleporte acontece no laço e não no socket: sem a espera o teste lê a posição
// antiga e passa com o servidor quebrado.
func esperarPosicao(t *testing.T, w *world.World, cond func(x, y int16) bool) (int16, int16) {
	t.Helper()
	for i := 0; i < 40; i++ {
		x, y := posicaoNoLaco(t, w)
		if cond(x, y) {
			return x, y
		}
		time.Sleep(25 * time.Millisecond)
	}
	t.Fatal("o jogador não saiu do piso")
	return 0, 0
}
