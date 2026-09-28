package donate

import "testing"

func ptr(v int16) *int16 { return &v }

// TestBrindeDaLinhaSeparaAQuantidade: o EF_AMOUNT sai dos efeitos e vira a
// quantidade, porque o entrega.Lote recusa um EF_AMOUNT escrito à mão; os outros
// efeitos ficam, na ordem, a partir do primeiro espaço.
func TestBrindeDaLinhaSeparaAQuantidade(t *testing.T) {
	for _, c := range []struct {
		nome string
		e    [6]*int16
		want Brinde
	}{
		{"pilha", [6]*int16{ptr(61), ptr(64)}, Brinde{Index: 1, Quantidade: 64}},
		{"montaria com prazo", [6]*int16{ptr(106), ptr(15)},
			Brinde{Index: 1, Quantidade: 1, Eff: [3][2]uint8{{106, 15}}}},
		{"sem efeito é uma unidade", [6]*int16{}, Brinde{Index: 1, Quantidade: 1}},
		{"EF_AMOUNT zero é uma unidade", [6]*int16{ptr(61), ptr(0)}, Brinde{Index: 1, Quantidade: 1}},
		{"quantidade no meio", [6]*int16{ptr(106), ptr(7), ptr(61), ptr(3)},
			Brinde{Index: 1, Quantidade: 3, Eff: [3][2]uint8{{106, 7}}}},
	} {
		t.Run(c.nome, func(t *testing.T) {
			if got := brindeDaLinha(1, c.e); got != c.want {
				t.Errorf("brindeDaLinha = %+v, want %+v", got, c.want)
			}
		})
	}
}

func TestNomeDoPacote(t *testing.T) {
	for id, want := range map[string]string{
		"apoiador-supremo":   "Apoiador Supremo",
		"apoiador-iniciante": "Apoiador Iniciante",
		"lenda":              "Lenda",
	} {
		if got := NomeDoPacote(id); got != want {
			t.Errorf("NomeDoPacote(%q) = %q, want %q", id, got, want)
		}
	}
}

// TestEspacosDoPacote: 64 Baús do Apoiador empilham num espaço; a montaria ocupa
// o dela. É o mesmo número que o site promete na página do pacote.
func TestEspacosDoPacote(t *testing.T) {
	p := Pacote{Brindes: []Brinde{
		{Index: 3991, Quantidade: 1},
		{Index: 3305, Quantidade: 64},
	}}
	if got := p.Espacos(); got != 2 {
		t.Errorf("Espacos = %d, want 2", got)
	}
}
