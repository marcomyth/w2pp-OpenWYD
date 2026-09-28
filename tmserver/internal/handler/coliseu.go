package handler

import (
	"time"

	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/protocol"
	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/world"
	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/worldevents"
)

// O Coliseu do legado, portado inteiro e DESLIGADO por padrão (decisão de
// 24/09/2026: "desativado, mas funcional"). Quem liga é o GM, com
// /gm coliseu ligar (gmcoliseu.go); o interruptor não sobrevive a um
// reinício. Desligado, nada aqui toca o mundo: os portões ficam como o boot os
// deixa (abertos e não desenhados), nenhum relógio anda e nenhuma regra da
// arena vale.
//
// São três peças, todas na mesma arena (2604-2648 × 1708-1744):
//
//   - o evento de ondas das 20h (ColoState, Server.cpp:6942-7057): tranca a
//     entrada, abre os portões de dentro e solta cinco ondas de Ciclopes; na
//     hora de novato, também expulsa e barra quem tem nível 150 ou mais;
//   - a Batalha Real (BrState, Server.cpp:6842-6939): três rodadas por hora, só
//     com um item de prêmio configurado. Na arena ninguém tem nome, capa, guilda
//     nem grupo, e um veneno fecha as pontas a partir do minuto 7;
//   - o Coliseu {N} (ProcessSecMinTimer.cpp:1550-1562, MobKilled.cpp:2502-2524),
//     que no legado é uma casca: um aviso por boot às 11h55 ou 19h55, a limpeza
//     de hora em hora de um bloco de mapa onde nada nasce, e um drop dos
//     Fantasmas dos blocos 4623-4634, que ficam no Portão Infernal.
//
// Fora do escopo, de propósito:
//   - o PotionReady (Server.cpp:7059-7066), que gera o bloco 22 às 20h55 da
//     hora de novato. Não é do Coliseu, e o boot daqui já põe o bloco no mundo;
//   - o GenerateMob que copia para os monstros de rosto 219/220 a capa do
//     jogador de índice BrState (Server.cpp:3572-3588). BrState vale 1 a 3, então
//     o legado lê a capa das conexões 1 a 3 — um valor de estado usado como
//     índice de jogador, e não uma regra que dê para portar.

var (
	// coliseuArena é a caixa que o legado testa em todo lugar (ClearAreaLevel,
	// _MSG_Action, GetCreateMob, _MSG_Attack, as duas de grupo e o chat).
	coliseuArena = areaBox{2604, 1708, 2648, 1744}
	// Os avisos da Batalha Real alcançam uma caixa maior, que pega a porta. A
	// de "inicia em 2min" começa em 1708, as outras em 1688 (Server.cpp:6862 e
	// :6897 contra :6909).
	batalhaAvisoPronta = areaBox{2580, 1708, 2665, 1765}
	batalhaAviso       = areaBox{2580, 1688, 2665, 1765}
	// O veneno da Batalha Real: faixas nas pontas do lado oeste, que crescem no
	// minuto 9 (ProcessSecMinTimer.cpp:1680-1698).
	batalhaVenenoEstreito = [2]areaBox{{2608, 1708, 2622, 1711}, {2608, 1739, 2622, 1743}}
	batalhaVenenoLargo    = [2]areaBox{{2608, 1708, 2622, 1718}, {2608, 1733, 2622, 1743}}
	// O chat também é embaralhado na Nova Guerra de Noatun durante a Batalha
	// Real (_MSG_MessageChat.cpp:167) — o legado junta as duas caixas no mesmo if.
	batalhaChatNoatun = areaBox{896, 1405, 1150, 1538}
	// coliseuNMapa é o bloco de mapa (27,11) que o Coliseu {N} limpa aos :10
	// (ClearMapa, Server.cpp:9628). Nenhum gerador nasce nele.
	coliseuNMapa = areaBox{27 * 128, 11 * 128, 27*128 + 127, 11*128 + 127}
)

