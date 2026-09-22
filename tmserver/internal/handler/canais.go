package handler

import (
	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/protocol"
	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/world"
)

// Os quatro canais de chat do legado — guilda, grupo, reino e cidadão.
//
// Nenhum deles tem pacote próprio: os quatro são o MESMO _MSG_MessageWhisper
// (0x0334) do sussurro, com o MobName VAZIO, e o canal é escolhido pelo PREFIXO
// do texto (_MSG_MessageWhisper.cpp:1422-1566):
//
//	-   guilda          --  guilda + aliada
//	=   grupo
//	@   cidadão (o servidor inteiro)    @@  reino (mesmo Clan)
//
// Era isso que faltava: o port ia direto de `MobName` para SessionByName, que
// com o nome vazio não acha ninguém e responde "O jogador não está conectado."
// Quem falava no canal global recebia a recusa de um sussurro.
//
// O legado responde a TODO whisper de MobName vazio e retorna — inclusive o que
// não traz prefixo nenhum, que ele descarta calado. Isso também é reproduzido:
// o cliente manda pacotes de manutenção por esse caminho, e responder a eles é
// justamente o defeito que estamos consertando.

const (
	prefixoGuilda  = '-'
	prefixoGrupo   = '='
	prefixoCanalAt = '@' // '@' cidadão, '@@' reino

	// canalEsperaMs é o intervalo entre duas linhas de Reino ou Cidadão, os dois
	// canais que alcançam quem não está na tela (pUser.Message, 3000 ms). World.Now
	// é milissegundo, como o GetTickCount do legado, e a subtração em uint32 dá a
	// volta junto com ele.
	canalEsperaMs = 3000
)

// msgAguardeCanal é o literal do legado (_MSG_MessageWhisper.cpp:1527/1552).
const msgAguardeCanal = "Aguarde 3 segundos."

// estadoDoCanal monta a confirmação de um desligador. O legado responde em
// inglês ("Party Chatting : Off"); aqui vai no idioma do resto do jogo, e o
// sentido é o mesmo — desligado significa que este jogador parou de RECEBER.
func estadoDoCanal(nome string, desligado bool) string {
	if desligado {
		return nome + ": desligado"
	}
	return nome + ": ligado"
}

// chatDeCanal trata um _MSG_MessageWhisper de MobName vazio. Sempre devolve o
// controle ao chamador — não existe caminho de sussurro para esse pacote.
func (d *Dispatcher) chatDeCanal(w *world.World, s *world.Session, body protocol.MsgWhisperBody) {
	e := w.Entity(s.Conn)
	if e == nil {
		return
	}
	texto := cstr(body.String)
	if texto == "" {
		return
	}
	// O texto segue com o prefixo: é por ele que o cliente sabe em que canal
	// desenhar a linha (protocol.EncodeWhisperChannelBody).
	switch {
	case texto[0] == prefixoGuilda:
		d.chatDaGuilda(w, s, e, body.String, len(texto) > 1 && texto[1] == prefixoGuilda)
	case texto[0] == prefixoGrupo:
		d.chatDoGrupo(w, s, e, body.String)
	case len(texto) > 1 && texto[0] == prefixoCanalAt && texto[1] == prefixoCanalAt:
		d.chatDoReino(w, s, e, body.String)
	case texto[0] == prefixoCanalAt:
		d.chatDoCidadao(w, s, e, body.String)
	}
}

// chatDaGuilda entrega uma linha de "-" aos membros da guilda; com "--" também
// à guilda aliada (_MSG_MessageWhisper.cpp:1425-1468). É o único canal que
// carrega o marcador de canal no pacote.
func (d *Dispatcher) chatDaGuilda(w *world.World, s *world.Session, e *world.Entity, texto []byte, comAliada bool) {
	if e.Guild == 0 {
		d.notify(w, s, NoticeOnlyGuildMember)
		return
	}
	aliada := d.guildAllies[e.Guild]
	corpo := protocol.EncodeWhisperChannelBody(e.Name, texto, true)
	d.entregaCanal(w, s, corpo, func(ts *world.Session, te *world.Entity) bool {
		if ts.GuildChat {
			return false
		}
		if te.Guild == e.Guild {
			return true
		}
		return comAliada && aliada != 0 && te.Guild == aliada
	})
	d.registraFala(w, s, world.ChatGuilda, "", cstr(texto))
}

