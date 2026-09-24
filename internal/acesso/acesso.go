// Package acesso lê a única chave que tranca o servidor.
//
// UM LEITOR SÓ, para os três serviços. Ele nasceu porque havia três: dois escritos
// à mão e um terceiro parecido, e os três aceitavam palavras diferentes. A variável
// combinada com quem cuida do cliente e do launcher é
// `W2PP_ACESSO_RESTRITO=staff`, e nenhum dos três aceitava "staff" — a tranca teria
// ficado DESLIGADA em silêncio, e o ambiente de teste ficaria aberto a jogador
// enquanto todo mundo achava que estava trancado.
//
// FALHAR ABERTO É O PIOR JEITO DE FALHAR AQUI, e é por isso que um valor que este
// pacote não reconhece não vira "desligado": vira erro, e o serviço não sobe. Um
// servidor que não sobe é um problema que alguém conserta em dois minutos; um
// servidor que sobe destrancado achando que está trancado é um problema que ninguém
// procura.
package acesso

import (
	"fmt"
	"os"
	"strings"
)

// Variavel é o nome combinado, e ele aparece nos erros para ninguém precisar
// procurar.
const Variavel = "W2PP_ACESSO_RESTRITO"

// Os valores aceitos. "staff" está aí porque é o que foi combinado com as outras
// duplas; os outros estão porque são o que qualquer pessoa digita sem pensar.
var (
	ligam    = []string{"1", "true", "yes", "sim", "on", "staff"}
	desligam = []string{"", "0", "false", "no", "nao", "não", "off"}
)

// Restrito lê a variável do ambiente.
func Restrito() (bool, error) { return Ler(os.Getenv(Variavel)) }

// Ler entende o valor, ou recusa dizendo o que aceita.
//
// A recusa traz a lista inteira de propósito: quem errou o valor está olhando um
// painel de variáveis e não o código, e mandá-lo procurar no fonte qual palavra
// serve é mandá-lo adivinhar de novo.
func Ler(valor string) (bool, error) {
	v := strings.ToLower(strings.TrimSpace(valor))
	for _, s := range ligam {
		if v == s {
			return true, nil
		}
	}
	for _, s := range desligam {
		if v == s {
			return false, nil
		}
	}
	return false, fmt.Errorf("%s=%q nao e um valor que eu entenda; "+
		"use um destes para LIGAR: %s; ou deixe vazio para desligar",
		Variavel, valor, strings.Join(ligam, ", "))
}

// Frase é o que vai no log do boot, nos três serviços com as mesmas palavras.
//
// OS DOIS ESTADOS SÃO ESCRITOS, e o desligado também: um log que só fala quando a
// tranca liga faz do silêncio duas coisas diferentes — "está desligada" e "esta
// versão nem tem a tranca" —, e é justamente isso que alguém precisa distinguir às
// duas da manhã.
func Frase(restrito bool) string {
	if restrito {
		return "acesso restrito LIGADO: só staff entra"
	}
	return "acesso restrito DESLIGADO: o servidor está aberto"
}