const (
	// coliseuHoraPadrao é a GuildHour e a NewbieHour do legado (Server.cpp:642
	// e :660). Iguais, e é por isso que os Orcs nunca saem (worldevents).
	coliseuHoraPadrao = 20
	// batalhaHoraPadrao é a BRHour (Server.cpp:625). No legado ela também vem
	// do arquivo de configuração (value[5], Server.cpp:1461).
	batalhaHoraPadrao = 19

	// batalhaPremioX/Y é o altar onde o prêmio cai (Server.cpp:6861).
	batalhaPremioX, batalhaPremioY = 2621, 1726

	// batalhaVenenoTicks: o veneno roda quando SecCounter%16 == 0, e o SecCounter
	// anda a cada 500 ms — a cada 8 s (ProcessSecMinTimer.cpp:1680).
	batalhaVenenoTicks = 8
	// batalhaVenenoDano tira 2.000 de vida e nunca mata: para em 1 (SendDamage,
	// Server.cpp:5970-6003).
	batalhaVenenoDano = 2000
	// envEffectVeneno é o efeito do veneno na tela (SendEnvEffect ..., 32, 0).
	envEffectVeneno = 32

	// Os Fantasmas cujo abate o Coliseu {N} premia (MobKilled.cpp:2503).
	coliseuNPrimeiroBloco = 4623
	coliseuNUltimoBloco   = 4634
	// O drop dele: um em 14 para cada um dos três.
	coliseuNSorteio = 14

	// nomeAnonimo é o nome que a Batalha Real põe em todos (GetFunc.cpp:1189).
	nomeAnonimo = "??????"
)

// coliseuNItens é o drop do Coliseu {N}, na ordem do sorteio: Resto de
// Oriharucon, Resto de Lactolerium e Moeda de Prata.
var coliseuNItens = [3]int16{419, 420, 4026}

// msgColiseuN é o aviso do Coliseu {N}, com o "!" do legado
// (ProcessSecMinTimer.cpp:1554). O NPC Xamã que ele cita não existe no legado.
const msgColiseuN = "!O Coliseu {N} iniciará em 5 minutos. Acesse pelo NPC Xamã."

// coliseuPortao é um dos sete portões da arena no InitItem.csv (linhas 13-19,
// ids 12-18 do legado).
type coliseuPortao struct {
	item    int16
	x, y    int16
	interno bool
}

// Os dois de fora são SetColoseumDoor (ids 12 e 13, Janela de Aço 472); os
// cinco de dentro, SetColoseumDoor2 (14 a 18, Entrada de Guilda 471), numa
// parede em x 2624 que separa o lado dos jogadores do lado das ondas.
var coliseuPortoes = [7]coliseuPortao{
	{472, 2603, 1717, false},
	{472, 2603, 1733, false},
	{471, 2624, 1739, true},
	{471, 2624, 1731, true},
	{471, 2624, 1725, true},
	{471, 2624, 1719, true},
	{471, 2624, 1711, true},
}

// estadoDoColiseu junta o interruptor, a configuração e os relógios. Loop-only.
type estadoDoColiseu struct {
	ligado bool

	// GuildHour e NewbieHour (os comandos guildhour/newbiehour do legado,
	// imple.cpp:811-826).
	horaGuilda, horaNovato int
	// BRHour e BRItem. Item 0 deixa a Batalha Real desligada, como no legado.
	horaBatalha int
	itemBatalha int16

	ondas   worldevents.Coliseu
	batalha worldevents.Batalha
	// grade e minutoDaRodada são o BrGrid e o BrMod: lidos do relógio a cada
	// passo, e é deles que saem o nível barrado e o tamanho do veneno.
	grade, minutoDaRodada int

	// portoes são os ids de chão dos sete portões: 0 = ainda não procurados,
	// -1 = ausente do InitItem.
	portoes [7]int

	// anunciouN é o g_quests.Annoucement: o legado nunca o volta a zero, então
	// o aviso do Coliseu {N} sai uma vez por boot (aqui, por vez que se liga).
	anunciouN bool
	// limpezaN é a hora cheia da última limpeza do bloco (27,11). O legado
	// testa o segundo exato; o tique daqui pode pular um segundo, então ele
	// marca a hora.
	limpezaN time.Time
}

func novoEstadoDoColiseu() estadoDoColiseu {
	return estadoDoColiseu{
		horaGuilda:  coliseuHoraPadrao,
		horaNovato:  coliseuHoraPadrao,
		horaBatalha: batalhaHoraPadrao,
	}
}

// batalhaAtiva é o `BrState && BRItem > 0` do legado: a regra que tira grupo,
// chat, guilda e nível da arena.
func (d *Dispatcher) batalhaAtiva() bool {
	c := &d.coliseu
	return c.ligado && c.itemBatalha > 0 && c.batalha.Fase() != worldevents.BatalhaParada
}

