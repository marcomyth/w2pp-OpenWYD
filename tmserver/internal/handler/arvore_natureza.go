package handler

import (
	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/world"
)

// A ÁRVORE NATUREZA DO BEAST MASTER (skills 64-71, 20/09/2026).
//
// É a árvore das TRANSFORMAÇÕES (transform.go) e das três passivas do BM. O
// cliente renomeia as passivas, e os nomes divergem do SkillData.csv:
//
//	skill 65  "Coração_de_Lobo"    → ARMADURA ELEMENTAL    (bit 17)
//	skill 67  "Escudo_do_Tormento" → ESCUDO DO TORMENTO    (bit 19)
//	skill 69  "Asas_do_Inferno"    → METAMORFOSE SUPERIOR  (bit 21)
//
// Os três bits são os MESMOS que transform.go usa para destravar os bônus
// planos de Lobo, Urso e Astaroth. No legado um bit só faz as duas coisas
// (Basedef.cpp:4112-4114 liga os planos da forma, CMob.cpp:776-785 liga a
// passiva), e é por isso que aqui os bits são reusados em vez de redefinidos.
//
// O EIXO FORÇA↔DESTREZA é a regra central da árvore, e vale para tudo que vem
// abaixo: quanto mais Força comparada à Destreza, mais ABSORÇÃO, dano e defesa;
// quanto mais Destreza, mais velocidade de ataque e crítico. O BM escolhe entre
// aguentar e bater rápido, e não pode ter os dois.
//
// A VIDA saiu do eixo em 20/09/2026, depois do teste em jogo. As duas builds
// estavam com vida demais, e o corte que o operador pediu (−5.000 na Destreza,
// −12.000 na Força) é grande o bastante para não caber num eixo que também
// deveria dizer "a Força é a que aguenta". Hoje a camada TIRA vida dos dois
// lados, e tira mais da Força — o contrário do que o eixo diz do resto. É uma
// escolha do operador, medida contra as fichas reais do DanoPRZ e do olaola
// (simulacao_natureza_test.go), e não um acidente.
//
// A Força NÃO ganha crítico por este eixo — o dela vem da empunhadura de
// assinatura (escudo + Hermai com o Éden), que é a etapa 2. Sem isso o crítico
// sairia de graça nos dois lados do eixo e ele deixaria de ser uma escolha.

const (
	// Os bits das passivas, com o nome que o JOGADOR vê. Apontam para as
	// constantes de transform.go de propósito: um bit, dois efeitos.
	learnedArmaduraElemental   = learnedWolf     // skill 65, bit 65%24 = 17
	learnedEscudoDoTormento    = learnedBear     // skill 67, bit 19
	learnedMetamorfoseSuperior = learnedAstaroth // skill 69, bit 21

	// learnedEden é a 8ª da Natureza (skill 71, bit 71%24 = 23).
	learnedEden = 1 << 23
)

// naturezaKind é a maestria que move a árvore: Special[3], a mesma que o legado
// lê na Armadura Elemental (CMob.cpp:779).
const naturezaKind = 3

// naturezaMaestriaCheia é a maestria em que a árvore entrega o bônus inteiro. É
// o mesmo teto da Evocação (arvore_evocacao.go) — as duas árvores do BM se
// calibram no mesmo ponto, senão uma paga mais caro que a outra pelo mesmo
// investimento.
const naturezaMaestriaCheia = 320

// maestriaDaNatureza é a maestria útil de e, já com o teto.
func maestriaDaNatureza(e *world.Entity) int {
	return max(0, min(effectiveSpecial(e, naturezaKind), naturezaMaestriaCheia))
}

// bmNatureza diz se as regras da árvore valem para e: Beast Master e jogador.
//
// Diferente de bmElemental (arvore_elemental.go), NÃO exige a 8ª: as passivas da
// Natureza são compradas e valem por si, como no legado. Quem exige o Éden é
// cada regra que o pede, uma a uma.
func bmNatureza(e *world.Entity) bool {
	return e != nil && e.Class == 2 && world.IsPlayer(e.ID)
}

// ---------------------------------------------------------------------------
// O EIXO FORÇA ↔ DESTREZA.

