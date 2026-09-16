package handler

import (
	"context"
	"encoding/binary"
	"io"
	"log/slog"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/world"
)

// sobreviventeTemplate copia a ficha REAL do porteiro do Kefra, byte por byte no
// que importa para o roteamento (conferido em Release/TMsrv/run/npc/Sobrevivente,
// fixado em TestTemplateRealDoSobrevivente):
//
//   - byte 17 (MOB.Merchant) = 100, que é por onde o legado roteia quest
//     (`npcMerc = pMob[npcIndex].MOB.Merchant`, _MSG_Quest.cpp:33);
//   - byte 104 (CurrentScore.Merchant) = 68, GODGOVERNMENT no legado;
//   - EF_GRADE0 = 22 no TERCEIRO efeito do Equip[0], que é SOBREVIVENTE
//     (_MSG_Quest.cpp:85-86).
//
// Os dois bytes diferentes são o problema todo: o Go roteia os NPCs de grau pelo
// 104, e 68 não é 100, então este NPC nunca casou com ramo nenhum e o clique nele
// não fazia nada.
func sobreviventeTemplate() []byte {
	b := expMobTemplate(80, 0, 0)
	copy(b[0:16], "Sobrevivente")
	b[17] = 100                               // MOB.Merchant, o byte do legado
	b[92+12] = 68                             // CurrentScore.Merchant, o byte que o mundo lê
	binary.LittleEndian.PutUint16(b[140:], 1) // Equip[0].sIndex
	b[142], b[143] = 43, 1                    // Effects[0]
	b[144], b[145] = 86, 13                   // Effects[1]
	b[146], b[147] = 100, 22                  // Effects[2] = EF_GRADE0 22
	return b
}

// startServerSobrevivente sobe um mundo com o porteiro do Kefra semeado e devolve
// o mundo, para ler o estado do jogador dentro do laço.
func startServerSobrevivente(t *testing.T, db world.Persistence) (string, func(), *world.World, int) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	d := New(Config{Log: log})
	w := world.New(world.Config{GridDim: 16}, log, db, d.Handle)
	npcID := w.SpawnMob(sobreviventeTemplate(), 5, 5)
	if npcID < 0 {
		t.Fatal("não consegui semear o Sobrevivente")
	}
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
	return ln.Addr().String(), stop, w, npcID
}

// entradasNoLaco lê as entradas do Kefra do único jogador em jogo.
func entradasNoLaco(t *testing.T, w *world.World) int32 {
	t.Helper()
	var entradas int32
	noLacoDoMundo(t, w, func(w *world.World) {
		w.ForEachPlaying(-1, func(_ *world.Session, e *world.Entity) {
			entradas = e.KefraTicket
		})
	})
	return entradas
}

// esperarSalvoComEntradas espera o save assíncrono do Pergaminho chegar ao banco.
func esperarSalvoComEntradas(t *testing.T, db *fakeDB, entradas int32) {
	t.Helper()
	prazo := time.Now().Add(2 * time.Second)
	for time.Now().Before(prazo) {
		db.mu.Lock()
		for _, s := range db.savedChars {
			if s.KefraTicket == entradas {
				db.mu.Unlock()
				return
			}
		}
		db.mu.Unlock()
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("nenhum save com %d entradas chegou ao banco", entradas)
}

// O Sobrevivente troca UM Pergaminho_Selado por 100 entradas no Hall
// (_MSG_Quest.cpp:2598-2623): limpa a casa da bolsa, soma 100 e diz quantas
// ficaram. O save vai na hora porque o pergaminho é comprado.
func TestSobreviventeTrocaOPergaminhoPorEntradas(t *testing.T) {
	db := newDB()
	st := baseMortalState(330)
	st.Carry[0] = world.Item{Index: itemPergaminhoSelado}
	db.loadResult = st
	addr, stop, w, npcID := startServerSobrevivente(t, db)
	defer stop()
	c := enterWorld(t, addr)
	defer c.Close()

	questFrame(t, c, npcID)

	if got, want := esperarMensagem(t, c, "vezes"), "Você pode usar isto 100 vezes."; got != want {
		t.Errorf("aviso = %q, want %q", got, want)
	}
	if entradas := entradasNoLaco(t, w); entradas != 100 {
		t.Errorf("entradas = %d, want 100", entradas)
	}
	if bolsa := bolsaDoJogador(t, w); bolsa[itemPergaminhoSelado].Index != 0 {
		t.Errorf("o Pergaminho ficou na bolsa: %+v", bolsa[itemPergaminhoSelado])
	}
	esperarSalvoComEntradas(t, db, 100)
}

// Sem o Pergaminho na bolsa o clique não dá nada e não fala nada: o legado só
// entra no ramo quando acha o item (`if (i != pMob[conn].MaxCarry)`).
func TestSobreviventeSemPergaminhoNaoDaEntradas(t *testing.T) {
	db := newDB()
	db.loadResult = baseMortalState(330)
	addr, stop, w, npcID := startServerSobrevivente(t, db)
	defer stop()
	c := enterWorld(t, addr)
	defer c.Close()

	questFrame(t, c, npcID)

	if ty, _, ok := readMaybe(t, c); ok {
		t.Fatalf("o servidor respondeu %#x a quem não tem o Pergaminho; o legado fica calado", ty)
	}
	if entradas := entradasNoLaco(t, w); entradas != 0 {
		t.Errorf("entradas = %d, want 0", entradas)
	}
}

// A ficha real do porteiro, fixada aqui porque é ela que explica o bug: o byte
// que o legado roteia (17) é 100, o que o mundo lê (104) é 68, e o grau 22 está
// no TERCEIRO efeito do Equip[0], não no primeiro.
func TestTemplateRealDoSobrevivente(t *testing.T) {
	b, err := os.ReadFile(filepath.Join("..", "..", "..", "Release", "TMsrv", "run", "npc", "Sobrevivente"))
	if err != nil {
		t.Fatalf("ler a ficha real do Sobrevivente: %v", err)
	}
	if len(b) != 816 {
		t.Fatalf("tamanho = %d, want 816", len(b))
	}
	if b[17] != 100 {
		t.Errorf("byte 17 (MOB.Merchant) = %d, want 100", b[17])
	}
	if b[104] != 68 {
		t.Errorf("byte 104 (CurrentScore.Merchant) = %d, want 68 (GODGOVERNMENT)", b[104])
	}
	if b[146] != 100 || b[147] != 22 {
		t.Errorf("Equip[0].Effects[2] = (%d,%d), want (100,22) = EF_GRADE0 22", b[146], b[147])
	}
}