// tickColiseu é a parte do Coliseu nos relógios do legado: o Coliseu {N} no de
// segundo, o veneno a cada 8 s, e as duas máquinas no GuildProcess, a cada 12 s.
func (d *Dispatcher) tickColiseu(w *world.World) {
	c := &d.coliseu
	if !c.ligado {
		return
	}
	now := d.now()
	d.tickColiseuN(w, now)
	if d.tickCount%batalhaVenenoTicks == 0 {
		d.venenoDaBatalha(w)
	}
	if d.tickCount%minTimerTicks != 0 {
		return
	}
	// A ordem do GuildProcess: a Batalha Real antes do ColoState.
	d.passoDaBatalha(w, now)
	d.passoDoColiseu(w, now)
}

// passoDoColiseu anda o ColoState e aplica o que ele manda.
func (d *Dispatcher) passoDoColiseu(w *world.World, now time.Time) {
	c := &d.coliseu
	hora, minuto := c.ondas.Relogio(now, c.horaGuilda)
	d.aplicaPassoDoColiseu(w, c.ondas.Passo(hora, minuto, c.horaGuilda, c.horaNovato))
}

func (d *Dispatcher) aplicaPassoDoColiseu(w *world.World, p worldevents.ColiseuPasso) {
	if p.Vazio() {
		return
	}
	if p.FechaEntrada {
		d.portoesDoColiseu(w, false, world.StateLocked)
		if p.LimiteNovato {
			d.clearAreaLevel(w, coliseuArena, 150, 400)
		}
	}
	gerados := 0
	for _, bloco := range p.Ondas {
		gerados += d.gerarOndaDoColiseu(w, bloco)
	}
	if p.AbreInternos {
		d.portoesDoColiseu(w, true, world.StateOpen)
	}
	apagados := 0
	if p.Fim {
		d.portoesDoColiseu(w, false, world.StateOpen)
		d.portoesDoColiseu(w, true, world.StateLocked)
		apagados = d.apagarOndasDoColiseu(w)
	}
	d.log.Info("coliseu: passo", "fase", d.coliseu.ondas.Fase(), "zera", p.Zera, "fecha_entrada", p.FechaEntrada,
		"limite_novato", p.LimiteNovato, "ondas", p.Ondas, "gerados", gerados,
		"abre_internos", p.AbreInternos, "fim", p.Fim, "apagados", apagados)
}

// gerarOndaDoColiseu é o GenerateColoseum (Server.cpp:6539): de 4 a 7 grupos
// do bloco. O sorteio da quantidade sai do fluxo de eventos (eventRNG), não do
// fluxo de paridade do mundo, como os outros relógios de evento daqui.
func (d *Dispatcher) gerarOndaDoColiseu(w *world.World, bloco int) int {
	grupos := 4 + d.eventRNG.Intn(4)
	n := 0
	for range grupos {
		ids := w.GenerateMob(bloco)
		d.revealSpawned(w, ids)
		n += len(ids)
	}
	return n
}

// apagarOndasDoColiseu é o DeleteColoseum (Server.cpp:6547): tira do mundo os
// vivos dos seis blocos das ondas, sem a fila de 15 s (são blocos de evento).
func (d *Dispatcher) apagarOndasDoColiseu(w *world.World) int {
	var ids []int
	w.ForEachMob(func(id int, e *world.Entity) {
		if e.HP <= 0 || !blocoDasOndas(int(e.GenIndex)) {
			return
		}
		ids = append(ids, id)
	})
	for _, id := range ids {
		w.DespawnMob(id, 2)
	}
	return len(ids)
}

func blocoDasOndas(idx int) bool {
	for _, b := range worldevents.ColiseuBlocosGuilda {
		if idx == b {
			return true
		}
	}
	for _, b := range worldevents.ColiseuBlocosNovato {
		if idx == b {
			return true
		}
	}
	return false
}

// clearAreaLevel é o ClearAreaLevel (Server.cpp:6311): manda para a cidade quem
// está na caixa com nível entre minLv e maxLv, levantando antes quem está caído.
func (d *Dispatcher) clearAreaLevel(w *world.World, box areaBox, minLv, maxLv int32) {
	w.ForEachPlaying(-1, func(s *world.Session, e *world.Entity) {
		if !box.contains(e.X, e.Y) || e.Level < minLv || e.Level > maxLv {
			return
		}
		if e.HP <= 0 {
			e.HP = 2
			d.sendScore(w, s, e)
		}
		d.recall(w, s, e)
	})
}

