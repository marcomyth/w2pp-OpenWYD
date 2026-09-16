package handler

import (
	"fmt"

	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/combat"
	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/world"
)

// Árvore SOBREVIVÊNCIA da Huntress (skills 72-79, maestria Special[1]) — REGRAS
// DO SERVIDOR, decididas pelo Marco em 16/09/2026. Os nomes são os dos livros
// (ItemList 5072-5079); o SkillData.csv traz 74/75 trocados.
//
// "Com a 8ª" é com a skill 79 (Tempestade de Flechas) aprendida, e f é a régua de
// build parcelaDeForca, a mesma da árvore Troca.
const (
	skillTempestadeDeFlechas = 79
	affectMeditacao          = 21

	learnedLancaDeFerro = 1 << 6 // skill 78
	learnedTempestade   = 1 << 7 // skill 79, a 8ª da árvore
)

// temOitavaDaSobrevivencia diz se e é uma Huntress com a 8ª skill da árvore
// Sobrevivência. O bit 7 de outra classe é outra skill.
func temOitavaDaSobrevivencia(e *world.Entity) bool {
	return e.Class == 3 && e.LearnedSkill&learnedTempestade != 0
}

// ---------------------------------------------------------------------------
// 74 · Agressividade: a tabela do legado fica (score_derive.go, htWeaponCoefs),
// e com a 8ª o Arco (nUnique 42) ganha +50%: 1,02 × DES + 1,08 × FOR, em vez de
// 0,68 × DES + 0,72 × FOR.
const (
	uniqueArco            = 42
	agressividadeArcoDex8 = 1.02
	agressividadeArcoStr8 = 1.08
)

// bonusAgressividade é o termo de arma da Huntress com a passiva aprendida.
func bonusAgressividade(e *world.Entity, nUnique int) int32 {
	if nUnique == uniqueArco && temOitavaDaSobrevivencia(e) {
		return dexStrWeaponBonus(e.Str, e.Dex, agressividadeArcoDex8, agressividadeArcoStr8)
	}
	return weaponTableBonus(e.Str, e.Dex, nUnique, htWeaponCoefs)
}

// ---------------------------------------------------------------------------
// 75 · Encantar Gelo: a lentidão por acerto vale com Arco (EF_WTYPE 101) e com
// Garra (41). No legado, só o arco.
const (
	wtypeArco  = 101
	wtypeGarra = 41
)

func armaDoEncantarGelo(wtype int) bool {
	return wtype == wtypeArco || wtype == wtypeGarra
}

// ---------------------------------------------------------------------------
// 77 · Meditação: o dano sobe do Value da skill (15%) até um teto, pela maestria
// do cast. Sem a 8ª fica a conta do legado, maestria/10 + Value, que chega a
// 40% na maestria 255 para qualquer build. Com a 8ª o teto vai de 40% (Destreza
// pura) a 55% (Força pura):
//
//	dano% = Value + (teto − Value) × maestria ÷ 255,  teto = 40 + 15 × f
//
// A perda de defesa continua a do legado, −(maestria/3 + 10).
const (
	meditacaoTeto        = 40
	meditacaoTetoForca8  = 15
	meditacaoMaestriaMax = 255
)

func danoMeditacaoPct(e *world.Entity, level, value int32) int32 {
	if !temOitavaDaSobrevivencia(e) {
		return level/10 + value
	}
	level = max(0, min(level, meditacaoMaestriaMax))
	f := int32(parcelaDeForca(int(effectiveStr(e)), int(effectiveDex(e))))
	teto := meditacaoTeto + meditacaoTetoForca8*f/1000
	if teto < value {
		return value
	}
	return value + (teto-value)*level/meditacaoMaestriaMax
}

// ---------------------------------------------------------------------------
// 78 · Lança de Ferro: perfuração. Golpe físico e skill de dano da Huntress
// ignoram parte da defesa do alvo, em PvE e em PvP:
//
//	perfuração % = (DES + FOR ÷ 2) ÷ 150, teto 20%
//
// A Destreza pesa o dobro, mas a Força também soma. DES 2000 / FOR 500 → 15%;
// FOR 2000 / DES 500 → 11%.
const (
	lancaDeFerroDivisor = 150
	lancaDeFerroTeto    = 20
)

