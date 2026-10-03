package world

import (
	"context"
	"time"
)

// A FILA DE GRAVAÇÃO DO FILTRO DAS FADAS.
//
// O filtro (handler/fadas_filtro.go) muda na memória na hora e vai ao banco em
// seguida, por uma RPC só dele (SaveFadaFiltro), fora do save do personagem. A
// fila mora no MUNDO, por personagem (conta e slot), e não na sessão, porque a
// sessão acaba antes da gravação:
//
//   - a volta de uma gravação presa à sessão é descartada quando o jogador sai
//     (callbackEvent), e a mudança que esperava atrás dela nunca ia ao banco. O
//     banco ficava com o estado antigo, e no login seguinte o filtro podia voltar
//     LIGADO depois de o jogador tê-lo desligado — descartando saque;
//   - o desligamento do servidor (shutdown) fecha o laço, e nenhuma volta roda
//     mais. O que estava na fila entra nas gravações de saída (fadaFiltrosDeSaida).
//
// UMA GRAVAÇÃO POR VEZ por personagem: a que chegasse por último ao banco
// mandaria, mesmo sendo a mais velha. A mudança que chega com uma no ar fica em
// `pendente` (só a mais nova interessa: cada estado é a lista inteira).
//
// NA FALHA, A MEMÓRIA VOLTA AO QUE ESTÁ NO BANCO. O que o jogador vê tem de ser o
// que vale depois do relogin; se o painel mostrasse "desligado" com o banco
// guardando "ligado", o relogin descartaria saque sem ele saber. Como uma falha
// por prazo pode ter gravado mesmo assim, o estado confirmado é REGRAVADO uma
// vez (o reparo), para o banco voltar a concordar com a tela.

// fadaFiltroPrazo é o teto de uma gravação do filtro.
const fadaFiltroPrazo = 5 * time.Second

type fadaFiltroChave struct {
	conta int64
	slot  int
}

// fadaFiltroFila é o estado da gravação do filtro de um personagem. Loop-only.
type fadaFiltroFila struct {
	// confirmado é o último estado que se sabe estar no banco: o do login, e
	// depois o de cada gravação que voltou sem erro.
	confirmado FadaFiltroSalvo
	// noAr é o estado que a gravação em curso leva; fim fecha quando ela volta do
	// banco (é o que o desligamento espera antes de gravar por cima).
	noAr *FadaFiltroSalvo
	fim  chan struct{}
	// reparo: a gravação no ar já é a segunda tentativa. Se falhar, a fila para.
	reparo bool
	// pendente é o estado mais novo, à espera da volta da que está no ar.
	pendente *FadaFiltroSalvo
	// falhou avisa o jogador, no laço, depois de a memória voltar ao confirmado.
	falhou func(*World, *Session, *Entity)
}

// GravaFadaFiltro leva o filtro de um personagem ao banco, fora do laço. antes é
// o estado da memória antes desta mudança; novo, o de agora. falhou roda no laço
// se a gravação falhar e o personagem ainda estiver em jogo, com a memória já de
// volta ao que o banco tem. Loop-only.
func (w *World) GravaFadaFiltro(antes, novo FadaFiltroSalvo, falhou func(*World, *Session, *Entity)) {
	if w.persist == nil {
		return
	}
	k := fadaFiltroChave{novo.Conta, novo.Slot}
	f := w.fadaFiltros[k]
	if f == nil {
		// Sem fila, a memória de antes da mudança é o que o banco tem.
		f = &fadaFiltroFila{confirmado: antes}
		w.fadaFiltros[k] = f
	}
	f.falhou = falhou
	if f.noAr != nil {
		f.pendente = &novo
		return
	}
	w.fadaFiltroDispara(k, f, novo, false)
}

// FadaFiltroACaminho devolve o estado do filtro que ainda está indo ao banco
// para este personagem. O login usa: quem sai e volta antes de a gravação
// terminar leria do banco o estado velho. Loop-only.
func (w *World) FadaFiltroACaminho(conta int64, slot int) (FadaFiltroSalvo, bool) {
	f := w.fadaFiltros[fadaFiltroChave{conta, slot}]
	switch {
	case f == nil:
		return FadaFiltroSalvo{}, false
	case f.pendente != nil:
		return *f.pendente, true
	case f.noAr != nil:
		return *f.noAr, true
	}
	return FadaFiltroSalvo{}, false
}

