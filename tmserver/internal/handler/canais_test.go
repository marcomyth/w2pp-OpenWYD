package handler

import (
	"net"
	"testing"

	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/protocol"
	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/world"
)

// canaisDB põe três personagens em jogo: Hero (conn 1) e HeroB (conn 2) na
// guilda 5 e no reino 1, e HeroC (conn 3) sem guilda e no reino 2. É o mínimo
// para provar que cada canal alcança QUEM DEVE e mais ninguém — um canal que
// entrega a todo mundo passa num teste de um destinatário só.
func canaisDB() *fakeDB {
	db := newDB()
	db.accounts["third"] = &fakeAccount{id: 13, pass: "secret", chars: []world.CharSummary{{Slot: 0, Name: "HeroC", Class: 1, Level: 50}}}
	db.loads = map[int64]world.CharacterState{
		7:  {Slot: 0, Name: "Hero", X: 5, Y: 5, HP: 1000, MaxHP: 1000, Level: 50, Clan: 1, GuildID: 5},
		11: {Slot: 0, Name: "HeroB", X: 5, Y: 5, HP: 1000, MaxHP: 1000, Level: 50, Clan: 1, GuildID: 5},
		13: {Slot: 0, Name: "HeroC", X: 5, Y: 5, HP: 1000, MaxHP: 1000, Level: 50, Clan: 2},
	}
	return db
}

// canalFrame fala num canal: é um sussurro de MobName VAZIO, que é como o
// cliente manda os quatro (canais.go).
func canalFrame(t *testing.T, c net.Conn, texto string) {
	t.Helper()
	whisperFrame(t, c, "", texto)
}

// esperaCanal lê a próxima linha de canal e devolve o nome de quem falou e o
// texto, já sem o preenchimento do corpo de tamanho fixo.
func esperaCanal(t *testing.T, c net.Conn) (id uint16, falante, texto string) {
	t.Helper()
	h, payload, ok := readMaybeHeader(t, c)
	if !ok || h.Type != protocol.MsgMessageWhisper {
		t.Fatalf("recebeu %#x ok=%v, queria MessageWhisper", h.Type, ok)
	}
	if len(payload) != protocol.WhisperChannelBodySize {
		t.Fatalf("corpo do canal = %d bytes, queria %d (o cliente lê o tamanho cheio)",
			len(payload), protocol.WhisperChannelBodySize)
	}
	return h.ID, cstr(payload[:16]), cstr(payload[16:])
}

// nadaChega falha se c receber qualquer coisa. É o que separa "o canal alcança
// quem deve" de "o canal alcança todo mundo".
func nadaChega(t *testing.T, c net.Conn, quem string) {
	t.Helper()
	if ty, _, ok := readMaybe(t, c); ok {
		t.Errorf("%s recebeu %#x e não devia receber nada", quem, ty)
	}
}

// TestCidadaoAlcancaOServidorInteiro é o defeito relatado: falar no canal
// global respondia "O jogador não está conectado." a quem falou, e ninguém
// ouvia nada. O pacote tem MobName vazio, e o port o levava para SessionByName.
func TestCidadaoAlcancaOServidorInteiro(t *testing.T) {
	addr, stop, _ := startServerClock(t, canaisDB())
	defer stop()
	a := enterWorldAs(t, addr, "tester")
	defer a.Close()
	b := enterWorldAs(t, addr, "tradeb")
	defer b.Close()
	c := enterWorldAs(t, addr, "third")
	defer c.Close()
	drainRaw(t, a)
	drainRaw(t, b)
	drainRaw(t, c)

	canalFrame(t, a, "@oi pessoal")

	// O Cidadão não olha guilda, reino nem distância: os dois recebem.
	for _, alvo := range []struct {
		conn net.Conn
		nome string
	}{{b, "HeroB"}, {c, "HeroC"}} {
		id, falante, texto := esperaCanal(t, alvo.conn)
		if id != 1 {
			t.Errorf("%s: HEADER.ID = %d, queria 1 (quem falou)", alvo.nome, id)
		}
		if falante != "Hero" {
			t.Errorf("%s: falante = %q, queria \"Hero\" — o legado reescreve o MobName", alvo.nome, falante)
		}
		// O prefixo segue no texto de propósito: é por ele que o cliente sabe em
		// que canal desenhar a linha.
		if texto != "@oi pessoal" {
			t.Errorf("%s: texto = %q, queria \"@oi pessoal\"", alvo.nome, texto)
		}
	}
	// E quem falou não ouve nada: nem o eco (o cliente já desenhou a própria
	// fala), nem a recusa de sussurro que era o defeito.
	nadaChega(t, a, "quem falou")
}

