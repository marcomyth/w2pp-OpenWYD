package handler

import (
	"testing"

	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/world"
)

const itemFrangoAssado int16 = 3314

// TestFrangoAcumulaAte24h é a regra pedida em 22/09/2026: cada frango soma 4h ao
// que já corre, até 24h — o mesmo acúmulo do Baú de Experiência (useExpChest).
//
// A parte que o teste existe para travar é o FIM: no teto o tempo PARA em 24h e
// o item é consumido assim mesmo, como no Baú. É uma escolha, não um descuido
// — o cliente só recebe Time&0xFF do ícone (PackAffect), então nada na tela vai
// avisar o jogador de que aquele frango não rendeu nada.
func TestFrangoAcumulaAte24h(t *testing.T) {
	d, w, s, e := mundoDaCoragem(t, itemFrangoAssado, 100)

	for i, querido := range []uint32{
		affect1H * 4, affect1H * 8, affect1H * 12,
		affect1H * 16, affect1H * 20, affect1H * 24,
	} {
		e.Carry[0] = world.Item{Index: itemFrangoAssado}
		d.useFrangoAssado(w, s, e, 0)

		af, ok := afetoDeCoragem(e)
		if !ok {
			t.Fatalf("frango %d: nenhum afeto", i+1)
		}
		if af.Time != querido {
			t.Errorf("frango %d: tempo %d (%s), want %d (%s)",
				i+1, af.Time, tempoAfeto(af.Time), querido, tempoAfeto(querido))
		}
		if int(af.Level) != frangoAtaqueEmMob {
			t.Errorf("frango %d: bônus %d, want %d", i+1, af.Level, frangoAtaqueEmMob)
		}
		if e.Carry[0].Index != 0 {
			t.Errorf("frango %d: o item não foi consumido", i+1)
		}
	}

	// Meio frango antes do teto o corte vale: o afeto anda para trás sozinho, e
	// 23h + 4h não podem virar 27h.
	for i := range e.Affect {
		if e.Affect[i].Type == world.AffectForceMobDamage {
			e.Affect[i].Time = affect1H * 23
		}
	}
	e.Carry[0] = world.Item{Index: itemFrangoAssado}
	d.useFrangoAssado(w, s, e, 0)
	if af, _ := afetoDeCoragem(e); af.Time != frangoTeto {
		t.Errorf("23h + 4h = %s, want o teto de %s", tempoAfeto(af.Time), tempoAfeto(frangoTeto))
	}

	// No teto o sétimo frango é comido e o tempo não passa de 24h.
	e.Carry[0] = world.Item{Index: itemFrangoAssado}
	d.useFrangoAssado(w, s, e, 0)
	af, _ := afetoDeCoragem(e)
	if af.Time != frangoTeto {
		t.Errorf("no teto o tempo virou %s, want %s", tempoAfeto(af.Time), tempoAfeto(frangoTeto))
	}
	if e.Carry[0].Index != 0 {
		t.Error("o frango do teto não foi consumido; a regra é a do Baú")
	}
}

// O afeto 30 é dividido com o Remédio/Elixir da Coragem (+500). O acúmulo vale
// só entre frangos: por cima de um bônus menor o frango sobe para 2000 e fica
// com o MAIOR dos dois tempos. Sem isso um Elixir de 3.000.000 de ouro (8h a
// +500) viraria um jeito barato de estocar horas para o bônus de +2000.
func TestFrangoNaoAcumulaSobreBonusMenor(t *testing.T) {
	d, w, s, e := mundoDaCoragem(t, itemFrangoAssado, 100)
	e.Affect[0] = world.Affect{Type: world.AffectForceMobDamage,
		Level: coragemAtaqueEmMob, Time: uint32(coragemDuracaoElixir)}

	d.useFrangoAssado(w, s, e, 0)

	af, ok := afetoDeCoragem(e)
	if !ok {
		t.Fatal("o afeto sumiu")
	}
	if af.Time != uint32(coragemDuracaoElixir) {
		t.Errorf("tempo %d, want %d: o Elixir virou banco de horas",
			af.Time, coragemDuracaoElixir)
	}
	if int(af.Level) != frangoAtaqueEmMob {
		t.Errorf("bônus %d, want %d: o frango não subiu o golpe", af.Level, frangoAtaqueEmMob)
	}
}

// ...e o contrário também: um bônus menor quase no fim não pode encurtar o
// frango para menos do que os 4h dele.
func TestFrangoNaoEncolheSobreBonusMenorCurto(t *testing.T) {
	d, w, s, e := mundoDaCoragem(t, itemFrangoAssado, 100)
	e.Affect[0] = world.Affect{Type: world.AffectForceMobDamage, Level: coragemAtaqueEmMob, Time: 5}

	d.useFrangoAssado(w, s, e, 0)

	af, _ := afetoDeCoragem(e)
	if af.Time != frangoDuracao {
		t.Errorf("tempo %d, want %d: o frango herdou o fim do bônus menor", af.Time, frangoDuracao)
	}
}
