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
