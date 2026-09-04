package itembrowser

// EffectInfo describes one EF_* token for the UI legend.
//
// The ids and the canonical meanings come from Source/Code/ItemEffect.h, whose
// comments are half Korean (CP949) and half community-translated Portuguese.
// The Korean ones were decoded and translated here; where the header says
// nothing, the description comes from the behaviour the Go server or the legacy
// C++ actually implements, and the source is cited in Note. Anything neither
// documents is marked explicitly rather than guessed.
type EffectInfo struct {
	ID    int    `json:"id"`
	Label string `json:"label"`          // short Portuguese name
	Note  string `json:"note,omitempty"` // extra detail, when there is one
	// Score reports that tmserver folds this effect into the item's BaseEffects
	// (the efName whitelist in tmserver/internal/content/catalog.go). Effects
	// outside it are still read — EF_VOLATILE, EF_GRID and EF_PRICE have their
	// own dedicated paths — they just do not become character stats.
	Score bool `json:"score"`
	// Refine reports that the effect is NOT in the refine-multiplier exemption
	// list at Basedef.cpp:1854, where the value is scaled by (sanc+10)/10.
	// It says the scaling applies when the value is read through that function —
	// not that every consumer goes through it. The expiry-date effects are the
	// clearest counter-example: BASE_CheckItemDate reads stEffect[].cValue
	// directly and never sees the multiplier.
	Refine bool `json:"refine"`
}

// noRefine is the Basedef.cpp:1854 exemption list: effects the refine
// multiplier deliberately skips because they are requirements, layout or
// classification rather than magnitudes.
var noRefine = map[string]bool{
	"EF_GRID": true, "EF_CLASS": true, "EF_POS": true, "EF_WTYPE": true,
	"EF_RANGE": true, "EF_LEVEL": true, "EF_REQ_STR": true, "EF_REQ_INT": true,
	"EF_REQ_DEX": true, "EF_REQ_CON": true, "EF_VOLATILE": true,
	"EF_INCUBATE": true, "EF_INCUDELAY": true, "EF_MOBTYPE": true,
	"EF_ITEMTYPE": true, "EF_ITEMLEVEL": true, "EF_NOTRADE": true,
	"EF_NOSANC": true, "EF_DONATE": true,
}

// scored mirrors the efName whitelist in tmserver/internal/content/catalog.go.
// Kept as a literal rather than imported because that package is internal to
// tmserver; the legend test pins the two lists against each other by name.
var scored = map[string]bool{
	"EF_DAMAGE": true, "EF_AC": true, "EF_HP": true, "EF_MP": true,
	"EF_STR": true, "EF_INT": true, "EF_DEX": true, "EF_CON": true,
	"EF_SPECIAL1": true, "EF_SPECIAL2": true, "EF_SPECIAL3": true,
	"EF_SPECIAL4": true, "EF_SPECIALALL": true, "EF_POS": true,
	"EF_WTYPE": true, "EF_CRITICAL": true, "EF_SANC": true, "EF_HPADD": true,
	"EF_MPADD": true, "EF_RESIST1": true, "EF_RESIST2": true,
	"EF_RESIST3": true, "EF_RESIST4": true, "EF_ACADD": true,
	"EF_RESISTALL": true, "EF_MAGIC": true, "EF_DAMAGEADD": true,
	"EF_MAGICADD": true, "EF_HPADD2": true, "EF_MPADD2": true,
	"EF_CRITICAL2": true, "EF_ITEMLEVEL": true, "EF_MOBTYPE": true,
	"EF_RUNSPEED": true, "EF_ITEMTYPE": true, "EF_NOSANC": true,
	"EF_INCUBATE": true, "EF_INCUDELAY": true,
}

