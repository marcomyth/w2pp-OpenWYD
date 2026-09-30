package handler

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/protocol"
	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/world"
)

// Guild costs and ranks (lote2-party-guilda-guerra.md and
// _MSG_MessageWhisper.cpp guild command blocks).
const (
	guildInviteCost  = 4_000_000
	guildSpecialCost = 100_000_000
	guildCreateCost  = 100_000_000
	guildSubCost     = 100_000_000
	guildLeaderLevel = 9
	guildNameMaxLen  = 16
)

// inviteGuild handles _MSG_InviteGuild (0x03D5, MSG_STANDARDPARM2:
// Parm1=TargetID, Parm2=InviteType): add a same-clan, guildless player to the
// inviter's guild for a gold cost.
func (d *Dispatcher) inviteGuild(w *world.World, s *world.Session, _ protocol.Header, payload []byte) {
	e := w.Entity(s.Conn)
	if e == nil || s.Mode != world.UserPlay {
		return
	}
	p1, p2, ok := protocol.StandardParm2(payload)
	if !ok {
		return
	}
	target, inviteType := int(p1), int(p2)
	if target <= 0 || target >= world.MaxUser || inviteType < 0 || inviteType >= 4 {
		return
	}
	if e.Guild == 0 || e.GuildLevel == 0 {
		return
	}
	if inviteType != 0 && e.GuildLevel != guildLeaderLevel {
		return
	}
	if d.now().Weekday() == time.Sunday {
		sendClientMessage(w, s, msgGuildDomingoConvite) // _MSG_InviteGuild.cpp:61
		return
	}
	other, te := w.Session(target), w.Entity(target)
	if other == nil || other.Mode != world.UserPlay || te == nil {
		return
	}
	// AS DUAS RECUSAS AVISAM. O legado voltava calado nas duas
	// (_MSG_InviteGuild.cpp:38 e :41), e o líder via "não acontece nada": em
	// 29/09/2026 a Josiel convidou quatro vezes um personagem sem reino com o
	// líder do reino vermelho, sem nenhuma pista. A regra fica a mesma.
	if te.Guild != 0 {
		sendClientMessage(w, s, fmt.Sprintf(msgConviteJaTemGuilda, te.Name))
		return
	}
	if te.Clan != e.Clan {
		sendClientMessage(w, s, fmt.Sprintf(msgConviteOutroReino, te.Name))
		return
	}
	cost := int32(guildInviteCost)
	if inviteType != 0 {
		cost = guildSpecialCost
	}
	if e.Coin < cost {
		// NoticeNotEnoughCoin has no text registered, so it drew nothing: the
		// inviter clicked and the invite simply did not happen.
		sendClientMessage(w, s, combineNeedsGold(cost))
		return
	}

	e.Coin -= cost
	te.Guild = e.Guild
	te.GuildLevel = 0
	d.refreshGuildTag(w, target)
	d.sendEtc(w, s, e)
	// _SN_JOINGUILD (_MSG_InviteGuild.cpp:90). This used to go out as a
	// MSG_MessagePanel with NO body — a frame shorter than the struct, which the
	// client reads past into whatever follows it.
	sendClientMessage(w, other, fmt.Sprintf("Você entrou na Guilda %s.", guildDisplayName(w, e.Guild)))
	w.SaveCharacterAsync(s)
	w.SaveCharacterAsync(other)
	d.persistGuildMember(w, s, other, te)
}

