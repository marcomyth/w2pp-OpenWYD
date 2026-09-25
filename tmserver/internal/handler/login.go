package handler

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/content"
	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/protocol"
	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/world"
)

// accountLogin handles _MSG_AccountLogin (0x020D), handlers/_MSG_AccountLogin.md.
// It validates the client version, session mode and brute-force gate, then
// relays the credentials to the dbServer asynchronously (the original forwards
// _MSG_DBAccountLogin and waits in USER_LOGIN).
func (d *Dispatcher) accountLogin(w *world.World, s *world.Session, _ protocol.Header, payload []byte) {
	if len(payload) < protocol.MsgAccountLoginBodySize {
		d.log.Warn("account login: short packet", "conn", s.Conn)
		w.Close(s)
		return
	}
	var body protocol.MsgAccountLoginBody
	if err := body.Decode(payload); err != nil {
		w.Close(s)
		return
	}

	// Server-authoritative version check (anti-cheat: blocks forged/old clients).
	// This 7662 "Cavaleiros de Kersef" build sends ClientVersion=12000 (set via
	// -client-version); mismatches get the rerun notice and are dropped.
	if body.ClientVersion != d.cfg.ClientVersion {
		d.log.Warn("account login: version mismatch",
			"conn", s.Conn, "got", body.ClientVersion, "want", d.cfg.ClientVersion)
		d.notify(w, s, NoticeVersionMismatch)
		w.Close(s)
		return
	}
	if s.Mode != world.UserAccept {
		d.notify(w, s, NoticeLoginNow)
		return
	}

	name := strings.ToLower(cstr(body.AccountName[:]))
	if name == "" {
		w.Close(s)
		return
	}
	if d.fails[name] >= d.cfg.MaxFailLogin {
		d.log.Warn("account login: locked out after wrong passwords", "conn", s.Conn, "account", name, "fails", d.fails[name])
		d.notify(w, s, Notice3WrongPass)
		return
	}

	pass := cstr(body.AccountPassword[:])
	// DBNeedSave is the client asking to take the account over from a session
	// that is still holding it (see accountInUse).
	takeOver := body.DBNeedSave != 0
	s.AccountName = name
	s.Maquina = body.AdapterName
	s.Mode = world.UserLogin
	d.log.Info("account login: relaying to dbServer", "conn", s.Conn, "account", name,
		"maquina", maquinaTexto(body.AdapterName))

	p := w.Persistence()
	epoca := w.EpocaDoPar()
	w.Go(s, func() func(*world.World, *world.Session) {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		out, err := p.AccountLogin(ctx, name, pass, epoca)
		return func(w *world.World, s *world.Session) { d.completeAccountLogin(w, s, out, err, takeOver) }
	})
}

