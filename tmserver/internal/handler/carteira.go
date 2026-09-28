package handler

import (
	"context"

	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/world"
)

// A carteira de Cash da sessão, relida do banco.
//
// O saldo que o jogador vê no rodapé da Loja do Servidor era lido UMA VEZ, no
// login, e daí em diante só somado e subtraído pelas compras feitas naquela
// sessão. Tudo que mexe na carteira por fora — a recarga pelo site, um ajuste da
// staff, uma RCoin usada, outra sessão da mesma conta — não chegava à tela até
// o próximo login. A Hanna viu o painel dizendo um número e o jogo dizendo outro,
// e os dois estavam "certos" pela sua própria conta.
//
// RELER É MAIS HONESTO QUE CORRIGIR EM CADA PONTO. Sair somando o crédito em
// cada lugar que mexe na carteira conserta só os caminhos que alguém lembrou de
// emendar, e o próximo caminho novo nasce errado de novo. Reler do banco cobre
// também o que mudou fora da sessão, que é justamente o caso que quebrou.
//
// A leitura é ASSÍNCRONA. O laço do mundo tem um dono só, e uma ida ao banco
// dentro dele seguraria o servidor inteiro para todo mundo enquanto um jogador
// abre uma vitrine.

// relerCash pede o saldo da conta ao banco e, quando ele volta, atualiza a
// sessão e chama `entao` DENTRO do laço.
//
// `entao` roda sempre, mesmo quando a leitura falha: quem chamou tem uma tela
// para desenhar, e uma tela que não abre é pior que uma tela com o saldo de um
// minuto atrás. Numa falha, o valor que sobra na sessão é o último conhecido e
// o erro vai para o log — sem frase para o jogador, porque não há nada que ele
// possa fazer a respeito, e porque ele nem pediu um saldo: pediu a vitrine.
func (d *Dispatcher) relerCash(w *world.World, s *world.Session,
	entao func(*world.World, *world.Session)) {
	conta := s.AccountID
	// Sessão sem conta não tem carteira para ler, e uma releitura já a caminho
	// não vira duas: a que está voando traz o mesmo número.
	if conta == 0 || s.CashEmLeitura {
		entao(w, s)
		return
	}
	s.CashEmLeitura = true
	p := w.Persistence()
	w.Go(s, func() func(*world.World, *world.Session) {
		saldo, err := p.DonateBalance(context.Background(), conta)
		return func(w *world.World, s *world.Session) {
			s.CashEmLeitura = false
			if err != nil {
				d.log.Warn("carteira: releitura falhou", "conta", conta, "err", err)
			} else {
				s.Cash = saldo
			}
			entao(w, s)
		}
	})
}
