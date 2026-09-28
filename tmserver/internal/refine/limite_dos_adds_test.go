package refine

import (
	"fmt"
	"math/rand"
	"testing"

	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/world"
)

// O limite dos adds de armadura (Marco, 27/09/2026):
//   - adds combinados (Defesa, Magia, Dano, Crítico) param em Defesa 30,
//     Crítico 7% (70 no byte) e Magia 10%;
//   - Defesa acima de 30 só existe sozinha.
//
// Estes testes varrem todo caminho que GERA add de armadura — o drop comum e a
// Repletion — e cobram o limite no item que sai, somando as três vagas (o drop
// pode pôr um bônus especial na vaga do refino, por cima dos dois adds).
const (
	limiteDefesaCombinada = 30
	limiteCriticoByte     = 70
	limiteMagia           = 10
)

// estouraLimite diz por que o item passa do limite, ou "" se está dentro.
func estouraLimite(it world.Item) string {
	var def, mag, dano, crit int
	for _, e := range it.Effects {
		switch e.Effect {
		case efAC:
			def += int(e.Value)
		case efMagic:
			mag += int(e.Value)
		case efDamageBonus:
			dano += int(e.Value)
		case efCritical2:
			crit += int(e.Value)
		}
	}
	outros := 0
	for _, v := range []int{mag, dano, crit} {
		if v > 0 {
			outros++
		}
	}
	if def > limiteDefesaCombinada && outros > 0 {
		return fmt.Sprintf("Defesa %d junto com outro add (magia %d, dano %d, crítico %d)", def, mag, dano, crit)
	}
	if def > 0 && outros > 0 || outros > 1 {
		switch {
		case crit > limiteCriticoByte:
			return fmt.Sprintf("Crítico %d (byte) em adds combinados", crit)
		case mag > limiteMagia:
			return fmt.Sprintf("Magia %d em adds combinados", mag)
		}
	}
	return ""
}

var pecasDeArmadura = []struct {
	nome string
	nPos int
}{{"elmo", posElmo}, {"peito", posArmadura}, {"calça", posCalca}, {"luva", posLuva}, {"bota", posBota}}

// sorteios são as três formas de rolar: sempre o menor valor (no jogo, o melhor
// resultado de toda escada), sempre o maior, e aleatório.
func sorteios(seed int64) map[string]func(int) int {
	r := rand.New(rand.NewSource(seed))
	return map[string]func(int) int{
		"melhor":    func(int) int { return 0 },
		"pior":      func(n int) int { return n - 1 },
		"aleatório": func(n int) int { return r.Intn(n) },
	}
}

// O drop comum não passa do limite em nenhuma faixa de distância, com ou sem
// bônus de drop, com ou sem cristal.
func TestDropNaoEstouraOLimiteDosAdds(t *testing.T) {
	tab := TabelasPadrao()
	const reqLvl = 100
	for _, p := range pecasDeArmadura {
		for dist := 0; dist <= 4; dist++ {
			nivel := reqLvl - 1 + 25*dist
			if dist == 4 {
				nivel = reqLvl + 110 // longe o bastante para o piso dos bônus
			}
			for _, bonus := range []int{0, 8, 16, 24} {
				for _, cristal := range []bool{false, true} {
					for nomeSorteio, roll := range sorteios(int64(p.nPos*1000 + dist*100 + bonus)) {
						vezes := 1
						if nomeSorteio == "aleatório" {
							vezes = 20000
						}
						for i := 0; i < vezes; i++ {
							it := world.Item{Index: 1345}
							tab.Drop(&it, Base{ReqLvl: reqLvl, Pos: p.nPos, Indice: 1345}, nivel, bonus, cristal, roll)
							if p.nPos == posLuva && luvaComDefesaAlta(it) {
								t.Fatalf("luva do drop com Defesa alta: %+v", it.Effects)
							}
							if motivo := estouraLimite(it); motivo != "" {
								t.Fatalf("%s, distância %d, bônus %d, cristal %v, sorteio %s: %s — %+v",
									p.nome, dist, bonus, cristal, nomeSorteio, motivo, it.Effects)
							}
						}
					}
				}
			}
		}
	}
}

// A Repletion inteira (ClasseBonus: adds e refino) não passa do limite em
// nenhuma peça, partindo de peças já refinadas ou não.
func TestRepletionNaoEstouraOLimiteDosAdds(t *testing.T) {
	for _, p := range pecasDeArmadura {
		for nomeSorteio, roll := range sorteios(int64(p.nPos)) {
			vezes := 1
			if nomeSorteio == "aleatório" {
				vezes = 50000
			}
			for i := 0; i < vezes; i++ {
				it := world.Item{Index: 1345, Effects: [3]world.Effect{{Effect: efSanc, Value: uint8(i % 7)}}}
				ClasseBonus(&it, p.nPos, roll, noAbility)
				if p.nPos == posLuva && luvaComDefesaAlta(it) {
					t.Fatalf("luva da Repletion com Defesa alta: %+v", it.Effects)
				}
				if motivo := estouraLimite(it); motivo != "" {
					t.Fatalf("%s, sorteio %s: %s — %+v", p.nome, nomeSorteio, motivo, it.Effects)
				}
			}
		}
	}
}

// Se alguém subir as faixas da Repletion acima do teto (Magia 14, Crítico 9%,
// Defesa 30 com Magia), a peça que sai continua cabendo: o teto no fim de
// ClasseBonus corta. É este teste que prova que o teto está ligado ali, já que as
// faixas de hoje cabem sozinhas.
func TestRepletionSeguraOTetoSeAsFaixasMudarem(t *testing.T) {
	dano, crit, defesa := classeDanoPeito, classeCriticoPeito, classeDefesaPeito
	t.Cleanup(func() { classeDanoPeito, classeCriticoPeito, classeDefesaPeito = dano, crit, defesa })
	classeDanoPeito = []classeValor{{efMagic, 14, 1}}
	classeCriticoPeito = []classeValor{{efCritical2, 90, 1}}
	classeDefesaPeito = []classeValor{{efAC, 30, 1}} // 30 não vem sozinha: combina com a Magia 14

	r := rand.New(rand.NewSource(7))
	for i := 0; i < 5000; i++ {
		it := world.Item{Index: 1345}
		ClasseBonus(&it, nPosChest, r.Intn, noAbility)
		if motivo := estouraLimite(it); motivo != "" {
			t.Fatalf("com as faixas acima do teto a Repletion deixou passar: %s — %+v", motivo, it.Effects)
		}
	}
}

// luvaComDefesaAlta diz se a luva tem um add de Defesa de 30 ou mais: a Defesa
// alta é só de peito e calça (Marco, 27/09/2026).
func luvaComDefesaAlta(it world.Item) bool {
	for i := 1; i <= 2; i++ {
		if it.Effects[i].Effect == efAC && int(it.Effects[i].Value) >= limiteDefesaCombinada {
			return true
		}
	}
	return false
}
