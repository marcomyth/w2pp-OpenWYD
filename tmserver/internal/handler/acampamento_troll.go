package handler

import (
	"github.com/jeanluca/w2pp-openwyd/internal/droprule"
	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/world"
)

// Os monstros da quest do Acampamento Troll (Release/TMsrv/run/npc/ATroll_*,
// blocos world.AcampamentoTrollGenFirst..Last). Regra nova, pedida pela equipe em
// 14/09/2026 sobre o molde do Castelo Orc: os quatro Trolls do acampamento e o
// Troll Enigma, copiados com os números do Orc, para Mortais de 320 a 400.
//
//	Troll Insano, Caçador Troll   tropa      (os números do Cavaleiro Orc)
//	Troll Mago                    seguidor   (os do Guarda do Lorde)
//	Troll Caos                    guardião   (os do Sentinela)
//	Troll Enigma                  boss       (os do Grão-Lorde)
//
// Paga em saque, nunca em XP. O saque é da Mesa de Drops (migração 0064); o que a
// Mesa não diz — o add de cada arma — sai daqui.
//
// A chave pela nome do arquivo, como a Mesa de Drops: um "criar" de GM ou uma
// exceção de status no painel sobre o mesmo template continua dentro da regra, e
// nada fora da quest é tocado.
var acampamentoTrollTemplates = map[string]bool{
	droprule.Canonical("ATroll_Enigma"):  true,
	droprule.Canonical("ATroll_Caos"):    true,
	droprule.Canonical("ATroll_Mago"):    true,
	droprule.Canonical("ATroll_Insano"):  true,
	droprule.Canonical("ATroll_Cacador"): true,
}

// acampamentoTrollBoss é o único que solta a arma com skill, sorteia os adds
// pendendo para o alto e solta os ovos de Cavalo Fantasma.
const acampamentoTrollBoss = "ATroll_Enigma"

func isAcampamentoTrollMob(mob *world.Entity) bool {
	return mob != nil && mob.TemplateName != "" && acampamentoTrollTemplates[droprule.Canonical(mob.TemplateName)]
}

// acampamentoTrollAwardsExp é falso para os monstros da quest. O zero fica aqui, e
// não só no Exp do template nem no Clan 4, pelo mesmo motivo do Orc
// (casteloOrcAwardsExp): o cmd/exptool regrava o Exp de todo monstro, e o Clan 4
// é o das evocações, que divide por 4 cada golpe que o monstro leva.
func acampamentoTrollAwardsExp(mob *world.Entity) bool {
	return !isAcampamentoTrollMob(mob)
}

// As Armas D de nível mais baixo que a quest solta, uma de cada família do par D.
// Físicas levam dano; as de lança e cajado levam magia, como o bônus de drop do
// legado as separa (refine/dropbonus.go, nUnique 44 e 47).
var (
	armasTrollFisicas = map[int16]bool{
		869: true, // Gram
		809: true, // Martelo Dragão
		910: true, // Luna
		824: true, // Arco Élfico
		935: true, // Martelo Psíquico
	}
	armasTrollMagicas = map[int16]bool{
		899: true, // Olho do Carbunkle
		854: true, // Gungnir
		902: true, // Cajado de Âmbar
	}
)

// addArma é uma linha do sorteio do add: o peso dela entre as outras, o efeito e o
// valor.
type addArma struct {
	peso   int
	efeito uint8
	valor  int
}

// Os adds do design, em bytes do próprio item. Cada número é um degrau do bônus de
// drop do legado (refine/dropbonus.go): o dano de arma anda de 9 em 9, a magia de
// 4 em 4 e o EF_SPECIALALL (a "skill") de 3 em 3, então nenhum valor aqui é
// estranho a um item que o jogo já solta.
//
// Recalibrado em 16/09/2026, pedido do Marco: todo monstro sorteia da mesma
// escada, e quanto maior o add mais raro; o teto é 63 de dano e 32 de magia.
//
//	física   27 · 36 · 45 · 54 · 63
//	mágica   12 · 16 · 20 · 24 · 28 · 32
//
// O Troll Enigma usa a mesma escada pendendo para o alto, e é o único que solta
// a skill (addTrollSkillBoss).
var (
	addTrollFisicaMob = []addArma{
		{35, efDamage, 27},
		{28, efDamage, 36},
		{20, efDamage, 45},
		{12, efDamage, 54},
		{5, efDamage, 63},
	}
	addTrollFisicaBoss = []addArma{
		{10, efDamage, 36},
		{20, efDamage, 45},
		{35, efDamage, 54},
		{35, efDamage, 63},
	}
	addTrollMagicaMob = []addArma{
		{30, efMagic, 12},
		{25, efMagic, 16},
		{18, efMagic, 20},
		{13, efMagic, 24},
		{9, efMagic, 28},
		{5, efMagic, 32},
	}
	addTrollMagicaBoss = []addArma{
		{10, efMagic, 20},
		{20, efMagic, 24},
		{35, efMagic, 28},
		{35, efMagic, 32},
	}
	// addTrollSkillBoss é a chance, em %, de uma arma do Enigma levar também o add
	// de skill, por cima do add de dano ou magia.
	addTrollSkillBoss = 20
	// skillTroll são os valores do add de skill, sorteados por igual.
	skillTroll = []int{15, 18, 21}
)

// acampamentoTrollFinish carimba o add de uma arma que um monstro da quest soltou,
// depois do bônus de drop comum.
//
// O slot 0 fica com o refino que o bônus sorteou (+0, +1 ou +2); se o bônus pôs ali
// outra coisa, a arma sai +0, para ser refinável. Os slots 1 e 2 são do design: o
// add sorteado e, numa parte das armas do Enigma, o EF_SPECIALALL. O que o bônus
// de drop tinha escrito neles sai.
func (d *Dispatcher) acampamentoTrollFinish(w *world.World, mob *world.Entity, it *world.Item) {
	if !isAcampamentoTrollMob(mob) {
		return
	}
	boss := droprule.Canonical(mob.TemplateName) == droprule.Canonical(acampamentoTrollBoss)
	var tabela []addArma
	switch {
	case armasTrollFisicas[it.Index] && boss:
		tabela = addTrollFisicaBoss
	case armasTrollFisicas[it.Index]:
		tabela = addTrollFisicaMob
	case armasTrollMagicas[it.Index] && boss:
		tabela = addTrollMagicaBoss
	case armasTrollMagicas[it.Index]:
		tabela = addTrollMagicaMob
	default:
		return
	}
	linha := sortearAddArma(w, tabela)
	if it.Effects[0].Effect != efSanc {
		it.Effects[0] = world.Effect{Effect: efSanc, Value: 0}
	}
	it.Effects[1] = world.Effect{Effect: linha.efeito, Value: uint8(linha.valor)}
	it.Effects[2] = world.Effect{}
	if boss && w.Rand().Intn(100) < addTrollSkillBoss {
		it.Effects[2] = world.Effect{Effect: efSpecialAll, Value: uint8(skillTroll[w.Rand().Intn(len(skillTroll))])}
	}
}

// sortearAddArma escolhe uma linha pelo peso.
func sortearAddArma(w *world.World, tabela []addArma) addArma {
	total := 0
	for _, l := range tabela {
		total += l.peso
	}
	n := w.Rand().Intn(total)
	for _, l := range tabela {
		if n < l.peso {
			return l
		}
		n -= l.peso
	}
	return tabela[len(tabela)-1]
}
