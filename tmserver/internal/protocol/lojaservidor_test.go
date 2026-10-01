package protocol

import (
	"bytes"
	"encoding/binary"
	"testing"
)

// O BLOCO DE ADDS DA LOJINHA (0x0F02 e 0x0F06).
//
// O contrato com o cliente tem duas metades, e estes testes prendem as duas:
//
//  1. O PREFIXO NÃO MUDA. O cliente que está na rua lê as ofertas e o cofre por
//     deslocamento fixo. Um byte fora do lugar ali não dá erro: desenha o item errado,
//     o preço errado ou o vendedor errado na tela de quem está comprando.
//  2. O BLOCO VEM DEPOIS, seis bytes por item, NA MESMA ORDEM. Fora de ordem também não
//     dá erro: cada item mostra os adds do vizinho, e o jogador paga por um add que o
//     item não tem.

// Os números do contrato, ESCRITOS À MÃO pelo mesmo motivo do teste da Loja de Rcoin:
// derivados das constantes eles concordariam com qualquer valor que alguém pusesse lá.
const (
	lojaListaAntes  = 1140 // 20 de cabeçalho + 35 ofertas de 32
	lojaListaAgora  = 1350 // + 35 blocos de 6
	lojaCofreAntes  = 1028 // 4 de cabeçalho + 128 itens de 8
	lojaCofreAgora  = 1796 // + 128 blocos de 6
	lojaTetoDoFio   = 8192 // o ReadMessage do cliente recusa acima disto
	lojaCabecaDoFio = 12   // o HEADER de todo pacote
)

// addsDaPosicao devolve seis bytes que só aquela posição tem.
//
// OS SEIS SÃO DIFERENTES ENTRE SI e diferentes dos de qualquer outra posição, e é isso
// que dá dente ao teste: com o mesmo valor repetido, trocar efeito com valor, o par 1
// com o par 3, ou o item 7 com o item 8 passaria sem ninguém ver. Nenhum é zero, para
// que um byte que não foi escrito também apareça.
//
// 251 é primo e 7 não o divide, então n*7 mod 251 não repete dentro de 251 valores
// seguidos de n — e os seis de uma posição são seis n seguidos.
func addsDaPosicao(i int) LojaEfeitos {
	b := func(k int) uint8 { return uint8((i*6+k)*7%251 + 1) }
	return LojaEfeitos{Ef1: b(0), V1: b(1), Ef2: b(2), V2: b(3), Ef3: b(4), V3: b(5)}
}

// bytesDosAdds é a ordem do fio escrita por extenso, sem passar pelo encode do pacote:
// efeito 1, valor 1, efeito 2, valor 2, efeito 3, valor 3.
func bytesDosAdds(e LojaEfeitos) []byte {
	return []byte{e.Ef1, e.V1, e.Ef2, e.V2, e.Ef3, e.V3}
}

// O gerador acima é a régua dos outros testes; régua torta mede tudo torto e calada.
func TestAddsDaPosicaoNaoRepete(t *testing.T) {
	vistos := map[[6]byte]int{}
	for i := 0; i < LojaCargoMax; i++ {
		var seis [6]byte
		copy(seis[:], bytesDosAdds(addsDaPosicao(i)))
		for a := 0; a < 6; a++ {
			if seis[a] == 0 {
				t.Fatalf("posicao %d: o byte %d e zero", i, a)
			}
			for b := a + 1; b < 6; b++ {
				if seis[a] == seis[b] {
					t.Fatalf("posicao %d: os bytes %d e %d sao iguais (%d)", i, a, b, seis[a])
				}
			}
		}
		if outro, ja := vistos[seis]; ja {
			t.Fatalf("as posicoes %d e %d tem os mesmos adds", outro, i)
		}
		vistos[seis] = i
	}
}

// ofertaDaPosicao é uma oferta com todos os campos da linha preenchidos e diferentes
// por posição, para o teste do prefixo ter o que comparar em cada um dos 32 bytes.
func ofertaDaPosicao(i int) LojaOferta {
	return LojaOferta{
		Vendedor: int32(1000 + i),
		Nome:     "Vendedor" + string(rune('A'+i%26)),
		Indice:   int16(800 + i),
		Slot:     int8(i % MaxAutoTradeWire),
		Refino:   uint8(i % 16),
		Qtd:      uint8(1 + i),
		Moeda:    uint8(i % 3),
		Perto:    1,
		Preco:    int32(1_000_000 + i*7919),
	}
}

