package authz

import (
	"strings"
	"testing"
)

// TestALeituraPassaEAEscritaNao.
//
// A lista fecha por padrão: um método que ninguém pôs nela é recusado. O contrário —
// listar as escritas — faria um método NOVO nascer liberado, e é exatamente esse o
// acidente que a lista existe para impedir.
func TestALeituraPassaEAEscritaNao(t *testing.T) {
	casos := map[string]bool{
		// As leituras das oito páginas, que é o que a staff do painel precisa hoje.
		"/web.v1.NpcAdminService/ListNpcs":                    true,
		"/web.v1.NpcAdminService/GetNpc":                      true,
		"/web.v1.MobTemplateAdminService/ListMobTemplates":    true,
		"/web.v1.DonateRevenueAdminService/GetRevenueSummary": true,
		"/web.v1.WorldEventAdminService/GetWorldEventConfig":  true,

		// As escritas, que gravariam "conta 0" como autor.
		"/web.v1.NpcAdminService/UpsertNpc":                      false,
		"/web.v1.NpcAdminService/SetNpcShop":                     false,
		"/web.v1.NpcAdminService/DeleteNpc":                      false,
		"/web.v1.MobTemplateAdminService/UpsertMobTemplateStat":  false,
		"/web.v1.DonateAdminService/CreditDonateBalance":         false,
		"/web.v1.AttributeMapAdminService/TransformAttributeMap": false,

		// Um método que não existe, e um ilegível: fechados.
		"/web.v1.NpcAdminService/MetodoQueNinguemEscreveuAinda": false,
		"lixo":   false,
		"":       false,
		"/x/y/z": false,
	}
	for metodo, quer := range casos {
		if got := PainelPodeChamar(metodo); got != quer {
			t.Errorf("PainelPodeChamar(%q) = %v, queria %v", metodo, got, quer)
		}
	}
}

// TestAListaComparaServicoEMetodoJuntos.
//
// Dois serviços têm um "ListShopItems" cada: o DonateAdminService, que é do painel, e o
// DonateShopService, que é do jogador. Comparar só o nome do método liberaria o do
// vizinho junto — e o do jogador não tem nada que ver com o painel.
func TestAListaComparaServicoEMetodoJuntos(t *testing.T) {
	if !PainelPodeChamar("/web.v1.DonateAdminService/ListShopItems") {
		t.Error("a leitura do painel foi recusada")
	}
	if PainelPodeChamar("/web.v1.DonateShopService/ListShopItems") {
		t.Error("o metodo do JOGADOR passou pela lista do painel: a comparacao esta " +
			"olhando so o nome do metodo")
	}
}

// TestAFraseDaRecusaNaoMandaUsarContaDeJogo.
//
// REGRA DA HANNA: o painel é só do usuário do painel, e conta de jogo não é caminho nem
// atalho. Uma frase que manda contornar por outra porta ensina o contorno — e depois
// ninguém desfaz.
func TestAFraseDaRecusaNaoMandaUsarContaDeJogo(t *testing.T) {
	for _, proibido := range []string{"conta de jogo", "cargo", "logar no jogo", "personagem"} {
		if strings.Contains(MsgEscritaAindaNao, proibido) {
			t.Errorf("a frase manda usar %q: %q", proibido, MsgEscritaAindaNao)
		}
	}
	if MsgEscritaAindaNao == "" {
		t.Error("a recusa nao diz nada")
	}
}
