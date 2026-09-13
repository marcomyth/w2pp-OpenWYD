package handler

import "github.com/jeanluca/w2pp-openwyd/tmserver/internal/world"

// O relógio de dez minutos das arenas de quest (ProcessSecMinTimer.cpp:560-578).
//
// O que o legado faz, e o que faltava aqui: a cada `SecCounter % 1200 == 0` ele
// esvazia dez áreas e, logo depois, zera o QuestFlag de TODO jogador em jogo.
// SecCounter anda a cada 500 ms (SetTimer TIMER_SEC, Server.cpp:4086), então
// 1200 passagens são 600 segundos — dez minutos.
//
// Sem esse relógio o passe nunca vencia. guardQuest256Areas só expulsa quem está
// numa arena com a bandeira ERRADA (mobai.go), e nada mais zerava a bandeira
// fora do próprio recall: quem entrava com a bandeira certa ficava lá para
// sempre, ganhando experiência em qualquer nível, porque o nível só é conferido
// na ENTRADA — no NPC e no bilhete. O legado nunca conferiu nível ali dentro
// tampouco; o que ele tinha, e nós não, era a hora de ir embora.
//
// Vale para as cinco quests de uma vez: é uma tabela só, um guarda só e este
// relógio só.
const questClearTicks = 600 // 600 tiques de 1 s = os 1200 x 500 ms do legado

// areaDeLimpeza é uma das áreas que o relógio esvazia. `zeraBandeira` separa as
// nove ClearAreaQuest da única ClearArea: a Lanhouse é esvaziada sem mexer em
// bandeira nenhuma (Server.cpp:6287 contra :6263).
type areaDeLimpeza struct {
	nome           string
	x1, y1, x2, y2 int16
	zeraBandeira   bool
}

// areasDeLimpeza é a lista do legado, na ordem em que ele a escreve
// (ProcessSecMinTimer.cpp:562-572). Os nomes são os comentários do próprio
// original, traduzidos: portar só as cinco que apareceram na reclamação deixaria
// as outras cinco com o mesmo defeito e sem ninguém olhando.
var areasDeLimpeza = []areaDeLimpeza{
	{"Cemitério (Coveiro)", 2379, 2076, 2426, 2133, true},
	{"Capa Verde", 2232, 1564, 2263, 1592, true},
	{"Reset de habilidades (Armia)", 2640, 1966, 2670, 2004, true},
	{"Jardim dos Deuses (Carbuncle)", 2228, 1700, 2257, 1728, true},
	{"Reset de habilidades (Erion)", 1950, 1586, 1988, 1614, true},
	{"Coração do Kaizen", 459, 3887, 497, 3916, true},
	{"Hidras", 658, 3728, 703, 3762, true},
	{"Elfos", 1312, 4027, 1348, 4055, true},
	{"Quest Gárgula", 793, 4046, 827, 4080, true},
	{"Lanhouse", 3570, 3446, 3965, 3711, false},
}

// contem usa limites INCLUSIVOS, como ClearAreaQuest (`< x1 || > x2` pula). É de
// propósito que isso não case com questArea.contains, que é exclusiva e vem de
// outro trecho do legado (o guarda em ProcessSecMinTimer.cpp:1660). As duas
// bordas divergem no original e manter cada uma como está é o que evita expulsar
// de uma caixa alguém que a outra deixaria ficar.
func (a areaDeLimpeza) contem(x, y int16) bool {
	return x >= a.x1 && x <= a.x2 && y >= a.y1 && y <= a.y2
}

// clearQuestAreas é o relógio. Roda dentro do laço, como todo o resto do Tick.
func (d *Dispatcher) clearQuestAreas(w *world.World) {
	if d.tickCount%questClearTicks != 0 {
		return
	}
	for _, a := range areasDeLimpeza {
		d.esvaziaArea(w, a)
	}
	// O laço final do legado (ProcessSecMinTimer.cpp:573-577): zera a bandeira de
	// todo jogador em jogo, esteja ele onde estiver. É o que faz o passe vencer
	// mesmo para quem saiu da arena e voltaria depois sem passar pelo NPC.
	w.ForEachPlayer(func(_ *world.Session, e *world.Entity) {
		e.QuestFlag = 0
	})
}

// esvaziaArea é ClearAreaQuest (zeraBandeira) ou ClearArea (sem ela).
//
// DIVERGÊNCIA DELIBERADA, e é só esta: o legado devolve o jogador para a cidade
// sem escrever uma linha sequer, e esse silêncio já custou tempo — passar uma
// tarde procurando por que alguém foi parar em Armia não pode ser o padrão. O
// registro não muda o que o jogador vê; muda o que a equipe consegue explicar.
func (d *Dispatcher) esvaziaArea(w *world.World, a areaDeLimpeza) {
	w.ForEachPlayer(func(s *world.Session, e *world.Entity) {
		if !a.contem(e.X, e.Y) {
			return
		}
		// Morto dentro da área volta vivo com 2 de vida, não como cadáver
		// (Server.cpp:6276-6280). O original manda o Score antes do recall.
		if e.HP <= 0 {
			e.HP = 2
			d.sendScore(w, s, e)
		}
		d.log.Info("limpeza das arenas de quest",
			"conn", s.Conn, "name", e.Name, "area", a.nome,
			"level", e.Level, "quest_flag", e.QuestFlag,
			"x", e.X, "y", e.Y, "zera_bandeira", a.zeraBandeira)
		if a.zeraBandeira {
			e.QuestFlag = 0
		}
		// recall já zera a bandeira e devolve à última cidade (character.go); para
		// a Lanhouse isso é mais do que o legado faz, mas o laço final do relógio
		// zeraria a bandeira dois segundos depois de qualquer jeito.
		d.recall(w, s, e)
	})
}
