package world

import (
	"context"
	"fmt"
	"time"
)

// O DESLIGAMENTO TEM POUCOS SEGUNDOS, e quem manda neles é a plataforma.
//
// Em 27/09/2026 a troca de versão matou o processo antigo uns 4 s depois do
// SIGTERM. O desligamento de então dormia 2 s pelo aviso ANTES de gravar e
// gravava um jogador por vez: saiu o primeiro de seis, e os outros cinco
// voltaram ao último save — o do login, para quem caçava sem deslogar, porque o
// jogo não tinha save periódico. Tudo aqui existe por causa disso:
//
//   - o aviso não espera antes (announceShutdown); os saves começam na hora;
//   - os saves correm em paralelo, com prazo, e cada falha aparece no log;
//   - o save de desligamento solta a posse da conta, como o de logout, para o
//     servidor novo carregar a conta sem esperar o prazo de 90 s;
//   - o save periódico (savePeriodicoTick) limita o que qualquer queda leva.

// prazoDoDesligamento é o teto das gravações do desligamento. Um banco que não
// grava em 20 s não gravaria em mais; e o processo precisa sair antes que a
// plataforma o mate, para o log dizer o que ficou para trás.
const prazoDoDesligamento = 20 * time.Second

// gravadoresNoDesligamento é quantas gravações vão ao banco ao mesmo tempo.
// Paralelo porque o tempo é da plataforma; limitado porque um servidor cheio
// não pode abrir mil chamadas de uma vez contra o mesmo dbServer.
const gravadoresNoDesligamento = 16

// IntervaloDoSavePeriodico é de quanto em quanto tempo um personagem em jogo é
// gravado sem que nada tenha pedido. É o teto do que uma queda leva: antes dele,
// quem caçava horas sem trocar, compor ou deslogar só era gravado no logout e
// no desligamento — e perdia as horas inteiras se o desligamento falhasse.
const IntervaloDoSavePeriodico = 5 * time.Minute

// gravacaoDeSaida é um save do desligamento: o instantâneo é tirado no laço, a
// gravação corre fora dele, e depois roda de volta no laço quando ela confirma.
type gravacaoDeSaida struct {
	conta int64
	oQue  string
	grava func(ctx context.Context) error
	// depois é a arrumação da memória quando a gravação confirmou. Loop-only.
	depois func(w *World)
}

// shutdown grava quem está no servidor e fecha as sessões.
//
// avisou diz se announceShutdown mandou o aviso a alguém: só então as sessões
// esperam o ShutdownGrace — contado a partir do aviso — antes de fechar. Loop-only.
func (w *World) shutdown(avisou bool) {
	inicio := time.Now()
	close(w.done) // signal conn goroutines to stop sending events
	// Um batimento atrasado re-carimbaria a posse que este desligamento solta.
	w.PararOBatimento()

	prazo := w.cfg.ShutdownSaveDeadline
	if prazo <= 0 {
		prazo = prazoDoDesligamento
	}
	ctx, cancel := context.WithTimeout(context.Background(), prazo)
	defer cancel()

	gravacoes := w.gravacoesDeSaida()
	erros := gravarEmParalelo(ctx, gravacoes, gravadoresNoDesligamento)
	salvos, falhas := 0, 0
	for i, g := range gravacoes {
		if erros[i] != nil {
			falhas++
			w.savesFalhados.Add(1)
			w.log.Warn("save on shutdown failed", "account", g.conta, "what", g.oQue, "err", erros[i])
			continue
		}
		salvos++
		if g.depois != nil {
			g.depois(w)
		}
	}

	// The last few seconds of chat, written straight rather than buffered: the
	// loop is ending, so there is no tick left to flush it and no reason to hand
	// it to a goroutine nobody will wait for.
	if len(w.chatBuf) > 0 {
		cctx, ccancel := context.WithTimeout(ctx, chatTempo)
		if err := w.persist.RecordChat(cctx, w.chatBuf); err != nil {
			w.log.Warn("chat log: last batch lost on shutdown", "linhas", len(w.chatBuf), "err", err)
		}
		ccancel()
		w.chatBuf = nil
	}

	// As gravações de logout que já estavam a caminho, dentro do mesmo prazo:
	// esperar sem teto aqui é o que deixava o processo morrer calado no meio.
	emVoo := make(chan struct{})
	go func() { w.saveWG.Wait(); close(emVoo) }()
	select {
	case <-emVoo:
	case <-ctx.Done():
		w.log.Error("shutdown: in-flight saves did not finish in time", "deadline", prazo)
	}

	// O aviso ganha o tempo dele no fio. Os saves já correram dentro dele, então
	// na maior parte das vezes não sobra nada para esperar.
	if avisou {
		if resta := w.cfg.ShutdownGrace - time.Since(inicio); resta > 0 {
			time.Sleep(resta)
		}
	}
	for _, s := range w.sessions {
		if s != nil {
			s.close()
		}
	}
	w.log.Info("world loop stopped",
		"sessions_saved", salvos, "saves_failed", falhas, "took", time.Since(inicio).Round(time.Millisecond))
}