func (d *Dispatcher) createGuild(w *world.World, s *world.Session, args []byte) {
	e := w.Entity(s.Conn)
	if e == nil || s.Mode != world.UserPlay {
		return
	}
	name := strings.TrimSpace(cstr(args))
	// Every refusal names itself, in the legacy's order
	// (_MSG_MessageWhisper.cpp "create"). The legacy stays silent on two of them
	// — no name, and already in a guild — and so did this, along with the
	// coin one (NoticeNotEnoughCoin has no text): a player typing /create and
	// seeing nothing could not tell a typo from a rule.
	if msg := d.guildCreateRefusal(w, e, name); msg != "" {
		sendClientMessage(w, s, msg)
		return
	}
	accountID, slot, charName, clan, citizen, serverIndex := s.AccountID, s.Slot, e.Name, e.Clan, e.Citizen, d.serverIndex
	// O OURO VIVO É DA MEMÓRIA, E O BANCO PRECISA VÊ-LO ANTES DE CONFERIR.
	//
	// O CreateGuild reconfere `coin < cost` contra character.coin NO BANCO, e faz bem:
	// é a última barreira contra sair devendo. O problema é que o ouro que a pessoa
	// acabou de sacar da carga só existe na memória até o próximo save — então o banco
	// via o valor VELHO e recusava. Em produção, 25/09/2026: a Hanna sacou um bilhão da
	// carga e levou nove recusas seguidas, com a frase que fala em nome repetido.
	//
	// A SAÍDA É GRAVAR ANTES, e não afrouxar a conferência. Passar o ouro da memória
	// como parâmetro faria o store confiar num número de quem chama, e aí a barreira
	// que impede o saldo negativo deixaria de ser barreira. Salvar primeiro mantém o
	// banco como dono da verdade e só o põe em dia.
	//
	// O snapshot é tirado AQUI, no laço, e o save acontece lá fora, na ordem: sem isso,
	// o CreateGuild correria contra um save que ainda nem começou.
	save := w.CharacterSaveFor(s, e)
	p := w.Persistence()
	s.Mode = world.UserWaitDB
	// A CARGA VAI JUNTO com o personagem, na mesma transação. Sem isso, o save que põe
	// o banco em dia grava só a mochila: quem acabou de sacar da carga fica com o ouro
	// no personagem gravado e ainda na carga gravada, e uma queda aí duplica.
	w.SalvarEncenadoComCarga(s, save, func(w *world.World, s *world.Session, errSave error) {
		// SE O SAVE FALHAR, NÃO SE CRIA GUILDA. Seguir adiante deixaria o banco decidir
		// com ouro velho de novo — que é exatamente o defeito — e, pior, poderia criar
		// a guilda cobrando de um saldo que não existe lá.
		if errSave != nil {
			if s.Mode == world.UserWaitDB {
				s.Mode = world.UserPlay
			}
			d.log.Warn("create guild: save do personagem falhou", "conn", s.Conn,
				"guild", name, "err", errSave)
			d.notify(w, s, NoticeDBError)
			return
		}
		w.Go(s, func() func(*world.World, *world.Session) {
			guild, ok, motivo, err := p.CreateGuild(context.Background(), accountID, slot, charName, name, clan, citizen, serverIndex, guildCreateCost)
			return func(w *world.World, s *world.Session) {
				if s.Mode == world.UserWaitDB {
					s.Mode = world.UserPlay
				}
				e := w.Entity(s.Conn)
				if e == nil {
					return
				}
				if err != nil {
					d.log.Warn("create guild failed", "conn", s.Conn, "guild", name, "err", err)
					d.notify(w, s, NoticeDBError)
					return
				}
				if !ok || guild.ID == 0 {
					// CADA RECUSA TEM A SUA FRASE, desde 25/09/2026.
					//
					// Antes as quatro viravam "confira se o nome já não existe", e para três
					// delas isso era MENTIRA. Foi essa frase que escondeu um defeito de ouro
					// por horas: a Hanna tentou nove vezes procurando nome repetido enquanto
					// o banco recusava por saldo.
					d.log.Info("create guild refused by dbServer", "conn", s.Conn,
						"guild", name, "motivo", motivo)
					sendClientMessage(w, s, msgDaRecusaDeGuilda(motivo))
					return
				}
				if e.Guild != 0 {
					return
				}
				e.Coin -= guildCreateCost
				e.Guild = guild.ID
				e.GuildLevel = guildLeaderLevel
				// Registered right away: without it the new guild had no name in
				// memory until the next boot, and every place that shows one printed
				// "Guild #N" instead.
				w.SetGuildName(guild.ID, name)
				d.sendEtc(w, s, e)
				d.refreshGuildTag(w, s.Conn)
				w.SaveCharacterAsync(s)
				// The number is what the guild's icon file is named after
				// (b01NNNNNN.bmp), so the leader learns it here, where it is created.
				// This also replaces a MSG_MessagePanel sent with no body at all.
				sendClientMessage(w, s, fmt.Sprintf("Guilda %s criada! Número da guilda: %d.", name, guild.ID))
				d.log.Info("guild created", "conn", s.Conn, "guild", name, "id", guild.ID)
			}
		})
	})
}

// Guild texts. The first four are Language.txt's (_NN_GUILDCREATECLAN 535,
// _DN_NO_TOWNSPEOPLE 513, _NN_GUILDCREATEWEEK 549, _NN_NotEquip_Saturday 390);
// the rest cover refusals the legacy left silent.
const (
	msgGuildReino           = "Você Terá que pertencer a um dos Reinos para poder criar guilda!"
	msgGuildSemCidadania    = "Você não possui cidadania."
	msgGuildDomingo         = "Não é permitido criar guilda aos domingos!"
	msgGuildDomingoConvite  = "Não é possivel utilizar domingo."
	msgConviteJaTemGuilda   = "%s já pertence a uma guilda."
	msgConviteOutroReino    = "%s não é do mesmo reino que você. Só entra na guilda quem é do mesmo reino."
	msgGuildUso             = "Use: /create NomeDaGuilda (até 16 letras)."
	msgGuildJaTem           = "Você já pertence a uma guilda."
	msgGuildCriacaoRecusada = "Não foi possível criar a guilda agora. Tente de novo."
	msgGuildNomeEmUso       = "Já existe uma guilda com esse nome. Escolha outro."
	msgGuildSemOuro         = "Você não tem ouro suficiente para criar a guilda."
	msgGuildJaTemGuilda     = "Você já está numa guilda. Saia dela antes de criar outra."
	msgGuildSemVaga         = "O servidor está sem números de guilda livres. Avise a equipe."
)

