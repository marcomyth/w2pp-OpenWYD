package protocol

import "testing"

// O NÍVEL DO PASSE VAI NO BYTE 15 DO BLOCO DE SCORE.
//
// É o contrato inteiro com o cliente: um byte, 0 a 4, no STRUCT_SCORE.ChaosRate do
// MSG_CreateMob. O cliente o lê em entidade+0x623, depois de copiar o bloco inteiro.
// Se este teste mudar de posição, a moldura de todo mundo muda junto — e ninguém
// descobre por um erro, descobre por um desenho errado na tela.
func TestOPasseVaiNoByte15DoScore(t *testing.T) {
	var b [40]byte
	writeCreateMobScore(b[:], CreateMobData{PasseNivel: 3})

	if b[15] != 3 {
		t.Fatalf("byte 15 = %d, quero 3", b[15])
	}
	// E nada mais do bloco foi tocado pelo passe: os vizinhos são a direção e o
	// MaxHp, e escrever por cima de qualquer um deles seria um estrago silencioso.
	if b[14] != 0 {
		t.Errorf("o passe vazou para o byte 14 (direcao): %d", b[14])
	}
	for i := 16; i < 20; i++ {
		if b[i] != 0 {
			t.Errorf("o passe vazou para o byte %d: %d", i, b[i])
		}
	}
}

// SEM PASSE É ZERO, que é o que o servidor sempre escreveu ali. Uma conta sem passe
// tem de produzir exatamente o pacote de antes.
func TestSemPasseOByte15ContinuaZero(t *testing.T) {
	var b [40]byte
	writeCreateMobScore(b[:], CreateMobData{})
	if b[15] != 0 {
		t.Fatalf("byte 15 = %d, quero 0", b[15])
	}
}

// O VALOR ACIMA DA FAIXA É PRESO, e não passa cru.
//
// Esta é a última linha antes de o número virar byte na rede. Ele já foi conferido no
// banco, no serviço e no cliente do dbserver — e mesmo assim: é aqui que se sabe o
// que o cliente aguenta, e um 200 no pacote viraria uma moldura que ninguém desenhou.
func TestOPasseAcimaDaFaixaEhPreso(t *testing.T) {
	for _, n := range []uint8{5, 9, 200, 255} {
		var b [40]byte
		writeCreateMobScore(b[:], CreateMobData{PasseNivel: n})
		if b[15] != 4 {
			t.Errorf("nivel %d virou %d, quero 4", n, b[15])
		}
	}
}