// passoDaBatalha anda o BrState. O legado embrulha tudo — inclusive a leitura
// do BrGrid e do BrMod — em `BRItem > 0` (Server.cpp:6842).
func (d *Dispatcher) passoDaBatalha(w *world.World, now time.Time) {
	c := &d.coliseu
	if c.itemBatalha <= 0 {
		return
	}
	hora, minuto := c.batalha.Relogio(now, c.horaBatalha)
	c.grade, c.minutoDaRodada = minuto/worldevents.BatalhaMinutos, minuto%worldevents.BatalhaMinutos
	p := c.batalha.Passo(hora, minuto, c.horaBatalha)
	if p.Vazio() {
		return
	}
	// O legado muda o BrState antes de avisar e redesenhar, e o redesenho já
	// sai com o anonimato da fase nova.
	d.atualizaAnonimato(w)
	switch {
	case p.Prepara:
		d.preparaBatalha(w, p.Grade)
	case p.Inicia:
		d.iniciaBatalha(w, p.Grade)
	case p.Premia:
		d.premiaBatalha(w)
	}
	if p.Reabre {
		d.portoesDoColiseu(w, false, world.StateOpen)
	}
	d.log.Info("coliseu: batalha real", "fase", c.batalha.Fase(), "grade", p.Grade,
		"prepara", p.Prepara, "inicia", p.Inicia, "premia", p.Premia, "reabre", p.Reabre)
}

// textoDaBatalha lê a fala do Language.txt, com o texto do arquivo como
// reserva (Language.txt:246-252).
func (d *Dispatcher) textoDaBatalha(chave, reserva string) string {
	if d.lang != nil {
		if t, ok := d.lang.Text(chave); ok && t != "" {
			return t
		}
	}
	return reserva
}

// preparaBatalha é o minuto 0 da rodada (Server.cpp:6914-6933).
func (d *Dispatcher) preparaBatalha(w *world.World, grade int) {
	switch grade {
	case 0:
		d.noticeAreaPainel(w, batalhaAvisoPronta, d.textoDaBatalha("_NN_BR_Ready1", "Batalha Real para Nv < 100 inicia no coliseu em 2min."))
		d.clearAreaLevel(w, coliseuArena, 100, 400)
	case 1:
		d.noticeAreaPainel(w, batalhaAvisoPronta, d.textoDaBatalha("_NN_BR_Ready2", "Batalha Real para Nv < 200 incia no coliseu em 2min."))
		d.clearAreaLevel(w, coliseuArena, 200, 400)
	default:
		d.noticeAreaPainel(w, batalhaAvisoPronta, d.textoDaBatalha("_NN_BR_Ready3", "Batalha Real para qualquer nível inicia no coliseu em 2min."))
	}
	d.redesenhaArena(w, true)
}

// iniciaBatalha é o minuto 5 (Server.cpp:6872-6906): tranca a entrada, avisa,
// tira de novo quem passou do nível e redesenha a arena já anônima.
func (d *Dispatcher) iniciaBatalha(w *world.World, grade int) {
	d.portoesDoColiseu(w, false, world.StateLocked)
	switch grade {
	case 0:
		d.noticeAreaPainel(w, batalhaAviso, d.textoDaBatalha("_NN_BR_Start1", "Batalha Real está iniciando para Nv < 100."))
		d.clearAreaLevel(w, coliseuArena, 100, 400)
	case 1:
		d.noticeAreaPainel(w, batalhaAviso, d.textoDaBatalha("_NN_BR_Start2", "Batalha Real está iniciando para Nv < 200."))
		d.clearAreaLevel(w, coliseuArena, 200, 400)
	default:
		d.noticeAreaPainel(w, batalhaAviso, d.textoDaBatalha("_NN_BR_Start3", "Batalha Real está iniciando para qualquer nível."))
	}
	d.redesenhaArena(w, true)
}

