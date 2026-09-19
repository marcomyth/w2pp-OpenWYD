package clientitemhelp

import (
	"bytes"
	"testing"
)

// itemhelpDeExemplo é um arquivo no formato real: CRLF, Windows-1252, blocos em
// ordem crescente, espaços gravados como "_".
//
// Os acentos vão como escapes de UM byte — \xe7 é o "ç" e \xe3 o "ã" —, e não
// como literais acentuados, que o fonte Go grava em UTF-8. A diferença é o
// ponto do teste: é exatamente aí que o Get errava, lendo dois bytes onde o
// cliente escreveu um.
func itemhelpDeExemplo() []byte {
	return []byte("100\r\n" +
		"FFFFFFFF Uma_po\xe7\xe3o_comum.\r\n" +
		"200\r\n" +
		"FFFF00FF [Item_Premium]\r\n" +
		"FFFFFFFF Recupera_500_de_HP.\r\n" +
		"FFFF0000 N\xe3o_pode_ser_negociada.\r\n" +
		"400\r\n" +
		"FFFFFFFF Fim.\r\n")
}

func TestGetLeAsLinhasComEspacosEAcentos(t *testing.T) {
	linhas, err := Get(itemhelpDeExemplo(), 200)
	if err != nil {
		t.Fatal(err)
	}
	quer := []Linha{
		{Cor: Premium, Texto: "[Item Premium]"}, // o "_" do arquivo sempre volta como espaço
		{Cor: Branco, Texto: "Recupera 500 de HP."},
		{Cor: Vermelho, Texto: "Não pode ser negociada."},
	}
	if len(linhas) != len(quer) {
		t.Fatalf("leu %d linhas, quer %d: %+v", len(linhas), len(quer), linhas)
	}
	for i := range quer {
		if linhas[i] != quer[i] {
			t.Errorf("linha %d = %+v, quer %+v", i, linhas[i], quer[i])
		}
	}
}

func TestGetSemBlocoDevolveNil(t *testing.T) {
	linhas, err := Get(itemhelpDeExemplo(), 300)
	if err != nil {
		t.Fatal(err)
	}
	if linhas != nil {
		t.Fatalf("item sem descrição devolveu %+v, quer nil", linhas)
	}
}

// A ida e volta é o que protege o arquivo: Set grava o bloco INTEIRO, então
// devolver o que o Get leu tem de reproduzir os mesmos bytes. Foi este teste
// que pegou o Get lendo Latin-1 como se fosse UTF-8 — o "ç" voltava como "?" e
// o acento sumia da tela de todo mundo.
func TestGetSetIdaEVoltaNaoMudaNada(t *testing.T) {
	orig := itemhelpDeExemplo()
	for _, item := range []int{100, 200, 400} {
		linhas, err := Get(orig, item)
		if err != nil {
			t.Fatal(err)
		}
		novo, err := Set(orig, item, linhas)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(novo, orig) {
			t.Fatalf("item %d: a ida e volta mudou o arquivo\n--- antes ---\n%q\n--- depois ---\n%q",
				item, orig, novo)
		}
	}
}

// Acrescentar uma linha tem de preservar as que já existiam — é o caso de uso
// do lojapontoscliente, e é onde um Get errado apagaria a descrição do item.
func TestAcrescentarLinhaPreservaAsAnteriores(t *testing.T) {
	orig := itemhelpDeExemplo()
	linhas, err := Get(orig, 200)
	if err != nil {
		t.Fatal(err)
	}
	linhas = append(linhas, Linha{Cor: Vermelho, Texto: "Vendido por pontos de lojinha."})
	novo, err := Set(orig, 200, linhas)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(novo, []byte("Recupera_500_de_HP.")) {
		t.Error("a descrição anterior sumiu")
	}
	if !bytes.Contains(novo, []byte("FFFF0000 Vendido_por_pontos_de_lojinha.")) {
		t.Error("a linha nova não foi gravada, ou saiu sem os sublinhados")
	}
	// Os vizinhos não podem ter sido tocados, com acento e tudo.
	if !bytes.Contains(novo, []byte("100\r\nFFFFFFFF Uma_po\xe7\xe3o_comum.\r\n")) {
		t.Error("o bloco do item 100 mudou")
	}
	if !bytes.Contains(novo, []byte("400\r\nFFFFFFFF Fim.\r\n")) {
		t.Error("o bloco do item 400 mudou")
	}
}

// O itemhelp.dat do cliente termina SEM quebra de linha. Um item de índice
// maior que todos entra no fim, e sem este cuidado o bloco colava no texto do
// último item — "..._Xp/Ouro_[Eterno]9999" —, estragando as duas descrições.
// Achado rodando o lojapontoscliente contra o cliente de verdade.
func TestSetNoFimDeArquivoSemQuebraNaoColaNoUltimoItem(t *testing.T) {
	semQuebra := []byte("100\r\nFFFFFFFF Ultimo_item_do_arquivo.")
	novo, err := Set(semQuebra, 9999, []Linha{{Cor: Vermelho, Texto: "Vendido por pontos de lojinha."}})
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(novo, []byte("arquivo.9999")) {
		t.Fatalf("o bloco colou na última linha: %q", novo)
	}
	if !bytes.Contains(novo, []byte("Ultimo_item_do_arquivo.\r\n9999\r\n")) {
		t.Fatalf("o bloco novo não entrou em linha própria: %q", novo)
	}
}

// E o contrário: um arquivo que JÁ termina com quebra não pode ganhar uma linha
// em branco a cada item acrescentado.
func TestSetNoFimDeArquivoComQuebraNaoDuplicaALinha(t *testing.T) {
	comQuebra := []byte("100\r\nFFFFFFFF Ultimo.\r\n")
	novo, err := Set(comQuebra, 9999, []Linha{{Cor: Vermelho, Texto: "Aviso."}})
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(novo, []byte("\r\n\r\n")) {
		t.Fatalf("sobrou linha em branco: %q", novo)
	}
}