// completeAccountLogin applies the dbServer login result back in the loop.
func (d *Dispatcher) completeAccountLogin(w *world.World, s *world.Session, out world.LoginOutcome, err error, takeOver bool) {
	if err != nil {
		d.log.Error("account login backend error", "conn", s.Conn, "account", s.AccountName, "err", err)
		s.Mode = world.UserAccept
		d.notify(w, s, NoticeDBError)
		w.Close(s)
		return
	}
	switch out.Result {
	case world.LoginOK:
		// A TRANCA DO SERVIDOR DE TESTE, e ela vem antes de tudo o que escreve.
		//
		// A senha estava certa: o que recusa aqui não é quem a pessoa é, é ONDE ela
		// está tentando entrar. Por isso o texto diz isso, em vez de "senha
		// inválida" — mandar alguém conferir a senha que estava certa é fazer a
		// pessoa perder a tarde.
		//
		// ANTES DO accountInUse E DE QUALQUER ESCRITA: mais abaixo esta função
		// instala o baú, drena entregas e reconcilia o escrow. Recusar depois disso
		// deixaria metade do login feito para alguém que não entrou.
		//
		// O texto sai pelo painel (0x101) e a CONEXÃO NÃO FECHA EM CIMA DELE.
		//
		// Medido no cliente pela dupla que cuida dele: ele mostra o 0x101 por quatro
		// segundos e reabilita os campos para a pessoa tentar de novo. O que ele faz
		// se o socket cair ANTES de o painel aparecer ninguém sabe — e a aposta
		// errada aí é a pessoa ver a janela sumir sem ler nada.
		//
		// Então a sessão volta ao estado de ANTES do login: sem conta, sem modo de
		// jogo. Ela não é mais ninguém, e nenhum comando de jogo passa. O socket cai
		// depois, sozinho, pelo prazo abaixo.
		if d.cfg.AcessoRestrito && !world.ParseAccess(out.Role).EhStaff() {
			d.log.Info("acesso restrito: login de jogador recusado",
				"conn", s.Conn, "account", s.AccountName)
			sendClientMessage(w, s, "Servidor de teste, acesso restrito.")
			s.AccountName = ""
			s.AccountID = 0
			s.Mode = world.UserAccept
			s.RecusasDeAcesso++
			d.fechaDepois(w, s, s.RecusasDeAcesso)
			return
		}
		delete(d.fails, s.AccountName)
		// Before AccountID is set: closing s below must not release the cargo
		// that the session already holding the account is using.
		if d.accountInUse(w, s, out.AccountID, takeOver) {
			return
		}
		s.AccountID = out.AccountID
		s.AccessLevel = world.ParseAccess(out.Role) // GM/moderation privilege (issue #122)
		s.Cash, s.Rmt = out.Cash, out.Rmt
		// O passe vem no mesmo login, como as carteiras, e pela mesma razão: o laço
		// não fala com o banco.
		s.PasseNivel = out.PasseNivel
		d.log.Info("account login: OK", "conn", s.Conn, "account", s.AccountName, "id", out.AccountID, "role", s.AccessLevel, "chars", len(out.Characters))
		// Install the account-shared cargo, loaded in the same backend round-trip.
		// It lives for the whole account session and is released on disconnect.
		cargo := out.Cargo
		w.SetCargo(out.AccountID, &cargo)
		vendidos := 0
		// Drain any pending donate web-shop grants (fetched in the same login
		// round-trip) into the freshly-loaded cargo (issue #34); items land in the
		// next free slot, or stay in the mailbox when it is full. Encode AFTER the
		// drain so the client vault cache includes freshly delivered items.
		_, held := w.ApplyDeliveries(s, out.PendingDeliveries)
		// E o contrário da entrega: os slots que uma venda em dinheiro real
		// deixou para trás. A marca do escrow segurou o item intocável até aqui,
		// e agora ele sai — ANTES de montar a tela, para o jogador não ver no
		// baú uma coisa que já não é dele.
		if saiu := w.LimpaSlotsVendidos(s, out.SlotsVendidos); saiu > 0 {
			d.log.Info("escrow: itens vendidos retirados no login",
				"conn", s.Conn, "account", s.AccountName, "itens", saiu)
			vendidos = saiu
		}
		// E a reconciliação do escrow, que é o contrário da retirada: aqui o item
		// FICA e só o cadeado sai. São dois caminhos porque são dois destinos —
		// confundi-los apagaria o item de quem não vendeu nada.
		//
		// DEPOIS do accountInUse, e em chamada própria. Ela ESCREVE no banco
		// supondo que quem está entrando não tem barraca de pé; numa tentativa
		// recusada — a conta já em jogo — a barraca é da sessão ANTIGA, que
		// continua viva, e a reconciliação cancelaria a venda dela em silêncio.
		d.reconciliaEscrow(w, s, out.AccountID)
		s.Mode = world.UserSelChar
		coin, cargoItems := d.cargoWire(w.Cargo(out.AccountID))
		body := protocol.EncodeCNFAccountLoginBody(s.AccountName, d.selCharsFrom(out.Characters), coin, cargoItems)
		w.SendTo(s, protocol.Header{Type: protocol.MsgCNFAccountLogin, ID: protocol.IDSelChar}, body)
		if vendidos > 0 {
			// O vendedor tem de saber por que o baú tem um espaço a mais. Sem
			// esta linha ele conta os itens, acha que sumiu um, e abre chamado —
			// e quem atender não vai ter o que olhar, porque está tudo certo.
			sendClientMessage(w, s, fmt.Sprintf(
				"%d item(ns) que você vendeu por dinheiro real saíram do baú.", vendidos))
		}
		if held > 0 {
			// The player paid for these and cannot see them yet. Saying why is
			// what keeps "abre espaço" from becoming a support ticket.
			sendClientMessage(w, s, world.MensagemEntregaPresa(held))
		}
	case world.LoginBadPassword:
		d.fails[s.AccountName]++
		d.log.Warn("account login: bad password", "conn", s.Conn, "account", s.AccountName, "fails", d.fails[s.AccountName])
		s.Mode = world.UserAccept // allow retry
		d.notify(w, s, NoticeBadPass)
	case world.LoginNoAccount:
		s.Mode = world.UserAccept
		d.notify(w, s, NoticeNoAccount)
	case world.LoginBlocked:
		d.notify(w, s, NoticeBlocked)
		w.Close(s)
	case world.LoginAlreadyPlaying:
		// AGORA O dbServer RESPONDE ISTO, e o caso é o da posse da conta: outra
		// execução do tmServer está com ela. Durante a sobreposição de um deploy,
		// isto é quase sempre a própria pessoa tentando voltar antes de o servidor
		// velho terminar de gravá-la.
		//
		// POR ISSO A FRASE MANDA ESPERAR, e não parece castigo: o
		// MsgAlreadyPlaying sozinho é a janela do legado, que não explica nada e
		// que, num deploy, o jogador leria como ban. A trava de DENTRO do processo
		// (accountInUse) continua respondendo só a janela do legado, porque lá a
		// causa é outra: a conta está mesmo aberta em outro lugar agora.
		sendClientMessage(w, s, msgContaAindaSaindo)
		w.SendTo(s, protocol.Header{Type: protocol.MsgAlreadyPlaying, ID: protocol.IDSelChar}, nil)
		w.Close(s)
	}
}

