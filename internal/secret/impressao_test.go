package secret

import (
	"strings"
	"testing"
)

// A IMPRESSÃO NÃO CONTÉM O SEGREDO. É a regra inteira do arquivo.
func TestAImpressaoNaoVazaOSegredo(t *testing.T) {
	const token = "um-token-bem-secreto-de-verdade"
	got := Impressao(token)
	if strings.Contains(got, token) {
		t.Fatalf("a impressao contem o segredo: %q", got)
	}
	// Nem um pedaço dele: oito caracteres seguidos do original já seriam demais.
	for i := 0; i+8 <= len(token); i++ {
		if strings.Contains(got, token[i:i+8]) {
			t.Fatalf("a impressao contem %q, um pedaco do segredo: %q", token[i:i+8], got)
		}
	}
}

// VALORES IGUAIS DÃO IMPRESSÕES IGUAIS, e diferentes dão diferentes. Sem isso ela
// não serve para comparar dois serviços, que é a única coisa que ela faz.
func TestImpressoesIguaisEDiferentes(t *testing.T) {
	// Duas chamadas com o MESMO conteúdo, por variáveis diferentes: escrito assim
	// porque o verificador reclama de comparar a mesma expressão consigo mesma — e
	// aqui o ponto é justamente que duas chamadas separadas coincidam.
	um, outro := "abc", "ab"+"c"
	if Impressao(um) != Impressao(outro) {
		t.Error("o mesmo valor deu impressoes diferentes")
	}
	if Impressao("abc") == Impressao("abd") {
		t.Error("valores diferentes deram a mesma impressao")
	}
	// Mesmo tamanho e conteúdo diferente tem de separar: o tamanho sozinho não
	// bastaria para nada.
	if Impressao("aaaa") == Impressao("bbbb") {
		t.Error("o mesmo tamanho com conteudo diferente deu a mesma impressao")
	}
}

// A REFERÊNCIA NÃO RESOLVIDA É DENUNCIADA, porque é o erro mais comum de todos e o
// que mais custa a achar: o valor parece existir e é o texto de uma variável.
func TestAReferenciaNaoResolvidaEhDenunciada(t *testing.T) {
	for _, v := range []string{"${{paineladm.W2PP_CONTROL_TOKEN}}", "  ${{a.b}}  "} {
		got := Impressao(v)
		if !strings.Contains(got, "REFERENCIA NAO RESOLVIDA") {
			t.Errorf("Impressao(%q) = %q; nao denunciou", v, got)
		}
	}
}

// O ESPAÇO NAS PONTAS É DITO, porque ele muda o byte e não muda a aparência: um
// token colado com um "enter" no fim compara diferente e parece igual na tela.
func TestOEspacoNasPontasEhDito(t *testing.T) {
	if !strings.Contains(Impressao("abc\n"), "espaco") {
		t.Error("nao avisou da quebra de linha")
	}
	if strings.Contains(Impressao("abc"), "espaco") {
		t.Error("avisou de espaco onde nao ha")
	}
}

// E o vazio é dito como vazio, e não como uma impressão de nada.
func TestOVazio(t *testing.T) {
	if Impressao("") != "vazio" {
		t.Errorf("Impressao(\"\") = %q", Impressao(""))
	}
}
