package handler

import (
	"github.com/jeanluca/w2pp-openwyd/internal/spawnrate"
	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/world"
)

// População das arenas da Quest 256 (pedido de 17/09/2026). Regra NOSSA.
//
// Cada bloco de monstro das cinco arenas passa a ser reposto por esta passada, no
// relógio de 12 s, e cada reposição enche o bloco até o teto de agora:
//
//	teto = ceil(MaxNumMob do conteúdo × base da arena) × jogadores dentro
//
// com jogadores dentro contando só quem está vivo, na área e com a bandeira
// daquela arena, no mínimo 1 e no máximo o limite de carga. Um jogador tem a
// arena da base; três têm três vezes. Quem sai baixa o teto na reposição
// seguinte, e monstro vivo não é morto: a população desce conforme os monstros
// morrem.
//
// A reposição:
//   - bloco da fila de 15 s (MinuteGenerate <= 0) vira uma passada de 12 s. A fila
//     devolve só o monstro que morreu, no lugar dele, e não sabe crescer com o
//     número de jogadores. world.DespawnMob deixa de pôr esses blocos na fila
//     (Generator.ArenaRefill).
//   - bloco de relógio mantém o próprio período, e na vez dele enche até o teto de
//     uma vez, em vez de um grupo só: com o teto maior que o conteúdo, um grupo só
//     não chegaria nunca.
//
// Os números saem do modelo (zzplano, TROFEU=populacao, divisores corrigidos). Um
// jogador do plano, a rodada inteira dentro, num nível do meio da faixa, precisa
// encher o teto total da rodada com morte e troféus. Com a fila virando relógio,
// Coveiro, Jardim e Hidras já enchem com a população de hoje (286%, 140% e 284%);
// Kaizen e Elfos ficavam em 85% e 87% e pedem um monstro a mais por bloco (base
// 1,1: 104% e 106%). A base nunca fica abaixo de hoje. As horas do plano não mudam
// de faixa (a melhor rota por nível vai de 248,3 h para 247,6 h de 1 a 349), então
// nenhum divisor muda.

// baseDaArenaDecimos é a base de cada arena em décimos, na ordem de quest256Steps
// (Coveiro, Jardim, Kaizen, Hidras, Elfos).
var baseDaArenaDecimos = [...]int{10, 10, 11, 10, 11}

// cargaMaxArena é o maior número de monstros que uma arena chega a ter: a corrida
// da Água Normal, a maior população que o servidor já enche de uma vez para um
// grupo (oito salas de waterRoomMobCap e os quatro do chefe). É referência de uso,
// não medida de carga.
const cargaMaxArena = waterRoomMobCap*8 + 4

// blocoDaArena é um bloco resolvido no boot.
type blocoDaArena struct {
	idx   int
	passo int // índice em quest256Steps
	base  int // teto do bloco para um jogador
}

// resolverBlocosDasArenas acha os blocos das cinco arenas e marca cada um para
// esta passada. Um bloco é da arena quando algum ponto da rota dele fica dentro da
// área: o monstro anda por ali. Roda no boot, depois do conteúdo e do NPC do banco
// (InstallRespawnDelay).
func (d *Dispatcher) resolverBlocosDasArenas(w *world.World) {
	d.blocosDasArenas = d.blocosDasArenas[:0]
	d.popBaseDasArenas = [len(baseDaArenaDecimos)]int{}
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
		base := (g.MaxNumMob*baseDaArenaDecimos[passo] + 9) / 10
		g.ArenaRefill = true
		d.blocosDasArenas = append(d.blocosDasArenas, blocoDaArena{idx: idx, passo: passo, base: base})
		d.popBaseDasArenas[passo] += base
	}
	for i, pop := range d.popBaseDasArenas {
		d.log.Info("arena: população", "faixa_min", quest256Steps[i].minLevel, "faixa_max", quest256Steps[i].maxLevel-1,
			"monstros_por_jogador", pop, "ate_jogadores", d.limiteDeJogadores(i), "carga_max", cargaMaxArena)
	}
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

// reporArenas é a passada das arenas, no relógio de 12 s, logo depois de
// generateMobs.
func (d *Dispatcher) reporArenas(w *world.World) {
	if len(d.blocosDasArenas) == 0 || d.tickCount%minTimerTicks != 0 {
		return
	}
	pass := d.tickCount / minTimerTicks
	dentro := jogadoresNasArenas(w)
	for _, b := range d.blocosDasArenas {
		g := w.GeneratorAt(b.idx)
		if g == nil || !g.ArenaRefill || g.Off || d.casteloOrcSuppresses(b.idx) {
			continue
		}
		periodo := 1
		if g.MinuteGenerate > 0 {
			periodo = spawnrate.ScaleMinutes(g.MinuteGenerate, d.spawnPercentFor(w, b.idx))
		}
		if pass%periodo != b.idx%periodo {
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
