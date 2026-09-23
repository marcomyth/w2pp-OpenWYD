package handler

import (
	"testing"

	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/protocol"
	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/world"
)

// fixturaPontos é um personagem de mesa, sem rede: as regras do lote são contas
// sobre a Entity, e o que importa aqui é o que sobra nela.
func fixturaPontos(t *testing.T, nivel int32) (*Dispatcher, *world.World, *world.Session, *world.Entity) {
	t.Helper()
	d, w, e := fixturaPilha(t)
	e.Level = nivel
	e.ClassMaster = classMasterMortal
	e.HP = 100
	s := &world.Session{Conn: 0, Mode: world.UserPlay}
	return d, w, s, e
}

// O clique e o lote têm de somar a MESMA coisa: os dois passam por
// somaUmAtributo. Int e Con também levam 2 de MaxMP/MaxHP, como o legado.
func TestSomaUmAtributo(t *testing.T) {
	_, _, _, e := fixturaPontos(t, 100)
	e.ScoreBonus = 10
	mpAntes, hpAntes := e.BaseMaxMP, e.BaseMaxHP

	if !somaUmAtributo(e, protocol.DetailInt) {
		t.Fatal("INT recusou com 10 pontos no monte")
	}
	if e.BaseInt != 1 || e.BaseMaxMP != mpAntes+2 {
		t.Errorf("INT = %d, MaxMP = %d (era %d): quero +1 e +2", e.BaseInt, e.BaseMaxMP, mpAntes)
	}
	if !somaUmAtributo(e, protocol.DetailCon) {
		t.Fatal("CON recusou")
	}
	if e.BaseCon != 1 || e.BaseMaxHP != hpAntes+2 {
		t.Errorf("CON = %d, MaxHP = %d (era %d): quero +1 e +2", e.BaseCon, e.BaseMaxHP, hpAntes)
	}
	if e.ScoreBonus != 8 {
		t.Errorf("monte = %d, quero 8 (dois gastos)", e.ScoreBonus)
	}

	e.ScoreBonus = 0
	if somaUmAtributo(e, protocol.DetailStr) {
		t.Error("somou atributo com o monte zerado")
	}
	e.ScoreBonus = 5
	if somaUmAtributo(e, 9) {
		t.Error("somou num campo que não existe")
	}
	if e.ScoreBonus != 5 {
		t.Errorf("campo inválido gastou ponto: monte = %d", e.ScoreBonus)
	}
}

// As duas paredes da aprendizagem: a do nível, que sobe sozinha, e a absoluta.
func TestCabeNaAprendizagem(t *testing.T) {
	_, _, _, e := fixturaPontos(t, 100)
	// Nível 100: a permissão é 3*(100+1)/2 = 151, abaixo dos 200.
	if got := cabeNaAprendizagem(e, 1); got != 151 {
		t.Errorf("nível 100: cabe %d, quero 151 (a parede do nível)", got)
	}
	e.BaseSpecial[1] = 51
	if got := cabeNaAprendizagem(e, 1); got != 100 {
		t.Errorf("com 51 dentro: cabe %d, quero 100", got)
	}
	// Nível alto: a parede vira a absoluta de 200.
	e.Level = 335
	e.BaseSpecial[1] = 0
	if got := cabeNaAprendizagem(e, 1); got != 200 {
		t.Errorf("nível 335: cabe %d, quero 200 (a parede absoluta)", got)
	}
	// Com a maestria aprendida a absoluta sobe para 255.
	e.LearnedSkill |= 1 << 7
	if got := cabeNaAprendizagem(e, 1); got != 255 {
		t.Errorf("com a maestria: cabe %d, quero 255", got)
	}
	e.BaseSpecial[1] = 255
	if got := cabeNaAprendizagem(e, 1); got != 0 {
		t.Errorf("cheio: cabe %d, quero 0", got)
	}
}

// TUDO OU NADA no atributo: pedir mais do que se tem não gasta NADA.
func TestLoteDeAtributoRecusaSemGastar(t *testing.T) {
	d, w, s, e := fixturaPontos(t, 335)
	e.ScoreBonus = 130

	d.loteDeAtributo(w, s, e, protocol.DetailStr, 150)

	if e.ScoreBonus != 130 {
		t.Errorf("monte = %d, quero 130 intactos", e.ScoreBonus)
	}
	if e.BaseStr != 0 {
		t.Errorf("FOR = %d, quero 0: a recusa não pode gastar", e.BaseStr)
	}
}