// guildCreateRefusal is the first rule /create breaks, as the line the player
// reads, or "" when the guild can be created.
func (d *Dispatcher) guildCreateRefusal(w *world.World, e *world.Entity, name string) string {
	switch {
	case !validGuildName(name):
		return msgGuildUso
	case e.Coin < guildCreateCost:
		return combineNeedsGold(guildCreateCost)
	case e.Guild != 0:
		return msgGuildJaTem
	case e.Clan != 7 && e.Clan != 8:
		return msgGuildReino
	case e.Citizen == 0:
		return msgGuildSemCidadania
	case d.now().Weekday() == time.Sunday:
		return msgGuildDomingo
	case w.GuildNameTaken(name):
		return fmt.Sprintf("Já existe uma guilda chamada %s.", name)
	}
	return ""
}

// guildDisplayName is the guild's registered name, or its number when this
// process has none for it.
func guildDisplayName(w *world.World, id uint16) string {
	if gi, ok := w.GuildInfo(id); ok && gi.Name != "" {
		return gi.Name
	}
	return fmt.Sprintf("#%d", id)
}

func validGuildName(name string) bool {
	if name == "" || len(name) > guildNameMaxLen {
		return false
	}
	return !strings.ContainsRune(name, 0)
}

func (d *Dispatcher) subcreate(w *world.World, s *world.Session, args []byte) {
	e := w.Entity(s.Conn)
	if e == nil || s.Mode != world.UserPlay || e.Guild == 0 || e.GuildLevel != guildLeaderLevel || e.Coin < guildSubCost {
		if e != nil && e.Coin < guildSubCost {
			d.notify(w, s, NoticeNotEnoughCoin)
		}
		return
	}
	fields := strings.Fields(cstr(args))
	if len(fields) < 1 {
		return
	}
	targetSession, target := w.SessionByName(fields[0])
	if targetSession == nil || target == nil {
		// Fora do jogo: o banco acha o membro pelo nome, como no expulsar.
		d.subcreateOffline(w, s, fields[0])
		return
	}
	if target.ID == s.Conn {
		d.notify(w, s, NoticeNotConnected)
		return
	}
	if targetSession.Mode != world.UserPlay {
		return
	}
	if target.Guild != e.Guild || target.GuildLevel != 0 {
		return
	}
	p := w.Persistence()
	guildID := e.Guild
	leaderConn, targetConn := s.Conn, target.ID
	leaderSession, memberSession := s, targetSession
	leaderAccountID, leaderSlot := s.AccountID, s.Slot
	accountID, slot, targetName := targetSession.AccountID, targetSession.Slot, target.Name
	// O MESMO BURACO DO /create MORA AQUI: o PromoteGuildMember reconfere o custo
	// contra character.coin NO BANCO, e o ouro recém-sacado da carga só existe na
	// memória. Sem gravar antes, o líder com um bilhão na tela leva uma recusa muda.
	// O snapshot sai daqui, do laço; o save vai lá fora, antes da cobrança.
	save := w.CharacterSaveFor(s, e)
	// A CARGA VAI JUNTO, na mesma transação, como no /create. Aqui o par é montado à
	// mão porque este caminho grava com GoDetached: as duas sessões podem sumir, e a
	// volta não pode depender de nenhuma delas continuar viva.
	carga, entregues, temCarga, seqDoPar := w.CargaParaOPar(s.AccountID)
	epocaDoPar := w.EpocaDoPar()
	s.Mode = world.UserWaitDB
	targetSession.Mode = world.UserWaitDB
	w.GoDetached(func() func(*world.World) {
		// Save que falha cancela a promoção, pelo mesmo motivo do /create: seguir
		// adiante devolveria o banco a decidir com ouro velho.
		if err := world.SalvarPar(context.Background(), p, save, carga, temCarga, entregues, epocaDoPar, seqDoPar, false); err != nil {
			return func(w *world.World) {
				ls := w.Session(leaderConn)
				if ls != leaderSession {
					ls = nil
				}
				ts := w.Session(targetConn)
				if ts != memberSession {
					ts = nil
				}
				if ls != nil && ls.Mode == world.UserWaitDB {
					ls.Mode = world.UserPlay
				}
				if ts != nil && ts.Mode == world.UserWaitDB {
					ts.Mode = world.UserPlay
				}
				d.log.Warn("subcreate: save do personagem falhou", "conn", leaderConn,
					"target", targetName, "err", err)
				if ls != nil {
					d.notify(w, ls, NoticeDBError)
				}
			}
		}
		level, ok, err := p.PromoteGuildMember(context.Background(), guildID, leaderAccountID, leaderSlot, accountID, slot, guildSubCost)
		return func(w *world.World) {
			if temCarga {
				w.EsqueceEntregues(leaderAccountID, entregues)
			}
			ls := w.Session(leaderConn)
			if ls != leaderSession {
				ls = nil
			}
			ts := w.Session(targetConn)
			if ts != memberSession {
				ts = nil
			}
			if ls != nil && ls.Mode == world.UserWaitDB {
				ls.Mode = world.UserPlay
			}
			if ts != nil && ts.Mode == world.UserWaitDB {
				ts.Mode = world.UserPlay
			}
			if err != nil {
				d.log.Warn("subcreate failed", "conn", leaderConn, "target", targetName, "err", err)
				if ls != nil {
					d.notify(w, ls, NoticeDBError)
				}
				return
			}
			if !ok || level < 6 || level > 8 {
				return
			}
			if ls != nil {
				if le := w.Entity(leaderConn); le != nil {
					le.Coin -= guildSubCost
					d.sendEtc(w, ls, le)
					w.SaveCharacterAsync(ls)
				}
			}
			if ts != nil {
				if te := w.Entity(targetConn); te != nil && te.Guild == guildID {
					te.GuildLevel = level
					d.refreshGuildTag(w, te.ID)
					w.SaveCharacterAsync(ts)
				}
			}
		}
	})
}

