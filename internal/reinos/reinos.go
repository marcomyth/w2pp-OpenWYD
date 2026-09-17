// Package reinos guarda as regras dos Reinos de Hekalotia e Akelonia que mais de
// um serviço precisa ler: onde fica a cidade dos Reinos, quem ali é monstro e de
// que lado a capa põe o jogador.
//
// Existe pelo mesmo motivo do internal/campotreino. O port decide quem é NPC pelo
// CurrentScore.Merchant (byte 104), e quase todo o exército dos Reinos tem esse
// byte ligado — Bruxa 64, Lanceiro 96, Cavaleiro Real 32, os Reis 111 —, então
// no port eles eram NPCs imortais, donos de uma linha de npc_definition. O legado
// só protege o STRUCT_MOB.Merchant (byte 17) 1, 4, 43 e 100
// (_MSG_Attack.cpp:340), e nele os Reis e os guardas levam golpe.
//
// DIVERGÊNCIA DELIBERADA, de alcance: a regra do legado vale só dentro da cidade
// dos Reinos e só para quem é de um dos dois reinos (clã 7 ou 8), por decisão do
// Marco em 17/09/2026 (Atlas de Quests, área "Reinos e Reis"). Os oráculos, o
// Kingdom_Broker e o Guarda_Real (byte 17 = 100) continuam NPCs.
package reinos

// Os dois clãs de reino de g_pClanTable (Basedef.cpp:207).
const (
	ClanHekalotia uint8 = 7 // capa azul
	ClanAkelonia  uint8 = 8 // capa vermelha
)

// A cidade dos Reinos: as duas salas do trono (handler/kingdom.go) e a faixa
// entre elas. Todos os 191 blocos do NPCGener com clã 7 ou 8 dessa região nascem
// e patrulham dentro dela; os outros monstros de clã 7/8 do mapa (as torres e os
// quartéis de RvR) ficam longe dela.
const (
	minX, minY = 1676, 1556
	maxX, maxY = 1776, 1892
)

// Contem diz se o tile está na cidade dos Reinos.
func Contem(x, y int) bool {
	return x >= minX && x <= maxX && y >= minY && y <= maxY
}

// ClanDeReino diz se o clã é de um dos dois reinos.
func ClanDeReino(clan uint8) bool {
	return clan == ClanHekalotia || clan == ClanAkelonia
}

// MonstroDoReino diz se um template de clã clan que nasce em (x, y) é monstro
// pela regra do legado mesmo com o byte de loja do port ligado: dentro da cidade
// dos Reinos, de um dos dois reinos e sem um STRUCT_MOB.Merchant (byte 17) que o
// legado protege. Fora da cidade responde sempre false.
func MonstroDoReino(mobMerchant, clan uint8, x, y int) bool {
	if !ClanDeReino(clan) || !Contem(x, y) {
		return false
	}
	switch mobMerchant {
	case 1, 4, 43, 100:
		return false
	}
	return true
}

// SlotDaCapa é Equip[15], onde o legado lê a capa (Basedef.cpp:3222).
const SlotDaCapa = 15

// ClanDaCapa é o clã que a capa dá (Basedef.cpp:3220-3244), ou 0 para capa sem
// reino e para quem está sem capa.
//
// O legado reescreve MOB.Clan com isso a cada recálculo de status. O port não:
// o clã gravado também vem da guilda, e trocá-lo mexeria na criação de guilda e
// nos convites. Por isso a capa decide o lado só no combate dos Reinos.
func ClanDaCapa(capa int16) uint8 {
	switch capa {
	case 543, 545, 734, 736, 3191, 3194, 3197:
		return ClanHekalotia
	case 544, 546, 735, 737, 3192, 3195, 3198:
		return ClanAkelonia
	}
	return 0
}

// Nome é o nome do reino de um clã, para os avisos.
func Nome(clan uint8) string {
	switch clan {
	case ClanHekalotia:
		return "Hekalotia"
	case ClanAkelonia:
		return "Akelonia"
	}
	return ""
}
