package handler

import (
	"io"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/protocol"
	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/world"
)

// painelDeGuilda monta um despachante e um mundo com uma guilda de pé, sem rede
// e sem banco: o que este arquivo testa é a MONTAGEM das abas, e ela é toda
// decidida em memória.
func painelDeGuilda(t *testing.T) (*Dispatcher, *world.World) {
	t.Helper()
	d := New(Config{Log: slog.New(slog.NewTextHandler(io.Discard, nil))})
	w := world.New(world.Config{GridDim: 64}, slog.New(slog.NewTextHandler(io.Discard, nil)), nil, nil)
	return d, w
}

// liderDaGuilda é quem abre o painel nos testes. Ele não precisa estar numa
// sessão: a montagem da aba recebe a presença pronta, e é justamente essa
// separação que deixa estes testes rodarem sem um servidor de pé.
func liderDaGuilda(guilda uint16) *world.Entity {
	return &world.Entity{ID: 1, Name: "SambariBlack", Guild: guilda,
		GuildLevel: protocol.GuildaCargoLider, HP: 100, Mode: world.MobUser}
}

// presenca monta uma presença à mão: quantos online, quantos em cada cidade, e
// quem pelo nome.
func presenca(online int16, porCidade map[int]int16, nomes ...string) presencaDeGuilda {
	p := presencaDeGuilda{online: online, conectados: map[string]bool{}}
	for zona, n := range porCidade {
		p.porCidade[zona] = n
	}
	for _, n := range nomes {
		p.conectados[strings.ToLower(n)] = true
	}
	return p
}

const guildaDeTeste = uint16(11)

// A aba Informações transporta o que já existe: nome, fama, recado, teto, e o
// cargo de quem abriu.
func TestGuildaInfoTransportaOQueJaExiste(t *testing.T) {
	d, w := painelDeGuilda(t)
	w.SetGuildName(guildaDeTeste, "REDDRAGONS")
	w.SetGuildFame(guildaDeTeste, 1450)
	w.SetGuildMemberCap(guildaDeTeste, 250)
	escrito := time.Date(2026, 9, 21, 18, 30, 0, 0, time.UTC)
	w.SetGuildNotice(guildaDeTeste, "Guerra às 21h", "SambariBlack", escrito)

	e := liderDaGuilda(guildaDeTeste)
	quadro := []world.GuildMemberRecord{{Name: "SambariBlack", Level: protocol.GuildaCargoLider}}

	corpo := d.montaInfoDaGuilda(w, e, quadro, presenca(1, nil, "SambariBlack"))

	if corpo.Nome != "REDDRAGONS" {
		t.Errorf("nome = %q", corpo.Nome)
	}
	if corpo.Fama != 1450 {
		t.Errorf("fama = %d, want 1450", corpo.Fama)
	}
	if corpo.Capacidade != 250 {
		t.Errorf("capacidade = %d, want 250", corpo.Capacidade)
	}
	if corpo.Recado != "Guerra às 21h" || corpo.RecadoPor != "SambariBlack" {
		t.Errorf("recado = %q por %q", corpo.Recado, corpo.RecadoPor)
	}
	if corpo.RecadoEm != escrito.Unix() {
		t.Errorf("RecadoEm = %d, want %d", corpo.RecadoEm, escrito.Unix())
	}
	if corpo.Lider != "SambariBlack" {
		t.Errorf("líder = %q", corpo.Lider)
	}
	if corpo.MeuCargo != protocol.GuildaCargoLider {
		t.Errorf("meu cargo = %d", corpo.MeuCargo)
	}
}

