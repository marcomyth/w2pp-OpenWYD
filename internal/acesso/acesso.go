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

// VariavelRMT é a chave que tranca a venda por dinheiro real.
//
// SEPARADA da tranca do servidor, e não um quarto valor dela, porque as duas respondem
// perguntas diferentes: uma é "quem pode entrar no jogo" e a outra é "quem pode vender
// por dinheiro". Juntá-las faria abrir o servidor para jogador abrir o mercado junto, que
// é exatamente o que não se quer no lançamento.
const VariavelRMT = "W2PP_RMT"

// EstadoRMT diz quem pode anunciar e comprar por dinheiro real.
type EstadoRMT int

const (
	// RMTFechado: ninguém anuncia nem compra. É o PADRÃO, e o padrão é o ponto.
	//
	// Com o padrão fechado, subir a versão JÁ tranca — não há variável para lembrar de
	// pôr em produção, e esquecer de configurar não deixa o mercado aberto. É a mesma
	// escolha do resto deste pacote: falhar fechado.
	RMTFechado EstadoRMT = iota
	// RMTStaff: só conta com cargo de moderador para cima. É o estado de teste.
	RMTStaff
	// RMTAberto: qualquer jogador.
	RMTAberto
)

var palavrasRMT = map[string]EstadoRMT{
	"fechado": RMTFechado,
	"staff":   RMTStaff,
	"aberto":  RMTAberto,
}

// RMT lê a variável do ambiente.
func RMT() (EstadoRMT, error) { return LerRMT(os.Getenv(VariavelRMT)) }

// LerRMT entende o valor, ou recusa dizendo o que aceita.
//
// VAZIO É FECHADO, e valor desconhecido é ERRO — as duas metades importam. Vazio ser
// fechado é o que faz a trava valer sem ninguém configurar nada. E desconhecido ser erro
// é o que impede o caso pior: alguém escreve "closed" ou "off" achando que trancou, o
// código não reconhece, e o mercado fica ABERTO enquanto todo mundo acha que está
// trancado. Servidor que não sobe alguém conserta em dois minutos; mercado aberto por
// engano move dinheiro de verdade e não se desfaz.
func LerRMT(valor string) (EstadoRMT, error) {
	v := strings.ToLower(strings.TrimSpace(valor))
	if v == "" {
		return RMTFechado, nil
	}
	if e, ok := palavrasRMT[v]; ok {
		return e, nil
	}
	return RMTFechado, fmt.Errorf("%s=%q nao e um valor que eu entenda; "+
		"use fechado, staff ou aberto; vazio vale fechado", VariavelRMT, valor)
}

// FraseRMT é o que vai no log do boot.
//
// OS TRÊS ESTADOS SÃO ESCRITOS, inclusive o fechado, pelo mesmo motivo da Frase de cima:
// um log que só fala quando o mercado abre faz do silêncio duas coisas diferentes —
// "está fechado" e "esta versão nem tem a trava".
func FraseRMT(e EstadoRMT) string {
	switch e {
	case RMTAberto:
		return "mercado em dinheiro real ABERTO para todos os jogadores"
	case RMTStaff:
		return "mercado em dinheiro real só para a STAFF (moderador ou acima)"
	default:
		return "mercado em dinheiro real FECHADO: ninguém anuncia nem compra"
	}
}
