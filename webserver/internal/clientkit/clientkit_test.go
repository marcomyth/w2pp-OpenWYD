package clientkit

import (
	"os"
	"path/filepath"
	"testing"
)

// catalogoFalso monta um ItemList.bin do tamanho certo com um único item
// preenchido no índice dado.
func catalogoFalso(t *testing.T, idx int, nome string, efeitos [][2]int16, preco int32) []byte {
	t.Helper()
	b := make([]byte, tamanhoArquivo)
	for i := range b {
		b[i] = xor // tudo zero, sob o XOR
	}
	reg := idx * tamanhoReg
	for i := 0; i < len(nome); i++ {
		b[reg+offNome+i] = nome[i] ^ xor
	}
	poe := func(off int, v int16) {
		b[off], b[off+1] = byte(uint16(v))^xor, byte(uint16(v)>>8)^xor
	}
	for i, ef := range efeitos {
		poe(reg+offEfeitos+4*i, ef[0])
		poe(reg+offEfeitos+4*i+2, ef[1])
	}
	poe(reg+offPreco, int16(preco))
	poe(reg+offPreco+2, int16(preco>>16))
	return b
}

// TestVarianteCopiaODesenhoEZeraOPreco: a variante tem de ficar com o ícone e os
// efeitos do original (é a razão de ser uma cópia), com o nome novo e com preço
// zero — o preço é o que a faz não virar gold no NPC.
func TestVarianteCopiaODesenhoEZeraOPreco(t *testing.T) {
	const efVolatile, efNoTrade = 38, 127
	il := catalogoFalso(t, 3314, "Frango_Assado", [][2]int16{{18, 255}, {efVolatile, 63}}, 200000)

	out, err := Aplicar(il, []Variante{{
		Origem: 3314, Destino: 5760, Nome: "Frango Assado (Novato)",
		Efeitos: [][2]int16{{efNoTrade, 1}},
	}})
	if err != nil {
		t.Fatalf("Aplicar: %v", err)
	}

	reg := 5760 * tamanhoReg
	if got := nomeDoRegistro(out, reg); got != "Frango Assado (Novato)" {
		t.Errorf("nome = %q, queria %q", got, "Frango Assado (Novato)")
	}
	leShort := func(off int) int16 {
		return int16(uint16(out[off]^xor) | uint16(out[off+1]^xor)<<8)
	}
	efeitos := map[int16]int16{}
	for i := 0; i < slotsDeEfeito; i++ {
		if c := leShort(reg + offEfeitos + 4*i); c != 0 {
			efeitos[c] = leShort(reg + offEfeitos + 4*i + 2)
		}
	}
	if efeitos[efVolatile] != 63 {
		t.Errorf("EF_VOLATILE = %d, queria 63 (o mesmo do original)", efeitos[efVolatile])
	}
	if efeitos[efNoTrade] != 1 {
		t.Error("a variante ficou sem EF_NOTRADE — o tooltip não diria que é intransferível")
	}
	if p := leShort(reg + offPreco); p != 0 {
		t.Errorf("preço = %d, queria 0", p)
	}
	// O original não pode ter sido tocado.
	if got := nomeDoRegistro(out, 3314*tamanhoReg); got != "Frango_Assado" {
		t.Errorf("o item de origem virou %q", got)
	}
}

// TestRecusaDestinoOcupado: gravar por cima de um item que o cliente já conhece
// trocaria o desenho de algo que o jogador tem na bolsa, e o erro só apareceria
// em jogo.
func TestRecusaDestinoOcupado(t *testing.T) {
	il := catalogoFalso(t, 3314, "Frango_Assado", nil, 0)
	reg := 5760 * tamanhoReg
	for i, c := range []byte("Ja_Existe") {
		il[reg+offNome+i] = c ^ xor
	}
	if _, err := Aplicar(il, []Variante{{Origem: 3314, Destino: 5760, Nome: "X"}}); err == nil {
		t.Fatal("Aplicar aceitou gravar por cima de um item existente")
	}
}

// TestRecusaOrigemVazia: copiar de um registro em branco produziria a variante
// sem nome e sem ícone, exatamente o problema que este pacote existe para
// resolver.
func TestRecusaOrigemVazia(t *testing.T) {
	il := catalogoFalso(t, 3314, "Frango_Assado", nil, 0)
	if _, err := Aplicar(il, []Variante{{Origem: 4140, Destino: 5761, Nome: "X"}}); err == nil {
		t.Fatal("Aplicar aceitou copiar de um item que não existe")
	}
}

// TestKitNoCatalogoDoCliente roda contra o ItemList.bin de verdade, quando ele
// está no repositório: é o mesmo formato que o cliente lê.
func TestKitNoCatalogoDoCliente(t *testing.T) {
	caminho := filepath.Join("..", "..", "..", "Release", "DBsrv", "run", "ItemList.bin")
	il, err := os.ReadFile(caminho)
	if err != nil {
		t.Skipf("ItemList.bin indisponível: %v", err)
	}
	out, err := Aplicar(il, KitDoNovato())
	if err != nil {
		t.Fatalf("Aplicar no catálogo real: %v", err)
	}
	for _, v := range KitDoNovato() {
		if got := nomeDoRegistro(out, int(v.Destino)*tamanhoReg); got != v.Nome {
			t.Errorf("item %d: nome = %q, queria %q", v.Destino, got, v.Nome)
		}
	}
}