// TestReinoSoAlcancaOMesmoClan: o "@@" é filtrado pelo Clan
// (SyncKingdomMulticast). Sem o filtro ele seria um segundo canal global.
func TestReinoSoAlcancaOMesmoClan(t *testing.T) {
	addr, stop, _ := startServerClock(t, canaisDB())
	defer stop()
	a := enterWorldAs(t, addr, "tester")
	defer a.Close()
	b := enterWorldAs(t, addr, "tradeb")
	defer b.Close()
	c := enterWorldAs(t, addr, "third")
	defer c.Close()
	drainRaw(t, a)
	drainRaw(t, b)
	drainRaw(t, c)

	canalFrame(t, a, "@@pelo reino")

	if _, falante, texto := esperaCanal(t, b); falante != "Hero" || texto != "@@pelo reino" {
		t.Errorf("mesmo reino recebeu (%q, %q)", falante, texto)
	}
	nadaChega(t, c, "outro reino")
}

// TestGuildaSoAlcancaAGuildaELevaOMarcador: o "-" é filtrado pela guilda, e é o
// ÚNICO canal que carrega o marcador de canal no pacote (String[96] = 3).
func TestGuildaSoAlcancaAGuildaELevaOMarcador(t *testing.T) {
	addr, stop, _ := startServerClock(t, canaisDB())
	defer stop()
	a := enterWorldAs(t, addr, "tester")
	defer a.Close()
	b := enterWorldAs(t, addr, "tradeb")
	defer b.Close()
	c := enterWorldAs(t, addr, "third")
	defer c.Close()
	drainRaw(t, a)
	drainRaw(t, b)
	drainRaw(t, c)

	canalFrame(t, a, "-só a guilda")

	h, payload, ok := readMaybeHeader(t, b)
	if !ok || h.Type != protocol.MsgMessageWhisper {
		t.Fatalf("guilda recebeu %#x ok=%v, queria MessageWhisper", h.Type, ok)
	}
	if got := cstr(payload[16:]); got != "-só a guilda" {
		t.Errorf("texto = %q, queria \"-só a guilda\"", got)
	}
	if got := payload[16+protocol.MessageLength]; got != 3 {
		t.Errorf("marcador de canal = %d, queria 3 (String[MESSAGE_LENGTH])", got)
	}
	nadaChega(t, c, "quem não é da guilda")
}

// TestGuildaSemGuildaRecusa: falar no canal de guilda sem ter uma é recusado com
// a linha do legado, não com silêncio.
func TestGuildaSemGuildaRecusa(t *testing.T) {
	addr, stop, _ := startServerClock(t, canaisDB())
	defer stop()
	c := enterWorldAs(t, addr, "third") // HeroC não tem guilda
	defer c.Close()
	drainRaw(t, c)

	canalFrame(t, c, "-tem alguém aí")

	ty, payload, ok := readMaybe(t, c)
	if !ok || ty != protocol.MsgMessageBoxOk {
		t.Fatalf("recebeu %#x ok=%v, queria o aviso", ty, ok)
	}
	if got := noticeCode(t, payload); got != NoticeOnlyGuildMember {
		t.Errorf("aviso = %d, queria NoticeOnlyGuildMember (%d)", got, NoticeOnlyGuildMember)
	}
}

// TestGrupoRespeitaODesligadorInclusiveDoLider cobre a divergência anotada em
// chatDoGrupo: no legado o líder recebe a linha do grupo mesmo com o canal
// desligado, porque ele é entregue fora do laço que consulta o desligador.
func TestGrupoRespeitaODesligadorInclusiveDoLider(t *testing.T) {
	addr, stop, _ := startServerClock(t, canaisDB())
	defer stop()
	a := enterWorldAs(t, addr, "tester") // conn 1, líder
	defer a.Close()
	b := enterWorldAs(t, addr, "tradeb") // conn 2, membro
	defer b.Close()

	reqPartyFrame(t, a, 1, 2)
	expectPartyFrame(t, b, protocol.MsgSendReqParty)
	acceptPartyFrame(t, b, 1, "Hero")
	expectCNFAddParty(t, a)
	expectCNFAddParty(t, b)
	drainRaw(t, a)
	drainRaw(t, b)

	// Com o canal ligado o líder ouve o membro.
	canalFrame(t, b, "=vamos")
	if _, falante, texto := esperaCanal(t, a); falante != "HeroB" || texto != "=vamos" {
		t.Errorf("líder recebeu (%q, %q), queria (\"HeroB\", \"=vamos\")", falante, texto)
	}

	// O líder desliga o canal e para de ouvir.
	chatFrame(t, a, "partychat")
	drainRaw(t, a)
	canalFrame(t, b, "=e agora")
	nadaChega(t, a, "líder que desligou o chat de grupo")
}

