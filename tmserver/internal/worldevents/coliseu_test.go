package worldevents

import (
	"reflect"
	"testing"
	"time"
)

// rodaHora anda o ColoState de 12 em 12 s pela hora inteira, como o
// ProcessMinTimer, e devolve os passos não vazios com o minuto em que saíram.
func rodaHora(c *Coliseu, hora, guilda, novato int) (minutos []int, passos []ColiseuPasso) {
	for s := 0; s < 3600; s += 12 {
		if p := c.Passo(hora, s/60, guilda, novato); !p.Vazio() {
			minutos = append(minutos, s/60)
			passos = append(passos, p)
		}
	}
	return minutos, passos
}

// TestColiseuNoiteDasVinte: a sequência de Server.cpp:6942-7057 com as horas do
// legado (20 e 20): Ciclopes, e o limite de novato ligado mesmo assim.
func TestColiseuNoiteDasVinte(t *testing.T) {
	var c Coliseu
	minutos, passos := rodaHora(&c, 20, 20, 20)
	want := []struct {
		min int
		p   ColiseuPasso
	}{
		{0, ColiseuPasso{Zera: true}},
		{3, ColiseuPasso{FechaEntrada: true, LimiteNovato: true}},
		{4, ColiseuPasso{Ondas: []int{0}}},
		{5, ColiseuPasso{AbreInternos: true}},
		{7, ColiseuPasso{Ondas: []int{1}}},
		{9, ColiseuPasso{Ondas: []int{0, 1}}},
		{11, ColiseuPasso{Ondas: []int{1, 2}}},
		{13, ColiseuPasso{Ondas: []int{1, 2}}},
		{15, ColiseuPasso{Fim: true}},
	}
	if len(passos) != len(want) {
		t.Fatalf("%d passos nos minutos %v, want %d", len(passos), minutos, len(want))
	}
	for i, w := range want {
		if minutos[i] != w.min || !reflect.DeepEqual(passos[i], w.p) {
			t.Errorf("passo %d: minuto %d %+v, want minuto %d %+v", i, minutos[i], passos[i], w.min, w.p)
		}
	}
	if c.Fase() != ColiseuParado || c.Limite150() {
		t.Errorf("depois da hora: fase %d, limite %v; want parado e sem limite", c.Fase(), c.Limite150())
	}
}

// TestColiseuHoraDeNovatoSolta os Orcs quando as horas são diferentes, e a de
// guilda não liga o limite de nível.
func TestColiseuHoraDeNovato(t *testing.T) {
	var c Coliseu
	_, passos := rodaHora(&c, 21, 20, 21)
	var ondas []int
	limite := false
	for _, p := range passos {
		ondas = append(ondas, p.Ondas...)
		limite = limite || p.LimiteNovato
	}
	if want := []int{5, 6, 5, 6, 6, 7, 6, 7}; !reflect.DeepEqual(ondas, want) {
		t.Errorf("ondas na hora de novato = %v, want %v", ondas, want)
	}
	if !limite {
		t.Error("hora de novato sem o limite de nível 150")
	}

	var g Coliseu
	_, passos = rodaHora(&g, 20, 20, 21)
	for _, p := range passos {
		if p.LimiteNovato {
			t.Error("hora de guilda ligou o limite de novato")
		}
	}
}

func TestColiseuForaDaHoraNaoAnda(t *testing.T) {
	var c Coliseu
	if _, passos := rodaHora(&c, 19, 20, 20); len(passos) != 0 {
		t.Errorf("19h: %d passos, want nenhum", len(passos))
	}
	// Ligado depois do minuto 3, espera o dia seguinte: o legado só zera nos
	// minutos 0-2.
	for m := 3; m < 60; m++ {
		if p := c.Passo(20, m, 20, 20); !p.Vazio() {
			t.Fatalf("20:%02d sem o zero dos minutos 0-2 deu %+v", m, p)
		}
	}
}

