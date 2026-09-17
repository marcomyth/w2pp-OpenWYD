package handler

import (
	"fmt"

	"github.com/jeanluca/w2pp-openwyd/internal/domain"
	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/world"
)

// O teto de XP por rodada do Mortal (decidido em 17/09/2026). Regra NOSSA.
//
// Por personagem (conta e posição do personagem, donoDaEntrada), a cada rodada do
// relógio das arenas (questClearTicks), toda XP que
// o Mortal recebe soma no máximo o teto da faixa do nível que ele tem NO MOMENTO
// do ganho; com o dobro ligado, o teto do dobro. Os valores vêm do painel
// (tetorodada_config.go).
//
// - Morte (a própria e a parte do grupo, que conta em quem recebe), prêmio do
//   Castelo, XP de cura e os 10% do troféu que vão para o grupo: passando do
//   teto, paga até o teto e o resto se perde.
// - Troféu (4117-4121): limitado no DROP, não no uso (pedido de 17/09). O espaço
//   do troféu só cai para quem recebe o saque enquanto a XP de troféu reservada
//   na rodada é menor que a metade do teto; quando cai, o valor dele entra na hora
//   no total da rodada, até o que ainda cabe. Usar o troféu depois não recusa, não
//   corta e não soma de novo.
//   O jogador vê quantos troféus ainda cabem: na entrada da arena, a cada troféu
//   que cai e no /xp, pela mesma conta da trava (trofeusQueCabem).
// - O troféu é do personagem: não vai ao chão nem ao baú da conta (item.go), e a
//   troca e a lojinha já recusam EF_NOTRADE (trade.go, autotrade.go). Sem isso, um
//   alt no mesmo baú farmaria troféu para o principal.
// - Poeira de Fada: ISENTA. A oferta é finita (saiu de venda e drop) e cortar
//   gastaria o item quase sem efeito (useFairyDust).
// - Arch e Celestial: sem teto.
//
// Tudo em memória e zerado no pulso do relógio. Um relog não zera; um reinício do
// servidor zera, o que é aceitável.

// donoDaEntrada é o personagem, e não a sessão: um relog cai na mesma chave.
type donoDaEntrada struct {
	conta int64
	slot  int
}

func donoDe(s *world.Session) donoDaEntrada {
	return donoDaEntrada{conta: s.AccountID, slot: s.Slot}
}

// passoDaArena é o índice do passo em quest256Steps, achado pela bandeira.
func passoDaArena(step quest256Step) int {
	passo := 0
	for i := range quest256Steps {
		if quest256Steps[i].flag == step.flag {
			passo = i
		}
	}
	return passo
}

type xpDaRodada struct {
	total, trofeu int64
	avisado       bool // o aviso de teto já saiu nesta rodada
	avisadoTrofeu bool // o aviso de troféus da rodada já saiu
}

// msgTrofeuDoPersonagem é a recusa de levar o troféu ao chão ou ao baú.
const msgTrofeuDoPersonagem = "O troféu é do personagem: não vai ao chão nem ao baú da conta."

// ehTrofeuDeQuest diz se o item é um dos cinco troféus da Quest 256.
func ehTrofeuDeQuest(index int16) bool {
	return index >= itemQuestRewardBase && index <= itemQuestRewardLast
}

// faixaDoTeto é o índice de domain.RoundXPCapTopLevels do nível guardado.
func faixaDoTeto(nivel int32) int {
	for i, topo := range domain.RoundXPCapTopLevels {
		if nivel <= topo {
			return i
		}
	}
	return len(domain.RoundXPCapTopLevels) - 1
}

// tetoDoGanho é o teto total da rodada para este personagem agora; ok falso
// quando não há teto (não é Mortal, ou a faixa está zerada no painel).
func (d *Dispatcher) tetoDoGanho(e *world.Entity) (int64, bool) {
	if e == nil || e.ClassMaster != classMasterMortal {
		return 0, false
	}
	base, dobro := d.valoresDoTeto()
	i := faixaDoTeto(e.Level)
	v := base[i]
	if d.expEvents.DoubleMode {
		v = dobro[i]
	}
	return v, v > 0
}

