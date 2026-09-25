package content

import (
	"bytes"
	"strings"
	"testing"

	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/protocol"
)

// TestNomeAcentuadoDoCatalogoChegaInteiroAoCliente mede o caminho todo, de ponta
// a ponta: o byte como ele está no ItemList.csv, o nome que o loader guarda, e o
// byte que sai na rede.
//
// O DEFEITO QUE ELE PRENDE. O arquivo é Windows-1252: "Titã" é T-i-t-0xE3, um byte
// só. Lido como UTF-8 esse byte não forma rune nenhuma, e o ClientText - que na
// saída não tem como adivinhar o que era - o achata em "?". O jogador lia "Tit?" na
// fala do NPC que pede o item.
func TestNomeAcentuadoDoCatalogoChegaInteiroAoCliente(t *testing.T) {
	const idx = 4242
	// A linha entra como BYTES, do jeito que o arquivo tem: escrever a fixture
	// como texto Go (UTF-8) provaria outra coisa que não o arquivo real.
	linha := append([]byte("4242,Tit"), 0xE3, ',', '0')
	l, err := parseItemList(bytes.NewReader(linha))
	if err != nil {
		t.Fatalf("parseItemList: %v", err)
	}
	nome := l.Names()[idx]
	if nome != "Titã" {
		t.Fatalf("nome = %q, quero %q", nome, "Titã")
	}
	// A volta: o mesmo byte do arquivo, não um "?".
	saiu := protocol.ClientText(nome)
	if !bytes.Equal(saiu, []byte{'T', 'i', 't', 0xE3}) {
		t.Errorf("na rede saiu % x, quero % x - o acento virou outra coisa", saiu, []byte{'T', 'i', 't', 0xE3})
	}
	if strings.ContainsRune(nome, '?') {
		t.Error("o nome guardado tem '?', que é o que sobra de um acento perdido")
	}
}
