package clientitemhelp

import (
	"strings"
	"testing"
)

// arquivo é um itemhelp.dat em miniatura, com a forma do real: sem cabeçalho,
// índices em ordem, linhas de cor e texto.
const arquivo = "410\r\n" +
	"FFFFFFFF Ao_ser_utilizado_no_campo,\r\n" +
	"FFFFFFFF o_personagem_retorna_à_cidade.\r\n" +
	"3343\r\n" +
	"FFFF00FF [Item_Premium]\r\n" +
	"FFFFFFFF Dispensa_os_pontos_caóticos.\r\n"

func TestSetTrocaODoItemESoDele(t *testing.T) {
	out, err := Set([]byte(arquivo), 3343, []Linha{
		{Premium, "[Item_Premium]"},
		{Branco, "Um lugar infernal, mas com grandes recompensas."},
		{Vermelho, "Só venha se tiver coragem, NOOB!"},
	})
	if err != nil {
		t.Fatal(err)
	}
	got := string(out)
	if !strings.Contains(got, "FFFFFFFF Um_lugar_infernal,_mas_com_grandes_recompensas.\r\n") {
		t.Errorf("o texto novo não saiu com os espaços como \"_\":\n%s", got)
	}
	// O "ó" sai em Windows-1252, um byte só, que é o que o cliente lê.
	if !strings.Contains(got, "FFFF0000 S\xf3_venha_se_tiver_coragem,_NOOB!\r\n") {
		t.Errorf("a linha vermelha não saiu certa:\n%q", got)
	}
	if strings.Contains(got, "Dispensa_os_pontos") {
		t.Error("a descrição antiga do item continuou no arquivo")
	}
	if !strings.Contains(got, "410\r\nFFFFFFFF Ao_ser_utilizado_no_campo,") {
		t.Errorf("o bloco do item vizinho mudou:\n%s", got)
	}
}

func TestSetInsereNaOrdem(t *testing.T) {
	out, err := Set([]byte(arquivo), 3222, []Linha{{Branco, "Chave do Inferno."}})
	if err != nil {
		t.Fatal(err)
	}
	got := string(out)
	i3222 := strings.Index(got, "3222\r\n")
	i3343 := strings.Index(got, "3343\r\n")
	i410 := strings.Index(got, "410\r\n")
	if i3222 < 0 {
		t.Fatalf("o bloco novo não foi escrito:\n%s", got)
	}
	if i410 >= i3222 || i3222 >= i3343 {
		t.Errorf("ordem dos índices errada: 410=%d 3222=%d 3343=%d", i410, i3222, i3343)
	}
}

func TestSetSemLinhasApagaODescricao(t *testing.T) {
	out, err := Set([]byte(arquivo), 3343, nil)
	if err != nil {
		t.Fatal(err)
	}
	got := string(out)
	if strings.Contains(got, "3343\r\n") || strings.Contains(got, "[Item_Premium]") {
		t.Errorf("o bloco não foi removido:\n%s", got)
	}
	if !strings.Contains(got, "410\r\n") {
		t.Error("apagar um bloco levou junto o do vizinho")
	}
}

// TestCopiarRepeteOBlocoByteAByte: a variante tem de mostrar o mesmo texto do
// original, e os bytes vão sem decodificar — o "ó" de "caóticos" sai como o
// mesmo byte único de Windows-1252.
func TestCopiarRepeteOBlocoByteAByte(t *testing.T) {
	out, ok, err := Copiar([]byte(arquivo), 410, 5760)
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("Copiar disse que o 410 não tem descrição")
	}
	got := string(out)
	quer := "5760\r\nFFFFFFFF Ao_ser_utilizado_no_campo,\r\nFFFFFFFF o_personagem_retorna_à_cidade.\r\n"
	if !strings.HasSuffix(got, quer) {
		t.Errorf("o bloco copiado não foi para o fim, na ordem:\n%q", got)
	}
	if !strings.HasPrefix(got, arquivo) {
		t.Errorf("os blocos que já existiam mudaram:\n%q", got)
	}
}

// TestCopiarOrigemSemBlocoNaoMexe: o Baú de Experiência (4140) não tem texto no
// cliente, e a variante dele fica igual — sem descrição, sem erro.
func TestCopiarOrigemSemBlocoNaoMexe(t *testing.T) {
	out, ok, err := Copiar([]byte(arquivo), 4140, 5761)
	if err != nil {
		t.Fatal(err)
	}
	if ok || string(out) != arquivo {
		t.Errorf("origem sem bloco alterou o arquivo (ok=%v):\n%q", ok, out)
	}
}