const (
	// A faixa ÚTIL da razão, em milésimos. Ninguém joga com um atributo zerado:
	// um BM de Força pura fica perto de 800 e um de Destreza pura perto de 200,
	// então é entre esses dois que a escala tem de abrir. Normalizar pela faixa
	// inteira (0 a 1000) deixaria todo personagem real espremido no meio dela.
	eixoRazaoMin = 200
	eixoRazaoMax = 800
)

// eixoForcaDestreza diz onde e está entre a DESTREZA pura (0) e a FORÇA pura
// (1000). É o número que toda regra da árvore interpola.
//
// Lê os atributos EFETIVOS (com afeto), não os de base: um Fanatismo ou um
// Toque de Athena mudam a build enquanto duram, e é o que o jogador está
// vestindo naquele golpe que decide, não o que ele distribuiu no nível.
func eixoForcaDestreza(e *world.Entity) int {
	if e == nil {
		return 0
	}
	forca, destreza := int(effectiveStr(e)), int(effectiveDex(e))
	soma := forca + destreza
	if soma <= 0 {
		return 0
	}
	razao := forca * 1000 / soma
	t := (razao - eixoRazaoMin) * 1000 / (eixoRazaoMax - eixoRazaoMin)
	return max(0, min(t, 1000))
}

// naturezaInterpola devolve o valor do eixo entre destreza (t=0) e forca
// (t=1000), já escalado pela maestria da Natureza.
//
// É a forma de TODA regra da árvore, e o mesmo formato do pTransBonus do legado
// (transform.go): um par mínimo/máximo e um interpolador. Quem não investiu na
// árvore não leva nada, e quem investiu tudo leva o par cheio.
func naturezaInterpola(e *world.Entity, destreza, forca int) int {
	maestria := maestriaDaNatureza(e)
	if maestria <= 0 {
		return 0
	}
	v := destreza + (forca-destreza)*eixoForcaDestreza(e)/1000
	return v * maestria / naturezaMaestriaCheia
}

// ---------------------------------------------------------------------------
// A ARMADURA ELEMENTAL (skill 65) — a absorção percentual.
//
// A passiva JÁ EXISTIA no port, mas só metade dela: reflectDamage (pvp.go) faz
// o desconto PLANO do legado, (Special[3]+1)/6, e só no caminho de PvP.
//
// O plano fica onde está, porque ele resolve um problema que o percentual não
// resolve: é o que segura um bando de evocações e uma chuva de golpes pequenos,
// onde 53 de desconto por golpe apaga quase tudo. Contra a Black batendo
// milhares por segundo, porém, 53 é ruído — e é para isso que entra a camada
// nova, PERCENTUAL, que é a que o cliente anuncia ("Aumenta a absorção do
// personagem").
//
// As duas juntas são de propósito: desconto plano e absorção percentual
// defendem de ameaças opostas, e o BM Natureza precisa aguentar as duas.
const (
	// A faixa da absorção, em DÉCIMOS de percentual — décimos para a maestria
	// não comer a resolução na divisão.
	armaduraAbsDestreza = 50  // 5,0% com Destreza pura
	armaduraAbsForca    = 300 // 30,0% com Força pura

	// naturezaAbsTeto é o teto da absorção vinda desta árvore, contando o que as
	// etapas seguintes somam (a empunhadura de assinatura e a forma). Existe
	// porque absorção não reduz dano, ela MULTIPLICA a vida efetiva: 40% já é
	// 1,67× de vida, e ela ainda compõe com a montaria.
	// Hoje as três parcelas no extremo somam EXATAMENTE isto, então o clamp não
	// corta nada — ele é a trava para quando alguém subir uma faixa.
	naturezaAbsTeto = 500 // 50,0%
)