// msgContaAindaSaindo é o que o jogador lê quando a conta ainda está presa à
// execução anterior do servidor. Ela manda ESPERAR: o caso normal é a sobreposição
// de um deploy, e é a própria pessoa tentando voltar.
const msgContaAindaSaindo = "Sua conta ainda está saindo do servidor. Tente de novo em alguns segundos."

// reconciliaEscrow põe o escrow em dia depois de esta conexão ganhar a conta.
//
// Fora do laço porque escreve no banco, e por `Go` e não `GoDetached`: se a
// sessão morrer no meio, não há baú para soltar e a próxima entrada refaz a
// pergunta. O vendedor não é avisado — nada mudou no baú que ele pudesse notar, e
// um aviso sobre uma trava que ele nunca viu seria conversa sobre encanamento.
func (d *Dispatcher) reconciliaEscrow(w *world.World, s *world.Session, accountID int64) {
	p := w.Persistence()
	// A TRAVA SOBE ANTES DA IDA, e desce na volta, dê no que der.
	//
	// A reconciliação cancela TODO anúncio ativo da conta, supondo que quem
	// acabou de entrar não tem barraca de pé. A suposição vale quando a pergunta
	// é feita e pode deixar de valer antes de a resposta chegar: com o banco
	// lento — o tempo limite aqui é de dez segundos — o vendedor tem tempo de
	// chegar na cidade e montar uma barraca em dinheiro real, e o anúncio
	// recém-nascido seria cancelado com a barraca nova de pé.
	//
	// É o mesmo defeito do login duplicado, por outra porta: venda desfeita em
	// silêncio. E a trava é no lojaAbrir e não numa estimativa de quanto tempo o
	// jogador leva para andar até a cidade; tempo não é garantia.
	s.ReconciliandoEscrow = true
	w.Go(s, func() func(*world.World, *world.Session) {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		slots, err := p.ReconcileRmtEscrow(ctx, accountID)
		return func(w *world.World, s *world.Session) {
			s.ReconciliandoEscrow = false
			if err != nil {
				// Os itens continuam presos e intocáveis, que é o erro barato. A
				// próxima entrada tenta de novo.
				d.log.Warn("escrow: reconciliacao do login falhou",
					"conn", s.Conn, "account", accountID, "err", err)
				return
			}
			if soltos := w.SoltaMarcasMortas(accountID, slots); soltos > 0 {
				d.log.Info("escrow: reconciliacao do login soltou cadeados mortos",
					"conn", s.Conn, "account", accountID, "itens", soltos)
			}
		}
	})
}

