package protocol

import (
	"bytes"
	"strings"
	"testing"
)

// O contrato da Loja de Rcoin é combinado com a dupla do cliente e está CONGELADO. Estes
// testes existem para uma coisa só: um campo que mude de tamanho ou de lugar aqui desenha
// lixo na tela de lá, porque o outro lado lê por deslocamento fixo — não há nome de campo
// no fio. Mudar um número destes é mudar o contrato, e isso se combina antes.

// TestOsTamanhosDoContratoDaLojaDeRcoin prende os números acertados.
//
// ESCRITOS À MÃO, e não calculados a partir das constantes: derivar 1288 de
// RcoinPorPagina faria o teste concordar com qualquer valor que alguém pusesse lá, que é
// o contrário do que ele serve.
func TestOsTamanhosDoContratoDaLojaDeRcoin(t *testing.T) {
	casos := []struct {
		nome  string
		tem   int
		quero int
	}{
		{"pedido da lista", RcoinPedeBodySize, 4},
		{"uma oferta", RcoinOfertaSize, 64},
		{"ofertas por pagina", RcoinPorPagina, 20},
		{"a pagina inteira", RcoinListaBodySize, 1288},
		{"o titulo", RcoinTituloSize, 42},
		{"a compra", RcoinCompraBodySize, 12},
		{"o resultado", RcoinResultadoBodySize, 16},
	}
	for _, c := range casos {
		if c.tem != c.quero {
			t.Errorf("%s = %d bytes, o contrato diz %d", c.nome, c.tem, c.quero)
		}
	}
	// E a página tem de fechar na conta: cabeçalho mais as ofertas, sem sobra.
	if RcoinListaBodySize != 8+RcoinPorPagina*RcoinOfertaSize {
		t.Error("o corpo da pagina nao e o cabecalho mais as ofertas")
	}
}

// TestAOfertaVoltaInteiraDoFio: cada campo tem de sobreviver à ida e à volta.
//
// Os valores são propositalmente diferentes uns dos outros e nenhum é zero: com dois
// campos vizinhos valendo a mesma coisa, trocá-los de lugar passaria no teste.
func TestAOfertaVoltaInteiraDoFio(t *testing.T) {
	quero := RcoinOferta{
		ID:        70123,
		ItemIndex: 3901,
		Efeitos:   [3]Effect{{106, 3}, {61, 12}, {45, 200}},
		Categoria: 5,
		Preco:     110,
		Dias:      3,
		Titulo:    "Fada Azul 3 dias",
	}
	var corpo RcoinListaBody
	corpo.Qtd, corpo.Paginas, corpo.Pagina, corpo.Saldo = 1, 4, 2, 1_234_567
	corpo.Ofertas[0] = quero

	var volta RcoinListaBody
	if err := volta.Decode(corpo.Encode()); err != nil {
		t.Fatal(err)
	}
	if volta.Pagina != 2 || volta.Paginas != 4 || volta.Qtd != 1 {
		t.Errorf("cabecalho = pagina %d de %d, qtd %d", volta.Pagina, volta.Paginas, volta.Qtd)
	}
	if volta.Saldo != 1_234_567 {
		t.Errorf("saldo = %d, queria 1234567", volta.Saldo)
	}
	if volta.Ofertas[0] != quero {
		t.Errorf("a oferta voltou %+v, mandei %+v", volta.Ofertas[0], quero)
	}
}

// TestOTituloVaiComAcento.
//
// MEDIDO NO CLIENTE em 25/09/2026: os painéis são TEXT nativos do WYD.exe, na fonte
// Tahoma, e o acento aparece na tela. Em Windows-1252 cada acento ocupa UM byte.
//
// O teste existe porque a primeira versão do contrato pedia para tirar os acentos, por
// precaução. Tirar transformaria "Poção de Ração" em "Pocao de Racao" em toda a loja — e
// o defeito de verdade, neste mesmo dia, foi o contrário: um nome chegando quebrado por
// FALTA de decodificação no catálogo.
func TestOTituloVaiComAcento(t *testing.T) {
	const titulo = "Poção de Ração"
	var o RcoinOferta
	o.Titulo = titulo
	b := make([]byte, RcoinOfertaSize)
	o.encodeEm(b)

	campo := b[22 : 22+RcoinTituloSize]
	// O ç e o ã são um byte cada em cp1252: 14 runas viram 14 bytes.
	if n := len(pegaAteOZero(campo)); n != 14 {
		t.Errorf("o titulo ocupou %d bytes, queria 14: acento em cp1252 e 1 byte", n)
	}
	if bytes.ContainsRune(campo, '?') {
		t.Error("algum acento virou '?': o texto nao passou pelo ClientText")
	}
	var volta RcoinOferta
	volta.decodeDe(b)
	if volta.Titulo != titulo {
		t.Errorf("o titulo voltou %q, mandei %q", volta.Titulo, titulo)
	}
}