// premiaBatalha é o minuto 13 (Server.cpp:6853-6866): o prêmio cai no altar.
func (d *Dispatcher) premiaBatalha(w *world.World) {
	c := &d.coliseu
	if c.itemBatalha > 0 && int(c.itemBatalha) < maxItemList {
		item := world.Item{Index: c.itemBatalha}
		rotate := uint8(d.eventRNG.Intn(4))
		id := -1
		if x, y, ok := w.EmptyItemCell(batalhaPremioX, batalhaPremioY); ok {
			if id = w.CreateGroundItem(item, x, y); id >= 0 {
				body := protocol.EncodeCreateItemBody(protocol.CreateItemData{
					GridX: uint16(x), GridY: uint16(y), ItemID: uint16(world.GroundItemIDOffset + id),
					Item: protocol.WireItem{Index: item.Index}, Rotate: rotate, State: world.StateOpen,
				})
				w.ForEachInViewAt(x, y, -1, func(s *world.Session, _ *world.Entity) {
					w.SendTo(s, protocol.Header{Type: protocol.MsgCreateItem, ID: protocol.IDScene}, body)
				})
			}
		}
		d.log.Info("coliseu: prêmio da batalha real", "item", c.itemBatalha, "ground_id", id)
	}
	d.noticeAreaPainel(w, batalhaAviso, d.textoDaBatalha("_NN_BR_Rewarded", "Prêmio do vencedor está no altar."))
}

// redesenhaArena é o laço do legado que, a cada aviso, reenvia quem está na
// arena (SendGridMob) e, com tiraDoGrupo, o tira do grupo (RemoveParty).
func (d *Dispatcher) redesenhaArena(w *world.World, tiraDoGrupo bool) {
	w.ForEachPlaying(-1, func(s *world.Session, e *world.Entity) {
		if !coliseuArena.contains(e.X, e.Y) {
			return
		}
		body := protocol.EncodeCreateMobBody(createMobFrom(w, e, 0))
		w.ForEachInView(s.Conn, func(vs *world.Session, _ *world.Entity) {
			w.SendTo(vs, protocol.Header{Type: protocol.MsgCreateMob, ID: protocol.IDScene}, body)
		})
		if tiraDoGrupo && e.Leader != 0 {
			d.leaveParty(w, s.Conn)
		}
	})
}

// atualizaAnonimato liga a caixa do anonimato enquanto há BrState.
func (d *Dispatcher) atualizaAnonimato(w *world.World) {
	c := &d.coliseu
	b := coliseuArena
	w.SetAnonimato(c.ligado && c.batalha.Fase() != worldevents.BatalhaParada, b.x1, b.y1, b.x2, b.y2)
}

// venenoDaBatalha fecha as pontas do lado oeste durante a luta
// (ProcessSecMinTimer.cpp:1680-1698): nada até o minuto 7 da rodada, faixas
// estreitas no 7 e no 8, largas do 9 em diante.
func (d *Dispatcher) venenoDaBatalha(w *world.World) {
	c := &d.coliseu
	if c.itemBatalha <= 0 || c.batalha.Fase() != worldevents.BatalhaEmLuta {
		return
	}
	var faixas [2]areaBox
	switch {
	case c.minutoDaRodada >= 9:
		faixas = batalhaVenenoLargo
	case c.minutoDaRodada >= 7:
		faixas = batalhaVenenoEstreito
	default:
		return
	}
	for _, b := range faixas {
		d.danoDoVeneno(w, b)
	}
	for _, b := range faixas {
		body := protocol.EncodeEnvEffect(b.x1, b.y1, b.x2, b.y2, envEffectVeneno, 0)
		cx, cy := b.center()
		w.ForEachInViewAt(cx, cy, -1, func(s *world.Session, _ *world.Entity) {
			w.SendTo(s, protocol.Header{Type: protocol.MsgEnvEffect, ID: protocol.IDScene}, body)
		})
	}
}

// danoDoVeneno é o SendDamage: 2.000 de vida, parando em 1.
func (d *Dispatcher) danoDoVeneno(w *world.World, b areaBox) {
	w.ForEachPlaying(-1, func(s *world.Session, e *world.Entity) {
		if e.HP <= 0 || !b.contains(e.X, e.Y) {
			return
		}
		d.applyEnvDamage(w, s, e, min(batalhaVenenoDano, e.HP-1))
	})
}