// listaComOfertas monta uma página com n ofertas, cada uma com os adds da sua posição
// quando comAdds é verdadeiro.
func listaComOfertas(n int, comAdds bool) LojaListaBody {
	m := LojaListaBody{
		Pagina: 2, Paginas: 5, Total: 170, Qtd: int16(n),
		Ouro: 123_456_789, Cash: 4321, RMT: 987,
	}
	for i := 0; i < n; i++ {
		m.Ofertas[i] = ofertaDaPosicao(i)
		if comAdds {
			m.Ofertas[i].Efeitos = addsDaPosicao(i)
		}
	}
	return m
}

// cofreComItens é o mesmo para o cofre.
func cofreComItens(n int, comAdds bool) LojaCargoListaBody {
	m := LojaCargoListaBody{Qtd: int16(n)}
	for i := 0; i < n; i++ {
		m.Itens[i] = LojaCargoItem{
			Slot: int16(i), Indice: int16(800 + i), Refino: uint8(i % 16), Qtd: uint8(1 + i%120),
		}
		if comAdds {
			m.Itens[i].Efeitos = addsDaPosicao(i)
		}
	}
	return m
}

// listaComoEraAntes é o Encode do 0x0F02 QUE ESTÁ NA RUA, copiado inteiro de antes do
// bloco de adds (origin/main a7a77078), com os deslocamentos por extenso.
//
// É A TESTEMUNHA DE FORA. Comparar o encode novo com ele mesmo (com e sem adds) pega o
// add que vaza para dentro da linha, mas não pega a linha que mudou de tamanho ou de
// lugar para todo mundo — os dois lados da comparação mudariam juntos. Esta cópia não
// muda junto.
func listaComoEraAntes(m *LojaListaBody) []byte {
	b := make([]byte, 20+35*32)
	binary.LittleEndian.PutUint16(b[0:], uint16(m.Pagina))
	binary.LittleEndian.PutUint16(b[2:], uint16(m.Paginas))
	binary.LittleEndian.PutUint16(b[4:], uint16(m.Total))
	binary.LittleEndian.PutUint16(b[6:], uint16(m.Qtd))
	binary.LittleEndian.PutUint32(b[8:], uint32(m.Ouro))
	binary.LittleEndian.PutUint32(b[12:], uint32(m.Cash))
	binary.LittleEndian.PutUint32(b[16:], uint32(m.RMT))
	for i := 0; i < 35; i++ {
		o, l := &m.Ofertas[i], b[20+i*32:]
		binary.LittleEndian.PutUint32(l[0:], uint32(o.Vendedor))
		nome := o.Nome
		if len(nome) >= 16 {
			nome = nome[:15]
		}
		copy(l[4:20], nome)
		binary.LittleEndian.PutUint16(l[20:], uint16(o.Indice))
		l[22] = byte(o.Slot)
		l[23] = o.Refino
		l[24] = o.Qtd
		l[25] = o.Moeda
		l[26] = o.Perto
		binary.LittleEndian.PutUint32(l[28:], uint32(o.Preco))
	}
	return b
}

// cofreComoEraAntes é o Encode do 0x0F06 de antes do bloco, pelo mesmo motivo.
func cofreComoEraAntes(m *LojaCargoListaBody) []byte {
	b := make([]byte, 4+128*8)
	binary.LittleEndian.PutUint16(b[0:], uint16(m.Qtd))
	for i := 0; i < 128; i++ {
		o := 4 + i*8
		binary.LittleEndian.PutUint16(b[o+0:], uint16(m.Itens[i].Slot))
		binary.LittleEndian.PutUint16(b[o+2:], uint16(m.Itens[i].Indice))
		b[o+4] = m.Itens[i].Refino
		b[o+5] = m.Itens[i].Qtd
	}
	return b
}

// primeiraDiferenca diz ONDE dois trechos divergem, que é o que quem quebrou o prefixo
// precisa saber; "são diferentes" manda a pessoa caçar entre mil bytes.
func primeiraDiferenca(a, b []byte) int {
	n := len(a)
	if len(b) < n {
		n = len(b)
	}
	for i := 0; i < n; i++ {
		if a[i] != b[i] {
			return i
		}
	}
	if len(a) != len(b) {
		return n
	}
	return -1
}

