package panel

import (
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/jeanluca/w2pp-openwyd/internal/domain"
)

// formComTetos é o formulário mínimo de eventos com as dez caixas do teto.
func formComTetos(token string, teto, dobro [5]int64) url.Values {
	v := url.Values{"csrf": {token}, "torre": {"1"}, "torre_hora": {"20"}, "chefes_horas": {"24"}}
	for i, topo := range domain.RoundXPCapTopLevels {
		v.Set(fmt.Sprintf("teto_%d", topo), fmt.Sprint(teto[i]))
		v.Set(fmt.Sprintf("teto_dobro_%d", topo), fmt.Sprint(dobro[i]))
	}
	return v
}

// TestTetoDaRodadaGravaAsDezCaixas: os valores vão do formulário para o banco e
// para a auditoria; zero é aceito (sem teto na faixa).
func TestTetoDaRodadaGravaAsDezCaixas(t *testing.T) {
	ev := &fakeEventos{cfg: domain.DefaultWorldEventConfig()}
	log := newFakeAudit()
	post, token := signedInPost(t, newTestPanelEventos(t, roleAdmin, ev, log))

	teto := [5]int64{1, 2, 0, 4, 5}
	dobro := [5]int64{10, 20, 30, 40, 50}
	rec := post("/eventos", formComTetos(token, teto, dobro))
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, want 303 (body: %s)", rec.Code, rec.Body.String())
	}
	if g := ev.gravado[0]; g.RoundXPCap != teto || g.RoundXPCapDouble != dobro {
		t.Errorf("gravado %v / %v, want %v / %v", g.RoundXPCap, g.RoundXPCapDouble, teto, dobro)
	}
	depois, _ := log.written[0].New.(map[string]any)
	if depois["teto_rodada"] != teto || depois["teto_rodada_dobro"] != dobro {
		t.Errorf("auditoria depois = %v / %v", depois["teto_rodada"], depois["teto_rodada_dobro"])
	}
}

// TestTetoDaRodadaSemAsCaixasGuardaOQueEstava: uma página de antes da migração
// não desliga o teto ao salvar outra coisa.
func TestTetoDaRodadaSemAsCaixasGuardaOQueEstava(t *testing.T) {
	ev := &fakeEventos{cfg: domain.DefaultWorldEventConfig()}
	post, token := signedInPost(t, newTestPanelEventos(t, roleAdmin, ev, newFakeAudit()))
	rec := post("/eventos", url.Values{"csrf": {token}, "torre": {"1"}, "torre_hora": {"20"}, "chefes_horas": {"24"}})
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, want 303", rec.Code)
	}
	if g := ev.gravado[0]; g.RoundXPCap != domain.DefaultRoundXPCap || g.RoundXPCapDouble != domain.DefaultRoundXPCapDouble {
		t.Errorf("gravado %v / %v, want os valores que estavam", g.RoundXPCap, g.RoundXPCapDouble)
	}
}

// TestTetoDaRodadaInvalidoERecusado: negativo, texto ou uma caixa vazia no meio
// não gravam nada.
func TestTetoDaRodadaInvalidoERecusado(t *testing.T) {
	for _, valor := range []string{"-1", "", "muito", "1.5"} {
		t.Run("valor="+valor, func(t *testing.T) {
			ev := &fakeEventos{cfg: domain.DefaultWorldEventConfig()}
			post, token := signedInPost(t, newTestPanelEventos(t, roleAdmin, ev, newFakeAudit()))
			form := formComTetos(token, domain.DefaultRoundXPCap, domain.DefaultRoundXPCapDouble)
			form.Set("teto_299", valor)
			rec := post("/eventos", form)
			if rec.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want 400", rec.Code)
			}
			if !strings.Contains(rec.Body.String(), "teto de XP por rodada") {
				t.Errorf("mensagem = %q, não diz que é o teto", rec.Body.String())
			}
			if len(ev.gravado) != 0 {
				t.Error("gravou com teto inválido")
			}
		})
	}
}

// TestAPaginaMostraOTetoDaRodada: as dez caixas com os valores gravados e o nível
// da tela ao lado do guardado.
func TestAPaginaMostraOTetoDaRodada(t *testing.T) {
	ev := &fakeEventos{cfg: domain.DefaultWorldEventConfig()}
	body := getSignedIn(t, newTestPanelEventos(t, roleAdmin, ev, newFakeAudit()), "/eventos").Body.String()
	for _, quer := range []string{
		"Teto de XP por rodada (Mortal)", "200 a 299 (tela 201 a 300)",
		`name="teto_299"`, `value="500454"`, `name="teto_dobro_299"`, `value="1000908"`,
	} {
		if !strings.Contains(body, quer) {
			t.Errorf("a página não traz %q", quer)
		}
	}
}
