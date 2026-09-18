package handler

import (
	"encoding/binary"

	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/combat"
	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/protocol"
	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/world"
)

// Árvore TROCA da Huntress (skills 80-87, maestria Special[2]) — REGRAS DO
// SERVIDOR, decididas pelo Marco em 16/09/2026. Nada aqui é legado.
//
// A árvore virou a da HT de Força, de garra e espada, e quase tudo lê a mesma
// régua de build, parcelaDeForca. Os nomes são os dos livros (ItemList 5080-5087),
// que são os do cliente; o SkillData.csv traz 83/84 e 85/86 trocados.
const (
	skillGolpeFelino    = 80
	skillExtracao       = 83 // passiva "Melhoria na Esquiva"
	skillAlquimia       = 84 // passiva de chance nas máquinas (alquimia.go)
	skillEscudoDourado  = 85
	skillExplosaoEterea = 86

	// learnedTrocaDeEspirito é o bit da 8ª skill da árvore (87). Várias regras
	// da árvore sobem quando ela está aprendida.
	learnedTrocaDeEspirito = 1 << 15
	learnedExtracao        = 1 << 11
)

// temOitavaDaTroca diz se e é uma Huntress com a 8ª skill da árvore Troca. O
// bit 15 de outra classe é outra skill, por isso a classe entra na conta.
func temOitavaDaTroca(e *world.Entity) bool {
	return e.Class == 3 && e.LearnedSkill&learnedTrocaDeEspirito != 0
}

// ---------------------------------------------------------------------------
// 80 · Golpe Felino: crítico próprio da skill.
//
// Chance de 10% + 20% × f (Força pura 30%, Destreza pura 10%), e o golpe crítico
// sai multiplicado por um sorteio de x2,0 a x3,0, de 0,1 em 0,1. Entra antes da
// esquiva e das reduções de PvP, como um golpe mais forte.
const (
	golpeFelinoCritBase  = 10
	golpeFelinoCritForca = 20
)

func chanceCriticoGolpeFelino(str, dex int) int {
	return golpeFelinoCritBase + golpeFelinoCritForca*parcelaDeForca(str, dex)/1000
}

// rolarCriticoGolpeFelino devolve o multiplicador em décimos (20 a 30), ou 0
// quando não houve crítico.
func rolarCriticoGolpeFelino(r combat.Rand, str, dex int) int {
	if r.Intn(100) >= chanceCriticoGolpeFelino(str, dex) {
		return 0
	}
	return 20 + r.Intn(11)
}

// ---------------------------------------------------------------------------
// 81 · Ligação Espectral: além da maestria somada a cada golpe (a regra antiga,
// que fica), um multiplicador de dano de 10% do da Meditação — que é
// maestria/10 + 15 por cento. Vai de +1% (maestria 0) a +4% (maestria 255). Como
// o da Meditação, só aumenta o ataque físico e a Tempestade de Flechas: as skills
// de dano da HT não leem o Ataque.
func multiplicadorLigacaoEspectral(level int32) int32 {
	return (level/10 + 15) / 10
}

// ---------------------------------------------------------------------------
// 83 · Extração → passiva "Melhoria na Esquiva".
//
// Esquiva de 4% + 4% × f, e +3% com a 8ª da árvore. Em milésimos, a unidade do
// sorteio de esquiva: Destreza pura 40 (70 com a 8ª), Força pura 80 (110). Vale
// em todo sorteio de esquiva do alvo — golpe físico de jogador, skill e golpe de
// monstro —, dentro do teto de 650 do sorteio.
const (
	esquivaExtracaoBase   = 40
	esquivaExtracaoForca  = 40
	esquivaExtracaoOitava = 30
	esquivaTeto           = 650 // botão de balanceamento, ver esquivaTetoAtual
)

func melhoriaNaEsquiva(e *world.Entity) int {
	if e == nil || e.Class != 3 || e.LearnedSkill&learnedExtracao == 0 {
		return 0
	}
	b := esquivaExtracaoBase + esquivaExtracaoForca*parcelaDeForca(int(effectiveStr(e)), int(effectiveDex(e)))/1000
	if temOitavaDaTroca(e) {
		b += esquivaExtracaoOitava
	}
	return b
}

// esquivaComMelhoria soma a Melhoria na Esquiva do alvo a um resultado de
// combat.ParryRate, respeitando o teto do sorteio.
func esquivaComMelhoria(parry int, target *world.Entity) int {
	// The Captura tree multiplies the dodge first (arvore_captura.go).
	if target != nil && target.AffEsquivaPct > 0 {
		parry = parry * (100 + int(target.AffEsquivaPct)) / 100
	}
	return min(parry+melhoriaNaEsquiva(target), esquivaTetoAtual)
}

// skillPassivaDaTroca diz se skillnum virou passiva nesta árvore. O cliente ainda
// mostra as duas como lançáveis (o nome e o tipo moram nos arquivos dele); o
// servidor recusa o cast e devolve a mana.
func skillPassivaDaTroca(skillnum int) bool {
	return skillnum == skillExtracao || skillnum == skillAlquimia
}

