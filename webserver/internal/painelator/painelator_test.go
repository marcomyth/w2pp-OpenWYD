package painelator

import (
	"context"
	"testing"
)

// TestSemAtorNoContextoNaoEDoPainel.
//
// O segundo valor do AutorizaPeloPainel é o que separa "recusado" de "não é do painel", e
// confundir os dois quebraria o login por CONTA DE JOGO, que continua existindo: toda
// chamada de conta de jogo seria tratada como recusa.
func TestSemAtorNoContextoNaoEDoPainel(t *testing.T) {
	pode, doPainel := AutorizaPeloPainel(context.Background())
	if doPainel {
		t.Error("um contexto vazio se disse do painel")
	}
	if pode {
		t.Error("um contexto vazio autorizou")
	}
}

// TestOsDoisPapeisDoPainelAdministram, e nada além deles.
//
// Os papéis vêm do CHECK da tabela (migração 0130), que só aceita 'admin' e 'moderator'.
// O caso vazio é o que importa: um ator montado errado, sem papel, não pode virar
// autorização por omissão.
func TestOsDoisPapeisDoPainelAdministram(t *testing.T) {
	casos := map[string]bool{
		"admin": true, "moderator": true,
		"": false, "player": false, "Admin": false, "administrador": false,
	}
	for papel, quer := range casos {
		ctx := NoContexto(context.Background(), Ator{ID: 7, Login: "x", Papel: papel})
		pode, doPainel := AutorizaPeloPainel(ctx)
		if !doPainel {
			t.Errorf("papel %q: nao se reconheceu como do painel", papel)
		}
		if pode != quer {
			t.Errorf("papel %q: autorizou = %v, queria %v", papel, pode, quer)
		}
	}
}

// TestSemPoolNaoConfereNinguem.
//
// Falhar FECHADO quando não há como conferir. Um leitor sem banco que devolvesse "pode"
// aceitaria um "sou admin" escrito pelo próprio chamador — e o cabeçalho é texto que
// qualquer um escreve.
func TestSemPoolNaoConfereNinguem(t *testing.T) {
	var l *Leitor
	if _, err := l.Confere(context.Background(), 1); err == nil {
		t.Error("um leitor nulo conferiu alguem")
	}
	if _, err := Novo(nil).Confere(context.Background(), 1); err == nil {
		t.Error("um leitor sem pool conferiu alguem")
	}
	// Id inválido também não passa, e nem chega ao banco.
	if _, err := Novo(nil).Confere(context.Background(), 0); err == nil {
		t.Error("o id zero passou")
	}
}
