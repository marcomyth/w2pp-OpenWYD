package panel

import (
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/jeanluca/w2pp-openwyd/adminserver/internal/gamedata"
	"github.com/jeanluca/w2pp-openwyd/internal/pilha"
)

// catalogoComDiamante é o catálogo falso com um item que empilha.
func catalogoComDiamante() *fakeGameData {
	g := newFakeGameData()
	g.itens = append(g.itens, gamedata.Item{Index: 2441, Name: "Diamante", DisplayName: "Diamante"})
	return g
}

// TestEntregaQuantidadeEmPilhas: 250 Diamantes viram três linhas (120, 120, 10),
// e a auditoria guarda a quantidade e os ids.
func TestEntregaQuantidadeEmPilhas(t *testing.T) {
	ent := &fakeEntregas{}
	log := newFakeAudit()
	post, token := signedInPost(t, newTestPanelEntrega(t, log, catalogoComDiamante(), ent))

	rec := post("/contas/ana/entregar", url.Values{"csrf": {token}, "item": {"2441"}, "quantidade": {"250"}})
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, want 303 (body: %s)", rec.Code, rec.Body.String())
	}
	var got []int
	for _, it := range ent.enfileirou {
		n := 1
		for _, ef := range it.Eff {
			if ef[0] == pilha.EfAmount {
				n = int(ef[1])
			}
		}
		got = append(got, n)
	}
	if len(got) != 3 || got[0] != 120 || got[1] != 120 || got[2] != 10 {
		t.Errorf("pilhas = %v, want [120 120 10]", got)
	}
	registros := log.recorded()
	if len(registros) != 1 {
		t.Fatalf("auditorias = %d, want 1 para o envio inteiro", len(registros))
	}
	novo, _ := registros[0].New.(map[string]any)
	if novo["quantidade"] != 250 || novo["espacos"] != 3 {
		t.Errorf("auditoria = %v, want quantidade 250 em 3 espaços", novo)
	}
	if ids, _ := novo["entregas"].([]int64); len(ids) != 3 {
		t.Errorf("auditoria sem os três ids: %v", novo["entregas"])
	}
	loc, _ := url.QueryUnescape(rec.Header().Get("Location"))
	if !strings.Contains(loc, "250") {
		t.Errorf("o aviso não diz a quantidade: %q", loc)
	}
}

// TestEntregaQuantidadeDeItemAvulso: o que não empilha sai um por linha, sem
// EF_AMOUNT.
func TestEntregaQuantidadeDeItemAvulso(t *testing.T) {
	ent := &fakeEntregas{}
	post, token := signedInPost(t, newTestPanelEntrega(t, newFakeAudit(), newFakeGameData(), ent))
	rec := post("/contas/ana/entregar", url.Values{"csrf": {token}, "item": {"2000"}, "quantidade": {"3"}})
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, want 303 (body: %s)", rec.Code, rec.Body.String())
	}
	if len(ent.enfileirou) != 3 {
		t.Fatalf("linhas = %d, want 3", len(ent.enfileirou))
	}
	for _, it := range ent.enfileirou {
		if it.Index != 2000 || it.Eff != ([3][2]uint8{}) {
			t.Errorf("linha = %+v, want a Espada Longa sem efeito", it)
		}
	}
}

// TestEntregaSemQuantidadeEhUm: o formulário antigo, sem o campo, entrega um.
func TestEntregaSemQuantidadeEhUm(t *testing.T) {
	ent := &fakeEntregas{}
	post, token := signedInPost(t, newTestPanelEntrega(t, newFakeAudit(), catalogoComDiamante(), ent))
	if rec := post("/contas/ana/entregar", url.Values{"csrf": {token}, "item": {"2441"}}); rec.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, want 303", rec.Code)
	}
	if len(ent.enfileirou) != 1 || ent.enfileirou[0].Eff != ([3][2]uint8{}) {
		t.Errorf("entregou %+v, want um Diamante sem efeito", ent.enfileirou)
	}
}

// TestEntregaQuantidadeForaDoLimite: acima do que cabe no baú, zero, negativo ou
// texto não enfileiram nada.
func TestEntregaQuantidadeForaDoLimite(t *testing.T) {
	for _, c := range []struct{ item, qtd string }{
		{"2441", "15361"}, {"2000", "129"}, {"2441", "0"}, {"2441", "-5"}, {"2441", "muitos"},
	} {
		t.Run(c.item+"x"+c.qtd, func(t *testing.T) {
			ent := &fakeEntregas{}
			post, token := signedInPost(t, newTestPanelEntrega(t, newFakeAudit(), catalogoComDiamante(), ent))
			rec := post("/contas/ana/entregar", url.Values{"csrf": {token}, "item": {c.item}, "quantidade": {c.qtd}})
			if rec.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want 400", rec.Code)
			}
			if len(ent.enfileirou) != 0 {
				t.Errorf("enfileirou %d linhas com quantidade inválida", len(ent.enfileirou))
			}
		})
	}
}
