package handler

import (
	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/combat"
	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/world"
)

// Árvore CAPTURA da Huntress (skills 88-95, maestria Special[3]) — REGRAS DO
// SERVIDOR, decididas pelo Marco em 17/09/2026. Os nomes são os dos livros
// (ItemList 5088-5095); o SkillData.csv traz 90-92 embaralhados.
//
// "Com a 8ª" é com a Invisibilidade (95) aprendida. f é a régua parcelaDeForca
// (0 Destreza pura, 1000 Força pura) e "mais Força que Destreza" é a metade de
// cima dela, forcaAlemDaDestreza: 0 no meio a meio, 1000 na Força pura.
const (
	skillLaminaDasSombras = 88
	affectEvasao          = 26

	learnedVisaoDeCacadora    = 1 << 18 // skill 90
	learnedLaminaAerea        = 1 << 21 // skill 93
	learnedProtecaoDasSombras = 1 << 22 // skill 94
	learnedInvisibilidade     = 1 << 23 // skill 95, a 8ª da árvore

	capturaMaestriaMax = 255
)

func temOitavaDaCaptura(e *world.Entity) bool {
	return e.Class == 3 && e.LearnedSkill&learnedInvisibilidade != 0
}

// maestriaCaptura é a maestria da árvore, limitada a 255 para as contas.
func maestriaCaptura(e *world.Entity) int {
	return max(0, min(effectiveSpecial(e, 3), capturaMaestriaMax))
}

func forcaDe(e *world.Entity) int {
	return parcelaDeForca(int(effectiveStr(e)), int(effectiveDex(e)))
}

// forcaAlemDaDestreza é 0 enquanto a Destreza for igual ou maior que a Força, e
// chega a 1000 na Força pura (3× a Destreza).
func forcaAlemDaDestreza(e *world.Entity) int {
	return max(0, 2*forcaDe(e)-1000)
}

// ---------------------------------------------------------------------------
// 88 · Lâmina das Sombras: dano × (1 + 0,5 × f); com a 8ª, crítico próprio com
// chance de 10% + 20% × f e multiplicador de 2x + 1x × f (2x Destreza pura, 3x
// Força pura).
const (
	laminaSombrasForcaPct    = 50
	laminaSombrasCritBase    = 10
	laminaSombrasCritForca   = 20
	laminaSombrasMultBase10  = 20 // décimos
	laminaSombrasMultForca10 = 10
)

// danoLaminaDasSombras aplica o ganho de Força e, com a 8ª, o crítico. Devolve o
// dano e se houve crítico.
func danoLaminaDasSombras(r combat.Rand, e *world.Entity, dmg int) (int, bool) {
	if dmg <= 0 {
		return dmg, false
	}
	f := forcaDe(e)
	dmg = dmg * (1000 + laminaSombrasForcaPct*f/100) / 1000
	if !temOitavaDaCaptura(e) {
		return dmg, false
	}
	if r.Intn(100) >= laminaSombrasCritBase+laminaSombrasCritForca*f/1000 {
		return dmg, false
	}
	return dmg * (laminaSombrasMultBase10 + laminaSombrasMultForca10*f/1000) / 10, true
}

// ---------------------------------------------------------------------------
// Esquiva da árvore: a Evasão Aprimorada (89) e a Proteção das Sombras (94)
// multiplicam a esquiva do personagem — +50% leva 30% a 45%. A soma das duas
// para em +125%, e o sorteio continua com o teto de 65% (esquivaComMelhoria).
const esquivaCapturaTeto = 125

// 89 · Evasão Aprimorada: com o buff, a esquiva sobe por forcaAlemDaDestreza até
// o máximo da arma.
var evasaoMaxPct = map[int][2]int{ // EF_WTYPE → {sem a 8ª, com a 8ª}
	wtypeArco:  {50, 75},
	wtypeGarra: {75, 110},
}

func bonusEvasaoAprimorada(e *world.Entity, wtype int) int {
	teto, ok := evasaoMaxPct[wtype]
	if !ok {
		return 0
	}
	maxPct := teto[0]
	if temOitavaDaCaptura(e) {
		maxPct = teto[1]
	}
	return maxPct * forcaAlemDaDestreza(e) / 1000
}

// ---------------------------------------------------------------------------
// 90 · Visão de Caçadora (passiva): crítico e defesa pela maestria. A Força puxa
// para o crítico e a Destreza para a defesa:
//
//	crítico = teto × maestria/255 × (0,4 + 0,6 × f)
//	defesa  = 300  × maestria/255 × (0,4 + 0,6 × (1 − f))
//
// O teto do crítico é 10% na janela C, 20% com soul. A janela mostra o byte de
// crítico × 0,4% (WYD.exe 0x44AE82), então 10% é o byte 25 e 20% o byte 50.
const (
	visaoCriticoTeto     = 25
	visaoCriticoTetoSoul = 50
	visaoDefesaTeto      = 300
)

func pesoVisao(parcela int) int { return 400 + 600*parcela/1000 }

func criticoVisaoDeCacadora(e *world.Entity) int {
	teto := visaoCriticoTeto
	if e.Soul != 0 {
		teto = visaoCriticoTetoSoul
	}
	return teto * maestriaCaptura(e) * pesoVisao(forcaDe(e)) / (capturaMaestriaMax * 1000)
}

func defesaVisaoDeCacadora(e *world.Entity) int32 {
	return int32(visaoDefesaTeto * maestriaCaptura(e) * pesoVisao(1000-forcaDe(e)) / (capturaMaestriaMax * 1000))
}

