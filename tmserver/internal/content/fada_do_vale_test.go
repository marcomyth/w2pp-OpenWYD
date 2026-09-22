package content

import (
	"testing"
)

// fadaDoVale é a Fada do Vale(7dias) do ItemList.csv, a única chave do Vale
// Escondido (handler/vale.go, GetFunc.cpp:924-931).
const fadaDoVale = 3916

// Nenhum comerciante oferece a Fada do Vale (decisão de 21/09/2026, migrações
// 0091 e 0094). O template é a fonte viva da vitrine: o dbServer ressemeia a
// loja de todo mercador a partir dele em cada boot, então apagar a linha do
// banco sem tirar do arquivo devolve o item no deploy seguinte. Um teste sobre
// o arquivo é o que fecha esse caminho de volta.
//
// A primeira versão deste teste varria só os templates de 816 bytes, e passou
// com a fada ainda à venda no Utilidades, que é de 756. A varredura agora é a
// cadaTemplate de vitrines_test.go, que conhece os três layouts.
func TestNenhumaLojaVendeAFadaDoVale(t *testing.T) {
	cadaTemplate(t, func(nome string, b []byte) {
		for i, item := range vitrine(b) {
			if item == fadaDoVale {
				t.Errorf("%s (%d bytes) oferece a Fada do Vale na vaga %d", nome, len(b), i)
			}
		}
	})
}
