package handler

import (
	"github.com/jeanluca/w2pp-openwyd/internal/droprule"
	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/world"
)

// As Armas D do Deserto (pedido do Marco em 28/09/2026): a Mantícora, o Tauron
// Adamantita, o Taron Assassino e o Verme soltam Armas D pelo template desde o
// legado, e agora cada uma sai com o add do spot dos Ciclopes — dano 45 a 72 nas
// físicas, magia 20 a 32 nas lanças e cajados — e com o dobro da chance.
//
// A chance mora na Mesa de Drops (migração 0180), e é por isso que o add é
// carimbado aqui: o gancho de acabamento só roda em item que a Mesa soltou. As
// vagas do template para essas armas ficam governadas pela Mesa e não rolam mais.
//
// Em 29/09 o Deserto perdeu as Armas E e o set E (0183 e 0186, pedido do Marco:
// "Sets E e Armas E só no gelo e Kefra"), e cada arma E virou uma Arma D do mesmo
// tipo pela Mesa. As três que não estavam aqui entraram (Arco Divino, Garra
// Draconiana, Fúria Divina), e o Aeon, que só soltava arma E, passou a ser um dos
// moldes com add.
//
// armasDDoDeserto são as dezenove Armas D que esses cinco soltam, com a escada
// de cada uma: as cinco com EF_MAGIC no catálogo (as lanças Gungnir e Lança do
// Triunfo, os cajados Olho do Carbunkle, Fúria Divina e Âmbar) levam magia, o
// resto dano. O Escudo de Runas do Verme também é item D e fica de fora: é
// escudo, não arma.
var (
	armasDDoDesertoFisicas = map[int16]bool{
		809: true, // Martelo Dragão
		810: true, // Martelo Assassino
		824: true, // Arco Élfico
		825: true, // Arco Divino (0186, no lugar do Arco Guardião E)
		839: true, // Presas de Behemoth
		840: true, // Garra Draconiana (0186, no lugar da Dianus E)
		869: true, // Gram
		870: true, // Espada Vorpal
		884: true, // Lança Relâmpago (garra, sem EF_MAGIC)
		885: true, // Cruz Sagrada
		910: true, // Luna
		911: true, // Solaris
		935: true, // Martelo Psíquico
		936: true, // Mjolnir
	}
	armasDDoDesertoMagicas = map[int16]bool{
		854: true, // Gungnir
		855: true, // Lança do Triunfo
		899: true, // Olho do Carbunkle
		900: true, // Fúria Divina (0186, no lugar do Cajado Caótico E e da Força Eterna E)
		902: true, // Cajado de Âmbar
	}
)

// monstrosDasArmasD são os cinco moldes do Deserto que ganham o add.
var monstrosDasArmasD = map[string]bool{
	droprule.Canonical("Manticora"):       true,
	droprule.Canonical("Adamant_Tauron"):  true,
	droprule.Canonical("Aeon_Tauron"):     true,
	droprule.Canonical("Taron_Assassino"): true,
	droprule.Canonical("Verme_"):          true,
}

// carimbaAddArmaD põe numa Arma D do Deserto o add da escada dela, no lugar do
// bônus de drop comum, e diz se a arma era da lista.
func carimbaAddArmaD(w *world.World, it *world.Item) bool {
	var tabela []addArma
	switch {
	case armasDDoDesertoFisicas[it.Index]:
		tabela = addCiclopeFisica
	case armasDDoDesertoMagicas[it.Index]:
		tabela = addCiclopeMagica
	default:
		return false
	}
	linha := sortearAddArma(w, tabela)
	// O refino que o bônus de drop já deu fica, como no carimbo da Arma C.
	if it.Effects[0].Effect != efSanc {
		it.Effects[0] = world.Effect{Effect: efSanc, Value: 0}
	}
	it.Effects[1] = world.Effect{Effect: linha.efeito, Value: uint8(linha.valor)}
	it.Effects[2] = world.Effect{}
	return true
}

// desertoFinish carimba o add de uma Arma D que um dos quatro monstros soltou
// pela Mesa, depois do bônus de drop comum.
func (d *Dispatcher) desertoFinish(w *world.World, mob *world.Entity, it *world.Item) {
	if !monstrosDasArmasD[droprule.Canonical(mob.TemplateName)] {
		return
	}
	carimbaAddArmaD(w, it)
}