// Frases da promoção de quem está fora do jogo. O comando nativo não tem nenhuma:
// com o alvo online, a tag nova na cabeça dele já é a resposta.
const (
	msgGuildaSubOffline     = "%s agora é sub-líder da guilda."
	msgGuildaJaTemCargo     = "%s já tem cargo na guilda."
	msgGuildaSemCargoLivre  = "A guilda já tem três sub-líderes."
	msgGuildaPromoverFalhou = "Não foi possível promover %s agora. Tente de novo."
)

// subcreateOffline promove a sub-líder um membro que não está no jogo. O caminho do
// líder é o mesmo do subcreate (save antes, carga junto, sessão em espera), porque a
// cobrança acontece no banco contra o ouro gravado. Do lado do alvo não há sessão:
// o cargo fica no banco e ele o recebe ao entrar.
func (d *Dispatcher) subcreateOffline(w *world.World, s *world.Session, name string) {
	e := w.Entity(s.Conn)
	if e == nil {
		return
	}
	p := w.Persistence()
	guildID := e.Guild
	leaderConn, leaderSession := s.Conn, s
	leaderAccountID, leaderSlot := s.AccountID, s.Slot
	save := w.CharacterSaveFor(s, e)
	carga, entregues, temCarga, seqDoPar := w.CargaParaOPar(s.AccountID)
	epocaDoPar := w.EpocaDoPar()
	s.Mode = world.UserWaitDB
	w.GoDetached(func() func(*world.World) {
		// A sessão devolvida é a do líder só se ainda for a mesma conexão.
		lider := func(w *world.World) *world.Session {
			ls := w.Session(leaderConn)
			if ls != leaderSession {
				return nil
			}
			if ls.Mode == world.UserWaitDB {
				ls.Mode = world.UserPlay
			}
			return ls
		}
		if err := world.SalvarPar(context.Background(), p, save, carga, temCarga, entregues, epocaDoPar, seqDoPar, false); err != nil {
			return func(w *world.World) {
				d.log.Warn("subcreate offline: save do personagem falhou", "conn", leaderConn,
					"target", name, "err", err)
				if ls := lider(w); ls != nil {
					d.notify(w, ls, NoticeDBError)
				}
			}
		}
		level, motivo, err := p.PromoteOfflineGuildMember(context.Background(), guildID, leaderAccountID, leaderSlot, name, guildSubCost)
		return func(w *world.World) {
			if temCarga {
				w.EsqueceEntregues(leaderAccountID, entregues)
			}
			ls := lider(w)
			if err != nil {
				d.log.Warn("subcreate offline failed", "conn", leaderConn, "guild", guildID, "target", name, "err", err)
				if ls != nil {
					sendClientMessage(w, ls, fmt.Sprintf(msgGuildaPromoverFalhou, name))
				}
				return
			}
			if motivo != world.GuildPromoteRefusalNone || level < 6 || level > 8 {
				if ls == nil {
					return
				}
				switch motivo {
				case world.GuildPromoteRefusalNotMember:
					sendClientMessage(w, ls, fmt.Sprintf(msgGuildaNaoEMembro, name))
				case world.GuildPromoteRefusalAlreadyRanked:
					sendClientMessage(w, ls, fmt.Sprintf(msgGuildaJaTemCargo, name))
				case world.GuildPromoteRefusalNoFreeRank:
					sendClientMessage(w, ls, msgGuildaSemCargoLivre)
				default:
					sendClientMessage(w, ls, fmt.Sprintf(msgGuildaPromoverFalhou, name))
				}
				return
			}
			d.guildaEsqueceQuadro(guildID)
			d.log.Info("guild member promoted offline", "conn", leaderConn, "guild", guildID,
				"target", name, "level", level)
			if ls != nil {
				if le := w.Entity(leaderConn); le != nil {
					le.Coin -= guildSubCost
					d.sendEtc(w, ls, le)
					w.SaveCharacterAsync(ls)
				}
				sendClientMessage(w, ls, fmt.Sprintf(msgGuildaSubOffline, name))
			}
			// Quem entrou no jogo enquanto o banco decidia carregou o cargo antigo; sem
			// isto, o próximo save dele devolveria o membro comum ao banco.
			if ts, te := w.SessionByName(name); ts != nil && te != nil && te.Guild == guildID && te.GuildLevel == 0 {
				te.GuildLevel = level
				d.refreshGuildTag(w, te.ID)
				w.SaveCharacterAsync(ts)
			}
		}
	})
}

