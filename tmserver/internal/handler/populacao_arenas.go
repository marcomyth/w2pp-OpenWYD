package handler

import (
	"sort"

	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/world"
)

// População das arenas da Quest 256 (pedido de 17/09/2026). Regra NOSSA.
//
// Cada bloco de monstro das cinco arenas passa a ser reposto por esta passada, a
// cada 12 s, e cada reposição enche o bloco até o teto de agora:
//
//	teto = base do bloco × jogadores dentro
//
// com jogadores dentro contando só quem está vivo, na área e com a bandeira
// daquela arena, no mínimo 1 e no máximo o limite de carga. Um jogador tem a
// arena da base; três têm três vezes. Quem sai baixa o teto na reposição
// seguinte, e monstro vivo não é morto: a população desce conforme os monstros
// morrem.
//
// A base de uma arena é o maior entre o que o modelo pede e o mínimo dela em
// densidadeMinimaPorArena, repartida pelos blocos em proporção ao teto de hoje
// (distribuiPopulacao). O modelo (zzplano, TROFEU=populacao, divisores corrigidos)
// pede a população de hoje no Coveiro, Jardim e Hidras e um monstro a mais por
// bloco no Kaizen e nos Elfos. Mas a Hanna viu o Jardim quase vazio com uma pessoa
// só, e o modelo não conta o tempo de andar entre monstros. Por isso o mínimo, que
// em 17/09/2026 levou o Jardim de 25 para 45 e os Elfos de 22 para 45.
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

// densidadeMinimaPorArena é o mínimo de monstros por jogador em cada arena, na
// ordem de quest256Steps. REGRA ESCOLHIDA, NÃO MEDIDA, e revista pelo que a Hanna
// vê em jogo: depois do mínimo de 45 em todas, o Coveiro e o Jardim continuaram
// vazios para ela, e em 17/09/2026 (fim do dia) pediu 90 nos dois. As outras três
// ficam em 45, porque nelas ela não reclamou.
var densidadeMinimaPorArena = [...]int{90, 90, 45, 45, 45}

// cargaMaxArena é o maior número de monstros que uma arena chega a ter. REGRA
// ESCOLHIDA, NÃO MEDIDA: o mundo inteiro tem milhares de monstros e a arena é uma
// área pequena. Subiu de 240 para 270 junto com os 90 do Coveiro e do Jardim, para
// caber gente: com 90 por jogador dá três jogadores, com 45 dá seis, e nas Hidras
// (51) cinco. O log do boot diz, por arena, até quantos jogadores a base multiplica.
const cargaMaxArena = 270

// blocoDaArena é um bloco resolvido no boot.
type blocoDaArena struct {
	idx   int
	passo int // índice em quest256Steps
	base  int // teto do bloco para um jogador
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
			"blocos", len(tetos[i]), "monstros_por_jogador", pop, "ate_jogadores", d.limiteDeJogadores(i), "carga_max", cargaMaxArena)
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

// limiteDeJogadores é quantas vezes a base a arena pode chegar a ter.
func (d *Dispatcher) limiteDeJogadores(passo int) int {
	pop := d.popBaseDasArenas[passo]
	if pop <= 0 {
		return 1
	}
	return max(1, cargaMaxArena/pop)
}

// multiplicadorDaArena é por quantos jogadores a base da arena conta agora.
func (d *Dispatcher) multiplicadorDaArena(passo, jogadores int) int {
	return min(max(1, jogadores), d.limiteDeJogadores(passo))
}

// jogadoresNasArenas conta, por arena, quem está vivo, dentro da área e com a
// bandeira daquela arena. Sem a bandeira o guarda devolve à cidade no mesmo tique;
// morto não caça.
func jogadoresNasArenas(w *world.World) [len(baseDaArenaDecimos)]int {
	var n [len(baseDaArenaDecimos)]int
	w.ForEachPlayer(func(_ *world.Session, e *world.Entity) {
		if e.HP <= 0 {
			return
		}
		for passo, step := range quest256Steps {
			if e.QuestFlag == step.flag && step.area.contains(e.X, e.Y) {
				n[passo]++
				return
			}
		}
	})
	return n
}

// reporArenas é a passada das arenas, a cada 12 s, logo depois de generateMobs.
func (d *Dispatcher) reporArenas(w *world.World) {
	if len(d.blocosDasArenas) == 0 || d.tickCount%minTimerTicks != 0 {
		return
	}
	dentro := jogadoresNasArenas(w)
	for _, b := range d.blocosDasArenas {
		g := w.GeneratorAt(b.idx)
		if g == nil || !g.ArenaRefill || g.Off || d.casteloOrcSuppresses(b.idx) {
			continue
		}
		teto := b.base * d.multiplicadorDaArena(b.passo, dentro[b.passo])
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