// absorcaoDaArmaduraElementalDecimos devolve a fatia que a Armadura Elemental
// tira de cada golpe, em décimos de percentual.
//
// É método do Dispatcher porque a EMPUNHADURA entra na conta, e ler a arma
// precisa do catálogo de itens.
func (d *Dispatcher) absorcaoDaArmaduraElementalDecimos(e *world.Entity) int {
	if !bmNatureza(e) || e.LearnedSkill&learnedArmaduraElemental == 0 {
		return 0
	}
	abs := naturezaInterpola(e, armaduraAbsDestreza, armaduraAbsForca)
	switch empunhaduraDaNatureza(e, d.itemAbility) {
	case empunhaduraEscudo:
		// O escudo soma, mas só na proporção da maestria, como todo o resto da
		// árvore: um BM que não investiu nela não vira tanque por equipar.
		abs += naturezaAbsEscudoAtual * maestriaDaNatureza(e) / naturezaMaestriaCheia
	case empunhaduraPenalizada:
		abs = penalidadeDaEmpunhadura(abs)
	}
	// A forma ativa soma por último (Metamorfose Superior).
	abs += absorcaoDaMetamorfose(e)
	return min(abs, naturezaAbsTeto)
}

// absorcaoDaArmaduraElemental tira a fatia da Armadura Elemental de um golpe.
//
// Vale em PvP e em PvE, e transformado ou não: é uma defesa do personagem, como
// a passiva do legado, que também não pergunta nada disso (CMob.cpp:776-780).
func (d *Dispatcher) absorcaoDaArmaduraElemental(vitima *world.Entity, dano int) int {
	if dano <= 0 {
		return dano
	}
	abs := d.absorcaoDaArmaduraElementalDecimos(vitima)
	if abs <= 0 {
		return dano
	}
	// Piso de 1, como o resto do pipeline: um golpe que chegou e marca zero é
	// lido como falha pelos dois lados.
	return max(dano-dano*abs/1000, 1)
}

// ---------------------------------------------------------------------------
// A EMPUNHADURA (etapa 2).
//
// O foco da classe são as armas de UMA MÃO: Balmung, Caliburn e Hermai, com ou
// sem escudo. Quem sai disso — espada e martelo de duas mãos, garra, lança,
// cajado, arco — não é punido por equipar, só não recebe o que a árvore paga, e
// ainda perde uma fatia do que já tinha.
//
// A DETECÇÃO é exata, e não uma aproximação: no slot da mão esquerda só cabem
// duas coisas no catálogo inteiro — escudo (nPos 128) e arma de uma mão
// (nPos 192, que é 64|128). Não existe um terceiro item com o bit do slot
// esquerdo. Então "esquerda ocupada SEM wtype" é escudo, necessariamente, e
// "esquerda ocupada COM wtype" é a segunda arma, necessariamente.
//
// O wtype da mão direita é que escolhe as armas da árvore, e ele separa mais
// fino que o nPos: o nPos 192 também carrega os 43 cajados de uma mão, que são
// arma de mago e não têm nada a ver com um BM físico.

const (
	wtypeEspadaUmaMao = 1   // espadas, adagas e maças de 1 mão — a Caliburn
	wtypeArremesso    = 103 // Hermai, Chakram, Disco de Argos, Tomahawk
)

// empunhadura é como a árvore lê as duas mãos do BM.
type empunhadura int

const (
	empunhaduraPenalizada empunhadura = iota // arma de 2 mãos, garra, lança, cajado, arco
	empunhaduraNeutra                        // arma da árvore sozinha: nem bônus nem castigo
	empunhaduraDuasArmas                     // duas de uma mão: o modo de DANO
	empunhaduraEscudo                        // escudo + arma da árvore: o modo de AGUENTAR
)

// armaDaNatureza são as armas que a árvore paga.
func armaDaNatureza(wtype int) bool {
	switch wtype {
	case wtypeEspadaUmaMao, wtypeMachadoUmaMao, wtypeArremesso:
		return true
	}
	return false
}

// empunhaduraDaNatureza lê as duas mãos e devolve qual dos quatro modos está de
// pé.
//
// Sem itemAbility o servidor não sabe o que está na mão: devolve NEUTRA, nunca
// um bônus e nunca um castigo — um nil aqui não pode dar 20% de graça nem tirar
// 20% de quem não fez nada.
func empunhaduraDaNatureza(e *world.Entity, itemAbility func(world.Item, uint8) int) empunhadura {
	if e == nil || itemAbility == nil {
		return empunhaduraNeutra
	}
	if !armaDaNatureza(itemAbility(e.Equip[weaponSlotR], efWType)) {
		return empunhaduraPenalizada
	}
	if e.Equip[weaponSlotL].Empty() {
		return empunhaduraNeutra
	}
	if itemAbility(e.Equip[weaponSlotL], efWType) == 0 {
		return empunhaduraEscudo
	}
	return empunhaduraDuasArmas
}

