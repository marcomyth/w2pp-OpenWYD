package handler

import (
	"testing"
	"time"

	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/protocol"
	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/world"
)

// O painel de guilda agindo: promover, passar a liderança, expulsar, sair e o
// imposto da cidade. Os testes rodam no servidor de verdade porque estas quatro
// ações mexem em sessão e em entidade, e não em uma montagem de tela.

const guildaDoPainel = uint16(11)

// acaoDoPainel monta o corpo do 0x0F4C.
func acaoDoPainel(acao uint8, nome string) []byte {
	b := make([]byte, 1+protocol.GuildaNomeMax)
	b[0] = acao
	copy(b[1:], nome)
	return b
}

// quadroGuardado põe uma leitura fresca no cache, para os testes poderem ver se
// ela foi jogada fora.
func quadroGuardado(d *Dispatcher, guilda uint16) {
	d.guildaQuadro[guilda] = quadroDeGuilda{
		membros: []world.GuildMemberRecord{{Name: "Hero", Level: protocol.GuildaCargoLider}},
		lidoEm:  time.Unix(0, 0),
	}
}

// SOBRE SI MESMO SÓ VALE O "SAIR".
//
// O painel não oferece promover-se nem expulsar-se, mas um cliente remendado
// oferece. A recusa vem ANTES de o quadro guardado ser jogado fora, e é por isso
// que o cache é o que este teste olha: ele prova que a ação parou na porta, e não
// que ela rodou e não fez nada.
func TestGuildaAcaoSobreSiMesmoSoOSair(t *testing.T) {
	srv := startServerRelogioDasArenas(t, mortalDoCemiterio(), inicioDaVolta)
	enterWorldAs(t, srv.addr, "tester")

	naContaDoRelogio(t, srv, 7, func(w *world.World, d *Dispatcher, s *world.Session, e *world.Entity) {
		e.Guild, e.GuildLevel = guildaDoPainel, guildLeaderLevel
		for _, acao := range []uint8{protocol.GuildaAcaoPromove, protocol.GuildaAcaoLideranca, protocol.GuildaAcaoExpulsa} {
			quadroGuardado(d, guildaDoPainel)
			d.guildaAcao(w, s, protocol.Header{}, acaoDoPainel(acao, e.Name))
			if _, ok := d.guildaQuadro[guildaDoPainel]; !ok {
				t.Errorf("a acao %d sobre si mesmo passou da porta", acao)
			}
			if e.Guild != guildaDoPainel {
				t.Fatalf("a acao %d sobre si mesmo tirou o jogador da guilda", acao)
			}
		}

		// E o sair, que é a única sobre si mesmo, passa.
		quadroGuardado(d, guildaDoPainel)
		d.guildaAcao(w, s, protocol.Header{}, acaoDoPainel(protocol.GuildaAcaoSai, ""))
		if e.Guild != 0 {
			t.Error("o sair nao tirou o jogador da guilda")
		}
		if _, ok := d.guildaQuadro[guildaDoPainel]; ok {
			t.Error("o quadro guardado sobreviveu ao sair")
		}
	})
}

// A AÇÃO JOGA FORA O QUADRO GUARDADO, senão o próximo pedido do painel devolve a
// lista de meio minuto atrás — com quem acabou de ser expulso ainda nela.
func TestGuildaAcaoEsqueceOQuadro(t *testing.T) {
	srv := startServerRelogioDasArenas(t, mortalDoCemiterio(), inicioDaVolta)
	enterWorldAs(t, srv.addr, "tester")

	naContaDoRelogio(t, srv, 7, func(w *world.World, d *Dispatcher, s *world.Session, e *world.Entity) {
		e.Guild, e.GuildLevel = guildaDoPainel, guildLeaderLevel
		quadroGuardado(d, guildaDoPainel)
		d.guildaAcao(w, s, protocol.Header{}, acaoDoPainel(protocol.GuildaAcaoExpulsa, "Outro"))
		if _, ok := d.guildaQuadro[guildaDoPainel]; ok {
			t.Error("o quadro continuou guardado depois de uma expulsao")
		}
	})
}