// TestColiseuForcado corre no relógio do GM, fora da hora, e acaba sozinho.
func TestColiseuForcado(t *testing.T) {
	var c Coliseu
	inicio := time.Date(2026, time.September, 24, 15, 7, 30, 0, time.Local)
	if !c.Forcar(inicio) {
		t.Fatal("Forcar recusou com o evento parado")
	}
	if c.Forcar(inicio) {
		t.Error("Forcar aceitou um segundo evento")
	}
	var fins, fechou int
	for s := 0; s <= 13*60; s += 12 {
		h, m := c.Relogio(inicio.Add(time.Duration(s)*time.Second), 20)
		p := c.Passo(h, m, 20, 20)
		if p.FechaEntrada {
			fechou = s
		}
		if p.Fim {
			fins++
		}
	}
	if fechou != 0 {
		t.Errorf("entrada trancou no segundo %d; o forçado começa no minuto 3", fechou)
	}
	if fins != 1 || c.Forcado() || c.Fase() != ColiseuParado {
		t.Errorf("fins %d, forçado %v, fase %d; want 1 fim e parado", fins, c.Forcado(), c.Fase())
	}
}

func TestColiseuEncerrar(t *testing.T) {
	var c Coliseu
	if p := c.Encerrar(); !p.Vazio() {
		t.Errorf("Encerrar parado = %+v, want vazio", p)
	}
	c.Forcar(time.Now())
	c.Passo(20, 3, 20, 20)
	if p := c.Encerrar(); !p.Fim || c.Fase() != ColiseuParado || c.Limite150() {
		t.Errorf("Encerrar em curso = %+v, fase %d, limite %v", p, c.Fase(), c.Limite150())
	}
}

// TestBatalhaTresRodadas: três rodadas na hora (Server.cpp:6849-6939), cada
// uma com preparo no 0, luta no 5, prêmio no 13 e reabertura no 14.
func TestBatalhaTresRodadas(t *testing.T) {
	var b Batalha
	type ev struct {
		min  int
		tipo string
	}
	var got []ev
	for s := 0; s < 3600; s += 12 {
		p := b.Passo(19, s/60, 19)
		switch {
		case p.Prepara:
			got = append(got, ev{s / 60, "prepara"})
		case p.Inicia:
			got = append(got, ev{s / 60, "inicia"})
		case p.Premia:
			got = append(got, ev{s / 60, "premia"})
		}
		if p.Reabre {
			got = append(got, ev{s / 60, "reabre"})
		}
		if !p.Vazio() && p.Grade != s/60/BatalhaMinutos {
			t.Errorf("minuto %d: grade %d", s/60, p.Grade)
		}
	}
	var want []ev
	for g := 0; g < 3; g++ {
		base := g * BatalhaMinutos
		want = append(want, ev{base, "prepara"}, ev{base + 5, "inicia"}, ev{base + 13, "premia"}, ev{base + 14, "reabre"})
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("rodadas = %v\nwant     %v", got, want)
	}
	var fora Batalha
	if fora.Passo(18, 0, 19).Prepara {
		t.Error("18h preparou uma rodada")
	}
}

// TestBatalhaMinutoExato: as três primeiras transições testam o minuto exato;
// um relógio que pula o minuto 5 deixa a rodada parada em "pronta", como no
// legado.
func TestBatalhaMinutoExato(t *testing.T) {
	var b Batalha
	b.Passo(19, 0, 19)
	if p := b.Passo(19, 6, 19); !p.Vazio() || b.Fase() != BatalhaPronta {
		t.Errorf("minuto 6 sem o 5: %+v fase %d, want nada e pronta", p, b.Fase())
	}
}

func TestBatalhaForcada(t *testing.T) {
	var b Batalha
	inicio := time.Date(2026, time.September, 24, 10, 0, 0, 0, time.Local)
	if b.Forcar(inicio, 3) {
		t.Error("Forcar aceitou a rodada 3")
	}
	if !b.Forcar(inicio, 1) {
		t.Fatal("Forcar recusou a rodada 1")
	}
	h, m := b.Relogio(inicio, 19)
	if p := b.Passo(h, m, 19); !p.Prepara || p.Grade != 1 {
		t.Fatalf("primeiro passo forçado = %+v, want prepara na rodada 1", p)
	}
	h, m = b.Relogio(inicio.Add(14*time.Minute), 19)
	b.Passo(h, m, 19) // pula direto do preparo: minuto exato, nada acontece
	if b.Fase() != BatalhaPronta {
		t.Fatalf("fase %d, want pronta", b.Fase())
	}
	// Pronta ainda não trancou a entrada: nada a reabrir.
	if b.Encerrar() {
		t.Error("Encerrar da rodada pronta pediu para reabrir a entrada")
	}
	if b.Fase() != BatalhaParada || b.Forcada() {
		t.Errorf("depois de Encerrar: fase %d, forçada %v", b.Fase(), b.Forcada())
	}
}