// TESTE 1: o tamanho exato dos dois corpos.
func TestLojaTamanhosComOBlocoDeAdds(t *testing.T) {
	var lista LojaListaBody
	var cofre LojaCargoListaBody
	casos := []struct {
		nome  string
		tem   int
		quero int
	}{
		{"um bloco de adds", LojaEfeitosSize, 6},
		{"ofertas por pagina", LojaPorPagina, 35},
		{"itens do cofre", LojaCargoMax, 128},
		{"a lista sem o bloco", lojaListaSemEfeitos, lojaListaAntes},
		{"a lista com o bloco", LojaListaBodySize, lojaListaAgora},
		{"o cofre sem o bloco", lojaCofreSemEfeitos, lojaCofreAntes},
		{"o cofre com o bloco", LojaCargoListaBodySize, lojaCofreAgora},
		// A conta do contrato, com as constantes: prefixo + um bloco por item.
		{"lista = prefixo + 35x6", LojaListaBodySize, lojaListaSemEfeitos + 35*6},
		{"cofre = prefixo + 128x6", LojaCargoListaBodySize, lojaCofreSemEfeitos + 128*6},
		// E o que SAI do Encode, que é o que vai para o fio — a constante certa com um
		// make() errado mandaria outro tamanho.
		{"Encode da lista", len(lista.Encode()), lojaListaAgora},
		{"Encode do cofre", len(cofre.Encode()), lojaCofreAgora},
	}
	for _, c := range casos {
		if c.tem != c.quero {
			t.Errorf("%s = %d, o contrato diz %d", c.nome, c.tem, c.quero)
		}
	}
}

// TESTE 2 (lista): os adds não mudam um byte do que o cliente da rua já lê.
func TestLojaListaOsAddsNaoMudamOPrefixo(t *testing.T) {
	for _, n := range []int{0, 1, LojaPorPagina} {
		sem := listaComOfertas(n, false)
		com := listaComOfertas(n, true)
		bSem, bCom := sem.Encode(), com.Encode()

		// Com e sem adds, o prefixo é o mesmo.
		if d := primeiraDiferenca(bSem[:lojaListaSemEfeitos], bCom[:lojaListaSemEfeitos]); d >= 0 {
			t.Errorf("n=%d: os adds mudaram o byte %d do prefixo (%#02x virou %#02x)",
				n, d, bSem[d], bCom[d])
		}
		// E é o prefixo DE ANTES, não um prefixo novo que por acaso não depende dos adds.
		antes := listaComoEraAntes(&com)
		if len(antes) != lojaListaAntes {
			t.Fatalf("a testemunha tem %d bytes; o pacote da rua tem %d", len(antes), lojaListaAntes)
		}
		if d := primeiraDiferenca(antes, bCom[:lojaListaAntes]); d >= 0 {
			t.Errorf("n=%d: o byte %d do pacote nao e mais o que o cliente da rua le "+
				"(era %#02x, agora %#02x)", n, d, antes[d], bCom[d])
		}
	}
}

// TESTE 2 (cofre).
func TestLojaCofreOsAddsNaoMudamOPrefixo(t *testing.T) {
	for _, n := range []int{0, 1, LojaCargoMax} {
		sem := cofreComItens(n, false)
		com := cofreComItens(n, true)
		bSem, bCom := sem.Encode(), com.Encode()

		if d := primeiraDiferenca(bSem[:lojaCofreSemEfeitos], bCom[:lojaCofreSemEfeitos]); d >= 0 {
			t.Errorf("n=%d: os adds mudaram o byte %d do prefixo (%#02x virou %#02x)",
				n, d, bSem[d], bCom[d])
		}
		antes := cofreComoEraAntes(&com)
		if len(antes) != lojaCofreAntes {
			t.Fatalf("a testemunha tem %d bytes; o pacote da rua tem %d", len(antes), lojaCofreAntes)
		}
		if d := primeiraDiferenca(antes, bCom[:lojaCofreAntes]); d >= 0 {
			t.Errorf("n=%d: o byte %d do pacote nao e mais o que o cliente da rua le "+
				"(era %#02x, agora %#02x)", n, d, antes[d], bCom[d])
		}
	}
}

// confereBloco olha o bloco inteiro, posição por posição: as n primeiras têm os adds
// da sua posição e as outras estão zeradas.
//
// AS ZERADAS IMPORTAM tanto quanto as cheias: o cliente desenha add para todo par com
// efeito diferente de zero, então lixo depois do último item vira add inventado num
// quadrado que a página seguinte vai ocupar.
func confereBloco(t *testing.T, nome string, bloco []byte, n, capacidade int) {
	t.Helper()
	if len(bloco) != capacidade*6 {
		t.Fatalf("%s n=%d: o bloco tem %d bytes; queria %d", nome, n, len(bloco), capacidade*6)
	}
	for i := 0; i < capacidade; i++ {
		quero := make([]byte, 6)
		if i < n {
			quero = bytesDosAdds(addsDaPosicao(i))
		}
		if tem := bloco[i*6 : i*6+6]; !bytes.Equal(tem, quero) {
			t.Errorf("%s n=%d: adds da posicao %d = % x; queria % x", nome, n, i, tem, quero)
		}
	}
}

// TESTE 3 (lista): o bloco bate com os adds de cada oferta, na mesma ordem.
func TestLojaListaOBlocoSegueAOrdemDasOfertas(t *testing.T) {
	for _, n := range []int{0, 1, LojaPorPagina} {
		m := listaComOfertas(n, true)
		confereBloco(t, "lista", m.Encode()[lojaListaAntes:], n, 35)
	}
}