// ---------------------------------------------------------------------------
// 94 · Proteção das Sombras (passiva, só com Garra): defesa e esquiva pela Força
// e pela maestria.
//
//	fator   = 0,5 × f + 0,5 × maestria/255
//	defesa  = 600 × fator (800 com a 8ª)
//	esquiva = +50% × fator (+65% com a 8ª)
//
// O legado ligava esta passiva pelo bit da Invisibilidade (1 << 23, com "// 22"
// no comentário) e sem olhar a arma; aqui é o bit da própria 94.
const (
	protecaoDefesa      = 600
	protecaoDefesa8     = 800
	protecaoEsquivaPct  = 50
	protecaoEsquivaPct8 = 65
)

func fatorProtecaoDasSombras(e *world.Entity) int {
	return (forcaDe(e) + 1000*maestriaCaptura(e)/capturaMaestriaMax) / 2
}

// ---------------------------------------------------------------------------
// applyPassivasDaCaptura grava a parte da árvore que depende de atributo e de
// arma: a esquiva (AffEsquivaPct) e a defesa da Visão e da Proteção (AffAC). Roda
// no fim do score de afetos, depois de os atributos dos buffs estarem somados;
// evasao diz se o buff da 89 está ativo.
func applyPassivasDaCaptura(e *world.Entity, itemAbility func(world.Item, uint8) int, evasao bool) {
	e.AffEsquivaPct = 0
	if e.Class != 3 || !world.IsPlayer(e.ID) {
		return
	}
	wtype := 0
	if itemAbility != nil {
		wtype = itemAbility(e.Equip[weaponSlotR], efWType)
	}
	esquiva := 0
	if evasao {
		esquiva += bonusEvasaoAprimorada(e, wtype)
	}
	if e.LearnedSkill&learnedVisaoDeCacadora != 0 {
		e.AffAC += defesaVisaoDeCacadora(e)
	}
	if e.LearnedSkill&learnedProtecaoDasSombras != 0 && wtype == wtypeGarra {
		fator := fatorProtecaoDasSombras(e)
		defesa, esq := protecaoDefesa, protecaoEsquivaPct
		if temOitavaDaCaptura(e) {
			defesa, esq = protecaoDefesa8, protecaoEsquivaPct8
		}
		e.AffAC += int32(defesa * fator / 1000)
		esquiva += esq * fator / 1000
	}
	e.AffEsquivaPct = int32(min(esquiva, esquivaCapturaTeto))
}

// ---------------------------------------------------------------------------
// 92 · Toxina de Serpente: com Garra, todo acerto envenena (100%), em jogador e
// em monstro, com a duração do legado. Em jogador o tick segue o do legado
// (−1000); em monstro vai de 0 ao teto pela maestria, e o teto sobe com a vida
// do monstro: MaxHP/100, entre 2.000 e 5.000 (200 mil de vida → 2.000, 500 mil
// ou mais → 5.000).
//
// O slot do veneno em monstro leva toxinaMarca no Value, para o tick saber que é
// da Toxina e não o veneno fixo de uma magia de monstro; o Level é a maestria.
const (
	toxinaMarca      = 0xFF
	toxinaMobTetoMin = 2000
	toxinaMobTetoMax = 5000
)

func danoToxinaEmMonstro(e *world.Entity, maestria int) int32 {
	teto := max(toxinaMobTetoMin, min(int(effectiveMaxHP(e))/100, toxinaMobTetoMax))
	return int32(teto * max(0, min(maestria, capturaMaestriaMax)) / capturaMaestriaMax)
}

// marcarVenenoDaToxina marca o slot de veneno de um monstro como da Toxina.
func marcarVenenoDaToxina(target *world.Entity, maestria int) {
	if world.IsPlayer(target.ID) {
		return
	}
	for i := range target.Affect {
		if target.Affect[i].Type == affectPoison {
			target.Affect[i].Value = toxinaMarca
			target.Affect[i].Level = uint16(max(0, maestria))
			return
		}
	}
}

// ---------------------------------------------------------------------------
// 93 · Lâmina Aérea (passiva): chance de golpe extra, que aparece junto do golpe
// como no legado ("3K crítico + 999").
//
//	chance = máximo × (0,5 × f + 0,5 × maestria/255)
//	máximo: Garra 60% (75% com a 8ª); outras armas 35% (50%)
//	dano   = (maestria + FOR) pela defesa do alvo, ÷ 2, mínimo 60 — o do legado
//	         — e nunca mais que 30% do golpe que acompanha, para não virar hit-kill
//	         agora que sai com frequência.
const (
	laminaAereaGarra       = 60
	laminaAereaGarra8      = 75
	laminaAereaOutras      = 35
	laminaAereaOutras8     = 50
	laminaAereaTetoDoGolpe = 30
	laminaAereaMinimo      = 60
)

func chanceLaminaAerea(e *world.Entity, wtype int) int {
	maximo := laminaAereaOutras
	switch {
	case wtype == wtypeGarra && temOitavaDaCaptura(e):
		maximo = laminaAereaGarra8
	case wtype == wtypeGarra:
		maximo = laminaAereaGarra
	case temOitavaDaCaptura(e):
		maximo = laminaAereaOutras8
	}
	return maximo * (forcaDe(e) + 1000*maestriaCaptura(e)/capturaMaestriaMax) / 2000
}

// limitarLaminaAerea aplica o mínimo do legado e o teto de 30% do golpe.
func limitarLaminaAerea(extra, golpe int) int {
	if extra > 0 {
		extra /= 2
	}
	extra = max(extra, laminaAereaMinimo)
	return min(extra, max(1, golpe*laminaAereaTetoDoGolpe/100))
}
