package protocol

import "testing"

func lixeiraPacote(itens ...[2]int16) []byte {
	b := make([]byte, 4+4*len(itens))
	b[0] = uint8(len(itens))
	for i, it := range itens {
		p := 4 + 4*i
		le.PutUint16(b[p:], uint16(it[0]))
		le.PutUint16(b[p+2:], uint16(it[1]))
	}
	return b
}

// O LOTE CHEGA INTEIRO, na ordem em que o jogador marcou.
func TestDecodeLixeiraLeOLote(t *testing.T) {
	corpo, err := DecodeLixeiraApaga(lixeiraPacote([2]int16{3, 1100}, [2]int16{7, 2390}))
	if err != nil {
		t.Fatal(err)
	}
	if len(corpo.Itens) != 2 {
		t.Fatalf("itens = %d", len(corpo.Itens))
	}
	if corpo.Itens[0] != (LixeiraPedido{Slot: 3, Indice: 1100}) {
		t.Errorf("primeiro = %+v", corpo.Itens[0])
	}
	if corpo.Itens[1] != (LixeiraPedido{Slot: 7, Indice: 2390}) {
		t.Errorf("segundo = %+v", corpo.Itens[1])
	}
}

// O TAMANHO TEM DE FECHAR COM O CONTADOR, e um pacote truncado é recusado INTEIRO.
//
// Ler o que der apagaria itens que o jogador não chegou a marcar — e apagar item não
// tem volta. É a recusa mais importante deste arquivo.
func TestDecodeLixeiraRecusaOTruncado(t *testing.T) {
	bom := lixeiraPacote([2]int16{3, 1100}, [2]int16{7, 2390})
	// Diz que são dois e manda o espaço de um.
	truncado := bom[:len(bom)-4]
	if _, err := DecodeLixeiraApaga(truncado); err == nil {
		t.Fatal("aceitou um pacote que promete dois itens e traz um")
	}
}

// E O CONTADOR FORA DA FAIXA TAMBÉM: zero não tem o que apagar, e acima de 60 não
// cabe na mochila.
func TestDecodeLixeiraRecusaAQuantidadeImpossivel(t *testing.T) {
	for _, n := range []uint8{0, LixeiraMax + 1, 200} {
		b := make([]byte, 4+4*int(n))
		b[0] = n
		if _, err := DecodeLixeiraApaga(b); err == nil {
			t.Errorf("aceitou um lote de %d itens", n)
		}
	}
}

// A RESPOSTA SEMPRE SAI, inclusive quando nada foi apagado e nada foi recusado —
// a janela do cliente espera por ela, e o silêncio vira "sem resposta do servidor".
func TestEncodeLixeiraResultadoSempreTemCorpo(t *testing.T) {
	b := EncodeLixeiraResultado(0, nil)
	if len(b) != 4 {
		t.Fatalf("corpo vazio tem %d bytes, quero 4", len(b))
	}
	if b[0] != 0 || b[1] != 0 {
		t.Errorf("apagados=%d recusados=%d", b[0], b[1])
	}
}

// E AS RECUSAS VÃO COM SLOT E MOTIVO, que é o que a janela mostra.
func TestEncodeLixeiraResultadoLevaOsMotivos(t *testing.T) {
	b := EncodeLixeiraResultado(2, []LixeiraRecusa{
		{Slot: 5, Motivo: LixeiraMotivoVazio},
		{Slot: 9, Motivo: LixeiraMotivoOutroItem},
	})
	if b[0] != 2 || b[1] != 2 {
		t.Fatalf("apagados=%d recusados=%d", b[0], b[1])
	}
	if got := int16(le.Uint16(b[4:])); got != 5 || b[6] != LixeiraMotivoVazio {
		t.Errorf("primeira recusa = slot %d motivo %d", got, b[6])
	}
	if got := int16(le.Uint16(b[8:])); got != 9 || b[10] != LixeiraMotivoOutroItem {
		t.Errorf("segunda recusa = slot %d motivo %d", got, b[10])
	}
}