// cabeNaRodada é quanto ainda cabe agora, sem gravar nada. ok falso = sem teto.
func (d *Dispatcher) cabeNaRodada(s *world.Session, e *world.Entity) (int64, bool) {
	if s == nil {
		return 0, false // sem sessão não há de quem contar; só acontece fora de jogo
	}
	teto, ok := d.tetoDoGanho(e)
	if !ok {
		return 0, false
	}
	st := d.xpDaRodada[donoDe(s)]
	return max(teto-st.total, 0), true
}

// cortaXPDaRodada devolve quanto do ganho pode ser pago e grava. Na primeira vez
// que corta na rodada, avisa o jogador com o tempo até a próxima.
func (d *Dispatcher) cortaXPDaRodada(w *world.World, s *world.Session, e *world.Entity, ganho int64) int64 {
	if ganho <= 0 {
		return ganho
	}
	cabe, ok := d.cabeNaRodada(s, e)
	if !ok {
		return ganho
	}
	pago := min(ganho, cabe)
	k := donoDe(s)
	st := d.xpDaRodada[k]
	st.total += pago
	if pago < ganho && !st.avisado {
		st.avisado = true
		sendClientMessage(w, s, d.textoTetoDaRodada())
		d.log.Info("teto de XP da rodada", "conta", s.AccountName, "conn", s.Conn, "nivel", e.Level,
			"total", st.total, "trofeu", st.trofeu, "cortado", ganho-pago)
	}
	if d.xpDaRodada == nil {
		d.xpDaRodada = make(map[donoDaEntrada]xpDaRodada)
	}
	d.xpDaRodada[k] = st
	return pago
}

func (d *Dispatcher) textoTetoDaRodada() string {
	seg := d.segundosAteALimpeza()
	return fmt.Sprintf("Você chegou ao limite de XP desta rodada. A próxima começa em %d min %02d s.", seg/60, seg%60)
}

func (d *Dispatcher) textoTrofeusDaRodada() string {
	seg := d.segundosAteALimpeza()
	return fmt.Sprintf("Você já recebeu os troféus desta rodada. A próxima começa em %d min %02d s.", seg/60, seg%60)
}

// trofeusQueCabem é quantos troféus de valor `valor` ainda podem cair na rodada, e
// é a ÚNICA conta disso: a trava do drop (trofeuPodeCair) e o número que o
// jogador vê (as mensagens abaixo e o /xp) passam por aqui, para o número mostrado
// nunca discordar do que cai. Um troféu cai enquanto a XP de troféu reservada é
// menor que a metade do teto E o valor inteiro cabe no que falta do total; como
// cada um que cai soma o valor nas duas contas, são dois limites contados de uma
// vez: os que ainda começam abaixo da metade e os que cabem inteiros no total.
func trofeusQueCabem(teto int64, st xpDaRodada, valor int64) int64 {
	metade := teto / 2
	if valor <= 0 || st.trofeu >= metade {
		return 0
	}
	pelaMetade := (metade - st.trofeu + valor - 1) / valor
	pelaSobra := max(teto-st.total, 0) / valor
	return min(pelaMetade, pelaSobra)
}

// trofeuPodeCair diz se o troféu pode cair para quem recebe o saque
// (trofeusQueCabem). Sem o limite do total, quem enche o total matando no campo e
// depois entra na arena ganharia troféus que não reservam nada e, com o uso livre,
// passaria do ritmo. Na primeira recusa da rodada, avisa. Item que não é troféu,
// troféu sem valor de XP e quem não tem teto sempre podem.
func (d *Dispatcher) trofeuPodeCair(w *world.World, e *world.Entity, it world.Item) bool {
	if !ehTrofeuDeQuest(it.Index) {
		return true
	}
	s := w.Session(e.ID)
	if s == nil {
		return true
	}
	teto, ok := d.tetoDoGanho(e)
	if !ok {
		return true
	}
	valor := d.valorDoTrofeu(it)
	if valor <= 0 {
		return true // não paga XP, não há o que limitar
	}
	k := donoDe(s)
	st := d.xpDaRodada[k]
	if trofeusQueCabem(teto, st, valor) > 0 {
		return true
	}
	if !st.avisadoTrofeu {
		st.avisadoTrofeu = true
		if d.xpDaRodada == nil {
			d.xpDaRodada = make(map[donoDaEntrada]xpDaRodada)
		}
		d.xpDaRodada[k] = st
		sendClientMessage(w, s, d.textoTrofeusDaRodada())
		d.log.Info("teto da rodada: troféu não caiu", "conta", s.AccountName, "conn", s.Conn, "nivel", e.Level, "trofeu", st.trofeu)
	}
	return false
}