// Aliada e guerra saem em NOME, não em id: o cliente não tem registro de guildas
// e não teria como traduzir um número.
func TestGuildaInfoResolveAliadaEGuerraEmNome(t *testing.T) {
	d, w := painelDeGuilda(t)
	w.SetGuildName(guildaDeTeste, "REDDRAGONS")
	w.SetGuildName(12, "IMPERIAL")
	w.SetGuildName(13, "BLOODLUST")
	d.guildAllies[guildaDeTeste] = 12
	d.guildWars[guildaDeTeste] = 13

	corpo := d.montaInfoDaGuilda(w, liderDaGuilda(guildaDeTeste), nil, presenca(0, nil))

	if corpo.Aliada != "IMPERIAL" {
		t.Errorf("aliada = %q, want IMPERIAL", corpo.Aliada)
	}
	if corpo.Guerra != "BLOODLUST" {
		t.Errorf("guerra = %q, want BLOODLUST", corpo.Guerra)
	}
}

// Os convocados de cada cidade e a marca de quem domina cada uma chegam
// separados à linha da cidade, e a ordem das cinco tem de ser a das zonas.
func TestGuildaInfoMontaAsCincoCidades(t *testing.T) {
	d, w := painelDeGuilda(t)
	w.SetGuildName(guildaDeTeste, "REDDRAGONS")
	// A guilda domina Armia (zona 0) e cobra 10%; Noatum (zona 4) é de outra.
	d.guildZones[0].ChargeGuild = guildaDeTeste
	d.guildZones[0].CityTax = 10
	d.guildZones[4].ChargeGuild = 99

	corpo := d.montaInfoDaGuilda(w, liderDaGuilda(guildaDeTeste), nil,
		presenca(42, map[int]int16{0: 14, 4: 28}))

	if corpo.Online != 42 {
		t.Errorf("online = %d, want 42", corpo.Online)
	}
	if !corpo.Cidades[0].Dona || corpo.Cidades[0].Imposto != 10 {
		t.Errorf("Armia: dona=%v imposto=%d", corpo.Cidades[0].Dona, corpo.Cidades[0].Imposto)
	}
	if corpo.Cidades[4].Dona {
		t.Error("Noatum saiu como nossa: ela é da guilda 99")
	}
	if corpo.Cidades[0].Convocados != 14 || corpo.Cidades[4].Convocados != 28 {
		t.Errorf("convocados = %d em Armia, %d em Noatum",
			corpo.Cidades[0].Convocados, corpo.Cidades[4].Convocados)
	}
	for i := range corpo.Cidades {
		if corpo.Cidades[i].Zona != uint8(i) {
			t.Errorf("cidade %d saiu com zona %d: a ordem tem de ser a das zonas", i, corpo.Cidades[i].Zona)
		}
	}
}

// O total nunca pode sair menor que o número de conectados. Quando o banco não
// responde o quadro vem vazio, e "42 online de 0 membros" seria a tela mentindo
// sobre uma coisa que ela mesma acabou de contar.
func TestGuildaInfoTotalNuncaMenorQueOnline(t *testing.T) {
	d, w := painelDeGuilda(t)
	w.SetGuildName(guildaDeTeste, "REDDRAGONS")

	// Quadro vazio (o banco falhou) e dois conectados.
	corpo := d.montaInfoDaGuilda(w, liderDaGuilda(guildaDeTeste), nil, presenca(2, nil))

	if corpo.Membros < corpo.Online {
		t.Errorf("membros = %d, online = %d: o total ficou menor que os conectados",
			corpo.Membros, corpo.Online)
	}
}

// A capacidade zero de uma linha escrita à mão no banco não pode virar "249/0"
// na tela.
func TestGuildaCapacidadeZeroCaiNoPadrao(t *testing.T) {
	if got := guildaCapacidade(0); got != guildaCapacidadePadrao {
		t.Errorf("capacidade(0) = %d, want %d", got, guildaCapacidadePadrao)
	}
	if got := guildaCapacidade(80); got != 80 {
		t.Errorf("capacidade(80) = %d, want 80", got)
	}
}

