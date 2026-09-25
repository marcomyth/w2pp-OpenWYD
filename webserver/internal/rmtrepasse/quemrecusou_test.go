package rmtrepasse

import "testing"

// O LOG TEM DE DIZER O NÚMERO, e não o endereço dele.
//
// Este teste nasce de um defeito MEDIDO em produção, na recusa da primeira venda real:
// o log saiu com "http_syncpay=0x3726df7a9b60". O campo é *int32, e o slog imprime o
// ponteiro quando recebe um. O valor estava certo no banco, porque o pgx desreferencia;
// era só o instrumento mentindo — e foi outra pessoa, lendo o meu log para entender por
// que o saque falhou, que perdeu tempo com isso.
func TestOLogDizONumeroENaoOEndereco(t *testing.T) {
	http := int32(403)
	if got := quemRecusou(&http); got != "403" {
		t.Errorf("quemRecusou(403) = %q, queria \"403\"", got)
	}
}

// E O NULO DIZ QUEM RECUSOU, que é a informação mais útil dos dois casos.
//
// Nulo quer dizer que a recusa veio da PRÓPRIA PONTE — teto diário ou trava do saque —
// e não da processadora. Um "<nil>" mandaria a pessoa procurar nada, e um "0" seria pior:
// leria como um HTTP 0, que não existe, e a busca começaria no lugar errado.
func TestNuloDizQueFoiAPonteENaoImprimeZero(t *testing.T) {
	got := quemRecusou(nil)
	if got == "0" || got == "<nil>" || got == "" {
		t.Fatalf("nulo virou %q, que manda procurar no lugar errado", got)
	}
	if got != "ponte (teto ou trava do saque)" {
		t.Errorf("quemRecusou(nil) = %q", got)
	}
}
