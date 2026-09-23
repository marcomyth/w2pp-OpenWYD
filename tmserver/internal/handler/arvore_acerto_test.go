package handler

import (
	"testing"

	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/world"
)

// As três regras de 21/09/2026 que tiraram o TK Trans, a FM Cancelamento física
// e a ponta de FORÇA do BM Natureza do fundo do torneio.

// TestPerfuracaoDoPorradeiro: a Armadura Crítica fura armadura pela FORÇA, e só
// para quem a aprendeu.
func TestPerfuracaoDoPorradeiro(t *testing.T) {
	forca := tkDoTrans(2800, 712, learnedArmaduraCritica, transMaestriaMax)
	meio := tkDoTrans(1000, 1000, learnedArmaduraCritica, transMaestriaMax)
	destreza := tkDoTrans(500, 2000, learnedArmaduraCritica, transMaestriaMax)
	semArvore := tkDoTrans(2800, 712, learnedNocaoDeCombate, transMaestriaMax)
	semMaestria := tkDoTrans(2800, 712, learnedArmaduraCritica, 0)

	if got := perfuracaoDaArmadura(forca); got != armaduraPerfuracaoForca {
		t.Errorf("Força pura, maestria cheia: perfuração = %d, want %d", got, armaduraPerfuracaoForca)
	}
	if got := perfuracaoDaArmadura(destreza); got != 0 {
		t.Errorf("Destreza pura: perfuração = %d, want 0 — a régua é a Força", got)
	}
	if got := perfuracaoDaArmadura(meio); got <= 0 || got >= armaduraPerfuracaoForca {
		t.Errorf("meio a meio: perfuração = %d, queria entre 0 e %d", got, armaduraPerfuracaoForca)
	}
	if got := perfuracaoDaArmadura(semArvore); got != 0 {
		t.Errorf("sem a Armadura Crítica: perfuração = %d, want 0", got)
	}
	if got := perfuracaoDaArmadura(semMaestria); got != 0 {
		t.Errorf("sem maestria: perfuração = %d, want 0", got)
	}
}

// TestPerfuracaoDoPorradeiroChegaNaDefesa prova que a perfuração está LIGADA à
// conta da defesa, e não apenas escrita: uma função certa que ninguém chama é o
// erro que este projeto já cometeu várias vezes.
func TestPerfuracaoDoPorradeiroChegaNaDefesa(t *testing.T) {
	d := &Dispatcher{}
	forca := tkDoTrans(2800, 712, learnedArmaduraCritica, transMaestriaMax)
	const def = 1000
	want := def * (100 - armaduraPerfuracaoForca) / 100
	if got := d.defesaPerfurada(forca, def); got != want {
		t.Fatalf("defesa enfrentada = %d, want %d", got, want)
	}
	if got := d.defesaPerfurada(tkDoTrans(2800, 712, 0, transMaestriaMax), def); got != def {
		t.Fatalf("sem a árvore a defesa tem de ficar inteira: %d, want %d", got, def)
	}
}

// TestDefesaPerfuradaNuncaFicaNegativa: uma soma de perfurações acima de 100%
// multiplicaria o golpe em vez de deixá-lo passar.
func TestDefesaPerfuradaNuncaFicaNegativa(t *testing.T) {
	d := &Dispatcher{}
	e := tkDoTrans(2800, 712, learnedArmaduraCritica, transMaestriaMax)
	velho := armaduraPerfuracaoForca
	t.Cleanup(func() { armaduraPerfuracaoForca = velho })
	armaduraPerfuracaoForca = 250
	if got := d.defesaPerfurada(e, 1000); got != 0 {
		t.Fatalf("defesa enfrentada = %d, want 0 — nunca negativa", got)
	}
}

