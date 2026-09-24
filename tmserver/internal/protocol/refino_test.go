package protocol

import (
	"bytes"
	"testing"
)

// Os bytes do pedido são os que o cliente (refinorede.cpp) monta à mão; se um
// campo mudar de lugar aqui, o painel manda um pedido que o servidor lê errado.
func TestRefinoPedeBytes(t *testing.T) {
	p := RefinoPedeBody{Lugar: 1, Slot: 7, SlotPoeira: 12, Alvo: 9, MaxPoeiras: 300}
	want := []byte{1, 0, 7, 0, 12, 0, 9, 0, 0x2C, 0x01, 0, 0}
	got := p.Encode()
	if !bytes.Equal(got, want) {
		t.Fatalf("Encode = % x, want % x", got, want)
	}
	var back RefinoPedeBody
	if err := back.Decode(got); err != nil {
		t.Fatal(err)
	}
	if back != p {
		t.Fatalf("Decode = %+v, want %+v", back, p)
	}
	if err := back.Decode(got[:11]); err == nil {
		t.Fatal("pedido curto passou")
	}
}

func TestRefinoResultadoBytes(t *testing.T) {
	r := RefinoResultadoBody{
		Motivo: RefinoMuro, NivelInicial: 3, NivelFinal: 6,
		Usadas: 260, Sucessos: 3, Falhas: 257, Lugar: 0, Slot: -1,
	}
	want := []byte{3, 3, 6, 0, 0x04, 0x01, 3, 0, 0x01, 0x01, 0, 0, 0xFF, 0xFF, 0, 0}
	got := r.Encode()
	if !bytes.Equal(got, want) {
		t.Fatalf("Encode = % x, want % x", got, want)
	}
	var back RefinoResultadoBody
	if err := back.Decode(got); err != nil {
		t.Fatal(err)
	}
	if back != r {
		t.Fatalf("Decode = %+v, want %+v", back, r)
	}
}