// Os dois que a simulação varre contra o teto de ataque da janela.
var (
	// naturezaDanoDuasArmasAtual é o prêmio de abrir mão do escudo. O dual wield
	// já rende por si (o legado soma "arma maior + metade da menor"), e isto é o
	// que o TK ganha com Mestre das Armas e a HT com Perícia do Caçador — o BM
	// não tinha equivalente nenhum.
	// ZERADO em 21/09/2026 pelo teto de ataque: a build de Destreza chega a
	// 8.787 de janela só com as duas armas na mão, antes de qualquer bônus, e o
	// teto é 9.000. Não havia onde pôr este prêmio.
	naturezaDanoDuasArmasAtual = 0

	// naturezaAbsEscudoAtual é o que o escudo soma à absorção, em décimos.
	naturezaAbsEscudoAtual = 100
)

const (
	// naturezaCriticoEscudo é o crítico da empunhadura de assinatura, no byte do
	// personagem (≈ +10% na janela).
	naturezaCriticoEscudo = 25

	// naturezaPenalidadePct é o que se perde fora das armas da árvore.
	naturezaPenalidadePct = 20
)

// penalidadeDaEmpunhadura tira a fatia de quem está fora das armas da árvore.
func penalidadeDaEmpunhadura(v int) int { return v - v*naturezaPenalidadePct/100 }

// criticoDaAssinatura é o crítico da build de FORÇA, e exige as QUATRO coisas
// juntas: o Éden aprendido, escudo na mão esquerda, arma da árvore na direita e
// mais Força que Destreza.
//
// A Destreza não entra aqui de propósito. O crítico dela vem do próprio eixo
// (etapa 3); se a Força também o ganhasse de graça, o crítico deixaria de ser
// uma escolha e viraria imposto dos dois lados.
func criticoDaAssinatura(e *world.Entity, itemAbility func(world.Item, uint8) int) int16 {
	if !bmNatureza(e) || e.LearnedSkill&learnedEden == 0 || maestriaDaNatureza(e) <= 0 {
		return 0
	}
	if empunhaduraDaNatureza(e, itemAbility) != empunhaduraEscudo {
		return 0
	}
	if eixoForcaDestreza(e) <= 500 {
		return 0
	}
	return int16(naturezaCriticoEscudo * maestriaDaNatureza(e) / naturezaMaestriaCheia)
}

// applyPassivasDaNatureza é o que a empunhadura muda no SCORE do personagem: o
// multiplicador de dano e o crítico. A absorção não passa por aqui — ela é por
// golpe, e mora em absorcaoDaArmaduraElemental.
func applyPassivasDaNatureza(e *world.Entity, itemAbility func(world.Item, uint8) int) {
	if !bmNatureza(e) || maestriaDaNatureza(e) <= 0 {
		return
	}
	switch empunhaduraDaNatureza(e, itemAbility) {
	case empunhaduraDuasArmas:
		e.AffDamageMultiPct += int32(naturezaDanoDuasArmasAtual * maestriaDaNatureza(e) / naturezaMaestriaCheia)
	case empunhaduraPenalizada:
		e.AffDamageMultiPct -= naturezaPenalidadePct
	}
	e.AffCritical += criticoDaAssinatura(e, itemAbility)
	// O Escudo do Tormento faz o AC do escudo contar em dobro.
	e.AffAC += defesaDoEscudoDoTormento(e, itemAbility)
}

// ---------------------------------------------------------------------------
// A METAMORFOSE SUPERIOR (skill 69, etapa 3).
//
// "Aumenta as capacidades de todas as transformações do BeastMaster" — é o que
// o cliente promete, e o que nunca teve código: no legado o bit 21 do BM só
// serve de portão do Astaroth (Basedef.cpp:4114), e aqui era a mesma coisa.
//
// Ela é o que resolve o problema do ÉDEN. A 8ª custa 212 pontos, a skill mais
// cara do BM, e não é a melhor forma em nada: perde do Urso em vida e do Titã
// em defesa. Com a Metamorfose ela não precisa ser — o Éden é a forma que
// recebe a CAMADA INTEIRA, e as outras quatro uma fatia dela. Deixa de ser uma
// forma entre cinco e vira a chave da árvore.
//
// A camada é o eixo de novo, agora em tudo que a transformação toca. Cada forma
// tem dois números:
//
//   - PESO: quanto da camada ela recebe. Só o Éden leva tudo.
//   - AFINIDADE: quanto ela desloca o eixo. O Lobo puxa para a Destreza, o Urso
//     e o Titã para a Força. É o que faz "algumas se beneficiam de Destreza e
//     outras de Força" sem que nenhuma vire uma segunda classe.