// ---------------------------------------------------------------------------
// 85 · Escudo Dourado: não cobra mais ouro, e a defesa sobe até 500.
//
//	150 + 150 × f + 100 × (maestria ÷ 255) + 100 com a 8ª da árvore
//
// A maestria é a do cast (Affect.Level), limitada a 255.
const (
	escudoDouradoBase   = 150
	escudoDouradoForca  = 150
	escudoDouradoSkill  = 100
	escudoDouradoOitava = 100
	escudoDouradoTeto   = 500
)

func defesaEscudoDourado(e *world.Entity, level int32) int32 {
	if level < 0 {
		level = 0
	}
	if level > 255 {
		level = 255
	}
	f := int32(parcelaDeForca(int(effectiveStr(e)), int(effectiveDex(e))))
	ac := escudoDouradoBase + escudoDouradoForca*f/1000 + escudoDouradoSkill*level/255
	if temOitavaDaTroca(e) {
		ac += escudoDouradoOitava
	}
	return min(ac, escudoDouradoTeto)
}

// ---------------------------------------------------------------------------
// 86 · Explosão Etérea: skill de PvE.
//
// Alcance de 10 casas, e o próprio servidor completa os alvos: os monstros num
// raio de 3 casas em volta do alvo principal entram nas vagas vazias do pacote
// até os 13 que ele comporta. Não depende do cliente mandar todos. Jogador só é
// atingido se o cliente o mandou, pelas regras de PvP de sempre; os extras são só
// monstros.
const (
	explosaoEtereaAlcance = 10
	explosaoEtereaRaio    = 3
)

// completarAlvosDaExplosao preenche as vagas vazias de body.Dam com os monstros
// em volta do alvo principal e devolve a máscara das entradas que o servidor pôs
// (bit i = Dam[i]; 0 quando não entrou nenhuma). O pacote é promovido ao
// MSG_Attack de 13 alvos quando chega menor, e payload/h são reescritos para o
// laço de dano e o eco.
//
// O alvo principal é a primeira entrada viva e dentro do alcance: sem ela não há
// centro para a área, e nenhum extra entra — senão um alvo inventado a cem casas
// puxaria a explosão para lá.
func (d *Dispatcher) completarAlvosDaExplosao(w *world.World, e *world.Entity, h *protocol.Header, body *protocol.MsgAttackBody, payload *[]byte) uint16 {
	var centro *world.Entity
	ocupadas := 0
	ja := make(map[int32]bool, protocol.MaxTarget)
	for _, dam := range body.Dam {
		if dam.TargetID <= 0 {
			continue
		}
		ocupadas++
		ja[dam.TargetID] = true
		if centro != nil {
			continue
		}
		t := w.Entity(int(dam.TargetID))
		if t == nil || t == e || t.Mode == world.MobEmpty || t.HP <= 0 ||
			mobDistance(e.X, e.Y, t.X, t.Y) > skillReach(e, explosaoEtereaAlcance) {
			continue
		}
		centro = t
	}
	if centro == nil {
		return 0
	}

	var extras []int32
	vagas := protocol.MaxTarget - ocupadas
	for dy := int16(-explosaoEtereaRaio); dy <= explosaoEtereaRaio && len(extras) < vagas; dy++ {
		for dx := int16(-explosaoEtereaRaio); dx <= explosaoEtereaRaio && len(extras) < vagas; dx++ {
			oid, ok := w.EntityAt(centro.X+dx, centro.Y+dy)
			if !ok || world.IsPlayer(oid) || ja[int32(oid)] {
				continue
			}
			alvo := w.Entity(oid)
			if alvo == nil || alvo.Mode == world.MobEmpty || alvo.HP <= 0 || alvo.NonCombatNPC || alvo.Summoner != 0 {
				continue
			}
			extras = append(extras, int32(oid))
			ja[int32(oid)] = true
		}
	}
	if len(extras) == 0 {
		return 0
	}

	// Promove para os 13 alvos, preservando os bytes do pacote original (os campos
	// que o decoder não lê ficam como o cliente mandou).
	full := make([]byte, protocol.MsgAttackDamOffset+protocol.MaxTarget*protocol.MsgAttackDamStride)
	copy(full, *payload)
	for len(body.Dam) < protocol.MaxTarget {
		body.Dam = append(body.Dam, protocol.DamEntry{})
	}
	var mascara uint16
	i := 0
	for _, id := range extras {
		for body.Dam[i].TargetID > 0 {
			i++
		}
		body.Dam[i] = protocol.DamEntry{TargetID: id, Damage: damSkill}
		off := protocol.MsgAttackDamOffset + i*protocol.MsgAttackDamStride
		binary.LittleEndian.PutUint32(full[off:off+4], uint32(id))
		binary.LittleEndian.PutUint32(full[off+4:off+8], uint32(damSkillWire))
		mascara |= 1 << i
	}
	*payload = full
	h.Type = protocol.MsgAttack
	return mascara
}

// damSkillWire is the -1 skill sentinel as the wire stores it.
var damSkillWire = int32(damSkill)

// esquivaTetoAtual é o teto do sorteio de esquiva em vigor. É var porque é um
// botão de balanceamento varrido pela simulação (simulacao_todos_test.go).
var esquivaTetoAtual = esquivaTeto
