// Package regiao diz, para um ponto do mapa, que TIPO de lugar ele é (mapa
// aberto, quest ou masmorra) e como a região se chama.
//
// Existe para o Painel de Drop das Fadas (tmserver/internal/handler/fadas.go): a
// lista de monstros mostra o tipo do lugar em vez do nível, e a frase dos drops
// diz a região. Só o tipo e o nome saem daqui — nunca a coordenada nem o lugar
// exato onde o monstro nasce (decisão da dona, 02/10/2026).
//
// A base é o Release/TMsrv/run/Regions.txt, a lista de retângulos com nome que o
// próprio jogo traz, TRANSCRITA em vez de lida em tempo de execução: o tipo de
// cada região não está no arquivo, e uma tabela que mudasse com um arquivo em
// disco não daria para testar. Um teste confere a transcrição contra o arquivo.
// Por cima dela vêm as arenas da Quest 256 (os retângulos de
// internal/level/expzone.go) e os remendos dos blocos que o arquivo não cobre.
//
// OS NOMES DO MAPA ABERTO SÃO PROPOSTA: quem decide é a dona. Para trocar um
// nome, muda-se a linha aqui; nada mais depende do texto.
package regiao

// Tipo é a classe do lugar. Os números viajam no pacote 0x0F71.
type Tipo uint8

const (
	MapaAberto Tipo = 0
	Quest      Tipo = 1
	Masmorra   Tipo = 2
)

// String é a palavra que o painel mostra.
func (t Tipo) String() string {
	switch t {
	case Quest:
		return "Quest"
	case Masmorra:
		return "Masmorra"
	default:
		return "Mapa Aberto"
	}
}

// Lugar é o tipo e o nome da região de um ponto.
type Lugar struct {
	Tipo Tipo
	Nome string
}

// NomeMax é o maior nome de região, em letras: o campo do pacote tem 24 bytes,
// com o zero do fim.
const NomeMax = 23

// Padrao é o lugar de todo ponto que nenhuma linha cobre: o campo aberto sem nome.
var Padrao = Lugar{MapaAberto, "Campo"}

type linha struct {
	x1, y1, x2, y2 int32
	lugar          Lugar
	// regions é o rótulo da linha no Regions.txt, ou "" para as linhas que são
	// daqui (arenas e remendos). O teste usa para conferir a transcrição.
	regions string
}

