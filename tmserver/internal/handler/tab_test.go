package handler

import (
	"net"
	"testing"

	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/protocol"
	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/world"
)

// tabDB põe dois personagens acima do nível do comando e um Mortal abaixo dele.
func tabDB() *fakeDB {
	db := newDB()
	db.accounts["third"] = &fakeAccount{id: 13, pass: "secret", chars: []world.CharSummary{{Slot: 0, Name: "HeroC", Class: 1, Level: 10}}}
	db.loads = map[int64]world.CharacterState{
		7:  {Slot: 0, Name: "Hero", X: 5, Y: 5, HP: 1000, MaxHP: 1000, Level: 80},
		11: {Slot: 0, Name: "HeroB", X: 5, Y: 5, HP: 1000, MaxHP: 1000, Level: 80},
		13: {Slot: 0, Name: "HeroC", X: 5, Y: 5, HP: 1000, MaxHP: 1000, Level: 10},
	}
	return db
}

func tabFrame(t *testing.T, c net.Conn, texto string) {
	t.Helper()
	whisperFrame(t, c, "tab", texto)
}

// tabDoCreateMob lê o próximo CreateMob e devolve o campo Tab[26] dele.
func tabDoCreateMob(t *testing.T, c net.Conn) string {
	t.Helper()
	for {
		ty, payload, ok := readMaybeRaw(t, c)
		if !ok {
			t.Fatal("nenhum CreateMob chegou — o /tab não redesenhou o personagem")
		}
		if ty != protocol.MsgCreateMob {
			continue
		}
		// Tab[26] @abs202 → body190 (protocol/createmob.go).
		return cstr(payload[190:216])
	}
}

// TestTabApareceParaQuemVe é o defeito relatado: o comando não existia, então
// "/tab oi" virava um sussurro para um jogador chamado "tab" e o servidor
// respondia "O jogador não está conectado.". E mesmo com o comando o texto não
// apareceria — o campo Tab[26] do CreateMob nunca era escrito.
func TestTabApareceParaQuemVe(t *testing.T) {
	addr, stop, _ := startServerClock(t, tabDB())
	defer stop()
	a := enterWorldAs(t, addr, "tester")
	defer a.Close()
	b := enterWorldAs(t, addr, "tradeb")
	defer b.Close()
	drainRaw(t, a)
	drainRaw(t, b)

	tabFrame(t, a, "Vendo tudo")

	if got := tabDoCreateMob(t, a); got != "Vendo tudo" {
		t.Errorf("tab na tela de quem escreveu = %q, queria \"Vendo tudo\"", got)
	}
	if got := tabDoCreateMob(t, b); got != "Vendo tudo" {
		t.Errorf("tab na tela de quem está em volta = %q, queria \"Vendo tudo\"", got)
	}
}

// TestTabNaoPassaDeVinteECincoBytes: o legado copia 26 bytes contando o NUL, e
// um texto maior que isso atravessaria o campo seguinte do pacote.
func TestTabNaoPassaDeVinteECincoBytes(t *testing.T) {
	addr, stop, _ := startServerClock(t, tabDB())
	defer stop()
	a := enterWorldAs(t, addr, "tester")
	defer a.Close()
	drainRaw(t, a)

	longo := "123456789012345678901234567890" // 30
	tabFrame(t, a, longo)

	got := tabDoCreateMob(t, a)
	if len(got) != 25 {
		t.Errorf("tab = %q (%d bytes), queria 25", got, len(got))
	}
	if got != longo[:25] {
		t.Errorf("tab = %q, queria %q", got, longo[:25])
	}
}

// TestTabAcentoChegaInteiro: o texto é digitado no cliente e chega em CP1252.
// Tratá-lo como uma string Go o faria voltar cheio de "?" (protocol.ClientText).
func TestTabAcentoChegaInteiro(t *testing.T) {
	addr, stop, _ := startServerClock(t, tabDB())
	defer stop()
	a := enterWorldAs(t, addr, "tester")
	defer a.Close()
	drainRaw(t, a)

	// "Preço" como o cliente manda: ç = 0xE7 em CP1252.
	cp1252 := []byte{'P', 'r', 'e', 0xE7, 'o'}
	var body protocol.MsgWhisperBody
	copy(body.MobName[:], "tab")
	body.String = cp1252
	send(t, a, protocol.MsgMessageWhisper, body.Encode())

	got := tabDoCreateMob(t, a)
	if got != string(cp1252) {
		t.Errorf("tab = % x, queria % x (os bytes do cliente, sem reconversão)", got, cp1252)
	}
}

// TestTabVazioApagaALinha: "/tab" sozinho limpa, que é o que o strncpy de uma
// string vazia faz no legado.
func TestTabVazioApagaALinha(t *testing.T) {
	addr, stop, _ := startServerClock(t, tabDB())
	defer stop()
	a := enterWorldAs(t, addr, "tester")
	defer a.Close()
	drainRaw(t, a)

	tabFrame(t, a, "alguma coisa")
	if got := tabDoCreateMob(t, a); got != "alguma coisa" {
		t.Fatalf("tab inicial = %q", got)
	}
	drainRaw(t, a)

	tabFrame(t, a, "")
	if got := tabDoCreateMob(t, a); got != "" {
		t.Errorf("tab depois de apagar = %q, queria vazio", got)
	}
}

// TestTabExigeNivel: um Mortal abaixo do nível é recusado com o aviso, e a
// linha dele não muda.
func TestTabExigeNivel(t *testing.T) {
	addr, stop, _ := startServerClock(t, tabDB())
	defer stop()
	c := enterWorldAs(t, addr, "third") // HeroC, Mortal nível 10
	defer c.Close()
	drainRaw(t, c)

	tabFrame(t, c, "eu tentei")

	ty, payload, ok := readMaybe(t, c)
	if !ok || ty != protocol.MsgMessageBoxOk {
		t.Fatalf("recebeu %#x ok=%v, queria o aviso de nível", ty, ok)
	}
	if got := noticeCode(t, payload); got != NoticeLevelLimit {
		t.Errorf("aviso = %d, queria NoticeLevelLimit (%d)", got, NoticeLevelLimit)
	}
	if ty, _, ok := readMaybeRaw(t, c); ok && ty == protocol.MsgCreateMob {
		t.Error("o personagem foi redesenhado mesmo com o comando recusado")
	}
}

// TestTabNaoViraSussurro: o comando tem de ser interceptado ANTES da entrega do
// sussurro, ou "/tab" volta a ser um sussurro para um jogador chamado "tab" —
// e a resposta é "O jogador não está conectado.", que foi o que apareceu na tela.
func TestTabNaoViraSussurro(t *testing.T) {
	addr, stop, _ := startServerClock(t, tabDB())
	defer stop()
	a := enterWorldAs(t, addr, "tester")
	defer a.Close()
	drainRaw(t, a)

	tabFrame(t, a, "oi")

	for {
		ty, payload, ok := readMaybeRaw(t, a)
		if !ok {
			return
		}
		if ty == protocol.MsgMessageBoxOk && noticeCode(t, payload) == NoticeNotConnected {
			t.Fatal("/tab respondeu \"não está conectado\": voltou a ser tratado como sussurro")
		}
	}
}
