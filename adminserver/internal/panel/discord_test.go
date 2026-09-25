package panel

import (
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/jeanluca/w2pp-openwyd/adminserver/internal/accounts"
)

// TestDesvincularDiscordExigeObservacao.
//
// Desvincular abre a porta para OUTRA pessoa pegar aquele Discord — e com ele o cargo
// que o Discord dá. Se foi por engano, ou a pedido de quem não era o dono, a linha da
// auditoria é a única coisa que sobra para reconstruir a história.
func TestDesvincularDiscordExigeObservacao(t *testing.T) {
	wr := newFakeWriter()
	wr.erroDesvincular = accounts.ErrNotaDoDiscord
	h := newTestPanelFull(t, withTarget(roleAdmin), newFakeAudit(), wr)
	post, token := signedInPost(t, h)

	rec := post("/contas/ana/discord", url.Values{"csrf": {token}, "observacao": {"   "}})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
	if len(wr.discordSolto) != 0 {
		t.Fatal("desvinculou sem observação")
	}
}

// TestDesvincularDiscordGuardaQuemFezEPorQue: a auditoria é gravada na mesma transação
// da soltura, então o que o painel precisa provar é que ela recebe o ator, o papel e a
// observação.
func TestDesvincularDiscordGuardaQuemFezEPorQue(t *testing.T) {
	wr := newFakeWriter()
	h := newTestPanelFull(t, withTarget(roleAdmin), newFakeAudit(), wr)
	post, token := signedInPost(t, h)

	rec := post("/contas/ana/discord", url.Values{
		"csrf": {token}, "observacao": {"dono antigo pediu por ticket 412"},
	})
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, want 303: %s", rec.Code, rec.Body.String())
	}
	if len(wr.discordSolto) != 1 {
		t.Fatalf("solturas = %d, want 1", len(wr.discordSolto))
	}
	s := wr.discordSolto[0]
	if s.nota != "dono antigo pediu por ticket 412" {
		t.Errorf("observação = %q", s.nota)
	}
	if s.papel == "" {
		t.Error("o papel de quem desvinculou não chegou à auditoria")
	}
	if s.atorConta == 0 && s.atorPainel == 0 {
		t.Error("a soltura foi gravada sem ator")
	}
}

// TestDesvincularDiscordDeContaSemVinculoNaoMente: quem clicou está investigando alguém
// que não consegue vincular. Dizer "pronto" o faria procurar a causa no lugar errado.
func TestDesvincularDiscordDeContaSemVinculoNaoMente(t *testing.T) {
	wr := newFakeWriter()
	wr.erroDesvincular = accounts.ErrSemDiscord
	h := newTestPanelFull(t, withTarget(roleAdmin), newFakeAudit(), wr)
	post, token := signedInPost(t, h)

	rec := post("/contas/ana/discord", url.Values{"csrf": {token}, "observacao": {"conferido"}})
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, want 303", rec.Code)
	}
	destino := rec.Header().Get("Location")
	if !strings.Contains(destino, "Discord+vinculado") && !strings.Contains(destino, "Discord%20vinculado") {
		t.Errorf("o aviso não diz que a conta não tinha vínculo: %q", destino)
	}
}

// TestDesvincularDiscordSoAdmin: o clique libera o Discord para outra conta, e com ele o
// cargo. Não é decisão de plantão.
func TestDesvincularDiscordSoAdmin(t *testing.T) {
	wr := newFakeWriter()
	// Como no TestSetCargoIsAdminOnly: quem está logado aqui é moderador.
	h := newTestPanelFull(t, withTarget(roleModerator), newFakeAudit(), wr)
	post, token := signedInPost(t, h)

	rec := post("/contas/ana/discord", url.Values{"csrf": {token}, "observacao": {"conferido"}})
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", rec.Code)
	}
	if len(wr.discordSolto) != 0 {
		t.Fatal("moderador desvinculou")
	}
}

// TestOBotaoDoDiscordSoApareceComVinculo: um botão que sempre recusa é pior do que um
// botão que não está lá. E com vínculo ele aparece dizendo QUAL Discord sai — sem o
// número, quem clica está confirmando às cegas.
func TestOBotaoDoDiscordSoApareceComVinculo(t *testing.T) {
	semVinculo := newTestPanelFull(t, withTarget(roleAdmin), newFakeAudit(), newFakeWriter())
	if corpo := signedIn(t, semVinculo)("/contas/ana").Body.String(); strings.Contains(corpo, `/discord"`) {
		t.Error("a conta sem Discord mostrou o botão de desvincular")
	}

	const id = "123456789012345678"
	comVinculo := newFakeWriter()
	comVinculo.details = accounts.Details{DiscordID: id}
	h := newTestPanelFull(t, withTarget(roleAdmin), newFakeAudit(), comVinculo)
	corpo := signedIn(t, h)("/contas/ana").Body.String()
	if !strings.Contains(corpo, `/discord"`) {
		t.Error("a conta COM Discord não mostrou o botão")
	}
	if !strings.Contains(corpo, id) {
		t.Errorf("a tela não diz qual Discord sai: o número %s não aparece", id)
	}
}
