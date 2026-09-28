// Package ciclopes é o spot dos Ciclopes Cruéis e dos Lanceiros Zakum, pedido
// pelo Marco em 26/09/2026 com print em (2239,1333): x 2195-2285, y 1300-1380.
//
// Os dois templates chegam com o byte de NPC de serviço que este port lê
// (CurrentScore.Merchant, @104) em 64, e o port trata qualquer valor diferente de
// zero como loja ou banco: o monstro nascia intocável e parado. O legado decide a
// imunidade pelo OUTRO byte, STRUCT_MOB.Merchant (@17), e só para 1, 4, 43 e 100
// (_MSG_Attack.cpp:339); nos dois ele é 0. É a mesma exceção da masmorra da Água,
// do Campo de Treino e dos Reinos, aqui por template: o Ciclope Cruel apanha em
// todo o mapa, porque o saque novo dele vale em todo o mapa; o Lanceiro Zakum só
// na cópia do spot, porque o resto dele não foi pedido.
package ciclopes

import "strings"

// Os templates do spot. As cópias têm o mesmo nome visível do original (o jogador
// vê "Ciclope_Cruel" e "Lanceiro_Zakum"); o nome do ARQUIVO é o que a Mesa de
// Drops e o NPCGener usam, e é por ele que o saque e o spawn 3x valem só aqui.
const (
	CiclopeCruel      = "Ciclope_Cruel"
	CiclopeCruelSpot  = "Ciclope_Cruel_Spot"
	LanceiroZakumSpot = "Lanceiro_Zakum_Spot"
	// Chefe é o chefe do spot: o corpo do Ciclope Cruel com os números do Troll
	// Enigma, de 4 em 4 horas (handler/ciclopes.go).
	Chefe = "Ciclope_Tirano"
)

// deCombate são os templates que apanham mesmo com o byte 64: todo Ciclope Cruel,
// e as cópias do spot.
var deCombate = map[string]bool{
	canonico(CiclopeCruel):      true,
	canonico(CiclopeCruelSpot):  true,
	canonico(LanceiroZakumSpot): true,
}

// MonstroDeCombate diz se um monstro destes templates apanha, pela regra do
// legado: o byte @17 zerado. Um template da lista com o byte @17 marcado continua
// NPC — a exceção é sobre o byte errado, não sobre o nome.
func MonstroDeCombate(template string, mobMerchant uint8) bool {
	return mobMerchant == 0 && deCombate[canonico(template)]
}

// canonico é a mesma chave da Mesa de Drops (droprule.Canonical): minúsculas e
// sem o ponto final que alguns arquivos do legado trazem.
func canonico(nome string) string {
	return strings.ToLower(strings.TrimRight(nome, "."))
}
