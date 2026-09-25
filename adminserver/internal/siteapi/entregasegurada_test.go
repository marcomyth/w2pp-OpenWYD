package siteapi

import (
	"encoding/json"
	"errors"
	"net/http"
	"testing"
)

// COM O BAÚ CHEIO, A PENDENTE VIRA HELD.
//
// É a queixa que originou isto: a página dizia "o item está a caminho" a quem não ia
// receber nada até esvaziar o baú. "A caminho" e "não cabe" são situações diferentes, e
// só uma delas o jogador pode resolver.
func TestBauCheioFazAPendenteVirarHeld(t *testing.T) {
	c := novoCenario(t)
	c.banco.bauCheio = true

	var e struct {
		Pendentes []itemSite `json:"pendentes"`
		Perdidos  []itemSite `json:"perdidos"`
	}
	rec := c.pede("GET", "/site/v1/contas/1/entregas", "")
	confereStatus(t, rec, http.StatusOK, "")
	if err := json.Unmarshal(rec.Body.Bytes(), &e); err != nil {
		t.Fatal(err)
	}
	for _, p := range e.Pendentes {
		if p.Estado != "HELD" {
			t.Errorf("pendente %d = %q, queria HELD com o bau cheio", p.ID, p.Estado)
		}
	}
	// E O QUE JÁ TERMINOU NÃO MUDA. Dizer "segurada" numa linha perdida seria inventar
	// uma espera que não existe — aquele item não vem mais, cheio ou vazio.
	for _, p := range e.Perdidos {
		if p.Estado != "LOST" {
			t.Errorf("perdido %d = %q; o bau cheio nao muda o que ja terminou", p.ID, p.Estado)
		}
	}
}

// FALHA AO CONTAR O BAÚ NÃO DERRUBA A LISTA.
//
// O que a página precisa para funcionar é saber o que está devido; o "por que não chegou"
// é informação. Na falha vale "não está cheio", que faz a tela dizer "a caminho" —
// exatamente o que ela dizia antes disto existir, e portanto a degradação certa.
func TestFalhaAoContarOBauNaoDerrubaAsEntregas(t *testing.T) {
	c := novoCenario(t)
	c.banco.erroBau = errors.New("o banco caiu ao contar o bau")

	rec := c.pede("GET", "/site/v1/contas/1/entregas", "")
	confereStatus(t, rec, http.StatusOK, "")

	var e struct {
		Pendentes []itemSite `json:"pendentes"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &e); err != nil {
		t.Fatal(err)
	}
	if len(e.Pendentes) == 0 {
		t.Fatal("a lista veio vazia por causa da contagem do bau")
	}
	for _, p := range e.Pendentes {
		if p.Estado != "WAITING" {
			t.Errorf("pendente %d = %q; sem saber do bau vale WAITING", p.ID, p.Estado)
		}
	}
}

// A COMPRA NO MERCADO TEM NOME.
//
// Sem este caso ela caía em "outro" — a página dizia "outro" justamente para a coisa que
// o jogador acabou de pagar com dinheiro real.
func TestCompraNoMercadoTemNome(t *testing.T) {
	if got := origem("rmt_anuncio:7"); got != "mercado" {
		t.Errorf("origem(rmt_anuncio:7) = %q, queria mercado", got)
	}
	// E os que já existiam continuam.
	if origem("donate_shop:1") != "loja" || origem("painel:9") != "equipe" {
		t.Error("as origens antigas mudaram")
	}
	// O id da staff NUNCA sai. É por isso que origem() existe.
	if got := origem("painel:12345"); got == "painel:12345" {
		t.Error("o id da staff vazou para o jogador")
	}
}