// gravacoesDeSaida tira, no laço, o instantâneo de tudo o que o desligamento
// grava. Loop-only.
//
// O PAR, NÃO O MODO. Cada conta com personagem em jogo vai numa transação só,
// personagem e carga juntos: gravar os dois separados é o espelho do dupe. Uma
// sessão apanhada em UserWaitDB — no meio de uma ida ao banco — tem mochila viva,
// e deixá-la de fora faria a carga dela ou ser gravada sozinha, ou não ser gravada.
//
// Depois vêm as cargas das contas SEM mochila viva (a tela de seleção), que não
// têm personagem para discordar delas. A conta cujo par falhar fica com a carga
// antiga no banco, de propósito: o par velho inteiro é consistente — perde-se o
// progresso, mas nas duas metades juntas, e nada duplica.
//
// Por fim, as contas que não têm nada a gravar mas cuja posse é desta execução.
func (w *World) gravacoesDeSaida() []gravacaoDeSaida {
	var gs []gravacaoDeSaida
	p, epoca := w.persist, w.parEpoca
	tratadas := make(map[int64]bool)

	for _, s := range w.sessions {
		if s == nil || !w.temMochilaViva(s) {
			continue
		}
		cs, carga, unacked, temCarga, seq := w.parDeSalvamento(s)
		conta := cs.AccountID
		tratadas[conta] = true
		// O filtro das fadas deste personagem que ainda não chegou ao banco vai NA
		// FRENTE do par, na mesma gravação: o par solta a posse da conta, e o
		// servidor novo pode carregar o personagem logo depois.
		filtro := w.fadaFiltroDeSaida(fadaFiltroChave{cs.AccountID, cs.Slot})
		gs = append(gs, gravacaoDeSaida{
			conta: conta,
			oQue:  "character",
			grava: func(ctx context.Context) error {
				if filtro != nil {
					if err := filtro(ctx); err != nil {
						// Falha própria, com nome próprio: o par é gravado de qualquer jeito.
						w.savesFalhados.Add(1)
						w.log.Warn("save on shutdown failed", "account", conta, "what", "fada filtro", "err", err)
					}
				}
				// soltarPosse=TRUE, como no logout: é o save de saída. Sem isso a
				// conta ficava presa a um processo morto até o prazo da posse vencer,
				// e o jogador batia na porta do servidor novo por até 90 s.
				return SalvarPar(ctx, p, cs, carga, temCarga, unacked, epoca, seq, true)
			},
			depois: func(w *World) {
				if temCarga {
					delete(w.cargo, conta)
					delete(w.deliveryPlaced, conta)
					delete(w.deliveryUnacked, conta)
				}
			},
		})
	}

	for conta := range w.cargo {
		if tratadas[conta] {
			continue
		}
		tratadas[conta] = true
		cs, unacked := w.cargoSave(conta), w.deliveryUnacked[conta]
		gs = append(gs, gravacaoDeSaida{
			conta: conta,
			oQue:  "cargo",
			grava: func(ctx context.Context) error {
				if err := saveCargoFor(ctx, p, cs, unacked); err != nil {
					return err
				}
				// A posse sai só depois que a carga confirmou, como no releaseCargo.
				return soltarPosse(ctx, p, conta, epoca)
			},
		})
	}

	// Os filtros das fadas de quem já saiu do jogo com a gravação a caminho.
	for k := range w.fadaFiltros {
		if filtro := w.fadaFiltroDeSaida(k); filtro != nil {
			gs = append(gs, gravacaoDeSaida{conta: k.conta, oQue: "fada filtro", grava: filtro})
		}
	}

	for _, s := range w.sessions {
		if s == nil || s.AccountID == 0 || tratadas[s.AccountID] {
			continue
		}
		conta := s.AccountID
		tratadas[conta] = true
		gs = append(gs, gravacaoDeSaida{
			conta: conta,
			oQue:  "account ownership",
			grava: func(ctx context.Context) error { return soltarPosse(ctx, p, conta, epoca) },
		})
	}
	return gs
}

