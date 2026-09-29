package authz

import (
	"strings"
	"testing"
)

// TestOPainelLeEEscreveEONaoDoPainelContinuaFechado.
//
// ESTE TESTE MUDOU DE LADO, e o registro importa: antes ele exigia que as ESCRITAS
// fossem recusadas, porque a auditoria não sabia o que era um usuário do painel e a
// escrita gravaria "conta 0" como autor. Desde a migração 0181 e o domain.Ator ela sabe,
// então as escritas entram — e o que o teste guarda agora é o que NÃO entra.
//
// A LISTA CONTINUA FECHANDO POR PADRÃO, e é isso que as últimas linhas provam: um método
// que ninguém pôs nela é recusado, inclusive um nome inventado e um caminho ilegível.
func TestOPainelLeEEscreveEONaoDoPainelContinuaFechado(t *testing.T) {
	casos := map[string]bool{
		// Leituras das páginas de administração.
		"/web.v1.NpcAdminService/ListNpcs":                    true,
		"/web.v1.NpcAdminService/GetNpc":                      true,
		"/web.v1.MobTemplateAdminService/ListMobTemplates":    true,
		"/web.v1.DonateRevenueAdminService/GetRevenueSummary": true,
		"/web.v1.WorldEventAdminService/GetWorldEventConfig":  true,
		"/web.v1.ItemCatalogService/ListItems":                true,

		// Escritas, uma de cada serviço que as tem.
		"/web.v1.NpcAdminService/UpsertNpc":                      true,
		"/web.v1.NpcAdminService/SetNpcShop":                     true,
		"/web.v1.NpcAdminService/DeleteNpc":                      true,
		"/web.v1.MobTemplateAdminService/UpsertMobTemplateStat":  true,
		"/web.v1.ItemStatAdminService/UpsertItemStat":            true,
		"/web.v1.MountGrowthAdminService/SetMountGrowthCurve":    true,
		"/web.v1.MountGrowthAdminService/ClearMountBonus":        true,
		"/web.v1.DailyRewardAdminService/UpsertRewardItem":       true,
		"/web.v1.DonateAdminService/CreditDonateBalance":         true,
		"/web.v1.AttributeMapAdminService/TransformAttributeMap": true,
		"/web.v1.WorldEventAdminService/SetWorldEventConfig":     true,

		// O QUE NÃO É DO PAINEL, e esta é a metade que a entrega quase perdeu.
		//
		// O plano dizia para a lista sumir inteira. Se ela sumisse, o usuário do painel
		// passaria a alcançar TODO o webServer, porque a chave do painel não separa
		// serviços — e estes cinco são do jogador e do site, e não conferem cargo
		// nenhum. Cada linha abaixo é um serviço que eu conferi um por um.
		"/web.v1.EmblemaService/ConcederEmblema":      false,
		"/web.v1.DonateTopupService/CreateTopupOrder": false,
		"/web.v1.MercadoService/AnunciarItem":         false,
		"/web.v1.CharactersService/ListCharacters":    false,
		"/web.v1.RankingService/GetRanking":           false,
		"/web.v1.AccountWebService/CreateAccount":     false,
		"/web.v1.ChavePixService/SalvarChavePix":      false,

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

// TestAsVinteECincoEscritasEstaoNaLista conta as escritas, e a contagem é o ponto.
//
// EXISTE PARA UMA ESCRITA NÃO FICAR DE FORA EM SILÊNCIO. Uma que falte não dá erro em
// lugar nenhum: a página simplesmente recusa salvar, com uma frase que parece falta de
// permissão, e alguém vai procurar no cargo em vez de na lista. Foi assim que o nome dos
// itens sumiu do censo, por uma linha de LEITURA que faltava aqui.
func TestAsVinteECincoEscritasEstaoNaLista(t *testing.T) {
	escritas := []string{
		"NpcAdminService/UpsertNpc",
		"NpcAdminService/SetNpcVisibility",
		"NpcAdminService/SetNpcShop",
		"NpcAdminService/SetItemPrice",
		"NpcAdminService/DeleteNpc",
		"MobTemplateAdminService/UpsertMobTemplateStat",
		"MobTemplateAdminService/SetMobTemplateEquip",
		"MobTemplateAdminService/DeleteMobTemplateStat",
		"ItemStatAdminService/UpsertItemStat",
		"ItemStatAdminService/DeleteItemStat",
		"MountGrowthAdminService/SetMountGrowthCurve",
		"MountGrowthAdminService/ClearMountGrowthCurve",
		"MountGrowthAdminService/SetMountAbsorb",
		"MountGrowthAdminService/ClearMountAbsorb",
		"MountGrowthAdminService/SetMountBonus",
		"MountGrowthAdminService/ClearMountBonus",
		"AttributeMapAdminService/TransformAttributeMap",
		"DailyRewardAdminService/UpsertRewardItem",
		"DailyRewardAdminService/SetRewardItemEnabled",
		"DailyRewardAdminService/DeleteRewardItem",
		"DonateAdminService/UpsertShopItem",
		"DonateAdminService/SetShopItemEnabled",
		"DonateAdminService/DeleteShopItem",
		"DonateAdminService/CreditDonateBalance",
		"WorldEventAdminService/SetWorldEventConfig",
	}
	if len(escritas) != 25 {
		t.Fatalf("a lista deste teste tem %d escritas, e sao 25", len(escritas))
	}
	for _, m := range escritas {
		if !PainelPodeChamar("/web.v1." + m) {
			t.Errorf("a escrita %q nao esta na lista: a pagina dela vai recusar salvar", m)
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
		if strings.Contains(MsgForaDoPainel, proibido) {
			t.Errorf("a frase manda usar %q: %q", proibido, MsgForaDoPainel)
		}
	}
	if MsgForaDoPainel == "" {
		t.Error("a recusa nao diz nada")
	}
}

// TestTodaLeituraQueOPainelFazEstaNaLista.
//
// A lista fecha por padrão, e o preço disso é ESTE teste: uma leitura que o painel faz
// e que ninguém pôs na lista não dá erro visível — a página abre e mostra o buraco.
//
// Foi o que aconteceu com o catálogo de itens entre a atualização das 23h e o conserto:
// o censo abria, contava certo, e mostrava "item 1100" em vez do nome, com a coluna do
// item vazia no quadro das cópias. Ninguém olhando a tela adivinharia que a causa era
// uma linha faltando numa lista de autorização.
//
// A lista abaixo é a das chamadas de LEITURA que o adminServer faz hoje
// (adminserver/internal/gamedata). Quem acrescentar uma leitura nova ao painel
// acrescenta aqui também, e o teste cobra a linha na lista de verdade.
func TestTodaLeituraQueOPainelFazEstaNaLista(t *testing.T) {
	leiturasDoAdminServer := []string{
		// O nome de cada item. Não é de uma página só: o censo, as cópias, a loja
		// do NPC, a mesa de drops e a recompensa diária mostram nome sobre índice.
		"/web.v1.ItemCatalogService/ListItems",
		// NPCs
		"/web.v1.NpcAdminService/ListNpcs",
		"/web.v1.NpcAdminService/GetNpc",
		"/web.v1.NpcAdminService/ListMerchantTemplates",
		"/web.v1.NpcAdminService/ListItemCatalog",
		"/web.v1.NpcAdminService/ListDropItems",
		"/web.v1.NpcAdminService/ListMobDrops",
		"/web.v1.NpcAdminService/ListItemPrices",
		"/web.v1.NpcAdminService/ListMapZones",
		// Monstros
		"/web.v1.MobTemplateAdminService/ListMobTemplates",
		"/web.v1.MobTemplateAdminService/GetMobTemplateStat",
		// Itens
		"/web.v1.ItemStatAdminService/GetItemStat",
		// Montarias
		"/web.v1.MountGrowthAdminService/ListMountGrowthCurves",
		"/web.v1.MountGrowthAdminService/ListMountBonus",
		"/web.v1.MountGrowthAdminService/ListMountAbsorb",
		"/web.v1.MountGrowthAdminService/MountConfigVersion",
		// Atributos
		"/web.v1.AttributeMapAdminService/GetAttributeMapInfo",
		// Recompensa diária
		"/web.v1.DailyRewardAdminService/ListRewardItems",
		// Loja de donate
		"/web.v1.DonateAdminService/ListShopItems",
		// Receita
		"/web.v1.DonateRevenueAdminService/GetRevenueSummary",
		"/web.v1.DonateRevenueAdminService/ListTopupOrders",
		"/web.v1.DonateRevenueAdminService/ListTopBuyers",
		"/web.v1.DonateRevenueAdminService/ListDonateSpend",
		"/web.v1.DonateRevenueAdminService/SearchAccounts",
		// Eventos do mundo
		"/web.v1.WorldEventAdminService/GetWorldEventConfig",
	}
	for _, metodo := range leiturasDoAdminServer {
		if !PainelPodeChamar(metodo) {
			t.Errorf("o painel faz esta leitura e a lista a recusa: %s", metodo)
		}
	}
}