// tabela é consultada em ordem, e a primeira linha que contém o ponto vale. Por
// isso o que é mais específico vem antes: as arenas da Quest 256 ficam DENTRO de
// regiões maiores (o Coveiro em Armia, o Jardim em Azran), e têm de ganhar delas.
var tabela = []linha{
	// --- as arenas da Quest 256 (internal/level/expzone.go, zoneRects) ---
	{2380, 2077, 2425, 2132, Lugar{Quest, "Coveiro"}, ""},
	{2229, 1701, 2256, 1727, Lugar{Quest, "Jardim"}, ""},
	{460, 3888, 496, 3915, Lugar{Quest, "Kaizen"}, ""},
	{659, 3729, 702, 3761, Lugar{Quest, "Hidras"}, ""},
	{1313, 4028, 1347, 4054, Lugar{Quest, "Elfos"}, ""},

	// --- o Regions.txt, na ordem do arquivo ---
	{129, 137, 253, 243, Lugar{MapaAberto, "Guerra de Cidades"}, "Guerra_de_Cidades"},
	{261, 261, 380, 380, Lugar{Masmorra, "Monster City"}, "Monster_City"},
	{1031, 1927, 1271, 2171, Lugar{MapaAberto, "RvR"}, "RvR"},
	{1285, 268, 1332, 367, Lugar{Masmorra, "Pesadelo Normal"}, "Pesadelo_N"},
	{1026, 260, 1158, 382, Lugar{Masmorra, "Pesadelo Místico"}, "Pesadelo_M"},
	{1155, 130, 1290, 225, Lugar{Masmorra, "Pesadelo Arcano"}, "Pesadelo_A"},
	{1137, 1669, 1282, 1786, Lugar{MapaAberto, "Deserto"}, "Deserto_Pilar"},
	{1282, 1664, 1396, 1785, Lugar{MapaAberto, "Deserto"}, "Deserto_Manticora"},
	{1283, 1788, 1397, 1910, Lugar{MapaAberto, "Deserto"}, "Deserto_Lugefer"},
	{1397, 1671, 1521, 1785, Lugar{MapaAberto, "Deserto"}, "Deserto_Baixo"},
	{1522, 1675, 1669, 1787, Lugar{MapaAberto, "Deserto"}, "Deserto_Reino"},
	{1669, 1544, 1785, 1639, Lugar{MapaAberto, "Reinos"}, "Reino_Red"},
	{1670, 1640, 1785, 1813, Lugar{MapaAberto, "Reinos"}, "Zona_Neutra"},
	{1785, 1815, 1814, 1913, Lugar{MapaAberto, "Reinos"}, "Reino_Blue"},
	{1788, 1538, 2185, 1785, Lugar{MapaAberto, "Azran"}, "Azran_Reino"},
	{2180, 1533, 2435, 1787, Lugar{MapaAberto, "Azran"}, "Azran_Jardim"},
	{2187, 1155, 2298, 1297, Lugar{Masmorra, "Castelo Zakun"}, "Castelo_Zakun"},
	{2179, 1293, 2295, 1534, Lugar{MapaAberto, "Azran"}, "Azran_Zakun"},
	{2589, 1671, 2681, 1785, Lugar{Masmorra, "Coliseu"}, "Coliseu"},
	{2444, 1748, 2552, 1840, Lugar{MapaAberto, "Azran"}, "Azran_Torre"},
	{2438, 1914, 2680, 2168, Lugar{MapaAberto, "Erion"}, "Erion"},
	{2055, 1927, 2166, 2059, Lugar{MapaAberto, "Campo de Treino"}, "Campo_de_Treino"},
	{2164, 2054, 2684, 2170, Lugar{MapaAberto, "Armia"}, "Armia"},
	{2313, 2170, 2428, 2296, Lugar{Masmorra, "Entrada da Dungeon"}, "Entrada_Dungeon"},
	{2435, 1916, 2688, 2070, Lugar{MapaAberto, "Erion"}, "Erion"},
	{2171, 2045, 2720, 2322, Lugar{MapaAberto, "Armia"}, "Armia"},
	{3391, 2649, 4027, 3255, Lugar{MapaAberto, "Karden"}, "Karden"},
	{1029, 3465, 1141, 3569, Lugar{Masmorra, "Água Normal"}, "Agua_N"},
	{1161, 3595, 1267, 3695, Lugar{Masmorra, "Água Místico"}, "Agua_M"},
	{1289, 3467, 1397, 3569, Lugar{Masmorra, "Água Arcano"}, "Agua_A"},
	{1651, 3577, 1927, 3723, Lugar{Masmorra, "Portão Infernal"}, "Portao_Infernal"},
	{2173, 3583, 2307, 3711, Lugar{Masmorra, "Vale Escondido"}, "Vale_Escondido"},
	{127, 3710, 767, 6841, Lugar{Masmorra, "Dungeon 1º Andar"}, "Dugeon_1_Andar_Hidra"},
	{381, 3841, 511, 4086, Lugar{Masmorra, "Dungeon 1º Andar"}, "Dungeon_1_Andar_Kaizen"},
	{632, 3847, 1022, 4091, Lugar{Masmorra, "Dungeon 2º Andar"}, "Dungeon_2_Andar"},
	{898, 3712, 1143, 3830, Lugar{Masmorra, "Dungeon 3º Andar"}, "Dungeon_3_Andar"},
	{1283, 3714, 1538, 3838, Lugar{Masmorra, "Submundo 1"}, "Submundo_1"},
	{1153, 3966, 1533, 4089, Lugar{Masmorra, "Submundo 2"}, "Submundo_2"},
	{1656, 3968, 1797, 4092, Lugar{Masmorra, "Cubo Normal"}, "Cubo_N"},
	{1794, 3845, 1910, 3963, Lugar{Masmorra, "Cubo Místico"}, "Cubo_M"},
	{1924, 3973, 2045, 4091, Lugar{Masmorra, "Cubo Arcano"}, "Cubo_A"},
	{2179, 3850, 2303, 4091, Lugar{Masmorra, "Kefra"}, "Kefra_Esquerda"},
	{2304, 3850, 2435, 4091, Lugar{Masmorra, "Kefra"}, "Kefra_Meio"},
	{2436, 3850, 2551, 4093, Lugar{Masmorra, "Kefra"}, "Kefra_Direita"},
	{3581, 3583, 3705, 3705, Lugar{Masmorra, "Lan Normal"}, "Lan_N"},
	{3717, 3463, 3833, 3577, Lugar{Masmorra, "Lan Místico"}, "Lan_M"},
	{3851, 3595, 3977, 3707, Lugar{Masmorra, "Lan Arcano"}, "Lan_A"},
	{3973, 3973, 4096, 4096, Lugar{MapaAberto, "Cassino"}, "Cassino"},
	{3330, 1025, 3602, 1659, Lugar{Masmorra, "Pistas"}, "Pistas"},
	{3199, 1667, 3321, 1785, Lugar{MapaAberto, "Kefra City"}, "Kefra_City"},
	{1272, 1427, 1403, 1532, Lugar{Masmorra, "Big Cubo"}, "Big_Cubo"},
	{895, 1409, 1146, 1534, Lugar{MapaAberto, "Noatun"}, "Nova_Guerra_Noatun"},

	// --- remendos: blocos com monstro que o Regions.txt não cobre ---
	{2432, 1792, 2559, 1919, Lugar{MapaAberto, "Azran"}, ""},        // a sul da Torre de Azran
	{2432, 1536, 2687, 1791, Lugar{MapaAberto, "Azran"}, ""},        // a leste do Jardim
	{1664, 1792, 1791, 1919, Lugar{MapaAberto, "Reinos"}, ""},       // entre os dois reinos
	{1024, 1664, 1151, 1791, Lugar{MapaAberto, "Deserto"}, ""},      // a oeste do Pilar
	{768, 3584, 895, 3711, Lugar{Masmorra, "Dungeon 2º Andar"}, ""}, // a faixa acima do 2º andar
	{2688, 3968, 2943, 4095, Lugar{Masmorra, "Kefra"}, ""},          // os cristais
	{3456, 3456, 3839, 3711, Lugar{Masmorra, "Lan Normal"}, ""},     // as gárgulas em volta da Lan
}