// accountInUse keeps one account to one session, the check the legacy DBSrv
// makes right after the password passes (CFileDB.cpp:685-703). Our dbServer
// holds no sessions, so the rule lives here, where they are. The account is in
// use while another session holds it — character screen or play — and also
// while a closed session's quit-saves are still in flight: the legacy keeps the
// account's slot until that save lands, and a login that read the database
// before then would load the state from before it.
//
// The new connection never gets in, and what happens to the old one is the
// client's DBNeedSave, as in the legacy:
//
//   - 0: _MSG_AlreadyPlaying to the new connection, which is closed; the old
//     session carries on (ProcessDBMessage.cpp:1253-1262).
//   - otherwise: _MSG_StillPlaying to the new connection, which is closed, and
//     the old session is told _NN_Your_Account_From_Others and closed with its
//     save (SendDBSavingQuit; ProcessDBMessage.cpp:1266-1276 and 1291-1322), so
//     a later attempt finds the account free once that save has landed.
//
// Both replies go out with HEADER.ID = ESCENE_FIELD+2, as SendClientSignal
// sends them there.
//
// Letting both in — what the port did until 11/09/2026 — put two live copies of
// the same characters, and two of the account cargo, in memory, each saving on
// its own: a duplication path.
func (d *Dispatcher) accountInUse(w *world.World, s *world.Session, accountID int64, takeOver bool) bool {
	old := w.AccountSession(accountID, s)
	if old == nil && !w.AccountSaving(accountID) {
		return false
	}
	oldConn := -1
	if old != nil {
		oldConn = old.Conn
	}
	d.log.Warn("account login: account already in use",
		"conn", s.Conn, "account", s.AccountName, "old_conn", oldConn, "take_over", takeOver)
	signal := protocol.MsgAlreadyPlaying
	if takeOver {
		signal = protocol.MsgStillPlaying
	}
	w.SendTo(s, protocol.Header{Type: signal, ID: protocol.IDSelChar}, nil)
	w.Close(s)
	if takeOver && old != nil {
		if old.Mode == world.UserPlay || old.Mode == world.UserSelChar {
			d.notify(w, old, NoticeAccountFromOthers)
		}
		w.Close(old)
	}
	return true
}

func (d *Dispatcher) cargoWire(st *world.CargoState) (int32, [128]protocol.SelItem) {
	var items [128]protocol.SelItem
	if st == nil {
		return 0, items
	}
	for i := range st.Items {
		if i >= len(items) {
			break
		}
		items[i] = itemToSel(st.Items[i])
	}
	return st.Coin, items
}