func (d *Dispatcher) handoverGuild(w *world.World, s *world.Session, args []byte) {
	e := w.Entity(s.Conn)
	if e == nil || s.Mode != world.UserPlay || e.Guild == 0 || e.GuildLevel != guildLeaderLevel {
		return
	}
	name := strings.TrimSpace(cstr(args))
	targetSession, target := w.SessionByName(name)
	if targetSession == nil || target == nil || target.ID == s.Conn {
		d.notify(w, s, NoticeNotConnected)
		return
	}
	if targetSession.Mode != world.UserPlay {
		return
	}
	if target.Guild != e.Guild {
		return
	}
	guildID := e.Guild
	oldAccountID, oldSlot := s.AccountID, s.Slot
	newAccountID, newSlot := targetSession.AccountID, targetSession.Slot
	leaderConn, targetConn := s.Conn, target.ID
	leaderSession, memberSession := s, targetSession
	p := w.Persistence()
	s.Mode = world.UserWaitDB
	targetSession.Mode = world.UserWaitDB
	w.GoDetached(func() func(*world.World) {
		err := p.TransferGuildLeader(context.Background(), guildID, oldAccountID, oldSlot, newAccountID, newSlot)
		return func(w *world.World) {
			ls := w.Session(leaderConn)
			if ls != leaderSession {
				ls = nil
			}
			ts := w.Session(targetConn)
			if ts != memberSession {
				ts = nil
			}
			if ls != nil && ls.Mode == world.UserWaitDB {
				ls.Mode = world.UserPlay
			}
			if ts != nil && ts.Mode == world.UserWaitDB {
				ts.Mode = world.UserPlay
			}
			if err != nil {
				d.log.Warn("handover guild failed", "conn", leaderConn, "guild", guildID, "err", err)
				if ls != nil {
					d.notify(w, ls, NoticeDBError)
				}
				return
			}
			if ls != nil {
				if le := w.Entity(leaderConn); le != nil && le.Guild == guildID {
					le.GuildLevel = 0
					d.refreshGuildTag(w, leaderConn)
					w.SaveCharacterAsync(ls)
				}
			}
			if ts != nil {
				if te := w.Entity(targetConn); te != nil && te.Guild == guildID {
					te.GuildLevel = guildLeaderLevel
					d.refreshGuildTag(w, targetConn)
					w.SaveCharacterAsync(ts)
				}
			}
		}
	})
}

// leaveGuild handles /sair and /abandonar. /expulsar <char> is handled by
// kickGuild; bare /expulsar keeps the legacy self-leave behavior.
func (d *Dispatcher) leaveGuild(w *world.World, s *world.Session) {
	e := w.Entity(s.Conn)
	if e == nil || e.Guild == 0 {
		return
	}
	d.guildaEsqueceQuadro(e.Guild) // antes de zerar: depois não há mais id
	e.Guild = 0
	e.GuildLevel = 0
	d.refreshGuildTag(w, s.Conn)
	w.SaveCharacterAsync(s)
	d.persistLeaveGuild(w, s)
}

func (d *Dispatcher) kickGuild(w *world.World, s *world.Session, args []byte) {
	e := w.Entity(s.Conn)
	if e == nil || e.Guild == 0 || e.GuildLevel == 0 {
		return
	}
	name := strings.TrimSpace(cstr(args))
	if name == "" {
		d.leaveGuild(w, s)
		return
	}
	targetSession, target := w.SessionByName(name)
	if targetSession == nil || target == nil {
		// FORA DO JOGO, A EXPULSÃO VAI PELO BANCO. Antes aqui era "não conectado"
		// e fim: membro que nunca mais voltava (bot, conta largada) ficava na
		// guilda para sempre.
		d.kickOffline(w, s, e, name)
		return
	}
	if target.Guild != e.Guild || target.ID == s.Conn || e.GuildLevel <= target.GuildLevel {
		return
	}
	guildName := guildDisplayName(w, target.Guild)
	d.guildaEsqueceQuadro(target.Guild) // antes de zerar: depois não há mais id
	target.Guild = 0
	target.GuildLevel = 0
	d.refreshGuildTag(w, target.ID)
	w.SaveCharacterAsync(targetSession)
	d.persistLeaveGuild(w, targetSession)
	// Was a MSG_MessagePanel with no body — shorter than the struct, so the
	// client read past the frame. The legacy says nothing here; a player who
	// just lost their guild tag is owed the reason.
	sendClientMessage(w, targetSession, fmt.Sprintf("Você foi expulso da guilda %s.", guildName))
}