// valorDoTrofeu é a XP que o troféu vale no uso, pela quantidade que ele traz.
func (d *Dispatcher) valorDoTrofeu(it world.Item) int64 {
	rate, ok := d.questRates.Tier(int(it.Index) - itemQuestRewardBase)
	if !ok || rate.MortalExp <= 0 {
		return 0
	}
	return rate.MortalExp * int64(itemAmount(it))
}

// reservaTrofeuDaRodada põe na rodada o valor do troféu que acabou de cair, inteiro
// na parte do troféu e no total (trofeuPodeCair já conferiu que cabe). A morte da
// rodada só ocupa o que sobrar, e guardar o troféu para usar depois não passa do
// ritmo.
func (d *Dispatcher) reservaTrofeuDaRodada(w *world.World, e *world.Entity, it world.Item) {
	if !ehTrofeuDeQuest(it.Index) {
		return
	}
	s := w.Session(e.ID)
	if s == nil {
		return
	}
	teto, ok := d.tetoDoGanho(e)
	if !ok {
		return
	}
	valor := d.valorDoTrofeu(it)
	if valor <= 0 {
		return
	}
	k := donoDe(s)
	st := d.xpDaRodada[k]
	st.trofeu += valor
	st.total = min(st.total+valor, teto)
	// A cada troféu que cai, quantos faltam; no último, o aviso de sempre, uma vez,
	// que a recusa seguinte não repete.
	faltam := trofeusQueCabem(teto, st, valor)
	avisaFim := faltam == 0 && !st.avisadoTrofeu
	if avisaFim {
		st.avisadoTrofeu = true
	}
	if d.xpDaRodada == nil {
		d.xpDaRodada = make(map[donoDaEntrada]xpDaRodada)
	}
	d.xpDaRodada[k] = st
	switch {
	case faltam > 0:
		sendClientMessage(w, s, fmt.Sprintf("Troféu: faltam %d nesta rodada.", faltam))
	case avisaFim:
		sendClientMessage(w, s, d.textoTrofeusDaRodada())
	}
}

// nomesDasArenas segue a ordem de quest256Steps, para o /xp dizer de qual quest é
// o número.
var nomesDasArenas = [...]string{"Coveiro", "Jardim dos Deuses", "Kaizen", "Hidras", "Elfos"}

// trofeusDaArena é quantos troféus da arena `passo` (índice em quest256Steps) o
// personagem ainda pode receber nesta rodada; ok falso quando não há teto ou o
// troféu não vale XP.
func (d *Dispatcher) trofeusDaArena(s *world.Session, e *world.Entity, passo int) (int64, bool) {
	if s == nil || passo < 0 || passo >= len(quest256Steps) {
		return 0, false
	}
	teto, ok := d.tetoDoGanho(e)
	if !ok {
		return 0, false
	}
	valor := d.valorDoTrofeu(world.Item{Index: int16(itemQuestRewardBase + passo)})
	if valor <= 0 {
		return 0, false
	}
	return trofeusQueCabem(teto, d.xpDaRodada[donoDe(s)], valor), true
}

// enviarTrofeusDaRodada diz, na entrada da arena, quantos troféus a rodada ainda dá.
func (d *Dispatcher) enviarTrofeusDaRodada(w *world.World, s *world.Session, e *world.Entity, passo int) {
	if n, ok := d.trofeusDaArena(s, e, passo); ok {
		sendClientMessage(w, s, fmt.Sprintf("Troféus nesta rodada: %d.", n))
	}
}

// linhaTrofeusDaRodada é a linha do /xp para quem está na faixa de nível de uma
// das cinco quests.
func (d *Dispatcher) linhaTrofeusDaRodada(s *world.Session, e *world.Entity) (string, bool) {
	if e == nil {
		return "", false
	}
	for i, step := range quest256Steps {
		if e.Level >= step.minLevel && e.Level < step.maxLevel {
			n, ok := d.trofeusDaArena(s, e, i)
			if !ok {
				return "", false
			}
			return fmt.Sprintf("Troféus da rodada: faltam %d (quest %s)", n, nomesDasArenas[i]), true
		}
	}
	return "", false
}

// zeraXPDaRodada abre a rodada nova. Chamado pelo pulso.
func (d *Dispatcher) zeraXPDaRodada() {
	d.xpDaRodada = nil
}
