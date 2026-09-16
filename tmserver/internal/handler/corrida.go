package handler

import (
	"fmt"
	"strings"

	"github.com/jeanluca/w2pp-openwyd/internal/droprule"
	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/protocol"
	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/world"
)

// Uma corrida é uma área fechada que um grupo tem só para si por um tempo. O
// líder entrega a chave ao NPC de fora; quem não é do grupo sai da área, os
// monstros da quest nascem (um trecho de blocos de evento no fim do NPCGener) e o
// grupo cai lá dentro com o relógio correndo. Um grupo por vez, no servidor
// inteiro: a área, os blocos e a varredura são de todos.
//
// Termina no relógio, alguns minutos depois que o boss cai (o tempo do saque), ou
// quando o grupo inteiro fica fora da área por um tempo. Aí os monstros da quest
// somem e quem ainda estiver dentro vai para a saída.
//
// É o motor do Castelo Orc (castelo_orc_run.go) com as coordenadas, a chave e os
// textos virando dados, para cada área nova do Atlas de Quests não copiar o motor
// inteiro. O Orc ainda roda a própria cópia, que tem portão e blocos do mundo
// aberto a segurar; passá-lo para cá é um passo à parte, com os testes dele como
// rede.
//
// Nada sobrevive a um reinício: um boot no meio encerra a corrida, como na Água,
// na Carta e no Orc.

// corridaSpec é o que muda de uma área para outra.
type corridaSpec struct {
	nome string // o nome da área nos logs

	chave       int16  // o item que o NPC pede e consome
	grauNPC     uint8  // o EF_GRADE0 do NPC (Merchant 100) que abre a corrida
	npcTemplate string // o arquivo do NPC em npc/
	npc         [2]int16

	caixa   areaBox  // a área que a corrida toma
	entrada [2]int16 // onde o grupo cai, cada um numa casa livre
	saida   [2]int16 // para onde vai quem é varrido da caixa

	genFirst, genLast int // os blocos da quest no NPCGener
	genBoss           int // o bloco cujo monstro, morto, corta o relógio para o saque
	genSeguidor       int // o bloco que volta a encher durante a corrida
	// genTropaFirst..genTropaLast também voltam a encher, no mesmo intervalo dos
	// seguidores. Zerados, a tropa não renasce.
	genTropaFirst, genTropaLast int

	// bossAposAbates segura o boss fora da abertura: ele só nasce quando o grupo
	// derruba essa quantidade de monstros da quest. Zero, nasce junto com o resto.
	bossAposAbates int
	// abatesAviso é de quantos em quantos abates o grupo ouve a contagem.
	abatesAviso int

	// Em segundos: a corrida, o saque depois do boss, o intervalo dos seguidores
	// e quanto tempo sem ninguém do grupo na área a encerra.
	duracao, saque, seguidorCada, abandono int

	textos corridaTextos
}

// corridaTextos são as falas da corrida. As que levam %d recebem minutos; a que
// leva %s, o nome da chave.
type corridaTextos struct {
	ocupada  string // %d: minutos até a área ficar livre
	soLider  string
	traga    string // %s: o nome da chave
	aberta   string // o NPC, ao abrir
	chegada  string // no painel de cada membro, ao cair na área
	estranho string // para quem estava dentro e não é do grupo
	fim      string // para quem estava dentro quando acabou
	relogio  string // %d: minutos que faltam, reenviado a cada minuto
	bossCaiu string
	abates   string // %d de %d: abates até agora e quantos chamam o boss
	bossVeio string // quando o boss nasce pelos abates
	// grupoCaiu é o aviso quando todo o grupo que está na área morreu.
	grupoCaiu string
}

// corridaEstado é a corrida em andamento. Zerado, não há corrida.
type corridaEstado struct {
	active        bool
	secondsLeft   int
	party         []int // as conexões dos jogadores admitidos na abertura
	leaderName    string
	bossDown      bool
	bossUp        bool // o boss já nasceu nesta corrida
	abates        int  // monstros da quest derrubados, fora o boss
	emptyFor      int
	sinceFollower int
}

