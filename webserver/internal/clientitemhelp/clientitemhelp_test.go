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

// O itemhelp.dat real não está em ordem: a Chave do Rei Orc (465) foi acrescentada
// depois de índices maiores. Trocar a descrição dela tem de trocar esse bloco, não
// inserir um segundo antes do primeiro índice maior — o cliente ficava com a velha.
func TestSetTrocaBlocoForaDeOrdem(t *testing.T) {
	desordenado := arquivo + "465\r\nFFFFFFFF Abre_o_Castelo_Orc.\r\n"
	out, err := Set([]byte(desordenado), 465, []Linha{{Branco, "Trolls ou Orcs o que vamos caçar hoje?"}})
	if err != nil {
		t.Fatal(err)
	}
	got := string(out)
	if n := strings.Count(got, "465\r\n"); n != 1 {
		t.Errorf("%d blocos do item 465, want 1:\n%s", n, got)
	}
	if strings.Contains(got, "Abre_o_Castelo_Orc") {
		t.Errorf("a descrição antiga continuou no arquivo:\n%s", got)
	}
	if !strings.HasPrefix(got, arquivo) {
		t.Errorf("os blocos antes do 465 mudaram:\n%s", got)
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

// O WYD.exe lê o arquivo de dez em dez linhas, sem procurar o índice: todo
// bloco escrito tem de ter o índice e nove linhas, ou os itens seguintes
// desalinham e perdem a descrição.
func TestSetCompletaNoveLinhas(t *testing.T) {
	out, err := Set([]byte(arquivo), 3222, []Linha{{Premium, "[RCoin]"}, {Branco, "Moeda."}})
	if err != nil {
		t.Fatal(err)
	}
	got := string(out)
	i := strings.Index(got, "3222\r\n")
	j := strings.Index(got, "3343\r\n")
	if i < 0 || j < i {
		t.Fatalf("bloco 3222 não está antes do 3343:\n%s", got)
	}
	bloco := strings.Split(strings.TrimSuffix(got[i:j], "\r\n"), "\r\n")
	if len(bloco) != 1+LinhasPorBloco {
		t.Fatalf("bloco com %d linhas, want %d:\n%q", len(bloco), 1+LinhasPorBloco, bloco)
	}
	for _, l := range bloco[3:] {
		if l != "FFFFFFFF " {
			t.Errorf("linha de enchimento %q, want %q", l, "FFFFFFFF ")
		}
	}

	// Ida e volta: o enchimento não volta pelo Get, então somar uma linha
	// ao que já existe continua cabendo.
	lidas, err := Get(out, 3222)
	if err != nil {
		t.Fatal(err)
	}
	if len(lidas) != 2 {
		t.Fatalf("Get devolveu %d linhas, want 2: %+v", len(lidas), lidas)
	}
	if _, err := Set(out, 3222, append(lidas, Linha{Branco, "Mais uma."})); err != nil {
		t.Errorf("regravar com uma linha a mais falhou: %v", err)
	}
}

func TestSetRecusaOQueOClienteNaoLe(t *testing.T) {
	dez := make([]Linha, LinhasPorBloco+1)
	if _, err := Set([]byte(arquivo), 3222, dez); err == nil {
		t.Error("aceitou dez linhas; o cliente lê nove e desalinha o resto")
	}
	longa := []Linha{{Branco, strings.Repeat("a", MaxBytesPorLinha+1)}}
	if _, err := Set([]byte(arquivo), 3222, longa); err == nil {
		t.Error("aceitou linha maior que o buffer de 128 bytes do cliente")
	}
	cabe := []Linha{{Branco, strings.Repeat("a", MaxBytesPorLinha)}}
	if _, err := Set([]byte(arquivo), 3222, cabe); err != nil {
		t.Errorf("recusou linha de %d bytes: %v", MaxBytesPorLinha, err)
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