// FadaFiltrosNaFila diz quantos personagens têm gravação do filtro a caminho.
func (w *World) FadaFiltrosNaFila() int { return len(w.fadaFiltros) }

func (w *World) fadaFiltroDispara(k fadaFiltroChave, f *fadaFiltroFila, estado FadaFiltroSalvo, reparo bool) {
	p := w.persist
	fim := make(chan struct{})
	f.noAr, f.fim, f.reparo, f.pendente = &estado, fim, reparo, nil
	// No saveWG, como os saves de saída: o desligamento espera o que está no ar.
	w.saveWG.Add(1)
	go func() {
		defer w.saveWG.Done()
		ctx, cancel := context.WithTimeout(context.Background(), fadaFiltroPrazo)
		err := p.SaveFadaFiltro(ctx, estado.Conta, estado.Slot, estado.Ligado, estado.Itens)
		cancel()
		close(fim)
		// Volta do MUNDO, e não da sessão: tem de rodar com o jogador fora.
		select {
		case w.callbacks <- worldCallbackEvent{cb: func(w *World) { w.fadaFiltroVoltou(k, f, err) }}:
		case <-w.done:
		}
	}()
}

// fadaFiltroVoltou é a volta de uma gravação, no laço.
func (w *World) fadaFiltroVoltou(k fadaFiltroChave, f *fadaFiltroFila, err error) {
	if w.fadaFiltros[k] != f || f.noAr == nil {
		return
	}
	escrito, eraReparo := *f.noAr, f.reparo
	f.noAr, f.fim, f.reparo = nil, nil, false
	if err == nil {
		f.confirmado = escrito
	} else {
		w.savesFalhados.Add(1)
		w.log.Error("filtro da fada: não gravou", "conta", k.conta, "slot", k.slot,
			"reparo", eraReparo, "err", err)
	}
	// Há estado mais novo: é ele que interessa, tenha a anterior gravado ou não.
	if prox := f.pendente; prox != nil {
		w.fadaFiltroDispara(k, f, *prox, false)
		return
	}
	if err == nil || eraReparo {
		delete(w.fadaFiltros, k)
		return
	}
	s, e := w.fadaFiltroEmJogo(k)
	if e == nil {
		// Saiu do jogo: não há tela para corrigir. Tenta o mesmo estado mais uma vez.
		w.fadaFiltroDispara(k, f, escrito, true)
		return
	}
	e.FadaFiltroLigado, e.FadaFiltro = f.confirmado.Ligado, f.confirmado.Itens
	if f.falhou != nil {
		f.falhou(w, s, e)
	}
	w.fadaFiltroDispara(k, f, f.confirmado, true)
}

// fadaFiltroEmJogo acha a sessão e o personagem em jogo de uma chave.
func (w *World) fadaFiltroEmJogo(k fadaFiltroChave) (*Session, *Entity) {
	for _, s := range w.sessions {
		if s == nil || s.Mode != UserPlay || s.AccountID != k.conta || s.Slot != k.slot {
			continue
		}
		if e := w.entities[s.Conn]; e != nil {
			return s, e
		}
	}
	return nil, nil
}

// fadaFiltroDeSaida tira a fila de um personagem e devolve a gravação que o
// desligamento tem de fazer por ela, ou nil se não há nada a caminho. A gravação
// espera a que está no ar voltar (para não chegar ao banco ANTES dela e ser
// coberta pelo estado velho) e grava o estado mais novo. Loop-only; o que
// devolve é seguro fora do laço.
func (w *World) fadaFiltroDeSaida(k fadaFiltroChave) func(ctx context.Context) error {
	f := w.fadaFiltros[k]
	if f == nil {
		return nil
	}
	delete(w.fadaFiltros, k)
	var estado FadaFiltroSalvo
	switch {
	case f.pendente != nil:
		estado = *f.pendente
	case f.noAr != nil:
		estado = *f.noAr
	default:
		return nil
	}
	p, fim := w.persist, f.fim
	return func(ctx context.Context) error {
		if fim != nil {
			select {
			case <-fim:
			case <-ctx.Done():
				return ctx.Err()
			}
		}
		return p.SaveFadaFiltro(ctx, estado.Conta, estado.Slot, estado.Ligado, estado.Itens)
	}
}