// TestGrupoNaoVazaParaForaDoGrupo: sem o filtro, "=" seria um terceiro canal
// global — e é o filtro mais fácil de errar, porque a lista mora no LÍDER.
func TestGrupoNaoVazaParaForaDoGrupo(t *testing.T) {
	addr, stop, _ := startServerClock(t, canaisDB())
	defer stop()
	a := enterWorldAs(t, addr, "tester")
	defer a.Close()
	b := enterWorldAs(t, addr, "tradeb")
	defer b.Close()
	c := enterWorldAs(t, addr, "third")
	defer c.Close()

	reqPartyFrame(t, a, 1, 2)
	expectPartyFrame(t, b, protocol.MsgSendReqParty)
	acceptPartyFrame(t, b, 1, "Hero")
	expectCNFAddParty(t, a)
	expectCNFAddParty(t, b)
	drainRaw(t, a)
	drainRaw(t, b)
	drainRaw(t, c)

	canalFrame(t, a, "=só nós dois")

	if _, falante, _ := esperaCanal(t, b); falante != "Hero" {
		t.Errorf("membro recebeu de %q, queria \"Hero\"", falante)
	}
	nadaChega(t, c, "quem está fora do grupo")
}

// TestReinoEsperaTresSegundos: a segunda linha seguida é recusada com o aviso,
// e NÃO é entregue. Sem a espera, os dois canais de longo alcance viram o
// caminho mais barato para encher a tela de todo mundo.
func TestReinoEsperaTresSegundos(t *testing.T) {
	addr, stop, _ := startServerClock(t, canaisDB())
	defer stop()
	a := enterWorldAs(t, addr, "tester")
	defer a.Close()
	b := enterWorldAs(t, addr, "tradeb")
	defer b.Close()
	drainRaw(t, a)
	drainRaw(t, b)

	canalFrame(t, a, "@@primeira")
	if _, _, texto := esperaCanal(t, b); texto != "@@primeira" {
		t.Fatalf("primeira linha = %q", texto)
	}

	canalFrame(t, a, "@@segunda")
	ty, payload, ok := readMaybe(t, a)
	if !ok || ty != protocol.MsgMessagePanel {
		t.Fatalf("quem falou recebeu %#x ok=%v, queria o aviso de espera", ty, ok)
	}
	if got := cstr(payload); got != msgAguardeCanal {
		t.Errorf("aviso = %q, queria %q", got, msgAguardeCanal)
	}
	nadaChega(t, b, "quem ouviria a segunda linha")
}

// TestSussurroSemNomeNemPrefixoMorreCalado: o cliente manda manutenção própria
// por este caminho. O legado descarta em silêncio; o port respondia "O jogador
// não está conectado." a cada um deles, que é o rastro que aparecia na tela do
// Marco junto das falas.
func TestSussurroSemNomeNemPrefixoMorreCalado(t *testing.T) {
	addr, stop, _ := startServerClock(t, canaisDB())
	defer stop()
	a := enterWorldAs(t, addr, "tester")
	defer a.Close()
	drainRaw(t, a)

	canalFrame(t, a, "texto sem prefixo nenhum")
	nadaChega(t, a, "quem mandou um pacote sem canal")
}

// TestSussurroNormalContinuaFuncionando: o desvio dos canais entra ANTES da
// entrega do sussurro, então é ele que pode quebrá-la.
func TestSussurroNormalContinuaFuncionando(t *testing.T) {
	addr, stop, _ := startServerClock(t, canaisDB())
	defer stop()
	a := enterWorldAs(t, addr, "tester")
	defer a.Close()
	b := enterWorldAs(t, addr, "tradeb")
	defer b.Close()
	drainRaw(t, a)
	drainRaw(t, b)

	whisperFrame(t, a, "HeroB", "só para você")
	h, payload, ok := readMaybeHeader(t, b)
	if !ok || h.Type != protocol.MsgMessageWhisper {
		t.Fatalf("recebeu %#x ok=%v, queria MessageWhisper", h.Type, ok)
	}
	if got := cstr(payload[16:]); got != "só para você" {
		t.Errorf("texto = %q", got)
	}
}
