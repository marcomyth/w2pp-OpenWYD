package authz

// O QUE O USUÁRIO DO PAINEL PODE CHAMAR.
//
// ANTES ESTA LISTA ERA SÓ DE LEITURA, e a razão era boa: as escritas gravavam o autor no
// internal/store com a conta de JOGO, e um usuário do painel não tem conta de jogo. Pior,
// isso não dava erro — o autor era um BIGINT sem chave estrangeira, então a escrita
// passaria e gravaria "conta 0". Uma edição que funciona e mente sobre quem a fez é pior
// que uma que recusa: a recusa alguém conserta, o registro falso ninguém descobre.
//
// AGORA A AUDITORIA CONHECE O USUÁRIO DO PAINEL (migração 0181 e domain.Ator), então as
// escritas entram aqui. O autor certo é gravado, e "escrita sem autor" deixou de ser
// representável: o internal/store recusa antes do INSERT e a trava do banco recusa depois.
//
// A LISTA CONTINUA EXISTINDO, E FECHA POR PADRÃO. Foi a decisão mais importante desta
// entrega, e é uma correção ao plano, que dizia para a lista sumir inteira. Apagá-la
// abriria para o usuário do painel TODOS os métodos do webServer, e não só os de
// administração: a chave do painel não separa serviços, então EmblemaService,
// DonateTopupService, MercadoService, CharactersService e RankingService — que são do
// jogador e do site, e que não conferem cargo nenhum — passariam a aceitar o painel.
// Trocar uma tranca de fora por nenhuma não é destravar, é abrir.
//
// MEDIDO, e é por isso que a decisão mudou: percorri os serviços do webServer um por um
// atrás de conferência de cargo. Só o emblema, a recarga de donate, o mercado, os
// personagens e o ranking não têm — e nenhum deles é do painel.
//
// UM MÉTODO NOVO NÃO ENTRA SOZINHO: ele é recusado até alguém o pôr aqui de propósito.
// Listar o que PODE, e não o que não pode, é o que garante isso.
//
// O CARGO NÃO É CONFERIDO AQUI. Esta lista responde "este método é do painel?"; quem
// responde "esta pessoa pode esta ação?" é cada serviço, com a régua que já usava para o
// moderador. As duas perguntas são diferentes e moram em lugares diferentes de propósito:
// misturá-las faria a lista precisar saber o cargo de cada método, e ela ficaria errada
// no dia em que um cargo mudasse.

// painelPodeChamar são os métodos que um usuário do painel pode chamar.
var painelPodeChamar = map[string]bool{
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
	// Catálogo de itens. É o NOME de cada item, e quem o lê não é uma página só:
	// o censo, as cópias, a loja do NPC, a mesa de drops e a recompensa diária
	// todas mostram nome por cima de um índice. Sem esta linha a pessoa lia
	// "item 1100" em tudo, e no censo de cópias a coluna do item saía vazia —
	// foi o que aconteceu entre a atualização das 23h e este conserto.
	"ItemCatalogService/ListItems": true,
	// Itens
	"ItemStatAdminService/GetItemStat": true,
	// Montarias
	"MountGrowthAdminService/ListMountGrowthCurves": true,
	"MountGrowthAdminService/ListMountBonus":        true,
	"MountGrowthAdminService/ListMountAbsorb":       true,
	"MountGrowthAdminService/MountConfigVersion":    true,
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

	// ------------------------------------------------------------------
	// AS ESCRITAS. Vinte e cinco, e o plano falava em vinte e três: a diferença são
	// as de montaria, que o plano abreviou como "Set/Clear de Curve, Absorb e Bonus"
	// e que são seis métodos.
	//
	// Cada uma passa por uma conferência de cargo no serviço, e cada uma grava o
	// autor certo na auditoria.
	// ------------------------------------------------------------------

	// NPCs, a loja do NPC e o preço de item
	"NpcAdminService/UpsertNpc":        true,
	"NpcAdminService/SetNpcVisibility": true,
	"NpcAdminService/SetNpcShop":       true,
	"NpcAdminService/SetItemPrice":     true,
	"NpcAdminService/DeleteNpc":        true,
	// Monstros
	"MobTemplateAdminService/UpsertMobTemplateStat": true,
	"MobTemplateAdminService/SetMobTemplateEquip":   true,
	"MobTemplateAdminService/DeleteMobTemplateStat": true,
	// Itens
	"ItemStatAdminService/UpsertItemStat": true,
	"ItemStatAdminService/DeleteItemStat": true,
	// Montarias. Eram as ÚNICAS sem conferência de cargo em serviço nenhum, e
	// ganharam uma nesta entrega (mountgrowth/autorizacao.go). Sem ela, tirar a
	// tranca de fora deixaria qualquer usuário do painel mexer em montaria.
	"MountGrowthAdminService/SetMountGrowthCurve":   true,
	"MountGrowthAdminService/ClearMountGrowthCurve": true,
	"MountGrowthAdminService/SetMountAbsorb":        true,
	"MountGrowthAdminService/ClearMountAbsorb":      true,
	"MountGrowthAdminService/SetMountBonus":         true,
	"MountGrowthAdminService/ClearMountBonus":       true,
	// Atributos. NÃO grava linha de auditoria, e não é esquecimento: este método lê
	// o AttributeMap.dat e devolve um .dat novo para a pessoa trocar à mão. Não há
	// autor para gravar porque não há gravação.
	"AttributeMapAdminService/TransformAttributeMap": true,
	// Recompensa diária
	"DailyRewardAdminService/UpsertRewardItem":     true,
	"DailyRewardAdminService/SetRewardItemEnabled": true,
	"DailyRewardAdminService/DeleteRewardItem":     true,
	// Loja de donate. O CreditDonateBalance dá saldo de doação a uma conta, e está
	// aqui como as outras: quem decide o cargo que pode é o serviço, não esta lista.
	"DonateAdminService/UpsertShopItem":      true,
	"DonateAdminService/SetShopItemEnabled":  true,
	"DonateAdminService/DeleteShopItem":      true,
	"DonateAdminService/CreditDonateBalance": true,
	// Eventos do mundo
	"WorldEventAdminService/SetWorldEventConfig": true,
}

// MsgForaDoPainel é o que a pessoa lê ao chamar algo que não é do painel.
//
// SUBSTITUI A MsgEscritaAindaNao, que prometia "o conserto vem na próxima atualização" —
// esta É a atualização, e a frase virou mentira no minuto em que esta lista passou a
// aceitar escrita.
//
// A frase NÃO manda usar conta de jogo, e isso é regra da Hanna: o painel é só do usuário
// do painel, e conta de jogo não é caminho nem atalho. Mandar alguém contornar por outra
// porta é ensinar o contorno — e depois ninguém desfaz.
const MsgForaDoPainel = "Esta função não faz parte do painel."

// PainelPodeChamar diz se um usuário do painel pode chamar este método.
//
// Recebe o método COMPLETO (/web.v1.NpcAdminService/ListNpcs) e compara pelo par
// serviço/método: dois serviços podem ter um "ListShopItems" cada, e comparar só o nome
// do método liberaria o do vizinho junto.
func PainelPodeChamar(fullMethod string) bool {
	return painelPodeChamar[servicoEMetodo(fullMethod)]
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
