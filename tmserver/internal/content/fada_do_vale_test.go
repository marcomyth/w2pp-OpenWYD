package content

import (
	"testing"
)

// fadaDoVale é a Fada do Vale do ItemList.csv, a única chave do Vale Escondido
// (handler/vale.go, GetFunc.cpp:924-931).
const fadaDoVale = 3916

// ehFada diz se o item é uma das fadas do slot 13: Verde, Azul e Vermelha
// (3900-3908), Verde e Suprema (3911-3913), Prateada, Dourada e do Vale
// (3914-3916). O buraco 3909-3910 são os dois Mapa_Vale_Escondido, que não são
// fada.
func ehFada(item int) bool {
	return item >= 3900 && item <= 3908 || item >= 3911 && item <= fadaDoVale
}

// Nenhum comerciante vende fada: a do Vale saiu em 21/09/2026 (migrações 0091 e
// 0094) e as outras em 23/09/2026 (0107). O template é a fonte viva da vitrine:
// o dbServer ressemeia a loja de todo mercador a partir dele em cada boot, então
// apagar a linha do banco sem tirar do arquivo devolve o item no deploy
// seguinte. Um teste sobre o arquivo é o que fecha esse caminho de volta.
//
// A primeira versão deste teste varria só os templates de 816 bytes, e passou
// com a fada ainda à venda no Utilidades, que é de 756. A varredura agora é a
// cadaTemplate de vitrines_test.go, que conhece os três layouts.
func TestNenhumaLojaVendeFada(t *testing.T) {
	cadaTemplate(t, func(nome string, b []byte) {
		for i, item := range vitrine(b) {
			if ehFada(item) {
				t.Errorf("%s (%d bytes) oferece a fada %d na vaga %d", nome, len(b), item, i)
			}
		}
	})
}
