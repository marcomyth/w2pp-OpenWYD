package store

import (
	"errors"
	"testing"
)

// O ALGORITMO TEM DE ACEITAR CPF VÁLIDO, e este é o medo que o comentário da chave
// Pix registrou: uma implementação errada recusaria gente de verdade.
//
// Os números aqui são CPFs válidos conhecidos, de teste, e o que eles provam é que a
// conta fecha nos dois dígitos — inclusive quando um deles é zero, que é o caso que
// mais quebra implementação (resto 10 vira dígito 0).
func TestDocumentoAceitaValidos(t *testing.T) {
	validos := []struct{ nome, entrada, quer string }{
		{"so digitos", "11144477735", "11144477735"},
		{"formatado como a pessoa copia", "111.444.777-35", "11144477735"},
		{"com espaco em volta", "  111.444.777-35 ", "11144477735"},
		// PRIMEIRO dígito verificador ZERO. É o caso que mais quebra implementação,
		// porque o resto da divisão dá 10 e tem de virar 0 em vez de virar o
		// caractere ':' — que é o que sai de '0'+10 se ninguém tratar.
		{"primeiro digito verificador zero", "12345678909", "12345678909"},
		{"outro valido conhecido", "52998224725", "52998224725"},
	}
	for _, c := range validos {
		got, err := NormalizaDocumento(c.entrada)
		if err != nil {
			t.Errorf("%s (%q): recusou um CPF valido: %v", c.nome, c.entrada, err)
			continue
		}
		if got != c.quer {
			t.Errorf("%s: normalizou para %q, quero %q", c.nome, got, c.quer)
		}
	}
}

// E TEM DE RECUSAR O QUE ESTÁ ERRADO, principalmente o que está QUASE certo.
//
// O caso dos onze dígitos iguais é o que separa uma implementação completa de uma
// pela metade: 111.111.111-11 FECHA os dois dígitos verificadores. É também o que
// alguém digita para pular o campo.
//
// Os dois trocados de lugar são o erro de digitação mais comum, e são justamente o
// que o dígito verificador existe para pegar — sem ele, esse CPF seria aceito, ficaria
// guardado, e só quebraria no dia do repasse com uma mensagem que não aponta para o
// campo errado.
func TestDocumentoRecusaInvalidos(t *testing.T) {
	invalidos := []struct{ nome, entrada string }{
		{"vazio", ""},
		{"curto", "1114447773"},
		{"comprido", "111444777351"},
		{"letras", "1114447773a"},
		{"onze iguais, que fecham a conta", "11111111111"},
		{"zeros", "00000000000"},
		{"ultimo digito trocado", "11144477734"},
		{"penultimo digito trocado", "11144477745"},
		{"dois algarismos transpostos", "11414477735"},
	}
	for _, c := range invalidos {
		if got, err := NormalizaDocumento(c.entrada); !errors.Is(err, ErrDocumentoInvalido) {
			t.Errorf("%s (%q): aceitou, devolvendo %q (err=%v)", c.nome, c.entrada, got, err)
		}
	}
}

// A MÁSCARA MOSTRA O FIM, e nunca o meio.
//
// A tela do site escreve "CPF terminado em NN" pegando os dois últimos dígitos do que
// vier. Se a máscara deixasse o meio aparecer, a pessoa conferiria contra dígitos do
// MEIO achando que eram o final, e não teria como perceber.
func TestMascaraDocumentoMostraSoOFim(t *testing.T) {
	if got := MascaraDocumento("11144477735"); got != "***.***.***-35" {
		t.Errorf("mascara = %q, quero ***.***.***-35", got)
	}
	// Os dois últimos dígitos são os do FINAL do documento, e não outros quaisquer.
	if got := MascaraDocumento("12345678909"); got != "***.***.***-09" {
		t.Errorf("mascara = %q; pegou digitos que nao sao o final", got)
	}
	// E nada além deles aparece: nenhum dígito do meio pode vazar na máscara.
	for _, doc := range []string{"11144477735", "12345678909", "52998224725"} {
		m := MascaraDocumento(doc)
		for i := 0; i < 9; i++ {
			if len(m) > 0 && containsByte(m, doc[i]) && doc[i] != doc[9] && doc[i] != doc[10] {
				t.Errorf("a mascara %q de %q mostra o digito %q, que e do meio",
					m, doc, string(doc[i]))
				break
			}
		}
	}
	// Sem documento, sem máscara: a tela mostra "não cadastrado" em vez de um desenho
	// de máscara sobre coisa nenhuma. É o caso de todo vendedor anterior à coluna.
	if got := MascaraDocumento(""); got != "" {
		t.Errorf("mascara de vazio = %q, quero vazio", got)
	}
	if got := MascaraDocumento("123"); got != "" {
		t.Errorf("mascara de lixo = %q, quero vazio", got)
	}
}

func containsByte(s string, b byte) bool {
	for i := 0; i < len(s); i++ {
		if s[i] == b {
			return true
		}
	}
	return false
}
