// Package pilha decide que item empilha e como uma quantidade vira pilhas.
//
// É o único lugar com essa regra. O tmServer a usa para dividir e juntar pilhas
// (handler.isSplittable) e o painel para entregar uma quantidade (entrega.Lote):
// uma segunda lista, em qualquer um dos dois, deixaria o painel mandar em pilha um
// item que o jogo não sabe dividir, ou mandar cem itens avulsos que o jogo
// empilharia.
package pilha

// EfAmount é EF_AMOUNT (ItemEffect.h): o efeito cujo valor é quantas unidades a
// pilha tem. Item sem ele conta como uma unidade.
const EfAmount = 61

// MaxPorPilha é o máximo de unidades numa pilha, o mesmo teto do /gm item e da
// junção de pilhas no jogo.
const MaxPorPilha = 120

// EspacosDoBau é quantos espaços o baú da conta tem (MAX_CARGO). Uma entrega do
// painel cai no baú, então é o limite de pilhas (ou itens avulsos) por envio.
const EspacosDoBau = 128

// Empilha diz se o item pode ser dividido e juntado em pilhas (_MSG_SplitItem.cpp:
// 45-52): um conjunto fixo de moedas e especiais, os Âmagos 2390-2419, e as
// divergências decididas pela equipe, comentadas em handler.isSplittable.
//
// O cliente tem a própria lista para abrir a caixa de dividir: a mesma, copiada
// em Divide (client/gamepatch/divisao.cpp). Item novo aqui entra lá também.
func Empilha(index int16) bool {
	switch index {
	case 412, 413, 414, 416, 419, 420:
		return true
	case 1774: // Pedra do Sábio, vendida em pacote pela loja
		return true
	case 4010, 4011, 4028, 4029: // Barras de Prata
		return true
	case 3224: // Fragmento de Alma, da Escolta do Trono dos Reinos (tmserver handler/reinos.go)
		return true
	}
	switch {
	case index >= 2390 && index <= 2419: // Âmagos, todos
		return true
	case index >= 2441 && index <= 2444: // Diamante, Esmeralda, Coral, Garnet
		return true
	case index >= 777 && index <= 785: // Pergaminho da Água (M) LV1-8 + Neses
		return true
	case index >= 3173 && index <= 3190: // Pergaminho da Água (N) e (A)
		return true
	case index >= 4016 && index <= 4025: // Classe A-E e (P)
		return true
	case index >= 4117 && index <= 4121: // troféus da Quest 256
		return true
	}
	return false
}

// Divide reparte uma quantidade pelos espaços que ela ocupa: pilhas de até
// MaxPorPilha para o que empilha, uma unidade por espaço para o que não empilha.
// Quantidade menor que 1 não ocupa espaço nenhum.
func Divide(index int16, quantidade int) []int {
	if quantidade < 1 {
		return nil
	}
	if !Empilha(index) {
		out := make([]int, quantidade)
		for i := range out {
			out[i] = 1
		}
		return out
	}
	out := make([]int, 0, (quantidade+MaxPorPilha-1)/MaxPorPilha)
	for quantidade > 0 {
		n := min(quantidade, MaxPorPilha)
		out = append(out, n)
		quantidade -= n
	}
	return out
}

// MaxPorEnvio é a maior quantidade do item que cabe num baú vazio.
func MaxPorEnvio(index int16) int {
	if Empilha(index) {
		return EspacosDoBau * MaxPorPilha
	}
	return EspacosDoBau
}
