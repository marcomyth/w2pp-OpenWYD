package handler

import (
	"context"
	"errors"
	"time"
)

// Saldos de Cash e RMT da Loja do Servidor.
//
// Ouro é do personagem e o tmServer move sozinho, dentro do próprio laço. Cash e
// RMT são da CONTA e moram no banco: `account.donate_balance` (a carteira que a
// recarga por PIX credita) e `account.rmt_balance` (migração 0077). O caminho
// até lá é o RPC TransferPlayerBalance (api/db), que faz débito e crédito na
// mesma transação e trava as duas carteiras em ordem de id.
//
// Sem dbServer ligado não há carteira: a compra nessas moedas é recusada e nada
// se move — nem item, nem saldo. É de propósito, melhor não vender do que vender
// sem cobrar.
//
// Ainda falta, e depende do mesmo caminho: "Realizar saque" no painel (tirar da
// conta o que se ganhou vendendo em Cash/RMT).

// SaldoDeConta move Cash ou RMT entre contas. O valor é sempre positivo e a
// moeda é protocol.LojaMoedaCash ou protocol.LojaMoedaRMT.
//
// O `motivo` vai inteiro para o diário do banco e é o que a staff lê quando
// alguém reclama de uma compra. Era uma frase fixa aqui dentro, igual para toda
// venda; quem sabe QUAL item foi vendido é quem chama, então é ele que a
// escreve. Nada de dado pessoal nessa frase: ela aparece na tela da staff.
type SaldoDeConta interface {
	Transfere(deConta, paraConta int64, moeda uint8, valor int32, motivo string) error
}

// ErrSaldoNaoLigado é o que sai enquanto a emenda acima não foi feita.
var ErrSaldoNaoLigado = errors.New("loja: saldo de cash/rmt ainda nao esta ligado ao banco")

type saldoNaoLigado struct{}

func (saldoNaoLigado) Transfere(int64, int64, uint8, int32, string) error {
	return ErrSaldoNaoLigado
}

var saldoContas SaldoDeConta = saldoNaoLigado{}

// UsaSaldoDeConta liga a loja ao saldo de verdade, na montagem do servidor.
func UsaSaldoDeConta(s SaldoDeConta) {
	if s != nil {
		saldoContas = s
	}
}

// BancoDeSaldo é o pedaço do dbclient que a loja usa.
type BancoDeSaldo interface {
	TransferePlayerBalance(ctx context.Context, deConta, paraConta int64, moeda uint8,
		valor int32, motivo string) error
}

// SaldoPeloBanco liga a loja ao dbServer. O contexto tem prazo porque isto roda
// dentro do laço do mundo: um banco lento não pode segurar o servidor inteiro.
func SaldoPeloBanco(b BancoDeSaldo) SaldoDeConta {
	if b == nil {
		return saldoNaoLigado{}
	}
	return saldoNoBanco{b: b}
}

type saldoNoBanco struct {
	b BancoDeSaldo
}

func (s saldoNoBanco) Transfere(deConta, paraConta int64, moeda uint8, valor int32, motivo string) error {
	ctx, cancela := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancela()
	return s.b.TransferePlayerBalance(ctx, deConta, paraConta, moeda, valor, motivo)
}