// TestAcertoDoEixoSegueAForca: a camada da Metamorfose paga acerto à ponta de
// Força e nada à de Destreza, que já fura esquiva pela DES no próprio parry.
func TestAcertoDoEixoSegueAForca(t *testing.T) {
	forca := bmDaNatureza(2800, 712, naturezaMaestriaCheia, learnedEden|learnedMetamorfoseSuperior)
	destreza := bmDaNatureza(712, 2800, naturezaMaestriaCheia, learnedEden|learnedMetamorfoseSuperior)

	aForca := camadaDaForma(forca, formaEden, naturezaCamada.acerto)
	aDestreza := camadaDaForma(destreza, formaEden, naturezaCamada.acerto)
	if aForca <= aDestreza {
		t.Fatalf("acerto: Força %d, Destreza %d — a Força é que acerta", aForca, aDestreza)
	}
	if aDestreza != 0 {
		t.Errorf("Destreza pura: acerto = %d, want 0", aDestreza)
	}
	// 2800/712 é a build real do elenco, e ela cai a um passo do topo do eixo
	// (razão 797 de 800), não exatamente nele — por isso a margem. Quem está no
	// topo de verdade leva a camada inteira.
	if aForca < naturezaCamada.acerto[1]*99/100 {
		t.Errorf("build de Força no Éden: acerto = %d, queria perto de %d (a camada inteira)", aForca, naturezaCamada.acerto[1])
	}
	topo := bmDaNatureza(3000, 300, naturezaMaestriaCheia, learnedEden|learnedMetamorfoseSuperior)
	if got := camadaDaForma(topo, formaEden, naturezaCamada.acerto); got != naturezaCamada.acerto[1] {
		t.Errorf("topo do eixo no Éden: acerto = %d, want %d (a camada inteira)", got, naturezaCamada.acerto[1])
	}
	// E o espelho: a velocidade é da Destreza.
	if vForca, vDestreza := camadaDaForma(forca, formaEden, naturezaCamada.vel),
		camadaDaForma(destreza, formaEden, naturezaCamada.vel); vForca >= vDestreza {
		t.Errorf("velocidade: Força %d, Destreza %d — a Destreza é que corre", vForca, vDestreza)
	}
}

// TestAcertoDaMetamorfoseEntraNoScore prova que o acerto chega ao AffAccuracy,
// que é o campo que precisaoDe lê no sorteio da esquiva.
func TestAcertoDaMetamorfoseEntraNoScore(t *testing.T) {
	e := bmDaNatureza(2800, 712, naturezaMaestriaCheia, learnedEden|learnedMetamorfoseSuperior)
	e.LearnedSkill |= learnedMetamorfoseSuperior
	antes := e.AffAccuracy
	aplicarMetamorfoseSuperior(e, 4)
	if e.AffAccuracy <= antes {
		t.Fatalf("AffAccuracy = %d, era %d — a camada de acerto não chegou ao score", e.AffAccuracy, antes)
	}
	// Sem a Metamorfose Superior comprada, nada.
	sem := bmDaNatureza(2800, 712, naturezaMaestriaCheia, learnedEden|learnedMetamorfoseSuperior)
	sem.LearnedSkill &^= learnedMetamorfoseSuperior
	aplicarMetamorfoseSuperior(sem, 4)
	if sem.AffAccuracy != 0 {
		t.Fatalf("sem a Metamorfose: AffAccuracy = %d, want 0", sem.AffAccuracy)
	}
}

// TestNevoaVenenosaDaCancel: o bônus segue a empunhadura de assinatura da
// árvore e exige a 8ª — a skill 40 é a primeira, e qualquer Foema a lança.
func TestNevoaVenenosaDaCancel(t *testing.T) {
	const espada, escudo, arco = 811, 1713, 1006
	ability := armasNasMaos(map[int16]int{espada: wtypeUmaMao, arco: wtypeArco})

	duasArmas := func(learned int32) *world.Entity {
		e := &world.Entity{ID: 1, Class: 1, ClassMaster: classMasterMortal,
			Int: 2147, Dex: 712, LearnedSkill: learned}
		e.Equip[weaponSlotR] = world.Item{Index: espada}
		e.Equip[weaponSlotL] = world.Item{Index: espada}
		return e
	}
	comEscudo := duasArmas(learnedCancelamento)
	comEscudo.Equip[weaponSlotL] = world.Item{Index: escudo}

	const bruto = 1000
	want := bruto * (100 + cancelNevoaPct) / 100
	if got := danoDaNevoaVenenosa(duasArmas(learnedCancelamento), ability, bruto); got != want {
		t.Errorf("duas espadas com a 8ª: %d, want %d", got, want)
	}
	if got := danoDaNevoaVenenosa(duasArmas(0), ability, bruto); got != bruto {
		t.Errorf("sem a 8ª: %d, want %d — a Névoa é a primeira skill da árvore", got, bruto)
	}
	if got := danoDaNevoaVenenosa(comEscudo, ability, bruto); got != bruto {
		t.Errorf("com escudo: %d, want %d — o bônus é da empunhadura de duas armas", got, bruto)
	}
	if got := danoDaNevoaVenenosa(duasArmas(learnedCancelamento), ability, 0); got != 0 {
		t.Errorf("golpe que não feriu: %d, want 0", got)
	}
}
