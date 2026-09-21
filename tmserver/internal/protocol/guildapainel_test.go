package protocol

import (
	"strings"
	"testing"
)

// A aba Informações tem de voltar inteira do fio: são quinze campos, e um
// deslocamento errado em qualquer um deles desalinha todos os seguintes sem
// quebrar nada — o painel mostraria a fama no lugar da capacidade.
func TestGuildaAbreIdaEVolta(t *testing.T) {
	origem := GuildaAbreBody{
		GuildaID:   11,
		Nome:       "REDDRAGONS",
		Lider:      "SambariBlack",
		Membros:    249,
		Online:     42,
		Capacidade: 250,
		Fama:       1450,
		Aliada:     "IMPERIAL",
		Guerra:     "BLOODLUST",
		Recado:     "Guerra de Cidades hoje às 21:00h. Foco nas torres.",
		RecadoPor:  "SambariBlack",
		RecadoEm:   1729800000,
		MeuCargo:   GuildaCargoLider,
	}
	origem.Cidades[0] = GuildaCidade{Zona: 0, Dona: true, Imposto: 10, Convocados: 14}
	origem.Cidades[4] = GuildaCidade{Zona: 4, Convocados: 28}

	volta, err := DecodeGuildaAbre(origem.Encode())
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if volta != origem {
		t.Errorf("ida e volta mudou o corpo:\n ida:  %+v\n volta: %+v", origem, volta)
	}
}

// Uma guilda sem aliada, sem guerra e sem recado é o caso COMUM — toda guilda
// recém-criada é assim -, e os campos vazios têm de voltar vazios em vez de
// virar lixo do enchimento.
func TestGuildaAbreVazia(t *testing.T) {
	origem := GuildaAbreBody{GuildaID: 7, Nome: "CAPAVERDE", Capacidade: 250}
	volta, err := DecodeGuildaAbre(origem.Encode())
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if volta.Aliada != "" || volta.Guerra != "" || volta.Recado != "" || volta.RecadoPor != "" {
		t.Errorf("campo vazio voltou preenchido: %+v", volta)
	}
	if volta.RecadoEm != 0 {
		t.Errorf("RecadoEm = %d, want 0: nunca houve recado", volta.RecadoEm)
	}
}

// O recado é livre e vem do cliente, então o corpo tem de aguentar um que passe
// do limite sem estourar e sem partir um caractere acentuado no meio — meio "ç"
// no fim da linha é lixo na tela de todo mundo da guilda.
func TestGuildaAbreCortaRecadoSemPartirCaractere(t *testing.T) {
	origem := GuildaAbreBody{
		GuildaID: 1,
		Recado:   strings.Repeat("ç", GuildaRecadoMax), // 2 bytes cada: o dobro do limite
	}
	volta, err := DecodeGuildaAbre(origem.Encode())
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(volta.Recado) > GuildaRecadoMax {
		t.Errorf("recado voltou com %d bytes, acima do limite %d", len(volta.Recado), GuildaRecadoMax)
	}
	if !utf8Inteiro(volta.Recado) {
		t.Error("o corte partiu um caractere no meio")
	}
}

func utf8Inteiro(s string) bool {
	for _, r := range s {
		if r == 0xFFFD {
			return false
		}
	}
	return true
}

// Uma página do quadro de membros, com as três coisas que a linha carrega além
// do nome: cargo, online e status.
func TestGuildaMembrosIdaEVolta(t *testing.T) {
	origem := GuildaMembrosBody{
		Pagina: 0,
		Total:  249,
		Membros: []GuildaMembro{
			{Nome: "SambariBlack", Cargo: GuildaCargoLider, Online: true, VistoEm: 1729800000, Status: "OIS"},
			{Nome: "PLAYERABUSE", Cargo: GuildaCargoSub, Online: false, Status: ""},
			{Nome: "Tozorer2", Cargo: GuildaCargoMembro, Online: true, Status: "SALVEEE"},
		},
	}
	volta, err := DecodeGuildaMembros(origem.Encode())
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if volta.Pagina != origem.Pagina || volta.Total != origem.Total {
		t.Errorf("cabeçalho = %d/%d, want %d/%d", volta.Pagina, volta.Total, origem.Pagina, origem.Total)
	}
	if len(volta.Membros) != len(origem.Membros) {
		t.Fatalf("voltaram %d linhas, want %d", len(volta.Membros), len(origem.Membros))
	}
	for i := range origem.Membros {
		if volta.Membros[i] != origem.Membros[i] {
			t.Errorf("linha %d = %+v, want %+v", i, volta.Membros[i], origem.Membros[i])
		}
	}
}

// Uma página nunca leva mais linhas do que cabe. O Encode corta em vez de mandar
// um pacote que o cliente recusa: o cliente tem um buffer do tamanho da página,
// e uma linha a mais é memória de outra coisa.
func TestGuildaMembrosCortaNaPagina(t *testing.T) {
	origem := GuildaMembrosBody{Total: 250}
	for i := 0; i < GuildaMembrosPorPagina+10; i++ {
		origem.Membros = append(origem.Membros, GuildaMembro{Nome: "Membro"})
	}
	volta, err := DecodeGuildaMembros(origem.Encode())
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(volta.Membros) != GuildaMembrosPorPagina {
		t.Errorf("voltaram %d linhas, want %d", len(volta.Membros), GuildaMembrosPorPagina)
	}
}

