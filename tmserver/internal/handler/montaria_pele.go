package handler

import (
	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/protocol"
	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/world"
)

// A PELE DA MONTARIA, e por que o terceiro par de efeito vai ZERADO no slot 14.
//
// MEDIDO NO WYD.exe 7662 pela dupla do cliente: todo 0x0182 de equipamento recalcula a
// aparência, e para a montaria o cliente lê o BYTE 7 do item de Equip[14] — que é o VALOR
// do terceiro par de efeito — como se fosse a "pele" do bicho:
//
//	menor que 11 ... a variante do próprio registro
//	de 11 a 21 ..... outro bicho
//	22 ou mais ..... o tigre de fogo listrado
//
// E o pulso de prazo escreve [106 dias, 107 horas, 108 minutos] nos três pares. Ou seja:
// o byte 7 é o número de MINUTOS que faltam, e a montaria TROCA DE BICHO a cada minuto.
// Nos primeiros dez minutos de cada hora ela volta ao normal, e o jogador vê a Esfera
// virar tigre listrado sozinha.
//
// O CONSERTO É NÃO MANDAR OS MINUTOS nesse slot: os pares viram [106 dias, 107 horas,
// 0 0]. O tooltip continua mostrando o prazo em dias e horas, que é o que o jogador lê.
//
// NÃO SE REORDENA OS PARES para "aproveitar" o terceiro: pôr as horas ali as faria cair
// na faixa de 11 a 21 durante metade do dia, e a montaria viraria outro bicho.
//
// O PULSO CONTINUA: é dele que o tooltip tira o prazo. Parar o pulso consertaria a pele
// e apagaria a contagem.
//
// AS CRIAS (2360-2389) NÃO MUDAM. Elas não têm prazo — os efeitos delas carregam HP e
// nível (ver mountmaster.go), e zerar o terceiro par apagaria dado de jogo.

// selDoSlot é o item no fio JÁ CORRIGIDO para o lugar onde ele vai.
//
// UMA FUNÇÃO SÓ, e é o ponto: a correção depende do SLOT, e o itemToSel não conhece o
// slot. Espalhar o ajuste por cada caminho que manda equipamento é como um deles fica
// para trás — e o que fica para trás é um caminho em que a montaria volta a trocar de
// pele, que ninguém liga ao commit meses depois.
func selDoSlot(place, slot int, it world.Item) protocol.SelItem {
	sel := itemToSel(it)
	if place == world.ItemPlaceEquip && slot == mountEquipSlot && it.ExpiresAt != 0 {
		// Só o terceiro par. Os dois primeiros são os dias e as horas, e o tooltip
		// depende deles.
		sel.Eff[2] = [2]uint8{0, 0}
	}
	return sel
}
