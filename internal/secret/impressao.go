package secret

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
)

// Impressao descreve um segredo o bastante para COMPARAR dois lados, e de menos
// para vazar qualquer coisa.
//
// POR QUE ELA EXISTE: dois serviços que precisam do mesmo token só descobrem que
// estão com valores diferentes quando um recusa o outro, e a recusa não diz qual dos
// dois está errado. Sem uma forma de comparar, a saída é imprimir o segredo — que é
// justamente o que não se pode fazer.
//
// O QUE ELA DEVOLVE: o tamanho e os oito primeiros hexadecimais do SHA-256. Os dois
// juntos bastam para dizer "é o mesmo valor" ou "não é", e nenhum dos dois volta
// para o texto original: a entrada tem entropia de token e oito hex são 32 bits de
// uma função que ninguém inverte.
//
// E ISSO SÓ É INOFENSIVO PORQUE O TOKEN É LONGO E ALEATÓRIO. Oito hex de um SHA-256
// num log são um jeito de CONFERIR PALPITES: com um token curto ou adivinhável,
// quem leia o log pode testar candidatos fora do servidor até achar o que dá o mesmo
// sha8. Com 32 bytes aleatórios não há lista de candidatos para testar.
//
// Por isso existe o TokenFraco logo abaixo, e por isso os serviços avisam no boot
// quando o token é curto: a impressão é segura enquanto o segredo for um segredo de
// verdade.
//
// E ELA DENUNCIA A REFERÊNCIA NÃO RESOLVIDA, que é o erro mais comum de todos numa
// plataforma que monta variável a partir de outra: um valor que ainda começa com
// "${{" não é um token, é o TEXTO de um token que ninguém substituiu. Isso não é
// segredo nenhum, e dizer em voz alta economiza a tarde inteira de quem procura.
func Impressao(valor string) string {
	if valor == "" {
		return "vazio"
	}
	if strings.HasPrefix(strings.TrimSpace(valor), "${{") {
		return "REFERENCIA NAO RESOLVIDA (o valor ainda e o texto ${{...}})"
	}
	soma := sha256.Sum256([]byte(valor))
	impressao := fmt.Sprintf("len=%d sha=%s", len(valor), hex.EncodeToString(soma[:])[:8])
	// ESPAÇO NAS PONTAS MUDA O BYTE E NÃO MUDA A APARÊNCIA, e é por isso que ele é
	// dito: um token colado com um "enter" no fim compara diferente e parece igual
	// em qualquer tela que alguém vá conferir.
	if valor != strings.TrimSpace(valor) {
		impressao += " ATENCAO: tem espaco ou quebra de linha nas pontas"
	}
	return impressao
}

// TamanhoMinimoDoToken é o que um segredo de controle precisa ter.
//
// Trinta e dois bytes aleatórios. Não é número redondo por gosto: é o tamanho a
// partir do qual não existe lista de candidatos que alguém possa testar contra o
// sha8 que o log mostra.
const TamanhoMinimoDoToken = 32

// TokenFraco diz se este segredo é curto demais para ter a impressão publicada.
//
// Quem chama registra isso como AVISO e não como erro: um token curto FUNCIONA, e
// derrubar o servidor por causa dele trocaria um risco por uma parada. O que não pode
// é passar calado — porque o log com o sha8 já foi escrito, e é justamente o token
// fraco que ele ajuda a adivinhar.
func TokenFraco(valor string) bool {
	return valor != "" && len(valor) < TamanhoMinimoDoToken
}

// TokenComCaraDeEndereco diz se este valor é o ENDEREÇO no lugar do segredo.
//
// POR QUE ISTO EXISTE: aconteceu. A variável do token do webserver ficou apontada
// para a variável do ENDEREÇO do servidor de jogo, e o serviço subiu dizendo que o
// link estava ligado. Ele não estava: o tmServer recusava toda chamada, e cada
// recusa saía como uma falha de entrega diferente. A causa — duas variáveis
// trocadas — não aparecia em lugar nenhum, e levou meses para ser vista.
//
// As duas formas de reconhecer, e as duas são o mesmo engano:
//
//   - O valor é IGUAL ao endereço. É o caso exato do dia: alguém apontou a variável
//     do token para a variável do endereço.
//   - O valor tem ":". Endereço de rede tem porta; token não tem. Um token gerado
//     como se gera token — aleatório em hexadecimal ou base64 — nunca tem dois
//     pontos, porque nenhum dos dois alfabetos o inclui.
//
// A SEGUNDA REGRA RECUSA UM TOKEN LEGÍTIMO que por acaso tivesse ":", como uma frase
// escolhida à mão. É de propósito: o estrago de aceitar um endereço como token é um
// link que finge estar de pé, e o estrago de recusar uma frase com dois pontos é uma
// mensagem de erro que diz exatamente o que trocar. O segundo custa minutos; o
// primeiro custou meses.
//
// Quem chama NÃO liga o link e escreve ERRO no boot — mas continua subindo o
// serviço, porque tudo o que não depende do link segue funcionando, e derrubar o
// site inteiro por causa de uma variável seria trocar um defeito por uma parada.
func TokenComCaraDeEndereco(token, endereco string) bool {
	t := strings.TrimSpace(token)
	if t == "" {
		return false
	}
	if e := strings.TrimSpace(endereco); e != "" && t == e {
		return true
	}
	return strings.Contains(t, ":")
}
