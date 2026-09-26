package handler

import (
	"testing"
	"time"

	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/world"
)

// O byte que o cliente lê como pele é o VALOR do terceiro par, e o cliente troca o bicho
// a partir de 11. Dez é o último valor seguro.
const maxPeleSegura = 10

// umaEsferaComPrazo devolve uma montaria com prazo que cai num minuto PERIGOSO.
//
// 37 minutos de propósito: é maior que 21, então cairia na faixa do tigre listrado. Um
// teste com 5 minutos passaria mesmo com o defeito de pé, porque 5 é um valor de pele
// válido — e é exatamente por isso que o defeito demorou a aparecer.
func umaEsferaComPrazo() world.Item {
	agora := time.Now()
	return world.Item{
		Index:     3980, // Shire
		ExpiresAt: agora.Add(3*24*time.Hour + 5*time.Hour + 37*time.Minute).Unix(),
	}
}

// TestAMontariaComPrazoNaoMandaOsMinutos.
//
// O DEFEITO: a Esfera trocava de bicho a cada minuto. O pulso de prazo escreve
// [dias, horas, minutos] nos três pares de efeito, e o cliente lê o valor do TERCEIRO
// par como a pele — então o bicho mudava junto com o relógio.
//
// Aqui se prova o conserto no lugar onde ele tem de valer: o item no fio.
func TestAMontariaComPrazoNaoMandaOsMinutos(t *testing.T) {
	sel := selDoSlot(world.ItemPlaceEquip, mountEquipSlot, umaEsferaComPrazo())

	if sel.Eff[2][1] > maxPeleSegura {
		t.Errorf("o terceiro par vai com valor %d; acima de %d o cliente troca a pele",
			sel.Eff[2][1], maxPeleSegura)
	}
	if sel.Eff[2] != [2]uint8{0, 0} {
		t.Errorf("o terceiro par = %v, queria zerado", sel.Eff[2])
	}

	// E O PRAZO CONTINUA INDO. Zerar o par errado, ou zerar demais, apagaria a
	// contagem que o tooltip mostra — o conserto viraria outro defeito.
	if sel.Eff[0][0] != efWDay || sel.Eff[0][1] != 3 {
		t.Errorf("os dias sumiram: par 1 = %v, queria [%d 3]", sel.Eff[0], efWDay)
	}
	if sel.Eff[1][0] != efHour || sel.Eff[1][1] != 5 {
		t.Errorf("as horas sumiram: par 2 = %v, queria [%d 5]", sel.Eff[1], efHour)
	}
}

// TestSoAMontariaComPrazoPerdeOTerceiroPar.
//
// O conserto tem de ser CIRÚRGICO. Zerar o terceiro par em qualquer outro caso apagaria
// dado de jogo — e o caso que mais dói são as crias de montaria, que não têm prazo e
// carregam HP e nível justamente nos efeitos.
func TestSoAMontariaComPrazoPerdeOTerceiroPar(t *testing.T) {
	// Uma cria: sem prazo, com os três pares cheios de dado de jogo.
	cria := world.Item{
		Index: 2361,
		Effects: [3]world.Effect{
			{Effect: 1, Value: 200}, {Effect: 2, Value: 37}, {Effect: 3, Value: 99},
		},
	}
	casos := []struct {
		nome  string
		place int
		slot  int
		it    world.Item
		zera  bool
	}{
		{"a cria no slot da montaria", world.ItemPlaceEquip, mountEquipSlot, cria, false},
		{"a esfera com prazo no slot da montaria", world.ItemPlaceEquip, mountEquipSlot, umaEsferaComPrazo(), true},
		{"a esfera com prazo na MOCHILA", world.ItemPlaceCarry, 3, umaEsferaComPrazo(), false},
		{"a esfera com prazo em OUTRO slot de equip", world.ItemPlaceEquip, 2, umaEsferaComPrazo(), false},
	}
	for _, c := range casos {
		t.Run(c.nome, func(t *testing.T) {
			sel := selDoSlot(c.place, c.slot, c.it)
			zerou := sel.Eff[2] == [2]uint8{0, 0}
			if zerou != c.zera {
				t.Errorf("terceiro par zerado = %v, queria %v (par = %v)", zerou, c.zera, sel.Eff[2])
			}
			if !c.zera && c.it.ExpiresAt == 0 && sel.Eff[2] != [2]uint8{3, 99} {
				t.Errorf("a cria perdeu o dado dela: par 3 = %v, queria [3 99]", sel.Eff[2])
			}
		})
	}
}
