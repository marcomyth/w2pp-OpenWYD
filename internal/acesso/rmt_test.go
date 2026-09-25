package acesso

import "testing"

// VAZIO É FECHADO, e este é o teste que sustenta a trava inteira.
//
// Se vazio virasse "aberto", subir a versão sem configurar nada deixaria o mercado em
// dinheiro real aberto a todo mundo — e ninguém procuraria, porque a configuração que
// falta não aparece em log nenhum. O padrão é a trava.
func TestVazioEhFechado(t *testing.T) {
	for _, v := range []string{"", "   ", "\t"} {
		got, err := LerRMT(v)
		if err != nil {
			t.Fatalf("LerRMT(%q) deu erro: %v", v, err)
		}
		if got != RMTFechado {
			t.Errorf("LerRMT(%q) = %v, queria fechado", v, got)
		}
	}
}

// O ZERO DO TIPO TAMBÉM É FECHADO.
//
// Não é a mesma pergunta do teste de cima: aqui o que se prova é que um Config montado
// SEM o campo — num teste, numa montagem nova amanhã — trava em vez de abrir. Se alguém
// reordenar as constantes e o zero virar "aberto", este teste é o que pega.
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
