package panel

import (
	"net/http"
	"net/url"
	"strings"
	"testing"
)

// TestEsconderDoRankingEhSoDeAdmin.
//
// O ranking é a vitrine pública do servidor, e tirar alguém dela é decisão de quem
// responde por ele. Moderador faz moderação do dia a dia — silenciar, bloquear — e isto
// não é isso.
//
// A trava é na ROTA, e este teste é o que impede alguém de trocar o onlyAdmin por
// requireStaff no dia em que um moderador pedir para marcar uma conta.
func TestEsconderDoRankingEhSoDeAdmin(t *testing.T) {
	wr := newFakeWriter()
	wr.foraDoRankingMudou = true
	// A sessão entra como MODERADOR: é o cargo que faz a moderação do dia a dia e o
	// que esta rota tem de recusar.
	h := newTestPanelFull(t, withTarget(roleModerator), newFakeAudit(), wr)

	post, token := signedInPost(t, h)
	rec := post("/contas/ana/fora-do-ranking", url.Values{"csrf": {token}, "fora": {"1"}})
	if rec.Code == http.StatusSeeOther || rec.Code == http.StatusOK {
		t.Errorf("moderador escondeu uma conta do ranking: status %d", rec.Code)
	}
	if len(wr.foraDoRanking) != 0 {
		t.Errorf("a escrita chegou ao store vinda de um moderador: %+v", wr.foraDoRanking)
	}
}

// TestEsconderDoRankingLevaOAtorEAObservacao.
//
// A auditoria é gravada DENTRO da transação do store, então o que a tela tem de provar é
// que ela entrega quem fez e o que escreveu. Um handler que perdesse o ator gravaria uma
// linha sem dono — e auditoria sem dono é pior que nenhuma, porque parece completa.
func TestEsconderDoRankingLevaOAtorEAObservacao(t *testing.T) {
	wr := newFakeWriter()
	wr.foraDoRankingMudou = true
	h := newTestPanelFull(t, withTarget(roleAdmin), newFakeAudit(), wr)

	post, token := signedInPost(t, h)
	rec := post("/contas/ana/fora-do-ranking", url.Values{
		"csrf": {token}, "fora": {"1"}, "observacao": {"conta de teste do FireBall"},
	})
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, want 303: %s", rec.Code, rec.Body.String())
	}
	if len(wr.foraDoRanking) != 1 {
		t.Fatalf("pedidos = %d, want 1", len(wr.foraDoRanking))
	}
	p := wr.foraDoRanking[0]
	if !p.Fora {
		t.Error("o pedido nao disse para esconder")
	}
	if p.Nota != "conta de teste do FireBall" {
		t.Errorf("observacao = %q", p.Nota)
	}
	// UM ator e só um: conta do jogo OU usuário do painel, nunca zero nos dois. Zero
	// nos dois é o defeito que eu cometi na primeira versão deste handler, e o store
	// recusa com um erro que a tela mostraria como falha de servidor.
	if p.AtorConta == 0 && p.AtorPainel == 0 {
		t.Error("nenhum ator chegou ao store: a auditoria sairia sem dono")
	}
	if p.Papel == "" {
		t.Error("o papel do ator nao chegou")
	}
}

// TestOSegundoCliqueNoRankingNaoViraErro: marcar o que já está marcado é o segundo
// clique, ou duas pessoas resolvendo a mesma coisa. Tem de avisar, não falhar.
func TestOSegundoCliqueNoRankingNaoViraErro(t *testing.T) {
	wr := newFakeWriter()
	wr.foraDoRankingMudou = false // o store diz: já estava assim
	h := newTestPanelFull(t, withTarget(roleAdmin), newFakeAudit(), wr)

	post, token := signedInPost(t, h)
	rec := post("/contas/ana/fora-do-ranking", url.Values{"csrf": {token}, "fora": {"1"}})
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, want 303", rec.Code)
	}
	if destino := rec.Header().Get("Location"); !strings.Contains(destino, "aviso=") {
		t.Errorf("o segundo clique nao avisou nada: %q", destino)
	}
}
