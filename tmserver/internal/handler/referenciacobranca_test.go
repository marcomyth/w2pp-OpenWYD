package handler

import (
	"regexp"
	"testing"
)

// A REGRA DA PONTE, copiada dela e não parafraseada: 32 caracteres hexadecimais
// minúsculos, e nada além disso. É a mesma expressão que src/cobranca.js aplica e a
// mesma que o reconhecimento do aviso de pagamento procura na descrição.
//
// Escrita aqui como âncora ^...$ de propósito: sem as âncoras, "rmt-" + 32 hex
// PASSARIA, porque a expressão acharia os 32 hex no meio. Foi exatamente esse o
// defeito que chegou à produção, e um teste sem âncora teria passado junto com ele.
var regexDaPonte = regexp.MustCompile(`^[0-9a-f]{32}$`)

// A REFERÊNCIA DA COBRANÇA TEM DE CASAR COM A DA PONTE.
//
// Em 24/09/2026 ela saía como "rmt-" + 32 hex = 36 caracteres, e a ponte recusava
// TODA cobrança com http 400. O teste de venda real ficou parado em "gerando o
// código", repetindo a recusa a cada cinco segundos, e ninguém do lado do jogador
// tinha como saber por quê.
func TestAReferenciaDaCobrancaCasaComAPonte(t *testing.T) {
	vistas := map[string]bool{}
	for i := 0; i < 200; i++ {
		ref, err := referenciaDeCobranca()
		if err != nil {
			t.Fatalf("referenciaDeCobranca: %v", err)
		}
		if !regexDaPonte.MatchString(ref) {
			t.Fatalf("referencia %q nao casa com %v — a ponte recusaria com http 400", ref, regexDaPonte)
		}
		if len(ref) != 32 {
			t.Fatalf("referencia tem %d caracteres, a ponte exige 32", len(ref))
		}
		// E ELA PRECISA SER ÚNICA, porque é a âncora da idempotência: duas cobranças
		// com a mesma referência fariam a ponte tratar a segunda como repetição da
		// primeira, e um comprador pagaria pelo item de outro.
		if vistas[ref] {
			t.Fatalf("referencia repetida em %d sorteios: %q", i+1, ref)
		}
		vistas[ref] = true
	}
}

// E A ÂNCORA DO TESTE É O QUE O FAZ VALER. Sem ela, o defeito que estava em produção
// passaria: este caso prova que a expressão recusa o formato antigo.
func TestOFormatoAntigoSeriaRecusado(t *testing.T) {
	antigo := "rmt-0123456789abcdef0123456789abcdef"
	if regexDaPonte.MatchString(antigo) {
		t.Errorf("a expressao aceitou %q; ela precisa das ancoras para pegar o prefixo", antigo)
	}
	// E as outras formas que a ponte também recusa, para a expressão não afrouxar.
	for _, ruim := range []string{
		"0123456789ABCDEF0123456789ABCDEF",  // maiúsculas
		"0123456789abcdef0123456789abcde",   // 31
		"0123456789abcdef0123456789abcdef0", // 33
		" 0123456789abcdef0123456789abcdef", // espaço na ponta
		"0123456789abcdef0123456789abcdeg",  // 'g' não é hex
	} {
		if regexDaPonte.MatchString(ruim) {
			t.Errorf("a expressao aceitou %q", ruim)
		}
	}
}
