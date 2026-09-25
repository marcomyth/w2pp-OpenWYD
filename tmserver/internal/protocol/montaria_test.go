package protocol

import "testing"

func TestMontariaPedeIdaEVolta(t *testing.T) {
	in := MontariaPedeBody{Pilhas: 7, SlotAmago: 42}
	b := in.Encode()
	if len(b) != MontariaPedeBodySize {
		t.Fatalf("len = %d, want %d", len(b), MontariaPedeBodySize)
	}
	// O cliente monta o pedido na mão: os bytes têm de estar onde montariarede.cpp põe.
	if b[0] != 7 || b[2] != 42 || b[3] != 0 {
		t.Errorf("bytes = % x", b)
	}
	var out MontariaPedeBody
	if err := out.Decode(b); err != nil || out != in {
		t.Errorf("decode = %+v, %v; want %+v", out, err, in)
	}
	if err := out.Decode(b[:7]); err == nil {
		t.Error("pedido curto devia dar erro")
	}
}

func TestMontariaResultadoIdaEVolta(t *testing.T) {
	in := MontariaResultadoBody{
		Motivo: MontariaSemAmago, NivelInicial: 37, NivelFinal: 62, Pilhas: 3,
		Usados: 700, Sucessos: 300, Falhas: 400, Quedas: 81, Montaria: 2370,
	}
	b := in.Encode()
	if len(b) != MontariaResultadoBodySize {
		t.Fatalf("len = %d, want %d", len(b), MontariaResultadoBodySize)
	}
	if b[0] != MontariaSemAmago || b[1] != 37 || b[2] != 62 || b[3] != 3 || b[4] != 0xBC || b[5] != 0x02 {
		t.Errorf("bytes = % x", b)
	}
	var out MontariaResultadoBody
	if err := out.Decode(b); err != nil || out != in {
		t.Errorf("decode = %+v, %v; want %+v", out, err, in)
	}
}
