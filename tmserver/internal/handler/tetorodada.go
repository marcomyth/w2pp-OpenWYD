package handler

import (
	"fmt"

	"github.com/jeanluca/w2pp-openwyd/internal/domain"
	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/world"
)

// O teto de XP por rodada do Mortal (decidido em 17/09/2026). Regra NOSSA.
//
// Por personagem (a mesma chave da entrada nas arenas: conta e posição do
// personagem), a cada rodada do relógio das arenas (questClearTicks), toda XP que
// o Mortal recebe soma no máximo o teto da faixa do nível que ele tem NO MOMENTO
// do ganho; com o dobro ligado, o teto do dobro. Os valores vêm do painel
// (tetorodada_config.go).
//
// - Morte (a própria e a parte do grupo, que conta em quem recebe), prêmio do
//   Castelo e XP de cura: passando do teto, paga até o teto e o resto se perde.
// - Troféu (4117-4121) e os 10% dele que vão para o grupo: além do total, somam no
//   máximo a metade do teto. O troféu que não cabe nada é recusado e fica na
//   bolsa; o que cabe em parte paga até o teto e é gasto.
// - Poeira de Fada: ISENTA. A oferta é finita (saiu de venda e drop) e cortar
//   gastaria o item quase sem efeito (useFairyDust).
// - Arch e Celestial: sem teto.
//
// Tudo em memória e zerado no pulso do relógio. Um relog não zera; um reinício do
// servidor zera, o que é aceitável.

type xpDaRodada struct {
	total, trofeu int64
	avisado       bool // o aviso de teto já saiu nesta rodada
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
func (d *Dispatcher) cabeNaRodada(s *world.Session, e *world.Entity, trofeu bool) (int64, bool) {
	if s == nil {
		return 0, false // sem sessão não há de quem contar; só acontece fora de jogo
	}
	teto, ok := d.tetoDoGanho(e)
	if !ok {
		return 0, false
	}
	st := d.xpDaRodada[donoDe(s)]
	cabe := teto - st.total
	if trofeu {
		cabe = min(cabe, teto/2-st.trofeu)
	}
	return max(cabe, 0), true
}

// cortaXPDaRodada devolve quanto do ganho pode ser pago e grava. Na primeira vez
// que corta na rodada, avisa o jogador com o tempo até a próxima.
func (d *Dispatcher) cortaXPDaRodada(w *world.World, s *world.Session, e *world.Entity, ganho int64, trofeu bool) int64 {
	if ganho <= 0 {
		return ganho
	}
	cabe, ok := d.cabeNaRodada(s, e, trofeu)
	if !ok {
		return ganho
	}
	pago := min(ganho, cabe)
	k := donoDe(s)
	st := d.xpDaRodada[k]
	st.total += pago
	if trofeu {
		st.trofeu += pago
	}
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

func (d *Dispatcher) textoTrofeuForaDoTeto() string {
	seg := d.segundosAteALimpeza()
	return fmt.Sprintf("O troféu passa do limite de XP desta rodada e ficou na bolsa. A próxima começa em %d min %02d s.", seg/60, seg%60)
}

// zeraXPDaRodada abre a rodada nova. Chamado pelo pulso.
func (d *Dispatcher) zeraXPDaRodada() {
	d.xpDaRodada = nil
}
