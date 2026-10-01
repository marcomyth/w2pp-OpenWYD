package npcadmin

import (
	"context"
	"testing"

	"github.com/jeanluca/w2pp-openwyd/webserver/internal/painelator"
)

// TestOsDoisAtoresJuntosSaoRecusados: o segundo serviço com este teste, como a revisão
// pediu — a montaria já tem o dela.
//
// POR QUE EM MAIS DE UM: a recusa está escrita sete vezes, uma por serviço, porque cada
// um tem o seu autorizaEscrita. Sete cópias é sete chances de uma ficar para trás, e foi
// o que aconteceu: eu consertei seis e esqueci a montaria, e só o teste dela achou. Um
// segundo teste, noutro serviço, é o que transforma "achei por sorte" em "há rede".
//
// PREFERIR UM SERIA DESCARTAR O OUTRO EM SILÊNCIO, e a linha de auditoria sairia dizendo
// que uma pessoa fez o que duas informações reivindicam. Numa tabela cuja única razão de
// existir é dizer QUEM fez, "eu escolhi um dos dois" é a pior resposta possível.
func TestOsDoisAtoresJuntosSaoRecusados(t *testing.T) {
	t.Parallel()
	s := New(newFake())
	ctx := painelator.NoContexto(context.Background(),
		painelator.Ator{ID: 42, Login: "hanna", Papel: "admin"})

	// ESTE TESTE MEDE AUTORIZAÇÃO E NÃO A ESCRITA, e por isso a asserção é "não foi
	// recusado por permissão" e não "deu OK". O dublê não tem o NPC 1, então a escrita
	// legítima termina em NotFound — que é o resultado certo para um NPC que não existe,
	// e que prova o que interessa: a chamada PASSOU da permissão.
	//
	// Exigir OK aqui faria o teste depender do conteúdo do dublê, e ele começaria a
	// falhar no dia em que alguém mexesse nos dados de teste por outro motivo.
	if r, err := s.SetVisibility(context.Background(), 2, 1, true); r == Forbidden || err != nil {
		t.Fatalf("conta de jogo admin sozinha = (%v, %v), e ela nao devia ser recusada", r, err)
	}
	if r, err := s.SetVisibility(ctx, 0, 1, true); r == Forbidden || err != nil {
		t.Fatalf("usuario do painel sozinho = (%v, %v), e ele nao devia ser recusado", r, err)
	}

	// JUNTOS, NÃO: moderatorID preenchido E ator do painel no contexto.
	if r, err := s.SetVisibility(ctx, 2, 1, true); r != Forbidden || err != nil {
		t.Errorf("conta de jogo E usuario do painel juntos = (%v, %v), queria Forbidden", r, err)
	}
}
