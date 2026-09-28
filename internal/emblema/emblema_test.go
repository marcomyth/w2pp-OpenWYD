package emblema

import (
	"encoding/binary"
	"testing"
)

// bmpBom monta o emblema exato que o cliente lê, para os testes partirem de um
// válido e estragar um campo de cada vez.
func bmpBom() []byte {
	b := make([]byte, Tamanho)
	b[0], b[1] = 'B', 'M'
	binary.LittleEndian.PutUint32(b[2:], Tamanho)
	binary.LittleEndian.PutUint32(b[10:], 54)
	binary.LittleEndian.PutUint32(b[14:], 40)
	binary.LittleEndian.PutUint32(b[18:], uint32(Largura))
	binary.LittleEndian.PutUint32(b[22:], uint32(Altura))
	binary.LittleEndian.PutUint16(b[26:], 1)
	binary.LittleEndian.PutUint16(b[28:], 24)
	binary.LittleEndian.PutUint32(b[30:], 0)
	return b
}

func TestValidoAceitaOEmblemaDoCliente(t *testing.T) {
	if !Valido(bmpBom()) {
		t.Fatal("o emblema que o cliente lê foi recusado")
	}
}

// TestValidoRecusaCadaCampoErrado: cada um destes é um BMP que uma biblioteca de
// imagem aceitaria e que o jogo não desenha — ou desenha errado, calado.
func TestValidoRecusaCadaCampoErrado(t *testing.T) {
	casos := []struct {
		nome  string
		mexer func(b []byte) []byte
	}{
		{"curto", func(b []byte) []byte { return b[:Tamanho-1] }},
		{"longo", func(b []byte) []byte { return append(b, 0, 0) }},
		{"vazio", func([]byte) []byte { return nil }},
		{"sem BM", func(b []byte) []byte { b[0] = 'X'; return b }},
		{"bfSize errado", func(b []byte) []byte { binary.LittleEndian.PutUint32(b[2:], 632); return b }},
		{"pixels em outro lugar", func(b []byte) []byte { binary.LittleEndian.PutUint32(b[10:], 122); return b }},
		{"cabecalho v5", func(b []byte) []byte { binary.LittleEndian.PutUint32(b[14:], 124); return b }},
		{"largura 32", func(b []byte) []byte { binary.LittleEndian.PutUint32(b[18:], 32); return b }},
		{"altura negativa", func(b []byte) []byte {
			binary.LittleEndian.PutUint32(b[22:], ^uint32(Altura)+1)
			return b
		}},
		{"dois planos", func(b []byte) []byte { binary.LittleEndian.PutUint16(b[26:], 2); return b }},
		{"32 bits", func(b []byte) []byte { binary.LittleEndian.PutUint16(b[28:], 32); return b }},
		{"comprimido", func(b []byte) []byte { binary.LittleEndian.PutUint32(b[30:], 1); return b }},
	}
	for _, c := range casos {
		t.Run(c.nome, func(t *testing.T) {
			if Valido(c.mexer(bmpBom())) {
				t.Errorf("%s passou", c.nome)
			}
		})
	}
}