// chatDoGrupo entrega uma linha de "=" ao grupo inteiro
// (_MSG_MessageWhisper.cpp:1470-1513).
//
// A lista de membros é a do LÍDER, que é onde ela vive; quem não está em grupo
// tem líder 0 e fala sozinho, exatamente como no legado (que também não recusa).
//
// DIVERGÊNCIA estreita: o legado entrega ao líder sem olhar o PartyChat dele —
// só os outros membros têm o desligador respeitado. Um líder que desligou o
// canal e continua recebendo é o desligador quebrado para uma pessoa por grupo,
// então aqui a lista é uma só e o teste cobre justamente o líder.
func (d *Dispatcher) chatDoGrupo(w *world.World, s *world.Session, e *world.Entity, texto []byte) {
	lider := e.Leader
	if lider <= 0 {
		lider = s.Conn
	}
	le := w.Entity(lider)
	if le == nil {
		return
	}
	noGrupo := map[int]bool{lider: true}
	for _, m := range le.PartyList {
		if m > 0 {
			noGrupo[m] = true
		}
	}
	corpo := protocol.EncodeWhisperChannelBody(e.Name, texto, false)
	d.entregaCanal(w, s, corpo, func(ts *world.Session, _ *world.Entity) bool {
		return !ts.PartyChat && noGrupo[ts.Conn]
	})
	d.registraFala(w, s, world.ChatGrupo, "", cstr(texto))
}

// chatDoReino entrega uma linha de "@@" a quem tem o mesmo Clan
// (_MSG_MessageWhisper.cpp:1515-1540, SyncKingdomMulticast).
func (d *Dispatcher) chatDoReino(w *world.World, s *world.Session, e *world.Entity, texto []byte) {
	if !d.passaNaEspera(w, s) {
		return
	}
	corpo := protocol.EncodeWhisperChannelBody(e.Name, texto, false)
	d.entregaCanal(w, s, corpo, func(ts *world.Session, te *world.Entity) bool {
		return !ts.KingChat && te.Clan == e.Clan
	})
	d.registraFala(w, s, world.ChatReino, "", cstr(texto))
}

// chatDoCidadao entrega uma linha de "@" ao servidor inteiro
// (_MSG_MessageWhisper.cpp:1542-1566, SyncMulticast).
//
// É o único canal sem desligador: o SyncMulticast do legado não consulta flag
// nenhuma, e inventar um aqui seria mecânica nova. A espera de 3 segundos é a
// única trava, e é a do legado.
func (d *Dispatcher) chatDoCidadao(w *world.World, s *world.Session, e *world.Entity, texto []byte) {
	if !d.passaNaEspera(w, s) {
		return
	}
	corpo := protocol.EncodeWhisperChannelBody(e.Name, texto, false)
	d.entregaCanal(w, s, corpo, func(*world.Session, *world.Entity) bool { return true })
	d.registraFala(w, s, world.ChatCidadao, "", cstr(texto))
}

// entregaCanal manda corpo a cada sessão em jogo que passa por quer, NUNCA de
// volta para quem falou: o legado pula conn em todos os quatro laços, porque o
// cliente já desenhou a própria fala quando a enviou. Devolver a linha faria
// cada um se ver falando duas vezes.
func (d *Dispatcher) entregaCanal(w *world.World, s *world.Session, corpo []byte, quer func(*world.Session, *world.Entity) bool) {
	w.ForEachSession(func(ts *world.Session, te *world.Entity) {
		if ts == nil || te == nil || ts.Conn == s.Conn || ts.Mode != world.UserPlay {
			return
		}
		if !quer(ts, te) {
			return
		}
		w.SendTo(ts, protocol.Header{Type: protocol.MsgMessageWhisper, ID: uint16(s.Conn)}, corpo)
	})
}

// passaNaEspera é a trava de 3 segundos do Reino e do Cidadão.
//
// DIVERGÊNCIA estreita: o legado regrava o relógio ANTES de conferi-lo, então
// quem insiste dentro dos 3 segundos empurra o próprio prazo para a frente e
// nunca mais fala. Aqui a recusa não mexe no relógio: a espera conta sempre da
// última linha ENVIADA, que é o que o aviso promete.
func (d *Dispatcher) passaNaEspera(w *world.World, s *world.Session) bool {
	agora := w.Now()
	if s.UltimaMensagemCanal != 0 && agora-s.UltimaMensagemCanal < canalEsperaMs {
		sendClientMessage(w, s, msgAguardeCanal)
		return false
	}
	s.UltimaMensagemCanal = agora
	return true
}
