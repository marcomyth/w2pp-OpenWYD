package protocol

import "testing"

func acaoPacote(acao uint8, nome string) []byte {
	b := make([]byte, 1+GuildaNomeMax)
	b[0] = acao
	copy(b[1:1+GuildaNomeMax], nome)
	return b
}

// A AÇÃO CHEGA COM NOME, e o nome vem do campo de 16 bytes do quadro.
func TestDecodeGuildaAcaoLeONome(t *testing.T) {
	corpo, err := DecodeGuildaAcao(acaoPacote(GuildaAcaoExpulsa, "SambariBlack"))
	if err != nil {
		t.Fatal(err)
	}
	if corpo.Acao != GuildaAcaoExpulsa || corpo.Nome != "SambariBlack" {
		t.Errorf("corpo = %+v", corpo)
	}
}

// A AÇÃO DESCONHECIDA É RECUSADA em vez de cair num switch que não a trata.
//
// Sem isto, um número fora da faixa passaria pelo decodificador, chegaria ao
// handler e sairia dele em silêncio depois de o handler já ter jogado fora o
// quadro guardado da guilda — trabalho perdido por um pacote que nunca foi ação.
func TestDecodeGuildaAcaoRecusaODesconhecido(t *testing.T) {
	for _, a := range []uint8{0, GuildaAcaoSai + 1, 200} {
		if _, err := DecodeGuildaAcao(acaoPacote(a, "Alguem")); err == nil {
			t.Errorf("aceitou a acao %d", a)
		}
	}
}

// O NOME VAZIO SÓ VALE NO "SAIR", que é a única ação sobre si mesmo.
//
// Nas outras três, o nome vazio viraria um comando de chat com argumento em
// branco — e o que cada comando faz com isso é problema de cada um. Recusar aqui
// é a resposta única.
func TestDecodeGuildaAcaoExigeNomeMenosNoSair(t *testing.T) {
	for _, a := range []uint8{GuildaAcaoPromove, GuildaAcaoLideranca, GuildaAcaoExpulsa} {
		if _, err := DecodeGuildaAcao(acaoPacote(a, "")); err == nil {
			t.Errorf("a acao %d passou sem nome", a)
		}
	}
	corpo, err := DecodeGuildaAcao(acaoPacote(GuildaAcaoSai, ""))
	if err != nil {
		t.Fatalf("o sair foi recusado sem nome: %v", err)
	}
	if corpo.Acao != GuildaAcaoSai {
		t.Errorf("acao = %d", corpo.Acao)
	}
}

// O PACOTE CURTO É RECUSADO: ler um nome de um espaço que não veio inteiro daria
// um nome truncado, e o comando agiria sobre outra pessoa.
func TestDecodeGuildaAcaoRecusaOCurto(t *testing.T) {
	bom := acaoPacote(GuildaAcaoExpulsa, "SambariBlack")
	if _, err := DecodeGuildaAcao(bom[:len(bom)-1]); err == nil {
		t.Fatal("aceitou um pacote de nome truncado")
	}
}

// O IMPOSTO CHEGA COM ZONA E TAXA. A zona viaja para o log; quem decide a cidade
// é o servidor.
func TestDecodeGuildaImpostoLeOsDois(t *testing.T) {
	corpo, err := DecodeGuildaImposto([]byte{4, 15})
	if err != nil {
		t.Fatal(err)
	}
	if corpo.Zona != 4 || corpo.Ticks != 15 {
		t.Errorf("corpo = %+v", corpo)
	}
}

// ACIMA DO TETO É RECUSADO NA PORTA, e não só lá dentro.
//
// O guildTax também recusa, e é ele quem manda; a recusa aqui é para o número
// impossível nem chegar a virar linha de comando. Zero é taxa válida: é a guilda
// que abre mão de cobrar.
func TestDecodeGuildaImpostoRecusaAcimaDoTeto(t *testing.T) {
	if _, err := DecodeGuildaImposto([]byte{0, GuildaImpostoMax + 1}); err == nil {
		t.Error("aceitou taxa acima do teto")
	}
	if _, err := DecodeGuildaImposto([]byte{0, 0}); err != nil {
		t.Errorf("recusou a taxa zero: %v", err)
	}
	if _, err := DecodeGuildaImposto([]byte{0, GuildaImpostoMax}); err != nil {
		t.Errorf("recusou o proprio teto: %v", err)
	}
}

func TestDecodeGuildaImpostoRecusaOCurto(t *testing.T) {
	if _, err := DecodeGuildaImposto([]byte{4}); err == nil {
		t.Fatal("aceitou um pacote de um byte")
	}
}