// corrida junta a área, o NPC que a abre e o estado. É do laço do mundo.
type corrida struct {
	spec    *corridaSpec
	npcTmpl []byte // nil: o NPC não carregou e a corrida não abre
	npcID   int
	estado  corridaEstado
}

const (
	// corridaNPCCheckEvery é de quantos em quantos segundos o NPC é procurado e
	// levantado de novo se algo o tirou do mapa.
	corridaNPCCheckEvery = 10
	// corridaResyncEvery reenvia o relógio ao grupo, com os minutos em texto: o
	// contador gráfico só aparece com o GamePatch (timerfields.cpp), o texto
	// aparece para todo cliente.
	corridaResyncEvery = 60
)

func (c *corrida) doGrupo(conn int) bool {
	for _, p := range c.estado.party {
		if p == conn {
			return true
		}
	}
	return false
}

// protege diz se conn é do grupo de uma corrida aberta e está dentro da área
// dela: quem cuida dele ali é a corrida, e nenhuma varredura de fora o tira.
func (c *corrida) protege(conn int, x, y int16) bool {
	return c.estado.active && c.spec.caixa.contains(x, y) && c.doGrupo(conn)
}

// movimentoPermitido recusa a um estranho o passo para dentro da área enquanto a
// corrida está aberta.
func (c *corrida) movimentoPermitido(conn int, x, y int16) bool {
	if !c.estado.active || !c.spec.caixa.contains(x, y) {
		return true
	}
	return c.doGrupo(conn)
}

// corridaNPC é o clique no NPC da corrida: ele responde falando.
func (d *Dispatcher) corridaNPC(w *world.World, c *corrida, s *world.Session, e, npc *world.Entity) {
	// No painel também: o balão do NPC some rápido e, com o grupo de fora olhando
	// para a área, "tem um grupo lá dentro" passava despercebido.
	d.tentarAbrirCorrida(w, c, s, e, func(text string) {
		sendSay(w, npc, text)
		sendClientMessage(w, s, text)
	})
}

// avisoDeFora é o que ouve quem tenta entrar andando na área de uma corrida
// aberta por outro grupo.
func (c *corrida) avisoDeFora() string {
	return fmt.Sprintf(c.spec.textos.ocupada, (c.estado.secondsLeft+59)/60)
}

// tentarAbrirCorrida abre a corrida para o grupo de e quando a área está livre, e
// é o líder (ou está sozinho) e traz a chave, que é consumida.
func (d *Dispatcher) tentarAbrirCorrida(w *world.World, c *corrida, s *world.Session, e *world.Entity, say func(string)) {
	t := &c.spec.textos
	if c.estado.active {
		say(fmt.Sprintf(t.ocupada, (c.estado.secondsLeft+59)/60))
		return
	}
	// Um membro carrega a conexão do líder; só o líder ou quem está sozinho abre.
	if e.Leader != 0 {
		say(t.soLider)
		return
	}
	slot := -1
	for i := 0; i < activeCarryLimit(e); i++ {
		if e.Carry[i].Index == c.spec.chave {
			slot = i
			break
		}
	}
	if slot < 0 {
		// O catálogo escreve os nomes com sublinhado; o NPC os fala com espaço.
		say(fmt.Sprintf(t.traga, strings.ReplaceAll(d.itemName(c.spec.chave), "_", " ")))
		return
	}
	consumeOneItem(&e.Carry[slot])
	d.sendSlot(w, s, world.ItemPlaceCarry, slot, e.Carry[slot])
	d.abrirCorrida(w, c, e)
	say(t.aberta)
}