// naturezaPorForma é o peso e a afinidade de cada transformação, na ordem de
// transBonus (0 Lobo, 1 Urso, 2 Astaroth, 3 Titã, 4 Éden).
var naturezaPorForma = [5]struct {
	peso      int // fatia da camada, em %
	afinidade int // deslocamento do eixo, em milésimos (negativo = Destreza)
}{
	{50, -150}, // Lobo: o ágil
	{60, +200}, // Urso: o tanque
	{65, 0},    // Astaroth: o meio-termo
	{65, +150}, // Titã: a defesa
	{100, 0},   // Éden: a 8ª, amplitude cheia e sem viés
}

// faixaDoEixo é um par (Destreza pura, Força pura) que o eixo interpola.
type faixaDoEixo [2]int

// naturezaCamada são as faixas da Metamorfose.
var naturezaCamada = struct {
	abs, dano, hp, ac, vel, crit, acerto faixaDoEixo
}{
	abs: faixaDoEixo{20, 100}, // décimos de percentual: 2% a 10%
	// O DANO DA CAMADA FICOU NEGATIVO em 21/09/2026, e isso é o desenho, não um
	// remendo. A Metamorfose já COBRA em vida (hp, logo abaixo) o que paga em
	// absorção, defesa, velocidade, crítico e acerto; agora cobra em dano também.
	//
	// O motivo é o teto de ataque da janela, fixado pelo operador em 9.000 — e o
	// teto é por causa do PvE, não do PvP: ataque inflado quebra a caçada antes
	// de quebrar o duelo. Um BM Natureza transformado em Éden já chega a 9.936
	// (Força) e 10.096 (Destreza) de janela com esta camada em ZERO, porque o
	// próprio Éden do legado multiplica o dano por 1,29 (transBonus, transform.go)
	// em cima de uma arma que já rende. Não havia onde somar; havia o que tirar.
	//
	// O eixo perde o dano como diferenciador — −18 e −19 são praticamente o mesmo
	// número — e passa a separar a Força da Destreza pelas outras seis faixas:
	// absorção, vida, defesa, velocidade, crítico e acerto. É uma perda real de
	// desenho, registrada aqui para não ser redescoberta como bug.
	dano: faixaDoEixo{-19, -18}, // pontos no multiplicador
	// A vida CAI nos dois lados, e mais na Força: ver o bloco do topo. Calibrado
	// para −5.077 no DanoPRZ e −12.047 no olaola.
	hp:   faixaDoEixo{-38, -55}, // % sobre o HP pós-equipamento
	ac:   faixaDoEixo{5, 25},    // % sobre a AC pós-equipamento
	vel:  faixaDoEixo{40, 5},    // velocidade de ataque: a Destreza é que corre
	crit: faixaDoEixo{25, 0},    // byte do crítico: o da Força vem da empunhadura
	// O ACERTO é o espelho da velocidade, e fecha o eixo. A Destreza tinha DUAS
	// funções de graça: esquivar quando defende e furar esquiva quando ataca (o
	// −attackerDex de combat/critical.go:106). A ponta de Força era esquivada 33%
	// das vezes contra 5% da de Destreza, com a melhor ficha do elenco na mão —
	// bater mais forte não resolvia, porque o golpe não chegava. Isto tira da
	// Destreza a metade ofensiva do presente, e entra em milésimos na precisão
	// (precisaoDe, combat.go), que é a mesma unidade da esquiva do alvo.
	// 140 é do torneio, não do dano por segundo: acima disso a ponta de Força
	// passa o Paladino e o irmão de Destreza (150 deu 62 vitórias contra 45 do
	// Destreza), e abaixo ela não vence a poção (100 deu 40). Em 140 as duas
	// pontas do eixo empatam, 54 e 58, e as duas ficam atrás do Paladino.
	acerto: faixaDoEixo{0, 140},
}

