package handler

import (
	"sort"

	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/world"
)

// População das arenas da Quest 256 (pedido de 17/09/2026). Regra NOSSA.
//
// Cada arena tem um número FIXO de monstros, e cada bloco dela é reposto por esta
// passada, a cada 12 s, até o teto do bloco. O teto NÃO depende de quem está
// dentro.
//
// POR QUE FIXO (decisão da Hanna, 17/09 à noite): a primeira versão multiplicava a
// base pelos jogadores dentro, e isso se abusa — bastava levar alts ou bots para a
// arena para multiplicar os monstros de um jogador só. O número de cada arena é
// exatamente o que a versão anterior dava para UM jogador, que é o que a conta dos
// 5 dias usou, então nada de troféu e nada da Mesa precisa ser refeito.
//
// CONSEQUÊNCIA ESCOLHIDA: com muita gente dentro, os monstros passam a ser
// disputados. Foi a troca que ela aceitou para fechar o abuso.
//
// A população de uma arena é o maior entre o que o modelo pede e o mínimo dela em
// densidadeMinimaPorArena, repartida pelos blocos em proporção ao teto de hoje
// (distribuiPopulacao). O modelo (zzplano, TROFEU=populacao, divisores corrigidos)
// pede a população de hoje no Coveiro, Jardim e Hidras e um monstro a mais por
// bloco no Kaizen e nos Elfos. Mas a Hanna viu o Jardim quase vazio, e o modelo não
// conta o tempo de andar entre monstros. Por isso o mínimo, que em 17/09/2026 levou
// o Jardim de 25 para 90 e os Elfos de 22 para 45. Dá, hoje: Coveiro 90, Jardim 90,
// Kaizen 45, Hidras 51 e Elfos 45.
//
// A reposição: TODO bloco da arena enche a cada 12 s, o da fila de 15 s e o de
// relógio (o Jardim tinha blocos de 96 s). O período do conteúdo não vale dentro
// da arena. A fila de 15 s devolve só o monstro que morreu, no lugar dele, e não
// sabe crescer com o número de jogadores; world.DespawnMob deixa de pôr esses
// blocos nela, e o relógio comum os pula (Generator.ArenaRefill).
//
// O teto da rodada segura a XP: com essas populações a melhor rota de 1 a 349 fica
// no alvo em toda faixa (246,6 h no modelo), e nenhum divisor muda.

// baseDaArenaDecimos é o que o modelo pede em cada arena, em décimos do teto de
// hoje, na ordem de quest256Steps (Coveiro, Jardim, Kaizen, Hidras, Elfos).
var baseDaArenaDecimos = [...]int{10, 10, 11, 10, 11}

// densidadeMinimaPorArena é o mínimo de monstros de cada arena, na ordem de
// quest256Steps. REGRA ESCOLHIDA, NÃO MEDIDA, e revista pelo que a Hanna vê em
// jogo: depois do mínimo de 45 em todas, o Coveiro e o Jardim continuaram vazios
// para ela, e em 17/09/2026 (fim do dia) pediu 90 nos dois. As outras três ficam em
// 45, porque nelas ela não reclamou.
//
// Não subir Kaizen, Hidras e Elfos sem falar com a planejadora: qualquer um deles
// acelera a faixa 200-349, que é quase toda a conta dos 5 dias, e as chances de
// troféu teriam de ser refeitas.
var densidadeMinimaPorArena = [...]int{90, 90, 45, 45, 45}

// O limite de carga por arena SAIU com o multiplicador: ele existia só para
// segurar quantas vezes a base podia ser multiplicada pelos jogadores dentro. Com
// a população fixa, o teto de uma arena é a própria população (no máximo 90 hoje),
// então um limite de 270 nunca seria alcançado e seria código morto.

// blocoDaArena é um bloco resolvido no boot.
type blocoDaArena struct {
	idx   int
	passo int // índice em quest256Steps
	base  int // teto do bloco, fixo
}