func TestLoteDeAtributoAplicaTudo(t *testing.T) {
	d, w, s, e := fixturaPontos(t, 335)
	e.ScoreBonus = 300
	mpAntes := e.BaseMaxMP

	d.loteDeAtributo(w, s, e, protocol.DetailInt, 150)

	if e.BaseInt != 150 {
		t.Errorf("INT = %d, quero 150", e.BaseInt)
	}
	if e.ScoreBonus != 150 {
		t.Errorf("monte = %d, quero 150 (300 − 150)", e.ScoreBonus)
	}
	if e.BaseMaxMP != mpAntes+300 {
		t.Errorf("MaxMP = %d, quero +300 (2 por ponto)", e.BaseMaxMP-mpAntes)
	}
}

// As duas recusas da aprendizagem são diferentes, e nenhuma gasta ponto.
func TestLoteDeAprendizagemRecusaSemGastar(t *testing.T) {
	casos := []struct {
		nome     string
		monte    uint16
		nivel    int32
		jaDentro int16
		pedido   int
	}{
		{"pontos de menos", 130, 335, 0, 150},
		{"não cabe: a parede do nível", 400, 100, 0, 200},
		{"não cabe: a parede absoluta", 400, 335, 100, 150},
	}
	for _, c := range casos {
		t.Run(c.nome, func(t *testing.T) {
			d, w, s, e := fixturaPontos(t, c.nivel)
			e.SpecialBonus = c.monte
			e.BaseSpecial[1] = c.jaDentro

			d.loteDeAprendizagem(w, s, e, 1, c.pedido)

			if e.SpecialBonus != c.monte {
				t.Errorf("monte = %d, quero %d intactos", e.SpecialBonus, c.monte)
			}
			if e.BaseSpecial[1] != c.jaDentro {
				t.Errorf("campo = %d, quero %d intacto", e.BaseSpecial[1], c.jaDentro)
			}
		})
	}
}

func TestLoteDeAprendizagemAplicaTudo(t *testing.T) {
	d, w, s, e := fixturaPontos(t, 335)
	e.SpecialBonus = 668

	d.loteDeAprendizagem(w, s, e, 1, 200)

	if e.BaseSpecial[1] != 200 {
		t.Errorf("Magia Branca = %d, quero 200", e.BaseSpecial[1])
	}
	if e.SpecialBonus != 468 {
		t.Errorf("monte = %d, quero 468 (668 − 200)", e.SpecialBonus)
	}
	if cabe := cabeNaAprendizagem(e, 1); cabe != 0 {
		t.Errorf("depois de encher, ainda cabe %d", cabe)
	}
}

// O corpo é do cliente, então a porta confere tudo antes de qualquer gasto:
// quadro desconhecido, campo fora de 0..3, quantidade zero, negativa ou absurda.
func TestPedidoDeLoteValido(t *testing.T) {
	ruins := []protocol.MsgPontosEmLoteBody{
		{Tipo: protocol.PontosAtributo, Campo: 4, Quantidade: 10},
		{Tipo: protocol.PontosAtributo, Campo: 200, Quantidade: 10},
		{Tipo: protocol.PontosAtributo, Campo: 0, Quantidade: 0},
		{Tipo: protocol.PontosAtributo, Campo: 0, Quantidade: -5},
		{Tipo: protocol.PontosAtributo, Campo: 0, Quantidade: protocol.PontosEmLoteMax + 1},
		{Tipo: 0, Campo: 0, Quantidade: 10},
		{Tipo: 77, Campo: 0, Quantidade: 10},
	}
	for _, b := range ruins {
		if pedidoDeLoteValido(b) {
			t.Errorf("%+v passou na porta", b)
		}
	}
	bons := []protocol.MsgPontosEmLoteBody{
		{Tipo: protocol.PontosAtributo, Campo: 0, Quantidade: 1},
		{Tipo: protocol.PontosAtributo, Campo: 3, Quantidade: protocol.PontosEmLoteMax},
		{Tipo: protocol.PontosAprendizagem, Campo: 1, Quantidade: 200},
	}
	for _, b := range bons {
		if !pedidoDeLoteValido(b) {
			t.Errorf("%+v foi recusado sem motivo", b)
		}
	}
}

// O corpo vai e volta sem perder nada: é o contrato com o GamePatch.
func TestPontosEmLoteIdaEVolta(t *testing.T) {
	quero := protocol.MsgPontosEmLoteBody{
		Tipo: protocol.PontosAprendizagem, Campo: 2, Quantidade: 137,
	}
	var veio protocol.MsgPontosEmLoteBody
	if err := veio.Decode(quero.Encode()); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if veio != quero {
		t.Errorf("voltou %+v, quero %+v", veio, quero)
	}
	if err := veio.Decode([]byte{1, 2}); err == nil {
		t.Error("um corpo curto passou")
	}
}