// effectDefs is the raw legend: token → id + Portuguese meaning.
var effectDefs = map[string][2]string{
	// The values are [label, note]; the id comes from effectIDs below so the two
	// tables cannot drift into different orders.
	"EF_LEVEL":      {"Nível necessário", "Requisito de nível para equipar"},
	"EF_DAMAGE":     {"Dano", "Aumento de dano"},
	"EF_AC":         {"Defesa", "Aumento de defesa (AC)"},
	"EF_HP":         {"HP", "Aumento de HP máximo"},
	"EF_MP":         {"MP", "Aumento de MP máximo"},
	"EF_EXP":        {"Experiência", ""},
	"EF_STR":        {"Força", "Aumento de força"},
	"EF_INT":        {"Inteligência", "Aumento de inteligência"},
	"EF_DEX":        {"Destreza", "Aumento de destreza"},
	"EF_CON":        {"Constituição", "Aumento de constituição"},
	"EF_SPECIAL1":   {"Especial 1", "Maestria / atributo especial 1"},
	"EF_SPECIAL2":   {"Especial 2", "Maestria / atributo especial 2"},
	"EF_SPECIAL3":   {"Especial 3", "Maestria / atributo especial 3"},
	"EF_SPECIAL4":   {"Especial 4", "Maestria / atributo especial 4"},
	"EF_SCORE14":    {"Score 14", "Sem descrição na fonte"},
	"EF_SCORE15":    {"Score 15", "Sem descrição na fonte"},
	"EF_POS":        {"Posição", "Bitmask dos slots onde o item pode ser equipado; padrão NOWHERE"},
	"EF_CLASS":      {"Classe", "Bitmask das classes que podem usar; padrão liberado"},
	"EF_R1SIDC":     {"Requisito 1 mão", "STR/INT/DEX/CON exigidos por arma de uma mão ou armadura"},
	"EF_R2SIDC":     {"Requisito 2 mãos", "STR/INT/DEX/CON exigidos por arma de duas mãos"},
	"EF_WTYPE":      {"Tipo de arma", "Classificação da arma — define a animação de ataque"},
	"EF_REQ_STR":    {"Requer força", ""},
	"EF_REQ_INT":    {"Requer inteligência", ""},
	"EF_REQ_DEX":    {"Requer destreza", ""},
	"EF_REQ_CON":    {"Requer constituição", ""},
	"EF_ATTSPEED":   {"Velocidade de ataque", ""},
	"EF_RANGE":      {"Alcance", "Distância de ataque; vale o maior valor entre os equipados"},
	"EF_PRICE":      {"Preço", "Definição comentada no header — o preço vem da coluna 6 do CSV"},
	"EF_RUNSPEED":   {"Velocidade de corrida", ""},
	"EF_SPELL":      {"Magia", "Magia disparada ao consumir ou equipar o item"},
	"EF_DURATION":   {"Duração", "Parâmetro 1: tempo de efeito ao consumir; item equipado é permanente"},
	"EF_PARM2":      {"Parâmetro 2", ""},
	"EF_GRID":       {"Tamanho", "Quantos quadrados o item ocupa na mochila"},
	"EF_GROUND":     {"Tamanho no chão", "Espaço ocupado no terreno (portões de castelo etc.)"},
	"EF_CLAN":       {"Raça / clã", ""},
	"EF_HWORDCOIN":  {"Moeda (word alto)", "Metade alta do valor em ouro"},
	"EF_LWORDCOIN":  {"Moeda (word baixo)", "Metade baixa do valor em ouro"},
	"EF_VOLATILE":   {"Consumível", "Classe de uso do item: 0 = equipamento; >0 roteia para um handler específico"},
	"EF_KEYID":      {"ID de chave", ""},
	"EF_PARRY":      {"Esquiva", "Bônus na taxa de esquiva"},
	"EF_HITRATE":    {"Acerto", "Bônus na taxa de acerto"},
	"EF_CRITICAL":   {"Crítico", "Chance de acerto crítico"},
	"EF_SANC":       {"Refino", "Nível de refino (Sanctuary); consumido como multiplicador, não como stat"},
	"EF_SAVEMANA":   {"Economia de mana", "1 = gasta 99% do custo"},
	"EF_HPADD":      {"HP percentual", "1 = 101% do HP máximo"},
	"EF_MPADD":      {"MP percentual", "1 = 101% do MP máximo"},
	"EF_REGENHP":    {"Regeneração de HP", ""},
	"EF_REGENMP":    {"Regeneração de MP", ""},
	"EF_RESIST1":    {"Resistência a fogo", "Confirmado por duas fontes: o comentário coreano e Orb_de_Fogo no catálogo"},
	"EF_RESIST2":    {"Resistência a gelo", ""},
	"EF_RESIST3":    {"Resistência sagrada", ""},
	"EF_RESIST4":    {"Resistência a raio", ""},
	"EF_ACADD":      {"Defesa percentual", "Aumento percentual de defesa"},
	"EF_RESISTALL":  {"Todas as resistências", ""},
	"EF_BONUS":      {"Bônus", ""},
	"EF_HWORDGUILD": {"Ataque PvP", ""},
	"EF_LWORDGUILD": {"Defesa PvP", ""},
	"EF_QUEST":      {"Missão", "Sempre começa em 1; quase sempre vem junto com EF_VOLATILE"},
	"EF_UNIQUE":     {"Único", ""},
	"EF_MAGIC":      {"Ataque mágico", "1 = 1% de amplificação"},
	"EF_AMOUNT":     {"Quantidade", "Tamanho da pilha"},
	"EF_HWORDINDEX": {"Índice (word alto)", ""},
	"EF_LWORDINDEX": {"Índice (word baixo)", ""},
	"EF_INIT1":      {"Inicial 1", ""},
	"EF_INIT2":      {"Inicial 2", ""},
	"EF_INIT3":      {"Inicial 3", ""},
	"EF_DAMAGEADD":  {"Bônus de dano", ""},
	"EF_MAGICADD":   {"Bônus mágico", ""},
	"EF_HPADD2":     {"HP percentual (base)", "Igual ao EF_HPADD, mas exibido junto dos atributos base"},
	"EF_MPADD2":     {"MP percentual (base)", "Igual ao EF_MPADD, mas exibido junto dos atributos base"},
	"EF_CRITICAL2":  {"Crítico (2)", "Segunda fonte de crítico"},
	"EF_ACADD2":     {"Defesa percentual (2)", ""},
	"EF_DAMAGE2":    {"Dano (2)", "Sem descrição na fonte"},
	"EF_SPECIALALL": {"Todas as maestrias", "Aumenta as maestrias, com uma exclusão que a fonte não detalha"},
	"EF_CURKILL":    {"Abates atuais", "Marcado como não usado na fonte"},
	"EF_LTOTKILL":   {"Abates totais (word baixo)", "Marcado como não usado na fonte"},
	"EF_HTOTKILL":   {"Abates totais (word alto)", "Marcado como não usado na fonte"},
	"EF_INCUBATE":   {"Incubação", "Limiar de chocagem do ovo de montaria"},
	"EF_MOUNTLIFE":  {"Montaria: vida total", ""},
	"EF_MOUNTHP":    {"Montaria: HP atual", ""},
	"EF_MOUNTSANC":  {"Montaria: crescimento", ""},
	"EF_MOUNTFEED":  {"Montaria: saciedade", ""},
	"EF_MOUNTKILL":  {"Montaria: abates", "A própria fonte marca com interrogação"},
	"EF_INCUDELAY":  {"Atraso de incubação", ""},
	"EF_SUBGUILD":   {"Sub-guilda", "0 = guilda principal; existem os níveis 1, 2 e 3"},
	"EF_ITEMLEVEL":  {"Grau do item", "Tier A–E, casado com o grau da classe"},
	"EF_DONATE":     {"Doação", "Marca item de cash/doação"},
	"EF_GRADE0":     {"Grau A", ""},
	"EF_GRADE1":     {"Grau B", ""},
	"EF_GRADE2":     {"Grau C", ""},
	"EF_GRADE3":     {"Grau D", ""},
	"EF_GRADE4":     {"Grau E", ""},
	"EF_GRADE5":     {"Grau E Arch", ""},
	"EF_WDAY":       {"Expira: dia", "Validade do item; BASE_CheckItemDate lê stEffect direto, sem passar pelo multiplicador de refino"},
	"EF_WMONTH":     {"Expira: mês", "Validade do item, lida direto por BASE_CheckItemDate"},
	"EF_HOUR":       {"Expira: hora", ""},
	"EF_MIN":        {"Expira: minuto", ""},
	"EF_YEAR":       {"Expira: ano", "Gravado como ano-100; lido direto por BASE_CheckItemDate"},
	"EF_MOBTYPE":    {"Tipo de mob", ""},
	"EF_ITEMTYPE":   {"Tipo de item", "Lido pelos matchers de combine como trava de receita"},
	"EF_NOSANC":     {"Não refinável", "Marca item que nunca pode ser refinado"},
	"EF_NOTRADE":    {"Não negociável", ""},
}

