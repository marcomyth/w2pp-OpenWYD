package refine

import (
	"slices"

	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/world"
)

// As faixas de adicional que um drop PODE ter, para a dica do Painel de Drop das
// Fadas: "Dano 16~63, Mágico 2~28". Só o menor e o maior valor de cada efeito —
// nunca a chance de ele sair (decisão da dona, 02/10/2026).
//
// NÃO HÁ UMA SEGUNDA CONTA AQUI. A faixa sai do próprio Drop, rodado com um
// sorteio dirigido que passa por todos os caminhos dele: cada escolha de efeito,
// cada degrau da escada, cada saída da vaga do refino, e os três valores do bônus
// de drop de quem mata. Assim o piso, o ajuste por peça e o teto de armadura
// (limite.go) entram sozinhos, e uma mudança no Drop muda a faixa junto. Uma
// conta escrita à parte teria de ser mantida igual à mão, e a dica mentiria no
// dia em que as duas se separassem.

// Faixa é o menor e o maior valor que um efeito pode ter nas vagas dos dois
// adicionais (Effects[1] e Effects[2]) de um item dropado.
type Faixa struct {
	Efeito   uint8
	Min, Max uint8
}

// possiveisChave é tudo de que o resultado depende. O índice do item não entra: a
// conta só olha a peça, a classe da arma e a distância de nível.
type possiveisChave struct {
	pos, dist int
	magica    bool // lança ou cajado: sorteia magia no lugar de dano
	piso      bool
	vaga0Fixa bool // o catálogo fixa a vaga do refino (aplicaCatalogo)
}

// Possiveis guarda as faixas já calculadas. É do laço, como as Tabelas: quem a
// usa é o Dispatcher, numa goroutine só.
type Possiveis struct {
	t     Tabelas
	cache map[possiveisChave][]Faixa
}

// NovoPossiveis prepara a conta para um jogo de tabelas.
func NovoPossiveis(t Tabelas) *Possiveis {
	return &Possiveis{t: t, cache: map[possiveisChave][]Faixa{}}
}

// Tabelas devolve as tabelas com que a conta foi preparada.
func (p *Possiveis) Tabelas() Tabelas { return p.t }

// distanciaDoDrop repete a conta de distância do Drop para o saque comum (sem
// cristal e sem marca de grade). O teste TestPossiveisCobreODrop a mantém igual.
func distanciaDoDrop(nivel, reqLvl int) (dist int, piso bool) {
	if nivel >= 210 {
		nivel -= 47
	}
	dist = (nivel - reqLvl + 1) / 25
	piso = dist >= 4
	return min(max(dist, 0), 3), piso
}

// Do devolve as faixas dos adicionais de um item que um monstro de nível
// `nivel` deixa cair, em ordem de efeito. Vazio: o item não ganha adicional
// (consumível, acessório, escudo, ou o sorteio desligado).
func (p *Possiveis) Do(base Base, nivel int) []Faixa {
	if !p.t.Ligado || base.Pos&posGate == 0 || base.Pos == posEscudo {
		return nil
	}
	dist, piso := distanciaDoDrop(nivel, base.ReqLvl)
	k := possiveisChave{
		pos: base.Pos, dist: dist, piso: piso,
		magica: base.Unique == uniqueLanca || base.Unique == uniqueCajado,
	}
	for _, e := range base.Efeitos {
		if e.Eff == efSanc || e.Eff == efAmount || e.Eff == efIncubate {
			k.vaga0Fixa = true
		}
	}
	if f, ok := p.cache[k]; ok {
		return f
	}
	f := p.t.enumera(base, nivel, dist)
	p.cache[k] = f
	return f
}

// enumera roda o Drop por todos os caminhos e junta o que saiu nas vagas 1 e 2.
func (t Tabelas) enumera(base Base, nivel, dist int) []Faixa {
	type lim struct{ min, max uint8 }
	achado := map[uint8]lim{}

	// Os sorteios que representam cada degrau da escada e cada saída do refino:
	// o primeiro valor de cada trecho.
	escada := []int{0}
	for _, l := range t.Faixa[dist].Limite {
		if v := int(l); v > 0 && v < 100 && !slices.Contains(escada, v) {
			escada = append(escada, v)
		}
	}
	refino := []int{0}
	for _, l := range t.Faixa[dist].Refino {
		if v := int(l); v > 0 && v < 100 && !slices.Contains(refino, v) {
			refino = append(refino, v)
		}
	}
	efeito := []int{0, 1, 2, 3, 4} // as escolhas de efeito vão de 0 a 3; 4 é "nenhum"
	tipos := []int{0, 1, 2, 3, 4, 5, 6, 7, 8, 9}
	um := []int{0}

	// O Drop é percorrido como um odômetro: cada chamada de roll é uma roda, e as
	// opções de cada roda são conhecidas na hora em que ela é chamada.
	var escolha []int                       // a opção atual de cada roda
	var opcoes [][]int                      // as opções de cada roda, anotadas na última passada
	for _, bonus := range []int{0, 8, 16} { // adicional 0, 1 e 2
		escolha, opcoes = escolha[:0], opcoes[:0]
		for {
			chamada, cem := 0, 0
			roll := func(n int) int {
				var ops []int
				switch n {
				case 101:
					ops = efeito
				case 100:
					cem++
					switch cem {
					case 1:
						ops = efeito
					case 2, 3:
						ops = escada
					default:
						ops = refino
					}
				case 10:
					ops = tipos
				case 128, 256, 4:
					ops = um
				default:
					// o valor do bônus especial: o menor e o maior
					ops = []int{0, n - 1}
				}
				if chamada == len(escolha) {
					escolha = append(escolha, 0)
					opcoes = append(opcoes, ops)
				} else {
					opcoes[chamada] = ops
				}
				v := ops[min(escolha[chamada], len(ops)-1)]
				chamada++
				return v
			}
			it := world.Item{Index: int16(base.Indice)}
			t.Drop(&it, base, nivel, bonus, false, roll)
			for _, vaga := range []int{1, 2} {
				e := it.Effects[vaga]
				if e.Effect == 0 || e.Effect == efUnique || e.Value == 0 {
					continue
				}
				l, tem := achado[e.Effect]
				if !tem {
					l = lim{e.Value, e.Value}
				}
				l.min, l.max = min(l.min, e.Value), max(l.max, e.Value)
				achado[e.Effect] = l
			}
			// avança o odômetro: a última roda que ainda tem opção
			escolha, opcoes = escolha[:chamada], opcoes[:chamada]
			i := chamada - 1
			for i >= 0 && escolha[i]+1 >= len(opcoes[i]) {
				i--
			}
			if i < 0 {
				break
			}
			escolha[i]++
			escolha, opcoes = escolha[:i+1], opcoes[:i+1]
		}
	}

	out := make([]Faixa, 0, len(achado))
	for ef, l := range achado {
		out = append(out, Faixa{Efeito: ef, Min: l.min, Max: l.max})
	}
	slices.SortFunc(out, func(a, b Faixa) int { return int(a.Efeito) - int(b.Efeito) })
	return out
}