// Frases do expulsar offline.
const (
	msgGuildaExpulsoOffline = "%s foi expulso da guilda."
	msgGuildaNaoEMembro     = "%s não é membro da sua guilda."
	msgGuildaCargoNaoDeixa  = "Você não pode expulsar %s: o cargo dele não é menor que o seu."
	msgGuildaExpulsarFalhou = "Não foi possível expulsar %s agora. Tente de novo."
)

// kickOffline expulsa pelo dbServer um membro que não está no jogo. As regras de
// cargo são conferidas lá, no banco, na mesma transação da expulsão.
//
// Aqui, diferente do online, QUEM EXPULSA OUVE O RESULTADO: não há etiqueta sumindo
// da cabeça de ninguém para mostrar que funcionou, e a lista do painel só se
// atualiza no próximo pedido.
func (d *Dispatcher) kickOffline(w *world.World, s *world.Session, e *world.Entity, name string) {
	guildID := e.Guild
	accountID, slot := s.AccountID, s.Slot
	p := w.Persistence()
	w.Go(s, func() func(*world.World, *world.Session) {
		motivo, err := p.KickOfflineGuildMember(context.Background(), guildID, accountID, slot, name)
		return func(w *world.World, s *world.Session) {
			if err != nil {
				d.log.Warn("kick offline guild member failed", "conn", s.Conn, "guild", guildID,
					"target", name, "err", err)
				sendClientMessage(w, s, fmt.Sprintf(msgGuildaExpulsarFalhou, name))
				return
			}
			switch motivo {
			case world.GuildKickRefusalNone:
				d.guildaEsqueceQuadro(guildID)
				d.log.Info("guild member kicked offline", "conn", s.Conn, "guild", guildID, "target", name)
				sendClientMessage(w, s, fmt.Sprintf(msgGuildaExpulsoOffline, name))
			case world.GuildKickRefusalNotMember:
				sendClientMessage(w, s, fmt.Sprintf(msgGuildaNaoEMembro, name))
			case world.GuildKickRefusalOutranked:
				sendClientMessage(w, s, fmt.Sprintf(msgGuildaCargoNaoDeixa, name))
			default:
				sendClientMessage(w, s, fmt.Sprintf(msgGuildaExpulsarFalhou, name))
			}
		}
	})
}

func (d *Dispatcher) summonGuild(w *world.World, s *world.Session) {
	e := w.Entity(s.Conn)
	if e == nil || e.Guild == 0 || e.GuildLevel == 0 || world.Village(e.X, e.Y) < 0 {
		return
	}
	count := 0
	w.ForEachPlaying(s.Conn, func(ts *world.Session, te *world.Entity) {
		if count >= 350 || te.Guild != e.Guild {
			return
		}
		x, y := e.X+int16(w.Rand().Intn(3)), e.Y+int16(w.Rand().Intn(3))
		if ex, ey, ok := w.EmptyCellNear(x, y); ok {
			x, y = ex, ey
		}
		d.doTeleport(w, ts, x, y)
		count++
	})
}

func (d *Dispatcher) guildTax(w *world.World, s *world.Session, text string) bool {
	e := w.Entity(s.Conn)
	if e == nil || e.Guild == 0 || e.GuildLevel != guildLeaderLevel {
		return true
	}
	fields := strings.Fields(text)
	if len(fields) != 2 {
		return true
	}
	tax, ok := parseSmallInt(fields[1])
	if !ok || tax < 0 || tax > 30 {
		return true
	}
	now := d.now()
	for i := range d.guildZones {
		z := &d.guildZones[i]
		if z.ChargeGuild != e.Guild {
			continue
		}
		if !podeMudarImposto(z.TaxChangedAt, now) {
			sendClientMessage(w, s, msgImpostoSoPorSemana)
			return true
		}
		z.CityTax = uint8(tax)
		z.TaxChangedAt = now
		d.persistGuildZone(w, s, *z)
		break
	}
	return true
}

// O IMPOSTO MUDA UMA VEZ POR SEMANA DE CALENDÁRIO, e a semana vira na segunda-feira
// à 00:00 de Brasília (pedido de 29/09/2026; o legado era uma por dia, TaxChanged[i]).
//
// Não é "de 7 em 7 dias" de propósito: a guerra de cidades é no domingo, e contando
// 7 dias da última troca, a troca feita no sábado seguraria a semana seguinte
// inteira. Com a virada na segunda, cada semana tem a sua troca. E quem ganha a
// cidade no domingo não herda a espera do dono anterior: a troca de dono zera a
// hora (store.SaveGuildZone e esqueceGuildaApagada).
//
// A hora fica no banco (guild_zone.tax_changed_at), e por isso um reinício não
// libera troca nenhuma.
const msgImpostoSoPorSemana = "O imposto só muda uma vez por semana. Libera de novo na segunda-feira."

// fusoDaSemana é o horário de Brasília. Fixo em -3 porque o Brasil não tem horário
// de verão desde 2019, e um fuso fixo não depende do tzdata na imagem do servidor.
var fusoDaSemana = time.FixedZone("BRT", -3*60*60)

