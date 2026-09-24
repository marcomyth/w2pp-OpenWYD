package store

import (
	"regexp"
	"testing"
)

var trintaEDoisHex = regexp.MustCompile(`^[0-9a-f]{32}$`)

// A REFERÊNCIA TEM DE CABER NO CONTRATO DA PONTE, que valida 32 hex minúsculos e recusa
// o resto com 400.
//
// O formato que o desenho pedia — "rep-<id>-<tentativa>" — não passaria. Então o número
// da tentativa vai dentro de um hash, e o que sai é hex.
func TestReferenciaDaTentativaTemAFormaQueAPonteAceita(t *testing.T) {
	for _, c := range []struct {
		id int64
		n  int
	}{
		{1, 1}, {1, 2}, {999999, 7}, {0, 1},
	} {
		ref := ReferenciaDaTentativa(c.id, c.n)
		if !trintaEDoisHex.MatchString(ref) {
			t.Errorf("repasse %d tentativa %d gerou %q, que a ponte recusa com 400",
				c.id, c.n, ref)
		}
	}
}

// DETERMINÍSTICA: a mesma tentativa dá sempre a mesma referência.
//
// É o que faz a idempotência da ponte funcionar quando a nossa chamada se perde: repetir
// a MESMA tentativa manda a MESMA referência, e a ponte devolve o resultado da primeira
// em vez de pagar de novo. Uma referência aleatória a cada envio transformaria cada
// tropeço de rede num pagamento a mais.
func TestReferenciaDaTentativaEDeterministica(t *testing.T) {
	a := ReferenciaDaTentativa(42, 3)
	b := ReferenciaDaTentativa(42, 3)
	if a != b {
		t.Errorf("a mesma tentativa deu %q e %q; um tropeco de rede viraria pagamento duplo", a, b)
	}
}

// E DIFERENTE ENTRE TENTATIVAS, que é a outra metade e a razão de ela existir.
//
// A ponte trava a referência de um incerto PARA SEMPRE. Sem referência nova, uma dívida
// que caiu em incerto nunca mais poderia ser paga — o reenvio devolveria o resultado
// velho, que é justamente o que ninguém sabe qual foi.
func TestReferenciaMudaEntreTentativasEEntreRepasses(t *testing.T) {
	if ReferenciaDaTentativa(42, 1) == ReferenciaDaTentativa(42, 2) {
		t.Error("duas tentativas da mesma divida dao a mesma referencia; a segunda nunca pagaria")
	}
	if ReferenciaDaTentativa(42, 1) == ReferenciaDaTentativa(43, 1) {
		t.Error("duas dividas diferentes dao a mesma referencia")
	}

	// E não colide por troca de posição: o repasse 12 tentativa 3 não é o mesmo que o
	// repasse 123 tentativa vazia, nem que o 1 tentativa 23. O separador cuida disso, e
	// sem ele um id concatenado viraria outro.
	vistas := map[string]string{}
	for _, c := range []struct {
		id int64
		n  int
	}{{12, 3}, {123, 1}, {1, 23}, {1, 233}} {
		ref := ReferenciaDaTentativa(c.id, c.n)
		if antes, repetiu := vistas[ref]; repetiu {
			t.Errorf("repasse %d tentativa %d colidiu com %s", c.id, c.n, antes)
		}
		vistas[ref] = "id " + string(rune('0'+c.n))
	}
}