// TestCopiarDeUltimoBlocoSemQuebraFinal: o último bloco do arquivo pode não
// terminar em CRLF, e a cópia não pode colar a última linha dele no índice
// seguinte.
func TestCopiarDeUltimoBlocoSemQuebraFinal(t *testing.T) {
	semQuebra := strings.TrimSuffix(arquivo, "\r\n")
	out, _, err := Copiar([]byte(semQuebra), 3343, 3000)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(out), "3000\r\nFFFF00FF [Item_Premium]\r\nFFFFFFFF Dispensa_os_pontos_caóticos.\r\n3343\r\n") {
		t.Errorf("a cópia do último bloco saiu colada:\n%q", out)
	}
}

// TestCopiarNoFimDeArquivoSemQuebra: o itemhelp.dat do cliente termina sem CRLF
// e o 5760 é maior que todo índice dele, então o bloco novo vai para o fim. Sem
// a quebra, "5760" colaria na última linha do arquivo e o cliente leria um texto
// estranho no item anterior e nenhum no Frango.
func TestCopiarNoFimDeArquivoSemQuebra(t *testing.T) {
	semQuebra := strings.TrimSuffix(arquivo, "\r\n")
	out, _, err := Copiar([]byte(semQuebra), 410, 5760)
	if err != nil {
		t.Fatal(err)
	}
	quer := semQuebra + "\r\n5760\r\nFFFFFFFF Ao_ser_utilizado_no_campo,\r\n"
	if !strings.HasPrefix(string(out), quer) {
		t.Errorf("o bloco novo colou no fim do arquivo:\n%q", out)
	}
}

// foraDeOrdem tem a forma do itemhelp.dat real: a ordem crescente quebra (no
// cliente, os blocos 3310-3315 vêm depois do 3463, e há outras 15 quedas).
const foraDeOrdem = "3463\r\n" +
	"FFFFFFFF Item_3463.\r\n" +
	"3314\r\n" +
	"FFFFFFFF Um_suculento_frango_assado.\r\n" +
	"3315\r\n" +
	"FFFFFFFF Item_3315.\r\n"

// TestCopiarAchaBlocoForaDeOrdem: parar no primeiro índice maior dava o 3314
// como inexistente, e a variante do Frango saía sem descrição, calada.
func TestCopiarAchaBlocoForaDeOrdem(t *testing.T) {
	out, ok, err := Copiar([]byte(foraDeOrdem), 3314, 5760)
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("Copiar não achou o 3314, que está depois do 3463")
	}
	if !strings.HasSuffix(string(out), "5760\r\nFFFFFFFF Um_suculento_frango_assado.\r\n") {
		t.Errorf("a descrição não foi copiada:\n%q", out)
	}
}

// TestSetTrocaBlocoForaDeOrdemSemDuplicar: trocar a descrição de um item fora
// de ordem inseria um SEGUNDO bloco dele antes do 3463, e o cliente passava a
// ter dois textos para o mesmo item.
func TestSetTrocaBlocoForaDeOrdemSemDuplicar(t *testing.T) {
	out, err := Set([]byte(foraDeOrdem), 3314, []Linha{{Branco, "Novo."}})
	if err != nil {
		t.Fatal(err)
	}
	got := string(out)
	if n := strings.Count(got, "3314\r\n"); n != 1 {
		t.Fatalf("o 3314 aparece %d vezes:\n%q", n, got)
	}
	if strings.Contains(got, "frango") || !strings.Contains(got, "3314\r\nFFFFFFFF Novo.\r\n3315\r\n") {
		t.Errorf("o bloco não foi trocado no lugar:\n%q", got)
	}
}

// TestSetGravaEmCP1252: o cliente lê Windows-1252, e o acento tem de chegar
// como um byte só — em UTF-8 ele viraria dois e a tela mostraria lixo.
func TestSetGravaEmCP1252(t *testing.T) {
	out, err := Set([]byte(arquivo), 3222, []Linha{{Branco, "coração"}})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(out), "FFFFFFFF cora\xe7\xe3o\r\n") {
		t.Errorf("acento não saiu em Windows-1252:\n%q", string(out))
	}
}
