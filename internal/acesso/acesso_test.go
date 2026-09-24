package acesso

import "testing"

// O VALOR COMBINADO COM AS OUTRAS DUPLAS TEM DE LIGAR.
//
// É o caso que originou este pacote: a variável foi anunciada como
// W2PP_ACESSO_RESTRITO=staff, e os leitores que existiam aceitavam só "1/true/yes/
// sim". Com "staff", a tranca teria ficado desligada em silêncio.
func TestOValorCombinadoLiga(t *testing.T) {
	if got, err := Ler("staff"); err != nil || !got {
		t.Fatalf("Ler(\"staff\") = %v, %v; quero ligado", got, err)
	}
}

func TestOsValoresQueLigamEDesligam(t *testing.T) {
	for _, v := range []string{"1", "true", "TRUE", "yes", "sim", "on", "staff", " Staff "} {
		if got, err := Ler(v); err != nil || !got {
			t.Errorf("Ler(%q) = %v, %v; quero ligado", v, got, err)
		}
	}
	for _, v := range []string{"", "  ", "0", "false", "no", "nao", "não", "off", "OFF"} {
		if got, err := Ler(v); err != nil || got {
			t.Errorf("Ler(%q) = %v, %v; quero desligado", v, got, err)
		}
	}
}

// O VALOR DESCONHECIDO É ERRO, E NÃO "DESLIGADO".
//
// É a regra inteira deste pacote. Falhar aberto aqui produz um servidor que sobe
// destrancado achando que está trancado — e ninguém procura esse problema, porque
// tudo parece normal. Um serviço que não sobe, alguém conserta em dois minutos.
func TestOValorDesconhecidoNaoViraDesligado(t *testing.T) {
	for _, v := range []string{"talvez", "restrito", "2", "sim!", "staf"} {
		got, err := Ler(v)
		if err == nil {
			t.Errorf("Ler(%q) = %v sem erro; um valor errado nao pode destrancar em silencio", v, got)
		}
		if got {
			t.Errorf("Ler(%q) devolveu ligado junto com o erro", v)
		}
	}
}

// E O ERRO DIZ O NOME DA VARIÁVEL E O QUE ACEITA, porque quem errou está olhando um
// painel de variáveis e não o código.
func TestOErroEnsinaOQueUsar(t *testing.T) {
	_, err := Ler("talvez")
	if err == nil {
		t.Fatal("sem erro")
	}
	for _, quero := range []string{Variavel, "talvez", "staff"} {
		if !contem(err.Error(), quero) {
			t.Errorf("o erro nao diz %q: %v", quero, err)
		}
	}
}

func contem(s, sub string) bool {
	return len(s) >= len(sub) && (len(sub) == 0 || indexOf(s, sub) >= 0)
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}

// AS DUAS FRASES DO BOOT SÃO DIFERENTES, e o desligado também é escrito: um log que
// só fala quando liga faz do silêncio duas coisas — "está desligada" e "esta versão
// nem tem a tranca".
func TestAsDuasFrasesDoBoot(t *testing.T) {
	if Frase(true) == Frase(false) {
		t.Fatal("as duas frases sao iguais")
	}
	if !contem(Frase(true), "LIGADO") || !contem(Frase(false), "DESLIGADO") {
		t.Errorf("frases = %q / %q", Frase(true), Frase(false))
	}
}
