package buildinfo

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
)

// ImpressaoDoConteudo identifica um arquivo de conteúdo do jogo em uma linha de log.
//
// POR QUE ISTO EXISTE, e é a mesma pergunta do Revision: o que este processo está
// rodando? Para o binário, a resposta é a revisão. Para o conteúdo, não havia
// resposta nenhuma — o tmServer registra no boot "content loaded ... items=3242", e
// CONTAGEM NÃO MUDA QUANDO O VALOR MUDA. No dia em que 456 armas ganharam 15% de
// dano, o log continuou dizendo 3242 itens, exatamente como antes, e provar que o
// servidor tinha mesmo os números novos exigiu uma investigação inteira que terminou
// numa DEDUÇÃO — "a plataforma diz que construiu o commit tal, e aquele commit contém
// a mudança" — em vez de numa medição.
//
// Com a impressão no log, a pergunta vira uma comparação de dezesseis caracteres:
// calcula-se o mesmo sha256 do arquivo em qualquer revisão do repositório e vê-se se
// bate com o que o servidor disse. É o que o webServer já fazia para o catálogo de
// itens, e é de propósito que o número aqui é o MESMO dele — os dois lados têm de ser
// comparáveis entre si, senão viram duas verdades sobre o mesmo arquivo.
// UMA ARMADILHA MEDIDA, para quem for conferir: a impressão é dos BYTES do arquivo,
// então ela muda com a quebra de linha. O mesmo ItemList.csv dá e441a7f87ee23cca com
// LF e 17e3f3f4474ef968 com CRLF. O servidor roda a partir de um checkout Linux, e é
// o número com LF que ele imprime — conferi contra três boots de produção. Quem
// calcular o esperado numa cópia de trabalho do Windows vai achar que divergiu.
// O jeito seguro é hashear o blob do repositório (`git show REV:caminho`), não o
// arquivo do disco.
func ImpressaoDoConteudo(caminho string) (string, error) {
	f, err := os.Open(caminho)
	if err != nil {
		return "", fmt.Errorf("buildinfo: abrir %s: %w", caminho, err)
	}
	defer func() { _ = f.Close() }()

	soma := sha256.New()
	if _, err := io.Copy(soma, f); err != nil {
		return "", fmt.Errorf("buildinfo: ler %s: %w", caminho, err)
	}
	return hex.EncodeToString(soma.Sum(nil))[:TamanhoDaImpressao], nil
}

// TamanhoDaImpressao é o recorte do sha256, em caracteres hexadecimais.
//
// Dezesseis porque é o que o webServer já usa, e o valor destas duas impressões está
// em elas serem o MESMO número. Não é segredo nenhum — é um arquivo de conteúdo que
// vai dentro do cliente —, então não há aqui a preocupação que existe em
// internal/secret sobre publicar o resumo de um valor adivinhável.
const TamanhoDaImpressao = 16
