package handler

import "github.com/jeanluca/w2pp-openwyd/tmserver/internal/world"

// O divisor de dano dos chefes.
//
// O legado não dá vida de chefe somando vida: ele põe um item no slot da fada
// (Equip[13]) do monstro e DIVIDE todo dano que ele recebe. São três itens, e o
// nome deles no catálogo mente sobre o fator:
//
//	786  "Item_de_2x_HP(Montro)"   divisor = valor
//	1936 "10x_HP_Item(Monstro)"    divisor = valor × 10
//	1937 "20x_HP_Item(Monstro)"    divisor = valor × 1.000
//
// O "valor" é o stEffect[0].cValue do item — na prática o refino (EF_SANC) que o
// template gravou —, com mínimo 2. Um Kefra com um 1937 sem refino divide por
// 2.000: 84.000 de vida no papel viram 168 milhões na luta.
//
// O legado repete esse bloco em quatro lugares, com fatores DIFERENTES em dois
// deles, e o port mantém cada um como está:
//
//	_MSG_Attack.cpp:1570      golpe e magia de jogador   ÷ divisor
//	Server.cpp:10066          golpe de monstro           ÷ divisor
//	ProcessSecMinTimer.cpp:2339  veneno e tiques         ÷ divisor, e o 786 dobra
//	_MSG_Attack.cpp:649       cura                       ÷ outro fator (foemaHealAmount)
//
// Duas consequências que valem mais que a conta: o número que o cliente desenha
// continua sendo o golpe inteiro (o legado só mexe no HP, nunca no m->Dam), e um
// golpe menor que o divisor tira ZERO — quem bate fraco não arranha o chefe.
const (
	itemDivisorDobro = 786
	itemDivisor10x   = 1936
	itemDivisor20x   = 1937
)

// valorDoDivisor é o `itemSanc` do legado: stEffect[0].cValue, com mínimo 2.
//
// Lê o efeito 0 pela POSIÇÃO, e não procurando o EF_SANC como itemRawSanc faz,
// porque é o que o legado faz — e a diferença aparece: o slot 13 do Kefra tem os
// três pares zerados, e é o mínimo 2 que decide o divisor dele.
func valorDoDivisor(it world.Item) int32 {
	if v := int32(it.Effects[0].Value); v > 2 {
		return v
	}
	return 2
}

// divisorDeGolpe é o divisor do golpe, da magia e do veneno de jogador
// (_MSG_Attack.cpp:1570, Server.cpp:10066). 1 = o alvo não carrega divisor.
func divisorDeGolpe(e *world.Entity) int32 {
	it := e.Equip[world.DividerEquipSlot]
	switch it.Index {
	case itemDivisorDobro:
		return valorDoDivisor(it)
	case itemDivisor10x:
		return valorDoDivisor(it) * 10
	case itemDivisor20x:
		return valorDoDivisor(it) * 1000
	}
	return 1
}

// divisorDeTique é o mesmo divisor no tique do relógio (veneno), onde o legado
// dobra o do item 786 e deixa os outros dois iguais (ProcessSecMinTimer.cpp:2339).
func divisorDeTique(e *world.Entity) int32 {
	if e.Equip[world.DividerEquipSlot].Index == itemDivisorDobro {
		return divisorDeGolpe(e) * 2
	}
	return divisorDeGolpe(e)
}

// danoNoPortador é o que sai da vida do alvo depois do divisor. O chamador
// continua mandando ao cliente o dano inteiro, como o legado.
func danoNoPortador(e *world.Entity, dano int) int {
	if d := divisorDeGolpe(e); d > 1 {
		return dano / int(d)
	}
	return dano
}

// tiqueNoPortador é danoNoPortador para o tique do relógio.
func tiqueNoPortador(e *world.Entity, dano int32) int32 {
	if d := divisorDeTique(e); d > 1 {
		return dano / d
	}
	return dano
}
