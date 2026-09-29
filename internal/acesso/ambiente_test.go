package acesso

import "testing"

// AS DUAS FUNÇÕES QUE LEEM O AMBIENTE NÃO TINHAM TESTE, e são elas que decidem o
// comportamento de verdade.
//
// O pacote testa bem o Ler e o LerRMT, que recebem o valor pronto. Mas quem o servidor
// chama no boot é o Restrito() e o RMT(), que leem a variável do ambiente — e é aí que
// mora a pergunta que importa: "o que acontece num ambiente onde ninguém configurou
// nada?". Essa pergunta não estava guardada por teste nenhum.
//
// NÃO É HIPÓTESE: foi o que aconteceu no LOTM em 28/09/2026. O ambiente subiu sem as
// duas variáveis, e ficou destrancado e com o mercado ABERTO enquanto todo mundo achava
// que estava trancado. O comentário do LerRMT avisa disso com todas as letras — "quem
// criar um ambiente de teste precisa pôr W2PP_RMT=fechado nele" — e um aviso em
// comentário é o que se esquece. Este teste é o mesmo aviso, num lugar que reclama.

// TestSemVariavelNenhumaOServidorNasceDestrancadoEComMercadoAberto fixa o padrão de um
// ambiente novo.
//
// ELE NÃO DIZ QUE O PADRÃO ESTÁ CERTO, e essa é a diferença: o servidor aberto é o
// padrão desejado para produção, e o mercado aberto foi decisão da Hanna em 25/09. O que
// o teste guarda é que esse par é o que um ambiente NOVO recebe — para quem criar o
// próximo ver o número em vez de descobrir com jogador dentro.
func TestSemVariavelNenhumaOServidorNasceDestrancadoEComMercadoAberto(t *testing.T) {
	// t.Setenv já limpa no fim, e ele também impede t.Parallel — que é o certo aqui:
	// duas goroutinas mexendo na mesma variável de ambiente é corrida de dados.
	t.Setenv(Variavel, "")
	t.Setenv(VariavelRMT, "")

	restrito, err := Restrito()
	if err != nil {
		t.Fatalf("Restrito() com a variável vazia devia ser silêncio, e deu: %v", err)
	}
	if restrito {
		t.Error("sem a variável, o servidor nasceu TRANCADO; o padrão é aberto")
	}

	rmt, err := RMT()
	if err != nil {
		t.Fatalf("RMT() com a variável vazia devia ser silêncio, e deu: %v", err)
	}
	if rmt != RMTAberto {
		t.Errorf("sem a variável, o mercado nasceu %v; desde 25/09/2026 o padrão é ABERTO, "+
			"e é por isso que um ambiente de teste precisa pôr W2PP_RMT=fechado nele", rmt)
	}
}

// TestOAmbienteTrancadoDoLOTM é o par de valores que tranca um ambiente de teste.
//
// ESTES SÃO OS DOIS VALORES QUE O LOTM USA, e o teste existe para que uma mudança na
// lista de palavras aceitas não os derrube em silêncio. Se alguém tirar "staff" de
// ligam, ou "fechado" de palavrasRMT, o LOTM não sobe — e é melhor descobrir aqui.
func TestOAmbienteTrancadoDoLOTM(t *testing.T) {
	t.Setenv(Variavel, "staff")
	t.Setenv(VariavelRMT, "fechado")

	restrito, err := Restrito()
	if err != nil || !restrito {
		t.Errorf("Restrito() = %v, %v; com staff tinha de trancar", restrito, err)
	}
	rmt, err := RMT()
	if err != nil || rmt != RMTFechado {
		t.Errorf("RMT() = %v, %v; com fechado tinha de fechar", rmt, err)
	}
}

// TestOValorErradoNoAmbienteNaoViraDesligado fecha o caminho do ambiente.
//
// O Ler já garante isso para um valor solto, e este garante para a variável DE VERDADE:
// um valor que o pacote não entende vira ERRO, e o serviço não sobe. Falhar aberto é o
// pior jeito de falhar aqui — um servidor que não sobe alguém conserta em dois minutos,
// um servidor que sobe destrancado achando que está trancado ninguém procura.
func TestOValorErradoNoAmbienteNaoViraDesligado(t *testing.T) {
	t.Setenv(Variavel, "sim-pode-entrar")
	if _, err := Restrito(); err == nil {
		t.Error("um valor desconhecido no ambiente passou calado, e tinha de ser erro")
	}
	t.Setenv(Variavel, "")

	t.Setenv(VariavelRMT, "closed")
	if _, err := RMT(); err == nil {
		t.Error("\"closed\" passou calado; quem escreve isso acha que fechou o mercado")
	}
}
