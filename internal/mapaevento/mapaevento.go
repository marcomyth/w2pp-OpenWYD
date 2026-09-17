// Package mapaevento lista os mapas guardados para evento: áreas do mundo onde
// nenhum gerador do NPCGener põe monstro, e que a equipe usa quando monta um
// evento. O tmServer lê a lista para não gerar nada dentro delas; o painel lê a
// mesma lista para mostrar onde ficam e como chegar.
//
// Por que existe (17/09/2026): o boot popula todo bloco com MinuteGenerate -1,
// divergência deliberada que os chefes sozinhos exigem, e com ela subiam de pé
// populações que no legado só existiam durante um evento — 2.318 Ladrões
// Fantasmas e Seguidores do Grinch na Nova Guerra de Noatun, 824 Taurons
// empilhados em Monster City (cada um dando peça LE a 1 em 891), a população das
// Pistas e dos Cubos, que não têm código de evento neste servidor. O Marco pediu
// os mapas limpos e guardados para evento.
//
// Os retângulos são os do Release/TMsrv/run/Regions.txt, com o mesmo nome, e o
// teste confere que continuam iguais.
package mapaevento

// Mapa é um retângulo guardado para evento, bordas inclusas.
type Mapa struct {
	// Regiao é o nome da linha no Regions.txt.
	Regiao string
	// Nome é como a equipe chama o lugar.
	Nome           string
	X1, Y1, X2, Y2 int
	// EntradaX/Y é um ponto de chegada para o GM (/gm pos): onde a população
	// antiga nascia, e portanto chão onde mob anda.
	EntradaX, EntradaY int
	// Nota é o que havia lá antes de o mapa ser limpo.
	Nota string
}

// Todos são os mapas guardados para evento, na ordem em que o painel mostra.
var Todos = []Mapa{
	{
		Regiao: "Nova_Guerra_Noatun", Nome: "Nova Guerra de Noatun (evento de Natal)",
		X1: 895, Y1: 1409, X2: 1146, Y2: 1534, EntradaX: 1085, EntradaY: 1467,
		Nota: "Blocos 4241-4510 e 4787-4848: Ladrão Fantasma, Seguidor do Grinch e o Grinch (nv 300).",
	},
	{
		Regiao: "Monster_City", Nome: "Monster City",
		X1: 261, Y1: 261, X2: 380, Y2: 380, EntradaX: 321, EntradaY: 309,
		Nota: "Blocos 4875-6051: 824 Taurons de um, quase todos no mesmo tile.",
	},
	{
		Regiao: "Pistas", Nome: "Pistas",
		X1: 3330, Y1: 1025, X2: 3602, Y2: 1659, EntradaX: 3426, EntradaY: 1430,
		Nota: "Mobs Inf, Lugefer, Valquíria, Rei Carbuncle, a Freyja e a Árvore de Natal; fechada até o Kefra abrir.",
	},
	{
		Regiao: "Cubo_N", Nome: "Cubo Normal",
		X1: 1656, Y1: 3968, X2: 1797, Y2: 4092, EntradaX: 1725, EntradaY: 4031,
		Nota: "Blocos 195-219: Carbunkle@@, Rei Gremlin, Lobo, Chefe Krill, Lagarto, Orc Arqueiro.",
	},
	{
		Regiao: "Cubo_M", Nome: "Cubo Místico",
		X1: 1794, Y1: 3845, X2: 1910, Y2: 3963, EntradaX: 1831, EntradaY: 3947,
		Nota: "Blocos 220-222: Rei Gremlin.",
	},
	{
		Regiao: "Cubo_A", Nome: "Cubo Arcano",
		X1: 1924, Y1: 3973, X2: 2045, Y2: 4091, EntradaX: 1984, EntradaY: 4032,
		Nota: "Já estava vazio.",
	},
	{
		Regiao: "Big_Cubo", Nome: "Big Cubo",
		X1: 1272, Y1: 1427, X2: 1403, Y2: 1532, EntradaX: 1342, EntradaY: 1463,
		Nota: "Blocos 3776-3780: Treant Amald, Seguidores, Lobo e Camponesa Amald.",
	},
}

// Contem diz se o tile está num mapa guardado para evento.
func Contem(x, y int) bool {
	_, ok := Em(x, y)
	return ok
}

// Em devolve o mapa de evento que contém o tile.
func Em(x, y int) (Mapa, bool) {
	for _, m := range Todos {
		if x >= m.X1 && x <= m.X2 && y >= m.Y1 && y <= m.Y2 {
			return m, true
		}
	}
	return Mapa{}, false
}
