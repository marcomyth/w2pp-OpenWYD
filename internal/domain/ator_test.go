package domain

import (
	"errors"
	"testing"
)

// O QUE ESTE TESTE GUARDA: que "escrita sem autor" não seja representável por
// acidente, e que zero nunca vire um id gravável.
func TestAtorConferir(t *testing.T) {
	t.Parallel()
	casos := []struct {
		nome string
		ator Ator
		quer error
	}{
		{"conta de jogo", AtorDaConta(7), nil},
		{"usuario do painel", AtorDoPainel(3), nil},
		{"nenhum dos dois", Ator{}, ErrSemAtor},
		{"os dois juntos", Ator{ContaID: 7, PainelUsuarioID: 3}, ErrAtorDuplo},
	}
	for _, c := range casos {
		if err := c.ator.Conferir(); !errors.Is(err, c.quer) {
			t.Errorf("%s: Conferir() = %v, queria %v", c.nome, err, c.quer)
		}
	}
}

// TestAtorParaSQLNuncaManda zero: o defeito que este tipo existe para impedir era
// gravar a conta 0 como autor. Aqui a prova é que o valor que vai ao banco é NULO, e
// não zero — são coisas diferentes, e só NULO satisfaz a trava da migração 0181.
func TestAtorParaSQLNuncaMandaZero(t *testing.T) {
	t.Parallel()

	conta, painel := AtorDaConta(7).ParaSQL()
	if conta != int64(7) {
		t.Errorf("conta = %v, queria 7", conta)
	}
	if painel != nil {
		t.Errorf("painel = %v, queria nulo", painel)
	}

	conta, painel = AtorDoPainel(3).ParaSQL()
	if conta != nil {
		t.Errorf("conta = %v, queria nulo -- zero aqui gravaria a conta 0 como autor", conta)
	}
	if painel != int64(3) {
		t.Errorf("painel = %v, queria 3", painel)
	}

	// O ator vazio devolve os dois nulos, e é a trava do banco que recusa a linha.
	// Conferir() é quem deve pegar isto antes; este caso existe para garantir que,
	// se alguém esquecer o Conferir, o que chega ao banco é NULO e a CHECK recusa —
	// e não zero, que passaria pela CHECK como conta válida.
	conta, painel = Ator{}.ParaSQL()
	if conta != nil || painel != nil {
		t.Errorf("ator vazio = (%v, %v), queria os dois nulos", conta, painel)
	}
}

func TestAtorEhDoPainel(t *testing.T) {
	t.Parallel()
	if AtorDoPainel(3).EhDoPainel() != true {
		t.Error("usuario do painel nao se reconheceu")
	}
	if AtorDaConta(7).EhDoPainel() != false {
		t.Error("conta de jogo se disse usuario do painel")
	}
}