// O líder é a primeira linha do quadro, mas só quando ela é de fato cargo 9:
// chamar um sublíder de líder é pior do que não dizer nada.
func TestGuildaLiderSoQuandoECargoNove(t *testing.T) {
	comLider := []world.GuildMemberRecord{
		{Name: "SambariBlack", Level: protocol.GuildaCargoLider},
		{Name: "Outro", Level: protocol.GuildaCargoSub},
	}
	if got := guildaLider(comLider); got != "SambariBlack" {
		t.Errorf("líder = %q", got)
	}
	semLider := []world.GuildMemberRecord{{Name: "Sub", Level: protocol.GuildaCargoSub}}
	if got := guildaLider(semLider); got != "" {
		t.Errorf("líder = %q, want vazio: a guilda não tem cargo 9", got)
	}
	if got := guildaLider(nil); got != "" {
		t.Errorf("líder de quadro vazio = %q", got)
	}
}

// A paginação recorta o quadro e marca quem está online pelo NOME, porque o
// quadro vem do banco e as sessões são deste processo.
func TestGuildaPaginaDeMembros(t *testing.T) {
	pres := presenca(1, nil, "Conectado")

	var quadro []world.GuildMemberRecord
	quadro = append(quadro, world.GuildMemberRecord{Name: "Conectado", Level: protocol.GuildaCargoLider})
	for i := 0; i < protocol.GuildaMembrosPorPagina+5; i++ {
		quadro = append(quadro, world.GuildMemberRecord{Name: "Ausente"})
	}

	primeira := guildaPaginaDeMembros(pres, quadro, 0)
	if len(primeira.Membros) != protocol.GuildaMembrosPorPagina {
		t.Fatalf("página 0 trouxe %d linhas, want %d", len(primeira.Membros), protocol.GuildaMembrosPorPagina)
	}
	if primeira.Total != int16(len(quadro)) {
		t.Errorf("total = %d, want %d", primeira.Total, len(quadro))
	}
	if !primeira.Membros[0].Online {
		t.Error("o membro conectado não foi marcado como online")
	}
	if primeira.Membros[1].Online {
		t.Error("um membro ausente foi marcado como online")
	}

	segunda := guildaPaginaDeMembros(pres, quadro, 1)
	if len(segunda.Membros) != 6 {
		t.Errorf("página 1 trouxe %d linhas, want 6", len(segunda.Membros))
	}

	// Uma página além do fim devolve vazio em vez de estourar: o cliente pede a
	// página seguinte enquanto Total disser que há mais, e um total que mudou
	// entre um pedido e outro leva exatamente a isso.
	alem := guildaPaginaDeMembros(pres, quadro, 9)
	if len(alem.Membros) != 0 {
		t.Errorf("página além do fim trouxe %d linhas", len(alem.Membros))
	}
}

// O quadro guardado tem de ser esquecido quando a guilda muda, senão um expulso
// fica na lista por meio minuto.
func TestGuildaEsqueceQuadro(t *testing.T) {
	d, _ := painelDeGuilda(t)
	d.guildaQuadro[guildaDeTeste] = quadroDeGuilda{
		membros: []world.GuildMemberRecord{{Name: "Alguém"}},
		lidoEm:  time.Now(),
	}
	d.guildaEsqueceQuadro(guildaDeTeste)
	if _, ok := d.guildaQuadro[guildaDeTeste]; ok {
		t.Error("o quadro continuou guardado")
	}
	// Guilda zero não é guilda: não pode criar nem apagar linha nenhuma.
	d.guildaEsqueceQuadro(0)
}

func TestLimpaLinhaDeTexto(t *testing.T) {
	cases := []struct{ entrada, quer string }{
		{"  Guerra às 21h  ", "Guerra às 21h"},
		{"duas\nlinhas", "duas linhas"},
		{"com\ttab", "com tab"},
		{"\x00nulo", "nulo"},
		{"   ", ""},
	}
	for _, c := range cases {
		if got := limpaLinhaDeTexto(c.entrada); got != c.quer {
			t.Errorf("limpa(%q) = %q, want %q", c.entrada, got, c.quer)
		}
	}
}