// resolverBlocosDasArenas acha os blocos das cinco arenas, marca cada um para
// esta passada e reparte a base. Um bloco é da arena quando algum ponto da rota
// dele fica dentro da área: o monstro anda por ali. Roda no boot, depois do
// conteúdo e do NPC do banco (InstallRespawnDelay).
func (d *Dispatcher) resolverBlocosDasArenas(w *world.World) {
	d.blocosDasArenas = d.blocosDasArenas[:0]
	d.popBaseDasArenas = [len(baseDaArenaDecimos)]int{}
	var tetos [len(baseDaArenaDecimos)][]int
	for idx := 0; idx < w.GeneratorCount(); idx++ {
		g := w.GeneratorAt(idx)
		if g == nil || g.LeaderTmpl == nil || g.MaxNumMob <= 0 ||
			world.IsWaterDungeonGenerator(idx) || world.IsEventOwnedGenerator(idx) || world.IsKefraGenerator(idx) {
			continue
		}
		passo, ok := passoDoBloco(g.SegX, g.SegY)
		if !ok {
			continue
		}
		g.ArenaRefill = true
		d.blocosDasArenas = append(d.blocosDasArenas, blocoDaArena{idx: idx, passo: passo})
		tetos[passo] = append(tetos[passo], g.MaxNumMob)
	}
	var bases [len(baseDaArenaDecimos)][]int
	for passo, ts := range tetos {
		modelo := 0
		for _, t := range ts {
			modelo += (t*baseDaArenaDecimos[passo] + 9) / 10
		}
		bases[passo] = distribuiPopulacao(ts, max(modelo, densidadeMinimaPorArena[passo]))
	}
	var prox [len(baseDaArenaDecimos)]int
	for i := range d.blocosDasArenas {
		b := &d.blocosDasArenas[i]
		b.base = bases[b.passo][prox[b.passo]]
		prox[b.passo]++
		d.popBaseDasArenas[b.passo] += b.base
	}
	for i, pop := range d.popBaseDasArenas {
		d.log.Info("arena: população", "faixa_min", quest256Steps[i].minLevel, "faixa_max", quest256Steps[i].maxLevel-1,
			"blocos", len(tetos[i]), "monstros", pop)
	}
}

// distribuiPopulacao reparte alvo monstros pelos blocos em proporção ao teto de
// hoje, pelos maiores restos (no empate, o bloco que vem antes). O modelo usa a
// mesma conta (zzplano, distribui).
func distribuiPopulacao(tetos []int, alvo int) []int {
	total := 0
	for _, t := range tetos {
		total += t
	}
	out := make([]int, len(tetos))
	if total <= 0 || alvo <= 0 {
		return out
	}
	resto := make([]int, len(tetos))
	usado := 0
	for i, t := range tetos {
		out[i] = t * alvo / total
		resto[i] = t * alvo % total
		usado += out[i]
	}
	ordem := make([]int, len(tetos))
	for i := range ordem {
		ordem[i] = i
	}
	sort.SliceStable(ordem, func(a, b int) bool { return resto[ordem[a]] > resto[ordem[b]] })
	for k := 0; k < alvo-usado; k++ {
		out[ordem[k%len(ordem)]]++
	}
	return out
}

// passoDoBloco diz de qual arena é um bloco, pelos pontos da rota.
func passoDoBloco(segX, segY [5]int16) (int, bool) {
	for i := range segX {
		if segX[i] == 0 && segY[i] == 0 {
			continue
		}
		for passo, step := range quest256Steps {
			if step.area.contains(segX[i], segY[i]) {
				return passo, true
			}
		}
	}
	return 0, false
}

// reporArenas é a passada das arenas, a cada 12 s, logo depois de generateMobs.
func (d *Dispatcher) reporArenas(w *world.World) {
	if len(d.blocosDasArenas) == 0 || d.tickCount%minTimerTicks != 0 {
		return
	}
	for _, b := range d.blocosDasArenas {
		g := w.GeneratorAt(b.idx)
		if g == nil || !g.ArenaRefill || g.Off || d.casteloOrcSuppresses(b.idx) {
			continue
		}
		teto := b.base
		// Cada grupo nasce com pelo menos um monstro; o limite de voltas só guarda
		// contra uma célula cheia que devolva grupo vazio para sempre.
		for voltas := teto; g.CurrentNumMob < teto && voltas > 0; voltas-- {
			ids := w.GenerateMobUpTo(b.idx, teto)
			if len(ids) == 0 {
				break
			}
			d.revealSpawned(w, ids)
		}
	}
}