// abrirCorrida começa a corrida para o grupo de e.
func (d *Dispatcher) abrirCorrida(w *world.World, c *corrida, e *world.Entity) {
	sp := c.spec
	party := []int{e.ID}
	for _, id := range e.PartyList {
		// Pet também mora na PartyList; só entram jogadores.
		if id <= 0 || id == e.ID || !world.IsPlayer(id) {
			continue
		}
		if ms := w.Session(id); ms != nil && ms.Mode == world.UserPlay {
			party = append(party, id)
		}
	}
	c.estado = corridaEstado{active: true, secondsLeft: sp.duracao, party: party, leaderName: e.Name, bossUp: sp.bossAposAbates <= 0}

	// As sobras de uma corrida anterior saem pelo ClearGenerator, e não por uma
	// varredura de despawn: assim o contador de cada bloco cai junto e nada fica
	// na fila de 15 s para renascer.
	for idx := sp.genFirst; idx <= sp.genLast; idx++ {
		w.ClearGenerator(idx)
	}
	d.varrerCorrida(w, c, true)

	spawned := 0
	for idx := sp.genFirst; idx <= sp.genLast; idx++ {
		if idx == sp.genBoss && sp.bossAposAbates > 0 {
			continue // vem pelos abates (corridaMobMorto)
		}
		spawned += d.encherBloco(w, idx)
	}

	for _, conn := range party {
		if s := w.Session(conn); s != nil {
			// Uma casa para cada: teleporte para casa ocupada apaga o ocupante do
			// grid (SetEntityPos).
			x, y, ok := w.EmptyCellNear(sp.entrada[0], sp.entrada[1])
			if !ok {
				x, y = sp.entrada[0], sp.entrada[1]
			}
			d.doTeleport(w, s, x, y)
			d.enviarRelogioCorrida(w, c, s)
			sendClientMessage(w, s, sp.textos.chegada)
		}
	}
	d.log.Info("corrida aberta", "area", sp.nome, "leader", e.Name, "party", len(party), "mobs", spawned)
}

// encherBloco levanta um bloco até o teto dele e diz quantos nasceram. Um bloco
// de tropa enche grupo a grupo; GenerateMob devolve vazio quando chega lá. O
// limite é só um freio.
func (d *Dispatcher) encherBloco(w *world.World, idx int) int {
	n := 0
	for range 10 {
		ids := w.GenerateMob(idx)
		if len(ids) == 0 {
			break
		}
		d.revealSpawned(w, ids)
		n += len(ids)
	}
	return n
}

// varrerCorrida manda para a saída quem está na área: todos no fim, só quem não é
// do grupo na abertura. A equipe (moderador para cima) fica onde está, como o
// portão de movimento a deixa passar.
func (d *Dispatcher) varrerCorrida(w *world.World, c *corrida, soEstranhos bool) {
	sp := c.spec
	var fora []*world.Session
	w.ForEachPlaying(-1, func(s *world.Session, e *world.Entity) {
		if !sp.caixa.contains(e.X, e.Y) || s.AccessLevel >= world.AccessModerator {
			return
		}
		if soEstranhos && c.doGrupo(s.Conn) {
			return
		}
		fora = append(fora, s)
	})
	// O teleporte fica fora do laço: doTeleport mexe na posição e na visão.
	for _, s := range fora {
		if e := w.Entity(s.Conn); e != nil && e.HP <= 0 {
			e.HP = 2 // o fluxo do destino espera alguém vivo (clearArea)
		}
		d.doTeleport(w, s, sp.saida[0], sp.saida[1])
		if soEstranhos {
			sendClientMessage(w, s, sp.textos.estranho)
		} else {
			sendClientMessage(w, s, sp.textos.fim)
		}
	}
}