// eixoDaForma é o eixo do personagem deslocado pela afinidade da forma.
func eixoDaForma(e *world.Entity, forma int) int {
	if forma < 0 || forma >= len(naturezaPorForma) {
		return eixoForcaDestreza(e)
	}
	return max(0, min(eixoForcaDestreza(e)+naturezaPorForma[forma].afinidade, 1000))
}

// camadaDaForma interpola um par da camada pelo eixo da forma e o corta pelo
// peso dela e pela maestria da árvore.
//
// A conta fica numa divisão só de propósito: encadear ×peso/100 e ×maestria/320
// perde os valores pequenos no arredondamento, e a velocidade e o crítico são
// pequenos.
func camadaDaForma(e *world.Entity, forma int, faixa faixaDoEixo) int {
	if forma < 0 || forma >= len(naturezaPorForma) {
		return 0
	}
	p := naturezaPorForma[forma]
	v := faixa[0] + (faixa[1]-faixa[0])*eixoDaForma(e, forma)/1000
	return v * p.peso * maestriaDaNatureza(e) / (100 * naturezaMaestriaCheia)
}

// metamorfoseAtiva devolve a forma de pé quando a Metamorfose Superior está
// comprada, e é o portão único de tudo que ela paga.
func metamorfoseAtiva(e *world.Entity) (forma int, ok bool) {
	if !bmNatureza(e) || e.LearnedSkill&learnedMetamorfoseSuperior == 0 {
		return 0, false
	}
	if maestriaDaNatureza(e) <= 0 {
		return 0, false
	}
	forma, _, ok = activeTransform(e)
	return forma, ok
}

// absorcaoDaMetamorfose é a fatia que a forma soma à absorção do personagem.
func absorcaoDaMetamorfose(e *world.Entity) int {
	forma, ok := metamorfoseAtiva(e)
	if !ok {
		return 0
	}
	return camadaDaForma(e, forma, naturezaCamada.abs)
}

// aplicarMetamorfoseSuperior soma a camada ao score, dentro do bloco da
// transformação (applyTransformScore). A absorção NÃO entra aqui: ela é por
// golpe, e sai em absorcaoDaArmaduraElementalDecimos.
//
// Os multiplicadores de HP e AC seguem a mesma ordem do resto do bloco: incidem
// sobre o valor pós-equipamento, que refreshScore já fixou antes da passagem de
// afetos, e entram como delta.
func aplicarMetamorfoseSuperior(e *world.Entity, forma int) {
	if !bmNatureza(e) || e.LearnedSkill&learnedMetamorfoseSuperior == 0 || maestriaDaNatureza(e) <= 0 {
		return
	}
	e.AffDamageMultiPct += int32(camadaDaForma(e, forma, naturezaCamada.dano))
	e.AffMaxHP += scoreMaxHP(e) * int32(camadaDaForma(e, forma, naturezaCamada.hp)) / 100
	e.AffAC += e.AC * int32(camadaDaForma(e, forma, naturezaCamada.ac)) / 100
	e.AffAttackSpeed += int32(camadaDaForma(e, forma, naturezaCamada.vel))
	e.AffCritical += int16(camadaDaForma(e, forma, naturezaCamada.crit))
	e.AffAccuracy += int32(camadaDaForma(e, forma, naturezaCamada.acerto))
}

// ---------------------------------------------------------------------------
// O ESCUDO DO TORMENTO (skill 67, etapa 4).
//
// "Bônus na defesa ao usar um escudo. Ao aprender a oitava skill, inflige dano
// a todos que atacarem o personagem." São as duas metades da tooltip, e a
// segunda nunca existiu em lugar nenhum — nem no legado.
//
// A primeira existe no legado (CMob.cpp:782-784) e nunca foi portada:
//
//	if(LearnedSkill & (1 << 19) && g_pItemList[Equip[7].sIndex].nPos == 128)
//	    Ac += (BASE_GetItemAbility(&Equip[7], EF_AC) + 1) / 7;
//
// Dividir por 7 dava +46 no melhor escudo do jogo (o Svalin, AC 323) para um
// personagem de AC 2.500: ruído. Aqui o escudo conta EM DOBRO, o que no Svalin
// vira +323 — uns 13% da defesa total, que se sente sem decidir a luta.

