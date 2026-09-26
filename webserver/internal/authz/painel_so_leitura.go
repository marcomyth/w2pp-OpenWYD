package authz

// O QUE O USUÁRIO DO PAINEL PODE FAZER ENQUANTO A AUDITORIA NÃO O CONHECE.
//
// O conserto que deixa o painel autorizar por usuário do painel destrava as LEITURAS das
// oito páginas de administração. As ESCRITAS ficam de fora, e por um motivo que não se vê
// olhando a tela: elas gravam auditoria dentro do internal/store com `actor_account_id`,
// e o usuário do painel não tem conta de jogo.
//
// E ISSO NÃO DARIA ERRO. As tabelas `npc_audit` e `donate_shop_audit` guardam o autor num
// BIGINT SEM chave estrangeira, então a escrita PASSARIA e gravaria "conta 0" como autor.
// Uma edição que funciona e mente sobre quem a fez é pior que uma que recusa: a recusa
// alguém conserta, o registro falso ninguém descobre.
//
// A LISTA É DE LEITURA E FECHA POR PADRÃO. Um método novo não entra sozinho: ele é
// recusado até alguém o pôr aqui de propósito. O contrário — listar as escritas — faria
// um método novo nascer liberado, que é exatamente o acidente que esta lista existe para
// impedir.
//
// Consertar de verdade é passar os dois atores até o internal/store, o que reconstrói o
// tmServer e derruba quem está jogando. Vai na próxima atualização, e então esta lista
// inteira some.

// leituraDoPainel são os métodos que um usuário do painel pode chamar hoje.
var leituraDoPainel = map[string]bool{
	// NPCs
	"NpcAdminService/ListNpcs":              true,
	"NpcAdminService/GetNpc":                true,
	"NpcAdminService/ListMerchantTemplates": true,
	"NpcAdminService/ListItemCatalog":       true,
	"NpcAdminService/ListDropItems":         true,
	"NpcAdminService/ListMobDrops":          true,
	"NpcAdminService/ListItemPrices":        true,
	"NpcAdminService/ListMapZones":          true,
	// Monstros
	"MobTemplateAdminService/ListMobTemplates":   true,
	"MobTemplateAdminService/GetMobTemplateStat": true,
	// Itens
	"ItemStatAdminService/GetItemStat": true,
	// Atributos
	"AttributeMapAdminService/GetAttributeMapInfo": true,
	// Recompensa diária
	"DailyRewardAdminService/ListRewardItems": true,
	// Loja de donate
	"DonateAdminService/ListShopItems": true,
	// Receita
	"DonateRevenueAdminService/GetRevenueSummary": true,
	"DonateRevenueAdminService/ListTopupOrders":   true,
	"DonateRevenueAdminService/ListTopBuyers":     true,
	"DonateRevenueAdminService/ListDonateSpend":   true,
	"DonateRevenueAdminService/SearchAccounts":    true,
	// Eventos do mundo
	"WorldEventAdminService/GetWorldEventConfig": true,
}

// MsgEscritaAindaNao é o que a pessoa lê ao tentar salvar.
//
// A frase NÃO manda usar conta de jogo, e isso é regra da Hanna: o painel é só do usuário
// do painel, e conta de jogo não é caminho nem atalho. Mandar alguém contornar por outra
// porta é ensinar o contorno — e depois ninguém desfaz.
const MsgEscritaAindaNao = "Esta edição ainda não está disponível para o seu usuário do painel; " +
	"o conserto vem na próxima atualização."

// PainelPodeChamar diz se um usuário do painel pode chamar este método.
//
// Recebe o método COMPLETO (/web.v1.NpcAdminService/ListNpcs) e compara pelo par
// serviço/método: dois serviços podem ter um "ListShopItems" cada, e comparar só o nome
// do método liberaria o do vizinho junto.
func PainelPodeChamar(fullMethod string) bool {
	return leituraDoPainel[servicoEMetodo(fullMethod)]
}

// servicoEMetodo recorta "/web.v1.NpcAdminService/ListNpcs" em "NpcAdminService/ListNpcs".
// Um método ilegível devolve vazio, que não está em lista nenhuma — fechado, aqui também.
func servicoEMetodo(fullMethod string) string {
	svc := servicoDe(fullMethod)
	if svc == "" {
		return ""
	}
	i := len(fullMethod) - 1
	for i >= 0 && fullMethod[i] != '/' {
		i--
	}
	if i < 0 || i == len(fullMethod)-1 {
		return ""
	}
	return svc + "/" + fullMethod[i+1:]
}
