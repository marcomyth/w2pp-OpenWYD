package panel

import (
	"net/http"
	"strings"
	"testing"
)

// TestAPortaDoDinheiroExiste.
//
// O item "Dinheiro" do menu apontava para /dinheiro desde que a seção nasceu, e a
// rota NUNCA foi registrada: clicar dava 404. O menu é a única porta da seção, então
// o defeito era invisível no código e imediato na tela.
func TestAPortaDoDinheiroExiste(t *testing.T) {
	// painelCompleto porque estas rotas só existem com Repasses e FilasRMT ligados.
	h := painelCompleto(t)
	get := signedIn(t, h)

	rec := get("/dinheiro")
	if rec.Code == http.StatusNotFound {
		t.Fatal("/dinheiro devolveu 404: o item do menu continua apontando para o nada")
	}
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, queria 303 — a porta é um desvio, não uma tela", rec.Code)
	}
	if destino := rec.Header().Get("Location"); destino != "/repasses" {
		t.Errorf("desviou para %q, queria /repasses — a primeira aba da seção", destino)
	}
}

// TestAsQuatroTelasDoDinheiroDesenhamAsAbas.
//
// O modelo das abas existia no _shared.html e NENHUMA página o chamava. Quem entrava
// numa fila não via as outras três nem os contadores delas — e era justamente o
// contador que dizia onde havia gente esperando.
func TestAsQuatroTelasDoDinheiroDesenhamAsAbas(t *testing.T) {
	// painelCompleto porque estas rotas só existem com Repasses e FilasRMT ligados.
	h := painelCompleto(t)
	get := signedIn(t, h)

	for _, caminho := range []string{"/repasses", "/reembolsos", "/orfaos", "/divergentes"} {
		t.Run(caminho, func(t *testing.T) {
			rec := get(caminho)
			if rec.Code != http.StatusOK {
				t.Fatalf("status = %d", rec.Code)
			}
			corpo := rec.Body.String()
			// As quatro abas aparecem em todas as quatro telas: é isso que faz a
			// seção ser uma seção.
			for _, irma := range []string{"/repasses", "/reembolsos", "/orfaos", "/divergentes"} {
				if !strings.Contains(corpo, `href="`+irma+`"`) {
					t.Errorf("a tela não oferece a aba %s", irma)
				}
			}
			// E a aba da própria tela vem acesa, senão a pessoa não sabe onde está.
			if !strings.Contains(corpo, `href="`+caminho+`" class="on"`) {
				t.Errorf("a aba de %s não está acesa nela mesma", caminho)
			}
		})
	}
}

// TestOMenuAcendeDentroDaSecaoDoDinheiro: estar numa das quatro filas é estar em
// Dinheiro. Antes o menu apagava assim que a pessoa entrava, e a seção parecia outra
// coisa.
func TestOMenuAcendeDentroDaSecaoDoDinheiro(t *testing.T) {
	// painelCompleto porque estas rotas só existem com Repasses e FilasRMT ligados.
	h := painelCompleto(t)
	get := signedIn(t, h)

	for _, caminho := range []string{"/repasses", "/reembolsos", "/orfaos", "/divergentes"} {
		rec := get(caminho)
		if rec.Code != http.StatusOK {
			t.Fatalf("%s: status = %d", caminho, rec.Code)
		}
		if !strings.Contains(rec.Body.String(), `href="/dinheiro" class="on"`) {
			t.Errorf("%s: o menu Dinheiro não está aceso", caminho)
		}
	}
}