const (
	// tormentoDefesaPct é quanto do AC do escudo entra de novo no personagem.
	tormentoDefesaPct = 100

	// tormentoReflexaoPct é a fatia do golpe que volta em quem bateu, e
	// tormentoReflexaoTeto o máximo por golpe.
	//
	// O teto existe porque o objetivo declarado da classe é ser forte E batível:
	// uma reflexão proporcional sem teto transforma "bater no BM" em suicídio
	// para quem bate forte, que é exatamente o contrário.
	tormentoReflexaoPct  = 10
	tormentoReflexaoTeto = 200
)

// escudoDoTormentoCom diz se as regras da passiva valem agora: a skill
// comprada, escudo na mão e maestria na árvore.
//
// A forma LIVRE existe porque a defesa é somada no score (applyAffectScore),
// que não tem Dispatcher e recebe o itemAbility como parâmetro. O método logo
// abaixo é a conveniência de quem já tem o Dispatcher na mão.
func escudoDoTormentoCom(e *world.Entity, itemAbility func(world.Item, uint8) int) bool {
	return itemAbility != nil && bmNatureza(e) &&
		e.LearnedSkill&learnedEscudoDoTormento != 0 &&
		maestriaDaNatureza(e) > 0 &&
		empunhaduraDaNatureza(e, itemAbility) == empunhaduraEscudo
}

func (d *Dispatcher) escudoDoTormento(e *world.Entity) bool {
	return escudoDoTormentoCom(e, d.itemAbility)
}

// defesaDoEscudoDoTormento é o AC extra que o escudo dá a quem tem a passiva.
func defesaDoEscudoDoTormento(e *world.Entity, itemAbility func(world.Item, uint8) int) int32 {
	if !escudoDoTormentoCom(e, itemAbility) {
		return 0
	}
	ac := itemAbility(e.Equip[weaponSlotL], efAc)
	if ac <= 0 {
		return 0
	}
	return int32(ac * tormentoDefesaPct * maestriaDaNatureza(e) / (100 * naturezaMaestriaCheia))
}

// reflexaoDoEscudoDoTormento devolve quanto do golpe volta em quem bateu.
//
// Exige o ÉDEN: é a metade da passiva que a tooltip promete "ao aprender a
// oitava skill". Sem ele o escudo só defende.
func (d *Dispatcher) reflexaoDoEscudoDoTormento(vitima *world.Entity, dano int) int {
	if dano <= 0 || !d.escudoDoTormento(vitima) || vitima.LearnedSkill&learnedEden == 0 {
		return 0
	}
	r := dano * tormentoReflexaoPct * maestriaDaNatureza(vitima) / (100 * naturezaMaestriaCheia)
	return min(r, tormentoReflexaoTeto)
}

// aplicarReflexaoDoTormento devolve o golpe em quem bateu e informa quanto foi.
//
// NUNCA MATA UM JOGADOR: o atacante para em 1 de vida. A passiva existe para
// cobrar de quem bate, não para decidir a luta sozinha — e uma reflexão que
// mata faria da classe algo em que ninguém encosta, o oposto do que ela deve
// ser. Contra MONSTRO ela mata normalmente: ali não há ninguém para frustrar.
//
// Também não marca PvP nem mexe em PKPoint. Quem escolheu bater foi o atacante;
// a vítima não está atacando ninguém por estar de escudo na mão.
func (d *Dispatcher) aplicarReflexaoDoTormento(vitima, atacante *world.Entity, dano int) int {
	if atacante == nil || vitima == nil || atacante.ID == vitima.ID {
		return 0
	}
	r := d.reflexaoDoEscudoDoTormento(vitima, dano)
	if r <= 0 || atacante.HP <= 0 {
		return 0
	}
	if world.IsPlayer(atacante.ID) {
		if int32(r) >= atacante.HP {
			r = int(atacante.HP) - 1
		}
		if r <= 0 {
			return 0
		}
	}
	atacante.HP -= int32(r)
	if atacante.HP < 0 {
		atacante.HP = 0
	}
	return r
}
