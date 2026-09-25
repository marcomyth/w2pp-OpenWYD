package protocol

import "testing"

// A MOLDURA TAMBÉM VIAJA NO UpdateScore, e a falta disso era um defeito.
//
// O CreateMob acontece uma vez, ao entrar no campo de visão. O UpdateScore acontece a
// cada troca de equipamento, buff ou nível — e sem o byte ele mandava ChaosRate zero,
// apagando a moldura de quem pagou, para ele e para todos em volta, até o próximo
// spawn.
//
// É o mesmo byte e a mesma posição do CreateMob: os dois carregam o STRUCT_SCORE, e
// se um dia um deles mudar de lugar, o outro tem de mudar junto.
func TestOPasseVaiNoByte15DoUpdateScore(t *testing.T) {
	b := EncodeUpdateScore(ScoreData{PasseNivel: 3})
	if b[15] != 3 {
		t.Fatalf("byte 15 = %d, quero 3", b[15])
	}
	// E não vazou para os vizinhos: o 13 é a velocidade e o 16 abre o MaxHp.
	if b[13] != 0 {
		t.Errorf("o passe vazou para o byte 13 (velocidade): %d", b[13])
	}
	for i := 16; i < 20; i++ {
		if b[i] != 0 {
			t.Errorf("o passe vazou para o byte %d: %d", i, b[i])
		}
	}
}

// SEM PASSE É ZERO: o pacote de uma conta sem passe tem de ser exatamente o de antes.
func TestSemPasseOUpdateScoreContinuaZero(t *testing.T) {
	b := EncodeUpdateScore(ScoreData{})
	if b[15] != 0 {
		t.Fatalf("byte 15 = %d, quero 0", b[15])
	}
}

// A FAIXA É PRESA AQUI TAMBÉM. Não basta prender num dos dois caminhos: o que passasse
// cru pelo outro desenharia uma moldura que não existe.
func TestOPasseAcimaDaFaixaEhPresoNoUpdateScore(t *testing.T) {
	for _, n := range []uint8{5, 9, 200, 255} {
		b := EncodeUpdateScore(ScoreData{PasseNivel: n})
		if b[15] != 4 {
			t.Errorf("nivel %d virou %d, quero 4", n, b[15])
		}
	}
}

// OS DOIS PACOTES ESCREVEM O MESMO BYTE NO MESMO LUGAR.
//
// Este teste existe para o dia em que alguém mexer num só. O cliente lê o bloco de
// score dos dois do mesmo jeito; se eles discordarem, a moldura pisca a cada troca de
// equipamento e ninguém sabe por quê.
func TestCreateMobEUpdateScoreConcordamSobreAMoldura(t *testing.T) {
	for _, n := range []uint8{0, 1, 4, 255} {
		var criar [40]byte
		writeCreateMobScore(criar[:], CreateMobData{PasseNivel: n})
		atualizar := EncodeUpdateScore(ScoreData{PasseNivel: n})
		if criar[15] != atualizar[15] {
			t.Errorf("nivel %d: CreateMob escreveu %d e UpdateScore escreveu %d",
				n, criar[15], atualizar[15])
		}
	}
}
