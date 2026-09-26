package acesso

import "testing"

// VAZIO É ABERTO DESDE 25/09/2026, e este teste MUDOU DE LADO por decisão da Hanna.
//
// ELE DIZIA O CONTRÁRIO, e o que ele defendia continua verdade: um padrão fechado faz a
// trava valer sem ninguém configurar nada, e subir a versão já tranca. Trocar isso é
// perder essa garantia, e está sendo feito de olhos abertos.
//
// O QUE PAGOU A TROCA: a abertura do mercado tinha de caber no MESMO reinício da
// atualização que traz a taxa, o mínimo e o teto. Criar a variável na Railway dispara um
// deploy à parte, e seriam dois reinícios — dois intervalos derrubando quem está jogando,
// na noite de estreia.
//
// O QUE SE PERDE, e alguém vai esbarrar nisso: um ambiente novo que suba esta versão sem
// configurar nada nasce com o mercado ABERTO. Quem criar um ambiente de teste precisa pôr
// W2PP_RMT=fechado nele. O teste continua existindo, e não foi apagado, justamente para
// essa mudança nunca voltar a acontecer calada.
func TestVazioEhAberto(t *testing.T) {
	for _, v := range []string{"", "   ", "\t"} {
		got, err := LerRMT(v)
		if err != nil {
			t.Fatalf("LerRMT(%q) deu erro: %v", v, err)
		}
		if got != RMTAberto {
			t.Errorf("LerRMT(%q) = %v, queria aberto", v, got)
		}
	}
}

// E A PALAVRA "fechado" GANHA DO PADRÃO, que é o que faz o fechamento de emergência
// funcionar.
//
// O plano de emergência é criar W2PP_RMT=fechado na Railway. Se o padrão aberto
// atropelasse a palavra escrita, não haveria como fechar sem soltar versão — e é
// exatamente na emergência que não dá tempo de soltar versão.
func TestAPalavraFechadoGanhaDoPadrao(t *testing.T) {
	got, err := LerRMT("fechado")
	if err != nil {
		t.Fatal(err)
	}
	if got != RMTFechado {
		t.Fatalf("LerRMT(\"fechado\") = %v; sem isto nao ha fechamento de emergencia", got)
	}
}

// O ZERO DO TIPO CONTINUA FECHADO, e agora ele é a ÚNICA garantia que sobrou.
//
// Não é a mesma pergunta do teste de cima, e a diferença ficou importante: a STRING vazia
// virou aberto, mas o VALOR ZERO do tipo não. Um Config montado sem o campo — num teste,
// numa montagem nova amanhã — continua travando em vez de abrir.
//
// Se alguém reordenar as constantes e o zero virar "aberto", este teste é o que pega, e
// depois da inversão de hoje ele é o que impede o mercado de abrir por esquecimento de
// código, em vez de por decisão.
func TestOZeroDoTipoEhFechado(t *testing.T) {
	var naoConfigurado EstadoRMT
	if naoConfigurado != RMTFechado {
		t.Fatalf("o zero de EstadoRMT = %v; tem de ser fechado, senao Config sem o campo abre o mercado", naoConfigurado)
	}
}

// AS TRÊS PALAVRAS COMBINADAS, e sem ligar para caixa nem espaço.
func TestAsTresPalavras(t *testing.T) {
	casos := map[string]EstadoRMT{
		"fechado": RMTFechado, "FECHADO": RMTFechado, " fechado ": RMTFechado,
		"staff": RMTStaff, "Staff": RMTStaff,
		"aberto": RMTAberto, "ABERTO": RMTAberto,
	}
	for v, quer := range casos {
		got, err := LerRMT(v)
		if err != nil {
			t.Errorf("LerRMT(%q) deu erro: %v", v, err)
			continue
		}
		if got != quer {
			t.Errorf("LerRMT(%q) = %v, queria %v", v, got, quer)
		}
	}
}

// PALAVRA DESCONHECIDA É ERRO, E NÃO "ABERTO" NEM "FECHADO EM SILÊNCIO".
//
// O caso que isto evita é concreto: alguém escreve "closed", "off" ou "no" achando que
// trancou. Se o código devolvesse fechado calado, a intenção teria sido atendida por
// acidente e a próxima palavra errada — "open" — abriria sem ninguém saber. Erro no boot
// é o único jeito de a pessoa descobrir que digitou algo que o código não lê.
//
// E a mensagem tem de LISTAR o que serve: quem errou está olhando um painel de variáveis,
// não o fonte, e mandá-lo procurar no código qual palavra vale é mandá-lo adivinhar.
func TestPalavraDesconhecidaEhErroEListaOQueServe(t *testing.T) {
	for _, v := range []string{"closed", "off", "no", "0", "true", "sim", "abertoo"} {
		_, err := LerRMT(v)
		if err == nil {
			t.Errorf("LerRMT(%q) passou sem erro", v)
			continue
		}
		for _, palavra := range []string{"fechado", "staff", "aberto"} {
			if !contem(err.Error(), palavra) {
				t.Errorf("LerRMT(%q): o erro %q nao diz %q", v, err, palavra)
			}
		}
	}
}

// OS TRÊS ESTADOS TÊM FRASE, inclusive o fechado.
//
// Um log que só fala quando o mercado abre faz do silêncio duas coisas diferentes: "está
// fechado" e "esta versão nem tem a trava". Quem estiver conferindo de madrugada precisa
// distinguir as duas.
func TestOsTresEstadosTemFraseEElasSeDistinguem(t *testing.T) {
	vistas := map[string]bool{}
	for _, e := range []EstadoRMT{RMTFechado, RMTStaff, RMTAberto} {
		f := FraseRMT(e)
		if f == "" {
			t.Errorf("estado %v nao tem frase", e)
		}
		if vistas[f] {
			t.Errorf("estado %v repete a frase de outro: %q", e, f)
		}
		vistas[f] = true
	}
}
