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

// O TOKEN CURTO É DENUNCIADO, porque é justamente com ele que a impressão no log
// deixa de ser inofensiva: oito hex viram um jeito de conferir palpites, e um segredo
// curto tem lista de candidatos.
func TestTokenFraco(t *testing.T) {
	if !TokenFraco("curtinho") {
		t.Error("oito caracteres passaram como token forte")
	}
	if !TokenFraco(strings.Repeat("a", TamanhoMinimoDoToken-1)) {
		t.Error("um byte abaixo do minimo passou")
	}
	if TokenFraco(strings.Repeat("a", TamanhoMinimoDoToken)) {
		t.Error("o minimo foi reprovado")
	}
	// O vazio não é "fraco": ele é AUSENTE, e quem trata disso é outro caminho — os
	// serviços já recusam ligar o link sem token.
	if TokenFraco("") {
		t.Error("o vazio foi chamado de fraco em vez de ausente")
	}
}

// O CASO REAL DE 24/09/2026: a variável do token apontava para a do ENDEREÇO, e o
// webserver subiu dizendo que o link estava ligado. Este é o teste que teria evitado
// a tarde inteira.
func TestOEnderecoNoLugarDoTokenEhPego(t *testing.T) {
	const endereco = "tmserver.railway.internal:7700"
	if !TokenComCaraDeEndereco(endereco, endereco) {
		t.Error("o proprio endereco passou como token")
	}
	// E pega mesmo sem ter com o que comparar: a porta entrega o engano sozinha.
	if !TokenComCaraDeEndereco(endereco, "") {
		t.Error("o endereco passou quando nao havia endereco para comparar")
	}
	// O espaço nas pontas não salva o engano de ser engano.
	if !TokenComCaraDeEndereco("  "+endereco+"\n", endereco) {
		t.Error("o endereco com espaco nas pontas passou")
	}
}

// UM TOKEN DE VERDADE PASSA. É a metade que costuma faltar: sem ela, uma regra que
// recusa tudo tambem passaria nos testes de recusa.
func TestOTokenDeVerdadePassa(t *testing.T) {
	bons := []string{
		"7f3a9c1e5b8d2406af71c3e9d05b8264", // hexadecimal, como se gera
		"Zm9vYmFyYmF6cXV4MTIzNDU2Nzg5MGFi", // base64
		strings.Repeat("a", 40),
	}
	for _, v := range bons {
		if TokenComCaraDeEndereco(v, "tmserver.railway.internal:7700") {
			t.Errorf("recusou um token legitimo: %q", v)
		}
	}
}

// O VAZIO NÃO É "CARA DE ENDEREÇO": ele é AUSENTE, e quem trata disso é o caminho de
// sempre — os serviços já recusam ligar o link sem token. Dizer as duas coisas com a
// mesma mensagem esconderia qual das duas aconteceu.
func TestOVazioNaoTemCaraDeEndereco(t *testing.T) {
	if TokenComCaraDeEndereco("", "tmserver.railway.internal:7700") {
		t.Error("o vazio foi chamado de endereco")
	}
	if TokenComCaraDeEndereco("   ", "") {
		t.Error("so espaco foi chamado de endereco")
	}
}