// Uma página vazia é o que a última página de uma guilda de 40 membros devolve,
// e ela tem de decodificar sem erro.
func TestGuildaMembrosPaginaVazia(t *testing.T) {
	origem := GuildaMembrosBody{Pagina: 3, Total: 40}
	volta, err := DecodeGuildaMembros(origem.Encode())
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(volta.Membros) != 0 || volta.Total != 40 {
		t.Errorf("página vazia = %+v", volta)
	}
}

func TestGuildaBuffsIdaEVolta(t *testing.T) {
	origem := GuildaBuffsBody{}
	origem.Buffs[0] = GuildaBuff{Tipo: 1, Ativo: true, Restam: 2880}
	origem.Buffs[1] = GuildaBuff{Tipo: 2}
	origem.Buffs[2] = GuildaBuff{Tipo: 3, Ativo: true, Restam: 7200}
	origem.Buffs[3] = GuildaBuff{Tipo: 4}

	volta, err := DecodeGuildaBuffs(origem.Encode())
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if volta != origem {
		t.Errorf("ida e volta mudou os buffs:\n ida:   %+v\n volta: %+v", origem, volta)
	}
}

// Ativo com zero segundos não existe: é um buff que acabou de vencer, e o painel
// tem de desenhá-lo apagado sem precisar de um segundo pacote.
func TestGuildaBuffAtivoSemTempoVoltaDesligado(t *testing.T) {
	origem := GuildaBuffsBody{}
	origem.Buffs[0] = GuildaBuff{Tipo: 1, Ativo: true, Restam: 0}
	volta, err := DecodeGuildaBuffs(origem.Encode())
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if volta.Buffs[0].Ativo {
		t.Error("buff com 0 segundos voltou ativo")
	}
}

func TestGuildaPedeIdaEVolta(t *testing.T) {
	origem := GuildaPedeBody{Alvo: 1, Pagina: 3}
	volta, err := DecodeGuildaPede(origem.Encode())
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if volta != origem {
		t.Errorf("volta = %+v, want %+v", volta, origem)
	}
}

// Um corpo curto é recusado em vez de lido pela metade: os pedidos vêm do
// cliente, e um cliente remendado manda o que quiser.
func TestGuildaCorposCurtosSaoRecusados(t *testing.T) {
	if _, err := DecodeGuildaPede([]byte{1}); err == nil {
		t.Error("pede de 1 byte foi aceito")
	}
	if _, err := DecodeGuildaAbre(make([]byte, 10)); err == nil {
		t.Error("abre curto foi aceito")
	}
	if _, err := DecodeGuildaMembros([]byte{0, 1}); err == nil {
		t.Error("membros sem cabeçalho foi aceito")
	}
	if _, err := DecodeGuildaBuffs(make([]byte, 4)); err == nil {
		t.Error("buffs curto foi aceito")
	}
}

// O tamanho que vem no texto é do CLIENTE, e é a primeira coisa que um cliente
// remendado mente. Um tamanho maior que o limite tem de ser recusado, e um
// tamanho maior que o pacote não pode virar leitura fora do buffer.
func TestGuildaTextoRecusaTamanhoMentido(t *testing.T) {
	// Diz 5000 bytes num pacote de 4.
	mentiroso := []byte{0x88, 0x13, 'o', 'i'}
	if _, err := DecodeGuildaTexto(mentiroso, GuildaRecadoMax); err == nil {
		t.Error("tamanho acima do limite foi aceito")
	}
	// Diz 100 bytes num pacote que só tem 2.
	curto := []byte{100, 0, 'o', 'i'}
	if _, err := DecodeGuildaTexto(curto, GuildaRecadoMax); err == nil {
		t.Error("tamanho maior que o pacote foi aceito")
	}
}

func TestGuildaTextoIdaEVolta(t *testing.T) {
	origem := GuildaTextoBody{Texto: "Guerra às 21h, todos no Discord"}
	volta, err := DecodeGuildaTexto(origem.Encode(GuildaRecadoMax), GuildaRecadoMax)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if volta != origem {
		t.Errorf("volta = %+v, want %+v", volta, origem)
	}
}

// Um nome de 16 bytes ocupa o campo inteiro e não sobra NUL para terminá-lo. Ele
// tem de sair cortado, porque quem lê procura o NUL e senão atravessaria para o
// campo seguinte — o nome da guilda comeria o do líder.
func TestGuildaNomeLongoNaoVazaParaOCampoSeguinte(t *testing.T) {
	origem := GuildaAbreBody{
		Nome:  strings.Repeat("A", GuildaNomeMax+8),
		Lider: "Fulano",
	}
	volta, err := DecodeGuildaAbre(origem.Encode())
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(volta.Nome) >= GuildaNomeMax {
		t.Errorf("nome voltou com %d bytes: não sobrou NUL", len(volta.Nome))
	}
	if volta.Lider != "Fulano" {
		t.Errorf("líder = %q: o nome vazou para o campo seguinte", volta.Lider)
	}
}