// coliseuDepoisDoPasso é o fim do _MSG_Action (_MSG_Action.cpp:309-337): quem
// acabou de andar para dentro da arena e passou do nível daquele momento volta
// para a cidade.
func (d *Dispatcher) coliseuDepoisDoPasso(w *world.World, s *world.Session, e *world.Entity) {
	c := &d.coliseu
	if !c.ligado {
		return
	}
	if d.batalhaAtiva() && coliseuArena.contains(e.X, e.Y) {
		lv := e.Level
		if (c.grade == 0 && lv >= 100 && lv < 1000) || (c.grade == 1 && lv >= 200 && lv < 1000) {
			d.recall(w, s, e)
		}
	}
	if c.ondas.Limite150() && e.Level >= 150 && coliseuArena.contains(e.X, e.Y) {
		d.recall(w, s, e)
	}
}

// batalhaTiraGrupo diz se a Batalha Real barra convite e aceite de grupo para
// quem está na arena (_MSG_SendReqParty.cpp:74, _MSG_AcceptParty.cpp:98). O
// legado recusa calado.
func (d *Dispatcher) batalhaTiraGrupo(e *world.Entity) bool {
	return d.batalhaAtiva() && world.IsPlayer(e.ID) && coliseuArena.contains(e.X, e.Y)
}

// falaDaBatalha embaralha a fala de quem está na arena ou na Nova Guerra de
// Noatun durante a Batalha Real: os seis primeiros caracteres viram "??????"
// (_MSG_MessageChat.cpp:165-170). Devolve o pacote a mandar.
func (d *Dispatcher) falaDaBatalha(e *world.Entity, payload []byte) []byte {
	if !d.batalhaAtiva() || !coliseuArena.contains(e.X, e.Y) && !batalhaChatNoatun.contains(e.X, e.Y) {
		return payload
	}
	out := append([]byte(nil), payload...)
	copy(out, nomeAnonimo)
	return out
}

// scoreParaEnvio é o score que sai para os clientes: na arena da Batalha Real
// o jogador vai sem guilda (SendScore, SendFunc.cpp:1284-1296).
func (d *Dispatcher) scoreParaEnvio(w *world.World, e *world.Entity) protocol.ScoreData {
	sc := d.computeScore(e)
	if world.IsPlayer(e.ID) && w.Anonimo(e.X, e.Y) {
		sc.Guild, sc.GuildLevel = 0, 0
	}
	return sc
}

// tickColiseuN é a parte do Coliseu {N} que corre no relógio de segundo.
func (d *Dispatcher) tickColiseuN(w *world.World, now time.Time) {
	c := &d.coliseu
	h, m := now.Hour(), now.Minute()
	if (h == 11 || h == 19) && m == 55 && !c.anunciouN {
		broadcastNotice(w, msgColiseuN)
		c.anunciouN = true
		d.log.Info("coliseu {N}: aviso")
	}
	cheia := now.Truncate(time.Hour)
	if m == 10 && !c.limpezaN.Equal(cheia) {
		c.limpezaN = cheia
		d.clearArea(w, coliseuNMapa)
	}
}

// coliseuNMorto é o drop do Coliseu {N}: um em 14 para cada item, com o bônus
// de drop zerado (SetItemBonus(&item, 0, 0, 0)), direto na bolsa de quem matou.
// Sorteia no fluxo do mundo, como o rand() do legado — e só com o Coliseu
// ligado, então não mexe na sequência de ninguém enquanto está desligado.
func (d *Dispatcher) coliseuNMorto(w *world.World, reward, mob *world.Entity) {
	if !d.coliseu.ligado || mob.GenIndex < coliseuNPrimeiroBloco || mob.GenIndex > coliseuNUltimoBloco {
		return
	}
	sorteio := w.Rand().Intn(coliseuNSorteio)
	if sorteio >= len(coliseuNItens) {
		return
	}
	it := world.Item{Index: coliseuNItens[sorteio]}
	d.rolarBonusDrop(w, &it, 0, 0)
	d.putMobDrop(w, reward, it)
}

// noticeAreaPainel é o SendNoticeArea do legado: SendClientMessage para quem
// está na caixa (SendFunc.cpp:247). O noticeArea do reino manda como fala de
// chat; este vai para o painel de mensagens, como o legado.
func (d *Dispatcher) noticeAreaPainel(w *world.World, box areaBox, texto string) {
	w.ForEachPlaying(-1, func(s *world.Session, e *world.Entity) {
		if box.contains(e.X, e.Y) {
			sendClientMessage(w, s, texto)
		}
	})
}
