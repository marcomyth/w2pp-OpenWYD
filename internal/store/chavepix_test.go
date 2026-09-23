package store

import (
	"errors"
	"strings"
	"testing"
)

// A validação é de FORMA e não de existência: só o banco do jogador sabe se a
// chave existe. O que estes casos guardam é o erro de digitação não chegar até a
// processadora e virar, no dia do repasse, um pagamento recusado que ninguém
// entende.
func TestValidaChavePix(t *testing.T) {
	casos := []struct {
		nome  string
		chave string
		tipo  TipoChavePix
		quer  bool // true = deve passar
	}{
		{"CPF com onze dígitos", "12345678901", ChavePixCPF, true},
		{"CPF com pontuação", "123.456.789-01", ChavePixCPF, false},
		{"CPF curto", "1234567890", ChavePixCPF, false},
		{"CPF longo", "123456789012", ChavePixCPF, false},

		{"e-mail comum", "jogador@exemplo.com", ChavePixEmail, true},
		{"e-mail sem arroba", "jogador.exemplo.com", ChavePixEmail, false},
		{"e-mail sem domínio", "jogador@exemplo", ChavePixEmail, false},

		{"telefone com +55", "+5511987654321", ChavePixTelefone, true},
		{"telefone fixo com +55", "+551133334444", ChavePixTelefone, true},
		{"telefone sem +", "5511987654321", ChavePixTelefone, false},
		{"telefone curto", "+5511999", ChavePixTelefone, false},

		{"aleatória", "123e4567-e89b-12d3-a456-426614174000", ChavePixAleatoria, true},
		{"aleatória com letra fora do hexa", "123e4567-e89b-12d3-a456-42661417400z", ChavePixAleatoria, false},

		// O tipo errado para a chave certa é o caso que pega quem confia no
		// formulário: um CPF válido declarado como e-mail não é chave de e-mail.
		{"CPF declarado como e-mail", "12345678901", ChavePixEmail, false},
		{"e-mail declarado como CPF", "jogador@exemplo.com", ChavePixCPF, false},

		{"vazia", "", ChavePixCPF, false},
		{"só espaço", "   ", ChavePixCPF, false},
		{"com espaço no meio", "1234 5678901", ChavePixCPF, false},
		{"tipo zero", "12345678901", 0, false},
	}
	for _, c := range casos {
		err := validaChavePix(c.chave, c.tipo)
		if c.quer && err != nil {
			t.Errorf("%s: recusada, devia passar (%v)", c.nome, err)
		}
		if !c.quer && !errors.Is(err, ErrChavePixInvalida) {
			t.Errorf("%s: aceita, devia recusar", c.nome)
		}
	}
}

// A máscara existe para a pessoa reconhecer a própria chave SEM que a chave saia
// do servidor. Estes casos guardam as duas metades disso: que dá para
// reconhecer, e que o suficiente foi escondido.
func TestMascaraChavePix(t *testing.T) {
	casos := []struct {
		chave string
		tipo  TipoChavePix
		quer  string
	}{
		{"12345678901", ChavePixCPF, "***8901"},
		{"+5511987654321", ChavePixTelefone, "***4321"},
		{"123e4567-e89b-12d3-a456-426614174000", ChavePixAleatoria, "***4000"},
		{"jogador@exemplo.com", ChavePixEmail, "j***@exemplo.com"},
		{"", ChavePixCPF, ""},
		{"ab", ChavePixCPF, "***"}, // curta demais para mostrar quatro
	}
	for _, c := range casos {
		if got := MascaraChavePix(c.chave, c.tipo); got != c.quer {
			t.Errorf("MascaraChavePix(%q) = %q, quero %q", c.chave, got, c.quer)
		}
	}
}

// E o que mais importa: a máscara NUNCA devolve a chave inteira. É a propriedade,
// e não os exemplos, que protege o dado — um caso novo mal escrito passaria pelos
// exemplos de cima e seria pego aqui.
func TestMascaraNuncaDevolveAChaveInteira(t *testing.T) {
	for _, c := range []struct {
		chave string
		tipo  TipoChavePix
	}{
		{"12345678901", ChavePixCPF},
		{"+5511987654321", ChavePixTelefone},
		{"123e4567-e89b-12d3-a456-426614174000", ChavePixAleatoria},
		{"jogador@exemplo.com", ChavePixEmail},
		{"a@b.co", ChavePixEmail},
	} {
		m := MascaraChavePix(c.chave, c.tipo)
		if m == c.chave {
			t.Errorf("a máscara de %q devolveu a chave inteira", c.chave)
		}
		if !strings.Contains(m, "***") {
			t.Errorf("a máscara de %q (%q) não escondeu nada", c.chave, m)
		}
		// O e-mail guarda o domínio de propósito; o resto não pode guardar mais
		// do que os quatro últimos.
		if c.tipo != ChavePixEmail && len(m) > len("***")+4 {
			t.Errorf("a máscara de %q mostrou demais: %q", c.chave, m)
		}
	}
}
