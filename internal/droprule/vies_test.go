package droprule

import (
	"math"
	"testing"
)

// randMSVCMax é o teto do rand() do MSVC (RAND_MAX): o sorteio da Mesa é
// rand()%MaxChance sobre ele.
const randMSVCMax = 32768

// TestViesDoSorteioDaMesa é um teste-documento: ele NÃO aprova o viés, ele o
// mede e prende o número, para que a conta que qualquer balanceamento faz em
// cima da Mesa parta do que o jogo entrega e não do que o painel escreve.
//
// O rand() do MSVC vai de 0 a 32767 e a Mesa sorteia rand()%10000. Isso reparte
// 32768 sorteios em 10.000 baldes: os 2.768 primeiros recebem QUATRO e os 7.232
// restantes recebem três. Toda chance inteiramente dentro dos primeiros 2.768 —
// ou seja, abaixo de 27,68% — sai 4/3,2768 = 22,07% maior do que o painel diz.
// Acima disso o excesso cai, até zerar em 100%.
//
// Se alguém corrigir Roll (a memória do projeto fala em duas chamadas de rand
// para uma base fina), este teste falha e cobra a atualização dos comentários
// que citam estes números — a migração 0098 é um deles.
func TestViesDoSorteioDaMesa(t *testing.T) {
	// taxa conta os sorteios, sem simular: é o valor exato, não uma amostra.
	taxa := func(chance int32) float64 {
		acertos := 0
		for r := 0; r < randMSVCMax; r++ {
			if Roll(chance, func(n int) int { return r % n }) {
				acertos++
			}
		}
		return float64(acertos) / randMSVCMax * 100
	}
	for _, c := range []struct {
		chance int32
		quer   float64 // o que o jogo entrega, em %
	}{
		{10000, 100}, {5000, 54.224}, {3000, 35.913}, {2500, 30.518},
		{1500, 18.311}, {1000, 12.207}, {800, 9.766}, {600, 7.324},
		{500, 6.104}, {250, 3.052}, {150, 1.831}, {50, 0.610}, {20, 0.244}, {0, 0},
	} {
		medido := taxa(c.chance)
		if math.Abs(medido-c.quer) > 0.001 {
			t.Errorf("chance %d: o jogo entrega %.3f%%, e o teste esperava %.3f%%", c.chance, medido, c.quer)
		}
		if c.chance > 0 && c.chance < 2768 && math.Abs(medido/(float64(c.chance)/100)-4.0/3.2768) > 0.0001 {
			t.Errorf("chance %d: o excesso abaixo de 27,68%% deixou de ser 4/3,2768", c.chance)
		}
	}
}