// TESTE 3 (cofre).
func TestLojaCofreOBlocoSegueAOrdemDosItens(t *testing.T) {
	for _, n := range []int{0, 1, LojaCargoMax} {
		m := cofreComItens(n, true)
		confereBloco(t, "cofre", m.Encode()[lojaCofreAntes:], n, 128)
	}
}

// TESTE 4: o que entra no Encode sai igual do Decode — a linha e os adds.
func TestLojaListaIdaEVoltaComAdds(t *testing.T) {
	for _, n := range []int{0, 1, LojaPorPagina} {
		entra := listaComOfertas(n, true)
		var sai LojaListaBody
		if err := sai.Decode(entra.Encode()); err != nil {
			t.Fatalf("n=%d: %v", n, err)
		}
		for i := range entra.Ofertas {
			if sai.Ofertas[i].Efeitos != entra.Ofertas[i].Efeitos {
				t.Errorf("n=%d: adds da oferta %d voltaram %+v; entraram %+v", n, i,
					sai.Ofertas[i].Efeitos, entra.Ofertas[i].Efeitos)
			}
		}
		if sai != entra {
			t.Errorf("n=%d: a pagina nao voltou igual", n)
		}
	}
}

func TestLojaCofreIdaEVoltaComAdds(t *testing.T) {
	for _, n := range []int{0, 1, LojaCargoMax} {
		entra := cofreComItens(n, true)
		var sai LojaCargoListaBody
		if err := sai.Decode(entra.Encode()); err != nil {
			t.Fatalf("n=%d: %v", n, err)
		}
		for i := range entra.Itens {
			if sai.Itens[i].Efeitos != entra.Itens[i].Efeitos {
				t.Errorf("n=%d: adds do item %d voltaram %+v; entraram %+v", n, i,
					sai.Itens[i].Efeitos, entra.Itens[i].Efeitos)
			}
		}
		if sai != entra {
			t.Errorf("n=%d: o cofre nao voltou igual", n)
		}
	}
}

// O pacote de antes do bloco é CURTO para o Decode de agora, e ele recusa em vez de
// ler adds de onde não há nada.
func TestLojaDecodeRecusaOPacoteSemOBloco(t *testing.T) {
	var lista LojaListaBody
	if err := lista.Decode(make([]byte, lojaListaAntes)); err == nil {
		t.Error("a lista de 1140 bytes foi aceita; nao ha bloco de adds nela")
	}
	var cofre LojaCargoListaBody
	if err := cofre.Decode(make([]byte, lojaCofreAntes)); err == nil {
		t.Error("o cofre de 1028 bytes foi aceito; nao ha bloco de adds nele")
	}
}

// TESTE 5: nenhum dos dois, com o cabeçalho de rede, passa do teto do cliente.
//
// O cliente recusa o pacote acima de 8192 e DERRUBA A CONEXÃO; não é "não mostra os
// adds", é o jogador cair toda vez que abre o mercado.
func TestLojaOsDoisPacotesCabemNoFio(t *testing.T) {
	cheia := listaComOfertas(LojaPorPagina, true)
	cofre := cofreComItens(LojaCargoMax, true)
	casos := []struct {
		nome  string
		tipo  Type
		corpo []byte
		quero int
	}{
		{"0x0F02 lista", MsgLojaLista, cheia.Encode(), 1362},
		{"0x0F06 cofre", MsgLojaCargoLista, cofre.Encode(), 1808},
	}
	for _, c := range casos {
		if total := lojaCabecaDoFio + len(c.corpo); total != c.quero || total > lojaTetoDoFio {
			t.Errorf("%s: %d bytes no fio; queria %d, e o teto e %d", c.nome, total, c.quero,
				lojaTetoDoFio)
		}
		// E pelo caminho de verdade: o quadro que o servidor monta para mandar.
		fio, err := Encode(Header{Type: c.tipo, ID: IDScene}, c.corpo, 0)
		if err != nil {
			t.Errorf("%s: o servidor nao consegue montar o quadro: %v", c.nome, err)
			continue
		}
		if len(fio) != c.quero || len(fio) > lojaTetoDoFio {
			t.Errorf("%s: o quadro tem %d bytes; queria %d, e o teto e %d", c.nome, len(fio),
				c.quero, lojaTetoDoFio)
		}
	}
	if MaxMessageSize != lojaTetoDoFio || HeaderSize != lojaCabecaDoFio {
		t.Errorf("teto %d e cabecalho %d; o cliente usa %d e %d", MaxMessageSize, HeaderSize,
			lojaTetoDoFio, lojaCabecaDoFio)
	}
}