// enviarRelogioCorrida manda o relógio da corrida: o mesmo MsgStartTime, na mesma
// unidade (segundos), da Água, do Pesadelo e do Orc. O WYD.exe 7662 só o desenha
// nos campos do mapa que conhece; os outros, o GamePatch acrescenta
// (client/gamepatch/timerfields.cpp).
func (d *Dispatcher) enviarRelogioCorrida(w *world.World, c *corrida, s *world.Session) {
	body := protocol.EncodeStandardParm(int32(c.estado.secondsLeft))
	w.SendTo(s, protocol.Header{Type: protocol.MsgStartTime, ID: protocol.IDScene}, body)
}

// avisarGrupoCorrida manda o relógio e uma linha no painel a cada membro em jogo.
func (d *Dispatcher) avisarGrupoCorrida(w *world.World, c *corrida, text string) {
	for _, conn := range c.estado.party {
		if s := w.Session(conn); s != nil && s.Mode == world.UserPlay {
			d.enviarRelogioCorrida(w, c, s)
			sendClientMessage(w, s, text)
		}
	}
}

// corridaReenvioDevido diz se o relógio é reenviado neste segundo.
func corridaReenvioDevido(secondsLeft int) bool {
	return secondsLeft > 0 && secondsLeft%corridaResyncEvery == 0
}

// tickCorrida é o relógio de cada segundo, e quem mantém o NPC de pé.
func (d *Dispatcher) tickCorrida(w *world.World, c *corrida) {
	if d.tickCount%corridaNPCCheckEvery == 0 {
		d.garantirNPCCorrida(w, c)
	}
	r := &c.estado
	if !r.active {
		return
	}
	r.secondsLeft--
	if r.secondsLeft <= 0 {
		d.encerrarCorrida(w, c, "tempo")
		return
	}
	if corridaReenvioDevido(r.secondsLeft) {
		d.avisarGrupoCorrida(w, c, fmt.Sprintf(c.spec.textos.relogio, r.secondsLeft/60))
	}

	r.sinceFollower++
	if r.sinceFollower >= c.spec.seguidorCada {
		r.sinceFollower = 0
		d.revealSpawned(w, w.GenerateMob(c.spec.genSeguidor))
		for idx := c.spec.genTropaFirst; c.spec.genTropaLast > 0 && idx <= c.spec.genTropaLast; idx++ {
			d.encherBloco(w, idx)
		}
	}

	inside, vivo := grupoNaArea(r.party, c.spec.caixa, membroEmJogo(w))
	// O grupo inteiro caído lá dentro zera a corrida (pedido do Marco, 16/09):
	// ninguém de pé para levantar os outros, e a área fica livre para a próxima.
	if inside && !vivo {
		d.avisarGrupoCorrida(w, c, c.spec.textos.grupoCaiu)
		d.encerrarCorrida(w, c, "grupo morreu")
		return
	}
	if inside {
		r.emptyFor = 0
		return
	}
	r.emptyFor++
	if r.emptyFor >= c.spec.abandono {
		d.encerrarCorrida(w, c, "abandono")
	}
}

// grupoNaArea diz se algum membro do grupo em jogo está na área, e se algum dos
// que estão lá está vivo. É dos dois motores (este e o do Castelo Orc). membro
// devolve a entidade de quem está em jogo, ou nil.
func grupoNaArea(party []int, caixa areaBox, membro func(conn int) *world.Entity) (dentro, vivo bool) {
	for _, conn := range party {
		e := membro(conn)
		if e == nil || !caixa.contains(e.X, e.Y) {
			continue
		}
		dentro = true
		vivo = vivo || e.HP > 0
	}
	return dentro, vivo
}

// membroEmJogo é o membro de grupoNaArea no mundo: um jogador com sessão em jogo.
func membroEmJogo(w *world.World) func(conn int) *world.Entity {
	return func(conn int) *world.Entity {
		e := w.Entity(conn)
		if e == nil || e.Mode != world.MobUser {
			return nil
		}
		if s := w.Session(conn); s == nil || s.Mode != world.UserPlay {
			return nil
		}
		return e
	}
}