// inicioDaSemana é a segunda-feira 00:00 (Brasília) da semana de t.
func inicioDaSemana(t time.Time) time.Time {
	b := t.In(fusoDaSemana)
	dias := (int(b.Weekday()) + 6) % 7 // segunda = 0 ... domingo = 6
	return time.Date(b.Year(), b.Month(), b.Day()-dias, 0, 0, 0, 0, fusoDaSemana)
}

// podeMudarImposto: nunca mudou, ou a última troca foi antes da segunda desta semana.
func podeMudarImposto(ultima, agora time.Time) bool {
	return ultima.IsZero() || ultima.Before(inicioDaSemana(agora))
}

func parseSmallInt(s string) (int, bool) {
	n := 0
	if s == "" {
		return 0, false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return 0, false
		}
		n = n*10 + int(r-'0')
	}
	return n, true
}

// A ALIANÇA E A GUERRA DECLARADA ENTRE GUILDAS SAÍRAM.
//
// Aqui moravam o guildAlly (0x0E12), o war (0x0E0E) e o guildRelay que os dois
// chamavam. Eram o par que se fazia com item, e a Hanna tirou os dois do jogo.
// Saíram daqui e das rotas: sem rota, o pacote cai no caminho de mensagem
// desconhecida, em vez de executar meio caminho.
//
// A GUERRA DE CIDADE CONTINUA INTEIRA. Ela é outra coisa: mora na torre
// (towerwar.go) e nunca passou por nenhuma destas funções.
//
// E O applyGuildRelation CONTINUA RODANDO na partida, para as linhas de
// guild_relation que já existem no banco. Elas não nascem mais, mas as antigas
// SEGUEM VALENDO até alguém apagar — o que é um DELETE explícito e não acontece
// sozinho. Deixá-las valendo é a escolha conservadora: apagar relação de guilda em
// migração é mexer no jogo de quem não pediu.
//
// O sendWarInfoToGuild (0x03A8) saiu junto, porque ficou sem quem o chamasse: o
// guildRelay era o único. E ISSO NÃO TIRA NADA DE NINGUÉM — ele só era mandado no
// instante da declaração, nunca no login, então quem entrava depois de uma guerra
// declarada já não recebia o aviso antes desta mudança. A aliada e a guerra das
// linhas antigas continuam aparecendo, com nome, na aba Informações do painel
// (montaInfoDaGuilda).

// challange handles _MSG_Challange (0x028E): status/collection for a guild zone.
func (d *Dispatcher) challange(w *world.World, s *world.Session, _ protocol.Header, payload []byte) {
	zoneParm, ok := protocol.StandardParm(payload)
	if !ok || zoneParm < 0 || int(zoneParm) >= len(d.guildZones) {
		return
	}
	e := w.Entity(s.Conn)
	if e == nil {
		return
	}
	z := &d.guildZones[zoneParm]
	if z.ChargeGuild == e.Guild && e.GuildLevel == guildLeaderLevel && z.TaxVault > 0 {
		coin := z.TaxVault
		if coin > 2_000_000_000-int64(e.Coin) {
			coin = 2_000_000_000 - int64(e.Coin)
		}
		if coin > 0 {
			e.Coin += int32(coin)
			z.TaxVault -= coin
			d.sendEtc(w, s, e)
			w.SaveCharacterAsync(s)
			d.persistGuildZone(w, s, *z)
		}
	}
}

// challangeConfirm handles _MSG_ChallangeConfirm (0x028F). We model the bid as
// Parm1=zone, Parm2=coin; the exact NPC target variant remains capture-pending.
func (d *Dispatcher) challangeConfirm(w *world.World, s *world.Session, _ protocol.Header, payload []byte) {
	zoneParm, coinParm, ok := protocol.StandardParm2(payload)
	if !ok || zoneParm < 0 || int(zoneParm) >= len(d.guildZones) || coinParm <= 0 {
		return
	}
	e := w.Entity(s.Conn)
	if e == nil || e.Guild == 0 || e.GuildLevel != guildLeaderLevel || e.Coin < coinParm {
		return
	}
	z := &d.guildZones[zoneParm]
	if z.ChargeGuild == e.Guild || int64(coinParm) <= z.ChallengeMoney {
		return
	}
	e.Coin -= coinParm
	z.ChallengeGuild = e.Guild
	z.ChallengeMoney = int64(coinParm)
	d.sendEtc(w, s, e)
	w.SaveCharacterAsync(s)
	d.persistGuildZone(w, s, *z)
}

func (d *Dispatcher) refreshGuildTag(w *world.World, id int) {
	e := w.Entity(id)
	if e == nil {
		return
	}
	body := protocol.EncodeCreateMobBody(createMobFrom(w, e, 0))
	w.ForEachInView(id, func(vs *world.Session, _ *world.Entity) {
		w.SendTo(vs, protocol.Header{Type: protocol.MsgCreateMob, ID: protocol.IDScene}, body)
	})
	if s := w.Session(id); s != nil && s.Mode == world.UserPlay {
		w.SendTo(s, protocol.Header{Type: protocol.MsgCreateMob, ID: protocol.IDScene}, body)
	}
}