// effectIDs are the numeric ids from Source/Code/ItemEffect.h.
var effectIDs = map[string]int{
	"EF_LEVEL": 1, "EF_DAMAGE": 2, "EF_AC": 3, "EF_HP": 4, "EF_MP": 5,
	"EF_EXP": 6, "EF_STR": 7, "EF_INT": 8, "EF_DEX": 9, "EF_CON": 10,
	"EF_SPECIAL1": 11, "EF_SPECIAL2": 12, "EF_SPECIAL3": 13, "EF_SPECIAL4": 14,
	"EF_SCORE14": 15, "EF_SCORE15": 16, "EF_POS": 17, "EF_CLASS": 18,
	"EF_R1SIDC": 19, "EF_R2SIDC": 20, "EF_WTYPE": 21, "EF_REQ_STR": 22,
	"EF_REQ_INT": 23, "EF_REQ_DEX": 24, "EF_REQ_CON": 25, "EF_ATTSPEED": 26,
	"EF_RANGE": 27, "EF_PRICE": 28, "EF_RUNSPEED": 29, "EF_SPELL": 30,
	"EF_DURATION": 31, "EF_PARM2": 32, "EF_GRID": 33, "EF_GROUND": 34,
	"EF_CLAN": 35, "EF_HWORDCOIN": 36, "EF_LWORDCOIN": 37, "EF_VOLATILE": 38,
	"EF_KEYID": 39, "EF_PARRY": 40, "EF_HITRATE": 41, "EF_CRITICAL": 42,
	"EF_SANC": 43, "EF_SAVEMANA": 44, "EF_HPADD": 45, "EF_MPADD": 46,
	"EF_REGENHP": 47, "EF_REGENMP": 48, "EF_RESIST1": 49, "EF_RESIST2": 50,
	"EF_RESIST3": 51, "EF_RESIST4": 52, "EF_ACADD": 53, "EF_RESISTALL": 54,
	"EF_BONUS": 55, "EF_HWORDGUILD": 56, "EF_LWORDGUILD": 57, "EF_QUEST": 58,
	"EF_UNIQUE": 59, "EF_MAGIC": 60, "EF_AMOUNT": 61, "EF_HWORDINDEX": 62,
	"EF_LWORDINDEX": 63, "EF_INIT1": 64, "EF_INIT2": 65, "EF_INIT3": 66,
	"EF_DAMAGEADD": 67, "EF_MAGICADD": 68, "EF_HPADD2": 69, "EF_MPADD2": 70,
	"EF_CRITICAL2": 71, "EF_ACADD2": 72, "EF_DAMAGE2": 73, "EF_SPECIALALL": 74,
	"EF_CURKILL": 75, "EF_LTOTKILL": 76, "EF_HTOTKILL": 77, "EF_INCUBATE": 78,
	"EF_MOUNTLIFE": 79, "EF_MOUNTHP": 80, "EF_MOUNTSANC": 81, "EF_MOUNTFEED": 82,
	"EF_MOUNTKILL": 83, "EF_INCUDELAY": 84, "EF_SUBGUILD": 85, "EF_ITEMLEVEL": 87,
	"EF_DONATE": 91, "EF_GRADE0": 100, "EF_GRADE1": 101, "EF_GRADE2": 102,
	"EF_GRADE3": 103, "EF_GRADE4": 104, "EF_GRADE5": 105, "EF_WDAY": 106,
	"EF_HOUR": 107, "EF_MIN": 108, "EF_YEAR": 109, "EF_WMONTH": 110,
	"EF_MOBTYPE": 112, "EF_ITEMTYPE": 113, "EF_NOSANC": 126, "EF_NOTRADE": 127,
}

// EffectTable builds the legend the UI renders: every EF_* token with its id,
// Portuguese meaning and the two behavioural flags.
func EffectTable() map[string]EffectInfo {
	out := make(map[string]EffectInfo, len(effectIDs))
	for name, id := range effectIDs {
		info := EffectInfo{ID: id, Score: scored[name], Refine: !noRefine[name]}
		if def, ok := effectDefs[name]; ok {
			info.Label, info.Note = def[0], def[1]
		}
		out[name] = info
	}
	return out
}