// corridaMobMorto conta um monstro da quest que caiu. O boss corta o relógio para
// o tempo do saque; os outros somam para chamá-lo, quando a área o segura até lá.
// Roda antes do despawn, como os outros ganchos de boss.
func (d *Dispatcher) corridaMobMorto(w *world.World, c *corrida, mob *world.Entity) {
	r := &c.estado
	sp := c.spec
	if !r.active || mob == nil {
		return
	}
	gen := int(mob.GenIndex)
	if gen < sp.genFirst || gen > sp.genLast {
		return
	}
	if gen != sp.genBoss {
		d.contarAbateCorrida(w, c)
		return
	}
	if r.bossDown {
		return
	}
	r.bossDown = true
	if r.secondsLeft > c.spec.saque {
		r.secondsLeft = c.spec.saque
	}
	d.avisarGrupoCorrida(w, c, c.spec.textos.bossCaiu)
	d.log.Info("corrida: boss caiu", "area", c.spec.nome, "leader", r.leaderName)
}

// contarAbateCorrida soma um abate e, na conta certa, levanta o boss no bloco
// dele. Depois que o boss nasce a contagem para: a área tem um boss só.
func (d *Dispatcher) contarAbateCorrida(w *world.World, c *corrida) {
	r := &c.estado
	sp := c.spec
	if r.bossUp {
		return
	}
	r.abates++
	if r.abates < sp.bossAposAbates {
		if sp.abatesAviso > 0 && r.abates%sp.abatesAviso == 0 {
			d.avisarGrupoCorrida(w, c, fmt.Sprintf(sp.textos.abates, r.abates, sp.bossAposAbates))
		}
		return
	}
	r.bossUp = true
	ids := w.GenerateMob(sp.genBoss)
	d.revealSpawned(w, ids)
	d.avisarGrupoCorrida(w, c, sp.textos.bossVeio)
	d.log.Info("corrida: boss nasceu", "area", sp.nome, "leader", r.leaderName, "abates", r.abates, "ids", len(ids))
}

// encerrarCorrida tira os monstros da quest e esvazia a área.
func (d *Dispatcher) encerrarCorrida(w *world.World, c *corrida, why string) {
	for idx := c.spec.genFirst; idx <= c.spec.genLast; idx++ {
		w.ClearGenerator(idx)
	}
	d.varrerCorrida(w, c, false)
	d.log.Info("corrida encerrada", "area", c.spec.nome, "leader", c.estado.leaderName, "why", why, "boss_down", c.estado.bossDown)
	c.estado = corridaEstado{}
}

// garantirNPCCorrida levanta o NPC da corrida quando ele não está de pé. Ele nasce
// aqui, e não pelo NPCGener, de propósito: um mercador (Merchant 100) no NPCGener
// passa a ser do overlay de NPCs quando W2PP_NPC_EDITING está ligado, e só
// apareceria depois de um `dbserver import-npcs`.
func (d *Dispatcher) garantirNPCCorrida(w *world.World, c *corrida) {
	if c.npcTmpl == nil {
		return
	}
	sp := c.spec
	if e := w.Entity(c.npcID); c.npcID >= world.MaxUser && e != nil && e.Mode != world.MobEmpty &&
		droprule.Canonical(e.TemplateName) == droprule.Canonical(sp.npcTemplate) {
		return
	}
	x, y, ok := w.EmptyCellNear(sp.npc[0], sp.npc[1])
	if !ok {
		return
	}
	id := w.SpawnMobAt(world.MobSpawn{Template: c.npcTmpl, TemplateName: sp.npcTemplate, X: x, Y: y, GenIndex: -1})
	if id < 0 {
		return
	}
	// Fora da fila de renascimento: quem o traz de volta é este guardião, não a fila.
	w.Entity(id).Template = nil
	c.npcID = id
	d.revealSpawned(w, []int{id})
	d.log.Info("corrida: npc de pé", "area", sp.nome, "id", id, "x", x, "y", y)
}
