package world

import (
	"context"
	"time"
)

// IntervaloDoBatimento é de quanto em quanto tempo esta execução diz ao banco que
// continua viva pelas contas que tem.
//
// TEM DE SER BEM MENOR QUE O PRAZO DO OUTRO LADO (store.PrazoDaPosse, 90 s): três
// batimentos cabem dentro do prazo, então uma pausa de coletor de lixo ou um
// soluço de rede não tiram a conta de quem está jogando. Um teste prende os dois
// números.
const IntervaloDoBatimento = 30 * time.Second

// posseTick renova a posse das contas desta execução, de tempos em tempos.
//
// Loop-only. Recebe a hora de fora para que o teste possa andar com o relógio em
// vez de dormir.
func (w *World) posseTick(agora time.Time) {
	if w.parEpoca <= 0 || w.batimentoParado {
		return
	}
	if !w.ultimoBatimento.IsZero() && agora.Sub(w.ultimoBatimento) < IntervaloDoBatimento {
		return
	}
	w.ultimoBatimento = agora
	contas := w.contasComSessao()
	if len(contas) == 0 {
		return
	}
	p, epoca := w.persist, w.parEpoca
	w.GoDetached(func() func(*World) {
		minhas, err := p.BaterPelasContas(context.Background(), epoca, contas)
		return func(w *World) {
			if err != nil {
				// Falhar o batimento não é perder a posse: o prazo ainda não venceu,
				// e derrubar jogador por um soluço de rede seria o remédio pior.
				w.log.Warn("batimento da posse falhou", "contas", len(contas), "err", err)
				return
			}
			w.largarContasPerdidas(contas, minhas)
		}
	})
}

// PararOBatimento desliga a renovação da posse.
//
// O DESLIGAMENTO CHAMA ISTO ANTES DE ESVAZIAR O SERVIDOR. Sem isso, um batimento
// atrasado poderia re-carimbar contas que o dreno acabou de soltar, e elas ficariam
// presas a um processo que já saiu — até o prazo vencer, com o jogador batendo na
// porta. Loop-only.
func (w *World) PararOBatimento() { w.batimentoParado = true }

// contasComSessao lista, sem repetir, as contas que esta execução tem em mão.
// Loop-only.
func (w *World) contasComSessao() []int64 {
	vistas := make(map[int64]bool, len(w.sessions))
	var contas []int64
	for _, s := range w.sessions {
		if s == nil || s.AccountID == 0 || vistas[s.AccountID] {
			continue
		}
		vistas[s.AccountID] = true
		contas = append(contas, s.AccountID)
	}
	return contas
}

// largarContasPerdidas trata as contas que deixaram de ser desta execução.
//
// ISTO É GRAVE E POR ISSO É ERRO NO LOG. Significa que o banco ficou lento o
// bastante para o prazo vencer e outra execução tomar a conta — e a partir daí
// continuar jogando nela é produzir a divergência que a posse existe para impedir.
//
// A ordem é: tentar gravar, depois derrubar. A gravação pode até ser recusada pela
// guarda de ordem (se a sessão nova já gravou, a minha é velha e tem de perder
// mesmo); se a sessão nova ainda não gravou nada, ela passa e nada se perde.
// Loop-only.
func (w *World) largarContasPerdidas(pedidas, minhas []int64) {
	if len(pedidas) == len(minhas) {
		return
	}
	continua := make(map[int64]bool, len(minhas))
	for _, id := range minhas {
		continua[id] = true
	}
	for _, id := range pedidas {
		if continua[id] {
			continue
		}
		w.log.Error("a posse desta conta foi perdida para outra execucao; salvando e derrubando",
			"account", id, "epoca", w.parEpoca)
		for _, s := range w.sessions {
			if s == nil || s.AccountID != id {
				continue
			}
			if w.temMochilaViva(s) {
				w.salvarParAsync(s)
			}
			w.Close(s)
		}
	}
}