// selCharsFrom maps the dbServer character summaries to protocol.SelChar rows for
// the byte-exact STRUCT_SELCHAR (MSG_CNFAccountLogin / MSG_CNFNewCharacter). The
// summary carries the real score (gold, HP/MP, attributes) so the selection
// screen previews each slot's actual character, not placeholders.
//
// Level rides RAW, exactly as the legacy does: DBGetSelChar copies the whole
// STRUCT_SCORE across with no adjustment (CFileDB.cpp:2651) and the client
// prints what it receives. A level-1 correction lived here between June 2026
// and this fix, on the theory that the client's display was one-based. It is
// not: a level-400 character previewed as 399 on the selection screen and then
// entered the world at 400.
func (d *Dispatcher) selCharsFrom(chars []world.CharSummary) []protocol.SelChar {
	out := make([]protocol.SelChar, 0, len(chars))
	for _, c := range chars {
		sc := protocol.SelChar{
			Slot:  c.Slot,
			Name:  c.Name,
			Level: int32(c.Level),
			Exp:   c.Exp,
			Guild: c.GuildID,
			Coin:  c.Coin,
			MaxHp: c.MaxHp, Hp: c.Hp, MaxMp: c.MaxMp, Mp: c.Mp,
			Str: c.Str, Int: c.Int, Dex: c.Dex, Con: c.Con,
		}
		// Preview the saved gear. Empty equipment falls back to the class BaseMob so
		// fresh characters still render with the right class model on the select screen.
		if !equipEmpty(c.Equip) {
			for i := range c.Equip {
				sc.Equip[i] = itemToSel(c.Equip[i])
			}
		} else if tmpl, ok := d.baseMobs[c.Class]; ok && len(tmpl) == content.BaseMobSize {
			sc.Equip = protocol.MobEquip(tmpl)
		}
		out = append(out, sc)
	}
	return out
}

// prazoDaRecusa é quanto o socket fica de pé depois de uma recusa de acesso.
//
// DEZ SEGUNDOS, e o número vem do cliente: ele mostra a mensagem por quatro e
// devolve os campos à pessoa. Fechar antes disso apaga a mensagem; deixar aberto
// para sempre segura um dos mil lugares de sessão por causa de quem nem entrou.
//
// O prazo de ocioso do servidor (-idle-timeout-sec) faria este trabalho, mas ele
// nasce DESLIGADO e é sobre outra coisa. Uma recusa não pode depender de uma opção
// que talvez ninguém tenha ligado.
const prazoDaRecusa = 10 * time.Second

// fechaDepois derruba o socket daqui a pouco, sem segurar o laço.
//
// A espera acontece FORA do laço — dentro dele, dez segundos parados seriam dez
// segundos de servidor congelado para todo mundo. O World.Go é o caminho de sempre
// para isso, e ele já descarta o retorno quando a sessão morreu antes: quem desistir
// e fechar o jogo não vira um Close numa sessão que já não existe.
func (d *Dispatcher) fechaDepois(w *world.World, s *world.Session, recusa int) {
	prazo := d.cfg.PrazoDaRecusa
	if prazo <= 0 {
		prazo = prazoDaRecusa
	}
	w.Go(s, func() func(*world.World, *world.Session) {
		time.Sleep(prazo)
		return func(w *world.World, s *world.Session) {
			// SÓ FECHA SE NADA ACONTECEU DEPOIS, e esta guarda é a correção de uma
			// corrida que derrubaria gente legítima.
			//
			// O cliente devolve os campos à pessoa depois de quatro segundos, e ela
			// pode entrar de novo NO MESMO SOCKET — é o caso de um staff que errou a
			// conta na primeira vez. Sem a guarda, o fechamento agendado pela recusa
			// antiga chegaria aos dez segundos e derrubaria a sessão que já entrou.
			//
			// O World.Go só descarta a volta quando a sessão MORREU; aqui ela está
			// viva, e mais do que isso: logada.
			//
			// Três perguntas, e as três precisam continuar valendo: a sessão não tem
			// conta, não está em modo de jogo, e nenhuma recusa NOVA aconteceu depois
			// desta — senão o fechamento da segunda seria feito duas vezes, e o da
			// primeira mataria a espera da segunda antes da hora.
			if s.AccountID != 0 || s.Mode != world.UserAccept || s.RecusasDeAcesso != recusa {
				return
			}
			w.Close(s)
		}
	})
}