// soltarPosse devolve a conta, e diz de qual passo veio a falha.
func soltarPosse(ctx context.Context, p Persistence, conta, epoca int64) error {
	if epoca <= 0 {
		return nil
	}
	if err := p.SoltarPosseDaConta(ctx, conta, epoca); err != nil {
		return fmt.Errorf("release account ownership: %w", err)
	}
	return nil
}

// gravarEmParalelo roda as gravações com no máximo n ao mesmo tempo e devolve o
// erro de cada uma, na mesma ordem. Seguro fora do laço.
//
// O PRAZO VALE PARA A ESPERA TAMBÉM. Uma gravação que ignorasse o contexto
// seguraria o desligamento até a plataforma matar o processo, que é justamente o
// fim calado que isto existe para evitar; vencido o prazo, o que não voltou conta
// como falha e o desligamento segue.
func gravarEmParalelo(ctx context.Context, gs []gravacaoDeSaida, n int) []error {
	erros := make([]error, len(gs))
	if len(gs) == 0 {
		return erros
	}
	type resultado struct {
		i   int
		err error
	}
	// Com espaço para todos: quem voltar depois do prazo não fica preso no envio.
	res := make(chan resultado, len(gs))
	go func() {
		vagas := make(chan struct{}, n)
		for i := range gs {
			select {
			case vagas <- struct{}{}:
			case <-ctx.Done():
				res <- resultado{i, ctx.Err()}
				continue
			}
			go func(i int) {
				defer func() { <-vagas }()
				res <- resultado{i, gs[i].grava(ctx)}
			}(i)
		}
	}()
	voltou := make([]bool, len(gs))
	for pendentes := len(gs); pendentes > 0; {
		select {
		case r := <-res:
			erros[r.i], voltou[r.i] = r.err, true
			pendentes--
		case <-ctx.Done():
			for i := range gs {
				if !voltou[i] {
					erros[i] = fmt.Errorf("did not finish before the shutdown deadline: %w", ctx.Err())
				}
			}
			return erros
		}
	}
	return erros
}

// savePeriodicoTick grava, a cada IntervaloDoSavePeriodico, cada personagem em
// jogo. Loop-only; o relógio vem de fora para o teste não dormir.
//
// Cada sessão tem o seu horário, espalhado pelo número da conexão: um servidor
// cheio não manda todo mundo ao banco no mesmo segundo. O primeiro save de uma
// sessão cai em até um intervalo depois de ela entrar em jogo.
//
// É o mesmo salvarParAsync dos saves de meio de jogo: par inteiro, com número de
// ordem, e a posse fica — a conta continua em jogo.
func (w *World) savePeriodicoTick(agora time.Time) {
	for _, s := range w.sessions {
		if s == nil || s.Mode != UserPlay || !w.temMochilaViva(s) {
			s.zerarSavePeriodico()
			continue
		}
		if s.proximoSavePeriodico.IsZero() {
			deslocamento := time.Duration(s.Conn%MaxUser) * IntervaloDoSavePeriodico / MaxUser
			s.proximoSavePeriodico = agora.Add(IntervaloDoSavePeriodico - deslocamento)
			continue
		}
		if agora.Before(s.proximoSavePeriodico) {
			continue
		}
		s.proximoSavePeriodico = agora.Add(IntervaloDoSavePeriodico)
		w.salvarParAsync(s)
	}
}

// zerarSavePeriodico faz a próxima entrada em jogo recomeçar a contagem. Uma
// sessão que voltou para a seleção e escolheu outro personagem não herda o
// horário do anterior. Aceita nil.
func (s *Session) zerarSavePeriodico() {
	if s != nil {
		s.proximoSavePeriodico = time.Time{}
	}
}
