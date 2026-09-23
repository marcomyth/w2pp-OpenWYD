package ponte

import (
	"errors"
	"strings"
	"testing"
)

var (
	segredo = []byte("um-segredo-de-teste-que-nao-e-o-de-producao")
	corpo   = []byte(`{"referencia_externa":"abc-123","pago_em":"2026-09-23T02:00:00Z"}`)
)

// A assinatura certa passa. É o caso fácil e ele sozinho não prova nada — está
// aqui para os outros terem contra o que ser comparados.
func TestAssinaturaCertaPassa(t *testing.T) {
	if err := Confere(segredo, corpo, Assina(segredo, corpo)); err != nil {
		t.Errorf("a assinatura legítima foi recusada: %v", err)
	}
}

// ESTE É O CASO QUE SEPARA CONFERIR DE FINGIR QUE CONFERE.
//
// Uma assinatura VÁLIDA, feita com o segredo certo, mas de OUTRO corpo. Ela
// passa em qualquer conferência que só olhe o formato, o tamanho, ou que
// verifique "veio assinado" sem amarrar a assinatura ao conteúdo. É exatamente o
// que um atacante tem na mão quando captura uma requisição legítima e troca o
// que está dentro: a referência externa de outra venda, um valor maior, outra
// conta de destino.
//
// Se este teste passar e os outros dois também, a conferência é real. Se só os
// outros dois passarem, ela é decorativa.
func TestAssinaturaValidaDeOutroCorpoNaoPassa(t *testing.T) {
	outroCorpo := []byte(`{"referencia_externa":"abc-123","pago_em":"2026-09-23T02:00:00Z","valor":999999}`)
	assinaturaLegitimaDoOutro := Assina(segredo, outroCorpo)

	err := Confere(segredo, corpo, assinaturaLegitimaDoOutro)

	if !errors.Is(err, ErrAssinaturaInvalida) {
		t.Errorf("uma assinatura legítima de OUTRO corpo foi aceita para este: err = %v", err)
	}
}

// E a volta: o mesmo corpo com uma assinatura feita com outro segredo. É o caso
// de quem descobriu o formato mas não a chave.
func TestAssinaturaDeOutroSegredoNaoPassa(t *testing.T) {
	err := Confere(segredo, corpo, Assina([]byte("segredo-errado"), corpo))

	if !errors.Is(err, ErrAssinaturaInvalida) {
		t.Errorf("assinatura feita com outro segredo foi aceita: err = %v", err)
	}
}

// A assinatura CERTA, cortada. É o único caso que pega uma conferência que
// compara só um PEDAÇO do resumo.
//
// Este teste nasceu de uma sabotagem que passou. Troquei a comparação por
// `recebida[:4] == esperada[:4]` e os seis testes que existiam continuaram
// verdes — e continuariam, porque comparar quatro bytes é funcionalmente correto
// para qualquer entrada realista: a chance de dois corpos diferentes baterem nos
// primeiros quatro bytes é de uma em quatro bilhões. A fraqueza não é de
// comportamento, é de SEGURANÇA — quem pode tentar muitas vezes só precisa
// acertar o pedaço conferido.
//
// Propriedade assim não se prova com exemplo. O que se prova é esta consequência
// dela: uma assinatura que bate no começo e acaba antes tem de ser recusada. Ela
// é barata de construir — é a assinatura legítima, cortada — e uma conferência
// por prefixo a aceita.
func TestAssinaturaCertaMasCortadaNaoPassa(t *testing.T) {
	completa := Assina(segredo, corpo)

	// 8 dígitos hexadecimais = os 4 primeiros bytes do resumo.
	for _, corte := range []int{8, 16, 32, len(completa) - 2} {
		if err := Confere(segredo, corpo, completa[:corte]); !errors.Is(err, ErrAssinaturaInvalida) {
			t.Errorf("a assinatura certa cortada em %d dígitos foi aceita: err = %v", corte, err)
		}
	}
}

func TestAssinaturaMalFormadaNaoPassa(t *testing.T) {
	for nome, assinatura := range map[string]string{
		"vazia":             "",
		"não é hexadecimal": "isto-nao-e-hex-nem-de-longe",
		"curta demais":      "ab12",
		"longa demais":      strings.Repeat("ab", 64),
	} {
		if err := Confere(segredo, corpo, assinatura); !errors.Is(err, ErrAssinaturaInvalida) {
			t.Errorf("assinatura %s foi aceita: err = %v", nome, err)
		}
	}
}

// SEGREDO VAZIO RECUSA, e recusa com erro PRÓPRIO.
//
// Sem isto, um serviço que subiu sem a variável de ambiente conferiria contra a
// chave vazia — que é uma chave que todo mundo conhece — e aceitaria qualquer
// requisição de quem soubesse disso. Falhar alto numa implantação incompleta é
// melhor do que abrir a porta calado.
//
// O erro é separado do de assinatura inválida porque as duas coisas pedem gente
// diferente: uma é tentativa de fraude, a outra é configuração faltando.
func TestSemSegredoRecusa(t *testing.T) {
	// A assinatura é a correta PARA A CHAVE VAZIA: é o que alguém mandaria
	// sabendo que o serviço subiu sem segredo. Tem de ser recusada mesmo assim.
	err := Confere(nil, corpo, Assina(nil, corpo))

	if !errors.Is(err, ErrSemSegredo) {
		t.Errorf("com segredo vazio, err = %v; queria ErrSemSegredo", err)
	}
}

// Um byte a menos no corpo já muda tudo. Prova que a assinatura cobre o conteúdo
// INTEIRO, e não um pedaço dele — um prefixo, um campo, um resumo.
func TestUmByteDeDiferencaJaRecusa(t *testing.T) {
	assinatura := Assina(segredo, corpo)

	for i := range corpo {
		adulterado := make([]byte, len(corpo))
		copy(adulterado, corpo)
		adulterado[i] ^= 0x01 // vira um bit, o menor estrago possível

		if err := Confere(segredo, adulterado, assinatura); !errors.Is(err, ErrAssinaturaInvalida) {
			t.Fatalf("virar um bit na posição %d passou pela conferência", i)
		}
	}
}