// TestOTituloLongoEhCortadoComZeroNoFim.
//
// O cliente desenha até achar o zero. Um título que ocupasse os 42 bytes sairia colado no
// campo seguinte, e o campo seguinte é a próxima oferta — o nome de uma vazaria para
// dentro da outra.
//
// O corte é em BYTES e depois da codificação: cortar a string em runas antes erraria para
// mais em qualquer título acentuado, que é justamente onde o limite aperta.
func TestOTituloLongoEhCortadoComZeroNoFim(t *testing.T) {
	longo := strings.Repeat("ã", 60) // 60 bytes em cp1252, bem acima dos 41
	var o RcoinOferta
	o.Titulo = longo
	b := make([]byte, RcoinOfertaSize)
	o.encodeEm(b)

	campo := b[22 : 22+RcoinTituloSize]
	if campo[RcoinTituloSize-1] != 0 {
		t.Error("o ultimo byte do titulo nao e zero: o cliente desenharia ate a proxima oferta")
	}
	if n := len(pegaAteOZero(campo)); n != 41 {
		t.Errorf("o titulo cortado tem %d bytes, queria 41", n)
	}
}

// TestOsReservadosVaoZerados.
//
// O cliente ignora estes bytes hoje, e é exatamente por isso que eles têm de ser zero: no
// dia em que um deles virar campo, o lado que já mandava lixo passa a mandar um valor.
// Zerar agora custa nada e é a única hora em que dá para garantir.
func TestOsReservadosVaoZerados(t *testing.T) {
	var o RcoinOferta
	o.ID, o.ItemIndex, o.Categoria, o.Preco, o.Dias = 1, 2, 3, 4, 5
	o.Titulo = "x"
	b := make([]byte, RcoinOfertaSize)
	// Sujo de propósito: um encoder que só ESCREVE os campos, sem zerar o resto,
	// deixaria o lixo passar. Num buffer novo, que já vem zerado, esse defeito não
	// apareceria.
	for i := range b {
		b[i] = 0xFF
	}
	o.encodeEm(b)
	if b[13] != 0 || b[20] != 0 || b[21] != 0 {
		t.Errorf("reservados da oferta = +13:%d +20:%d +21:%d, queria zeros", b[13], b[20], b[21])
	}

	var corpo RcoinListaBody
	corpo.Qtd = 1
	corpo.Ofertas[0] = o
	if cab := corpo.Encode(); cab[3] != 0 {
		t.Errorf("reservado do cabecalho = %d, queria zero", cab[3])
	}

	res := RcoinResultadoBody{Resultado: RcoinOK, OfertaID: 9, Pedido: 8, Saldo: 7}.Encode()
	if res[1] != 0 || res[2] != 0 || res[3] != 0 {
		t.Errorf("reservados do resultado = %d %d %d, queria zeros", res[1], res[2], res[3])
	}
}

// TestACompraEOResultadoVoltamInteiros: os dois corpos pequenos, ida e volta.
//
// O PEDIDO é o campo que impede a cobrança dupla, e o PREÇO VISTO é o que impede cobrar
// um valor que a pessoa não viu. Se qualquer um dos dois se perder no fio, a trava que
// depende dele deixa de existir sem nada quebrar.
func TestACompraEOResultadoVoltamInteiros(t *testing.T) {
	compra := RcoinCompraBody{OfertaID: 4242, Pedido: 77, PrecoVisto: 110}
	var vc RcoinCompraBody
	if err := vc.Decode(compra.Encode()); err != nil {
		t.Fatal(err)
	}
	if vc != compra {
		t.Errorf("a compra voltou %+v, mandei %+v", vc, compra)
	}

	res := RcoinResultadoBody{Resultado: RcoinPrecoMudou, OfertaID: 4242, Pedido: 77, Saldo: 900}
	var vr RcoinResultadoBody
	if err := vr.Decode(res.Encode()); err != nil {
		t.Fatal(err)
	}
	if vr != res {
		t.Errorf("o resultado voltou %+v, mandei %+v", vr, res)
	}
}

// TestOsCodigosDeResultadoSaoOsCombinados: os números viajam, os nomes não.
func TestOsCodigosDeResultadoSaoOsCombinados(t *testing.T) {
	casos := []struct {
		nome  string
		tem   uint8
		quero uint8
	}{
		{"OK", RcoinOK, 0},
		{"SEM_SALDO", RcoinSemSaldo, 1},
		{"INDISPONIVEL", RcoinIndisponivel, 2},
		{"PRECO_MUDOU", RcoinPrecoMudou, 3},
		{"OCUPADO", RcoinOcupado, 4},
		{"ERRO", RcoinErro, 5},
	}
	for _, c := range casos {
		if c.tem != c.quero {
			t.Errorf("%s = %d, o contrato diz %d", c.nome, c.tem, c.quero)
		}
	}
}

// TestOPedidoCurtoNaoDerrubaNada: um cliente remendado manda o que quiser, e o servidor
// recusa em vez de entrar em pânico lendo fora do pacote.
func TestOPedidoCurtoNaoDerrubaNada(t *testing.T) {
	var pede RcoinPedeBody
	if err := pede.Decode([]byte{1}); err == nil {
		t.Error("um pedido de 1 byte foi aceito")
	}
	var compra RcoinCompraBody
	if err := compra.Decode(make([]byte, RcoinCompraBodySize-1)); err == nil {
		t.Error("uma compra curta foi aceita")
	}
	var lista RcoinListaBody
	if err := lista.Decode(make([]byte, 7)); err == nil {
		t.Error("uma pagina curta foi aceita")
	}
}