// Reserva diz se o lugar e um deposito de blocos, e nao um lugar de verdade. A
// Monster City sao 824 blocos do NPCGener com um Tauron cada, todos no mesmo
// ponto (320,320): blocos de reserva. Para o painel, um lugar de reserva so vale
// quando o monstro nao nasce em nenhum outro; senao o Tauron, que nasce de
// verdade no Deserto (53 blocos), apareceria como "Masmorra: Monster City"
// (pedido da dona, 02/10/2026). E so o que o PAINEL mostra: o jogo nao muda.
func Reserva(l Lugar) bool {
	return l.Tipo == Masmorra && l.Nome == "Monster City"
}

// Em devolve o lugar de um ponto do mapa.
func Em(x, y int32) Lugar {
	for i := range tabela {
		l := &tabela[i]
		if x >= l.x1 && x <= l.x2 && y >= l.y1 && y <= l.y2 {
			return l.lugar
		}
	}
	return Padrao
}

// Todos lista cada lugar que a tabela conhece, sem repetir, com o padrão: para a
// revisão dos nomes e para os testes.
func Todos() []Lugar {
	visto := map[Lugar]bool{Padrao: true}
	out := []Lugar{Padrao}
	for _, l := range tabela {
		if !visto[l.lugar] {
			visto[l.lugar] = true
			out = append(out, l.lugar)
		}
	}
	return out
}