func (d *Dispatcher) persistGuildMember(w *world.World, actor, member *world.Session, e *world.Entity) {
	if member == nil || e == nil {
		return
	}
	accountID, slot, name, guildID, level := member.AccountID, member.Slot, e.Name, e.Guild, e.GuildLevel
	// O quadro guardado pelo Painel de Guilda descreve uma guilda que acabou de
	// mudar, então ele vai fora (guildapainel.go). Sem isto, um recém-entrado ou
	// um recém-promovido demoraria até trinta segundos para aparecer certo — que
	// é justamente o meio minuto em que alguém vai conferir.
	d.guildaEsqueceQuadro(guildID)
	p := w.Persistence()
	w.Go(actor, func() func(*world.World, *world.Session) {
		err := p.SetGuildMember(context.Background(), accountID, slot, name, guildID, level)
		return func(_ *world.World, _ *world.Session) {
			if err != nil {
				d.log.Warn("persist guild member failed", "account", accountID, "slot", slot, "guild", guildID, "err", err)
			}
		}
	})
}

func (d *Dispatcher) persistLeaveGuild(w *world.World, s *world.Session) {
	accountID, slot := s.AccountID, s.Slot
	p := w.Persistence()
	w.Go(s, func() func(*world.World, *world.Session) {
		apagada, err := p.LeaveGuild(context.Background(), accountID, slot)
		return func(w *world.World, _ *world.Session) {
			if err != nil {
				d.log.Warn("persist leave guild failed", "account", accountID, "slot", slot, "err", err)
				return
			}
			if apagada != 0 {
				d.esqueceGuildaApagada(w, apagada)
			}
		}
	})
}

// esqueceGuildaApagada solta o que o tmServer guarda de uma guilda que o
// dbServer apagou por ter ficado sem ninguém: o nome (que o /create consulta),
// os buffs ligados, o quadro do painel, e a posse de cidade, torre e Kefra.
// Nenhum jogador a carrega mais, então não há etiqueta de ninguém para refazer.
//
// A POSSE É LIMPA AQUI SEM GRAVAR: o dbServer já zerou as mesmas colunas na
// transação que apagou a guilda. O que importa é a memória não devolver o id
// velho ao banco no próximo persistGuildZone, que grava a cidade inteira.
func (d *Dispatcher) esqueceGuildaApagada(w *world.World, guilda uint16) {
	nome := guildDisplayName(w, guilda)
	w.ForgetGuild(guilda)
	delete(d.guildaBuffs, guilda)
	d.guildaEsqueceQuadro(guilda)
	for i := range d.guildZones {
		z := &d.guildZones[i]
		if z.ChargeGuild == guilda {
			z.ChargeGuild = 0
			z.TaxChangedAt = time.Time{}
		}
		if z.ChallengeGuild == guilda {
			z.ChallengeGuild, z.ChallengeMoney = 0, 0
		}
	}
	if d.events.towerOwner == guilda {
		d.events.towerOwner = 0
		d.towerState.OwnerGuild = 0
	}
	if d.kefraGuildID == int32(guilda) {
		d.kefraGuildID = 0
	}
	d.log.Info("guilda vazia apagada", "guild", nome, "id", guilda)
}

func (d *Dispatcher) persistGuildZone(w *world.World, s *world.Session, z world.GuildZone) {
	p := w.Persistence()
	w.Go(s, func() func(*world.World, *world.Session) {
		err := p.SaveGuildZone(context.Background(), z)
		return func(_ *world.World, _ *world.Session) {
			if err != nil {
				d.log.Warn("persist guild zone failed", "zone", z.Zone, "err", err)
			}
		}
	})
}

// msgDaRecusaDeGuilda escolhe a frase pelo motivo que o dbServer deu.
//
// O DESCONHECIDO CAI NA FRASE GERAL, e é de propósito: um dbServer mais novo pode mandar
// um motivo que esta versão não conhece, e nesse caso é melhor não afirmar nada do que
// afirmar o motivo errado. Foi exatamente o motivo errado — "confira o nome" — que custou
// horas de procura no dia em que o problema era o ouro.
//
// E POR ISSO A FRASE GERAL NÃO FALA EM NOME. Ela sobrou para o motivo desconhecido e para
// o CharacterGone, e em nenhum dos dois o nome é o problema; mandar conferir o nome ali
// seria a mesma mentira, só menor.
func msgDaRecusaDeGuilda(m world.GuildRefusal) string {
	switch m {
	case world.GuildRefusalNameTaken:
		return msgGuildNomeEmUso
	case world.GuildRefusalNotEnoughCoin:
		return msgGuildSemOuro
	case world.GuildRefusalAlreadyInGuild:
		return msgGuildJaTemGuilda
	case world.GuildRefusalNoFreeSlot:
		return msgGuildSemVaga
	default:
		// Inclui o CharacterGone, que é raro e que a pessoa não consegue consertar
		// sozinha: a frase geral manda tentar de novo, e é o que resolve.
		return msgGuildCriacaoRecusada
	}
}