// SEM GUILDA NÃO HÁ AÇÃO, e o quadro de uma guilda alheia não é tocado.
func TestGuildaAcaoSemGuildaNaoAge(t *testing.T) {
	srv := startServerRelogioDasArenas(t, mortalDoCemiterio(), inicioDaVolta)
	enterWorldAs(t, srv.addr, "tester")

	naContaDoRelogio(t, srv, 7, func(w *world.World, d *Dispatcher, s *world.Session, e *world.Entity) {
		e.Guild = 0
		quadroGuardado(d, guildaDoPainel)
		d.guildaAcao(w, s, protocol.Header{}, acaoDoPainel(protocol.GuildaAcaoExpulsa, "Outro"))
		if _, ok := d.guildaQuadro[guildaDoPainel]; !ok {
			t.Error("quem nao tem guilda apagou o quadro de uma")
		}
	})
}

// O IMPOSTO VAI NA CIDADE QUE A GUILDA COBRA, E NÃO NA QUE O PACOTE PEDE.
//
// É a regra inteira do 0x0F4D: a zona viaja só para o log. Aceitá-la seria deixar
// um cliente remendado mexer no imposto da cidade dos outros.
func TestGuildaImpostoIgnoraAZonaDoPedido(t *testing.T) {
	srv := startServerRelogioDasArenas(t, mortalDoCemiterio(), inicioDaVolta)
	enterWorldAs(t, srv.addr, "tester")

	naContaDoRelogio(t, srv, 7, func(w *world.World, d *Dispatcher, s *world.Session, e *world.Entity) {
		e.Guild, e.GuildLevel = guildaDoPainel, guildLeaderLevel
		d.guildZones[2] = world.GuildZone{Zone: 2, ChargeGuild: guildaDoPainel, CityTax: 5}
		d.guildZones[4] = world.GuildZone{Zone: 4, ChargeGuild: 77, CityTax: 7}

		// O pacote pede a zona 4, que é dos outros.
		d.guildaImposto(w, s, protocol.Header{}, []byte{4, 15})

		if d.guildZones[2].CityTax != 15 {
			t.Errorf("a cidade cobrada ficou com %d%%, quero 15%%", d.guildZones[2].CityTax)
		}
		if d.guildZones[4].CityTax != 7 {
			t.Errorf("a cidade dos outros virou %d%%", d.guildZones[4].CityTax)
		}
	})
}

// QUEM NÃO É LÍDER NÃO MEXE NO IMPOSTO. A regra é do guildTax, e este teste é o
// que prova que o botão do painel não passa por cima dela.
func TestGuildaImpostoSoOLider(t *testing.T) {
	srv := startServerRelogioDasArenas(t, mortalDoCemiterio(), inicioDaVolta)
	enterWorldAs(t, srv.addr, "tester")

	naContaDoRelogio(t, srv, 7, func(w *world.World, d *Dispatcher, s *world.Session, e *world.Entity) {
		e.Guild, e.GuildLevel = guildaDoPainel, guildLeaderLevel-1
		d.guildZones[2] = world.GuildZone{Zone: 2, ChargeGuild: guildaDoPainel, CityTax: 5}
		d.guildaImposto(w, s, protocol.Header{}, []byte{2, 15})
		if d.guildZones[2].CityTax != 5 {
			t.Errorf("um sub-lider mudou o imposto para %d%%", d.guildZones[2].CityTax)
		}
	})
}

// O TETO É DE 30%, e acima dele o pacote nem chega a virar comando.
func TestGuildaImpostoAcimaDoTetoNaoMuda(t *testing.T) {
	srv := startServerRelogioDasArenas(t, mortalDoCemiterio(), inicioDaVolta)
	enterWorldAs(t, srv.addr, "tester")

	naContaDoRelogio(t, srv, 7, func(w *world.World, d *Dispatcher, s *world.Session, e *world.Entity) {
		e.Guild, e.GuildLevel = guildaDoPainel, guildLeaderLevel
		d.guildZones[2] = world.GuildZone{Zone: 2, ChargeGuild: guildaDoPainel, CityTax: 5}
		d.guildaImposto(w, s, protocol.Header{}, []byte{2, protocol.GuildaImpostoMax + 1})
		if d.guildZones[2].CityTax != 5 {
			t.Errorf("a taxa acima do teto passou: %d%%", d.guildZones[2].CityTax)
		}
	})
}