func perfuracaoLancaDeFerro(e *world.Entity) int {
	if e == nil || e.Class != 3 || e.LearnedSkill&learnedLancaDeFerro == 0 {
		return 0
	}
	dex, str := max(0, int(effectiveDex(e))), max(0, int(effectiveStr(e)))
	return min((dex+str/2)/lancaDeFerroDivisor, lancaDeFerroTeto)
}

// defesaPerfurada é a defesa do alvo que o golpe de attacker enfrenta.
func defesaPerfurada(attacker *world.Entity, def int) int {
	p := perfuracaoLancaDeFerro(attacker)
	if p == 0 || def <= 0 {
		return def
	}
	return def * (100 - p) / 100
}

// ---------------------------------------------------------------------------
// 79 · Tempestade de Flechas (era Tempestade de Raios): um alvo só, recarga de
// 40 s, e 5 flechas de 40% do dano do personagem cada.
//
// Cada flecha sorteia sozinha um multiplicador de 1x a 5x, com os valores altos
// cada vez mais raros (pesos 40/30/17/9/4), e só até o teto da build:
// 1 + 4 × f — Destreza pura sempre 1x, Força pura até 5x. Abaixo de 5x os pesos
// acima do teto saem e o sorteio fica entre os que sobram. Na Força pura, duas
// flechas 5x no mesmo cast saem em ~1,5% dos casts.
//
// A soma das flechas é um golpe só: passa uma vez pela defesa (a do alvo ×3 em
// jogador, como a Tempestade do legado) e o número na tela é o total. A esquiva
// reduzida a 30% (combat.ResolveParry) e os +200 de dano da passiva ficam.
const (
	tempestadeFlechas     = 5
	tempestadeFlechaPct   = 40
	tempestadeRecargaMs   = 40_000
	tempestadeMultMaximo  = 5
	tempestadeDefesaPvPx3 = 3
)

var pesosFlecha = [tempestadeMultMaximo]int{40, 30, 17, 9, 4}

// tetoFlecha é o maior multiplicador que a build alcança (1 a 5).
func tetoFlecha(str, dex int) int {
	return 1 + (tempestadeMultMaximo-1)*parcelaDeForca(str, dex)/1000
}

// rolarFlecha sorteia o multiplicador de uma flecha. Com teto 1 não gasta sorteio.
func rolarFlecha(r combat.Rand, teto int) int {
	if teto <= 1 {
		return 1
	}
	total := 0
	for _, p := range pesosFlecha[:teto] {
		total += p
	}
	roll := r.Intn(total)
	for i, p := range pesosFlecha[:teto] {
		if roll < p {
			return i + 1
		}
		roll -= p
	}
	return teto
}

// danoBrutoTempestade devolve o dano das 5 flechas antes da defesa e a soma dos
// multiplicadores sorteados.
func danoBrutoTempestade(r combat.Rand, damage, str, dex int) (int, int) {
	teto := tetoFlecha(str, dex)
	soma := 0
	for range tempestadeFlechas {
		soma += rolarFlecha(r, teto)
	}
	return damage * tempestadeFlechaPct * soma / 100, soma
}

// tempestadeRecargaRestante devolve quanto falta, em ms, para a Tempestade poder
// ser lançada de novo. Por (conta, slot), como a Invisibilidade: relogar não zera.
func (d *Dispatcher) tempestadeRecargaRestante(s *world.Session, now uint32) uint32 {
	desde, ok := d.tempestadeRecarga[personagem{s.AccountID, s.Slot}]
	if !ok {
		return 0
	}
	if passou := now - desde; passou < tempestadeRecargaMs {
		return tempestadeRecargaMs - passou
	}
	return 0
}

func (d *Dispatcher) marcarRecargaTempestade(s *world.Session, now uint32) {
	if d.tempestadeRecarga == nil {
		d.tempestadeRecarga = map[personagem]uint32{}
	}
	d.tempestadeRecarga[personagem{s.AccountID, s.Slot}] = now
}

// msgTempestadeRecarga é a recusa do cast em recarga. O cliente com o
// SkillData.bin novo já segura o botão pelos 40 s; a mensagem é para o antigo.
const msgTempestadeRecarga = "Tempestade de Flechas em recarga: faltam %d s."

func textoRecargaTempestade(faltaMs uint32) string {
	return fmt.Sprintf(msgTempestadeRecarga, (faltaMs+999)/1000)
}
