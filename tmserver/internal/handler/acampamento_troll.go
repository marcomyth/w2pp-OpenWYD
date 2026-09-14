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

// acampamentoTrollBoss é o único que solta os adds de boss (72 de dano, 32 de
// magia e a arma com skill) e os ovos de Cavalo Fantasma.
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
// valor, e se a arma leva também um add de skill.
type addArma struct {
	peso   int
	efeito uint8
	valor  int
	skill  bool
}

// Os adds do design, em bytes do próprio item. Cada número é um degrau do bônus de
// drop do legado (refine/dropbonus.go): o dano de arma anda de 9 em 9, a magia de
// 4 em 4 e o EF_SPECIALALL (a "skill") de 3 em 3, então nenhum valor aqui é
// estranho a um item que o jogo já solta.
//
//	física   36 · 45 · 63 (raro) · 72 (só boss) · 27 com skill (só boss)
//	mágica   20 · 24 · 28 (raro) · 32 (só boss) · 20 com skill (só boss)
//
// A arma mágica com skill fica nos 20 de magia porque o design fixou 20-24-28-32
// para as magas; a física com skill cai para 27, um degrau abaixo dos 36.
var (
	addTrollFisicaMob = []addArma{
		{50, efDamage, 36, false},
		{35, efDamage, 45, false},
		{15, efDamage, 63, false},
	}
	addTrollFisicaBoss = []addArma{
		{25, efDamage, 45, false},
		{30, efDamage, 63, false},
		{25, efDamage, 72, false},
		{20, efDamage, 27, true},
	}
	addTrollMagicaMob = []addArma{
		{50, efMagic, 20, false},
		{35, efMagic, 24, false},
		{15, efMagic, 28, false},
	}
	addTrollMagicaBoss = []addArma{
		{25, efMagic, 24, false},
		{30, efMagic, 28, false},
		{25, efMagic, 32, false},
		{20, efMagic, 20, true},
	}
	// skillTroll são os valores do add de skill, sorteados por igual.
	skillTroll = []int{15, 18, 21}
)

// acampamentoTrollFinish carimba o add de uma arma que um monstro da quest soltou,
// depois do bônus de drop comum.
//
// O slot 0 fica com o refino que o bônus sorteou (+0, +1 ou +2); se o bônus pôs ali
// outra coisa, a arma sai +0, para ser refinável. Os slots 1 e 2 são do design: o
// add sorteado e, na arma com skill, o EF_SPECIALALL. O que o bônus de drop tinha
// escrito neles sai.
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
	if linha.skill {
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

// itemChaveDosTrolls é a chave que o Xamã Troll pede: o item 3223, que o catálogo
// trazia como um dos "Cupom da Sorte" sem uso, o mesmo caminho da Chave do Inferno
// (3222). Nome, ícone e descrição no cliente são um passo à parte
// (webserver/cmd/itemnovocliente).
const itemChaveDosTrolls = 3223

// acampamentoTrollKeyOneIn é de quantas entradas pagas da Quest 256 sai uma chave,
// pela flag do passo: uma a cada três na arena dos Elfos (passo 5), pedido da
// equipe em 14/09/2026. É a mesma entrada que sorteia a Chave do Rei Orc, com um
// sorteio próprio: uma entrada pode dar as duas chaves.
var acampamentoTrollKeyOneIn = map[uint8]int{5: 3}

// acampamentoTrollKeyOnEntry sorteia a chave para quem acabou de gastar um ticket
// da Quest 256 no passo dos Elfos. Só a entrada paga sorteia, pelos mesmos motivos
// da chave do Orc (casteloOrcKeyOnEntry): o Mestre Grifo leva de graça, e a chave
// no abate premiaria quem acampa na arena.
//
// O sorteio é do gerador dos eventos, não do dos drops: a ordem que os testes de
// drop e refino fixam não muda.
func (d *Dispatcher) acampamentoTrollKeyOnEntry(w *world.World, s *world.Session, e *world.Entity, step quest256Step) {
	n, ok := acampamentoTrollKeyOneIn[step.flag]
	if !ok || d.eventRNG.Intn(n) != 0 {
		return
	}
	slot := firstEmptyAccessibleCarry(e)
	if slot < 0 {
		sendClientMessage(w, s, "Bolsa cheia: a Chave dos Trolls se perdeu.")
		d.log.Info("acampamento troll key lost to a full bag", "conn", s.Conn, "quest_flag", step.flag)
		return
	}
	e.Carry[slot] = world.Item{Index: itemChaveDosTrolls}
	d.sendSlot(w, s, world.ItemPlaceCarry, slot, e.Carry[slot])
	sendClientMessage(w, s, "Você ganhou a Chave dos Trolls: ela abre o Acampamento Troll.")
	d.log.Info("acampamento troll key on quest entry", "conn", s.Conn, "quest_flag", step.flag)
}
