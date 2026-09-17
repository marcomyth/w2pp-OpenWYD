package world

import "github.com/jeanluca/w2pp-openwyd/internal/reinos"

// InimigoDoReinoDuracaoMs é quanto a marca de Inimigo do Reino dura depois do
// último golpe: 10 minutos, renovados a cada golpe (Atlas de Quests, 17/09/2026).
const InimigoDoReinoDuracaoMs = 10 * 60 * 1000

// A marca de Inimigo do Reino não existe no legado: lá, quem não tem capa de
// reino nunca é atacado pelos guardas (g_pClanTable, clã 7/8 contra clã 0). Ela
// é a regra nova dos Reinos — quem não é de reino nenhum e bate num monstro de
// um reino vira inimigo DAQUELE reino, e os guardas dele passam a caçá-lo.

func indiceDoReino(clan uint8) (int, bool) {
	switch clan {
	case reinos.ClanHekalotia:
		return 0, true
	case reinos.ClanAkelonia:
		return 1, true
	}
	return 0, false
}

// InimigoDoReino diz se e está marcado como inimigo do reino do clã clan agora.
// Loop-only.
func (w *World) InimigoDoReino(e *Entity, clan uint8) bool {
	i, ok := indiceDoReino(clan)
	if e == nil || !ok || !e.inimigoDoReino[i] {
		return false
	}
	// Diferença sem sinal: o relógio é o UnixMilli truncado em 32 bits e dá a
	// volta a cada ~49 dias; subtrair antes de comparar atravessa a volta.
	return w.Now()-e.inimigoDoReinoDesde[i] < InimigoDoReinoDuracaoMs
}

// MarcarInimigoDoReino liga (ou renova) a marca de e para o reino do clã clan e
// diz se ela é nova — se e não era inimigo desse reino um instante antes, que é
// quando o jogador precisa do aviso. Loop-only.
func (w *World) MarcarInimigoDoReino(e *Entity, clan uint8) bool {
	i, ok := indiceDoReino(clan)
	if e == nil || !ok {
		return false
	}
	nova := !w.InimigoDoReino(e, clan)
	e.inimigoDoReino[i] = true
	e.inimigoDoReinoDesde[i] = w.Now()
	return nova
}

// LimparInimigoDoReino apaga as duas marcas: a morte perdoa. Loop-only.
func LimparInimigoDoReino(e *Entity) {
	if e == nil {
		return
	}
	e.inimigoDoReino = [2]bool{}
}

// clanParaOReino é o clã com que um guarda de reino, dentro da cidade dos
// Reinos, enxerga o alvo: jogador pela capa que veste (Basedef.cpp:3220-3244),
// monstro pelo clã do template.
func clanParaOReino(id int, e *Entity) uint8 {
	if id < MaxUser {
		return reinos.ClanDaCapa(e.Equip[reinos.SlotDaCapa].Index)
	}
	return e.Clan
}
