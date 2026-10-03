package handler

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"io"
	"log/slog"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/jeanluca/w2pp-openwyd/internal/droprule"
	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/protocol"
	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/world"
)

// Os testes dos consertos pedidos pela revisão do Painel de Drop das Fadas.

func corpoDoMuda(acao uint8, item int16) []byte {
	b := make([]byte, 4)
	b[0] = acao
	binary.LittleEndian.PutUint16(b[2:], uint16(item))
	return b
}

func corpoDoPede(tipo uint8) []byte {
	b := make([]byte, 8+protocol.FadasNome)
	b[0] = tipo
	return b
}

// quadrosAteASentinela lê os quadros do painel até o 0x0F71 (a resposta de uma
// busca, que o teste manda por último) e devolve os tipos e os corpos, em ordem.
func quadrosAteASentinela(t *testing.T, lidos func() (protocol.Header, []byte, bool)) (tipos []protocol.Type, corpos [][]byte) {
	t.Helper()
	fim := time.Now().Add(3 * time.Second)
	for time.Now().Before(fim) {
		h, p, ok := lidos()
		if !ok {
			continue
		}
		switch h.Type {
		case protocol.MsgFadasMonstros:
			return tipos, corpos
		case protocol.MsgFadasFiltro, protocol.MsgFadasFaixas, protocol.MsgFadasDrops:
			tipos, corpos = append(tipos, h.Type), append(corpos, p)
		}
	}
	t.Fatal("a sentinela (0x0F71) não chegou")
	return nil, nil
}

// O FREIO COBRE TODOS OS PEDIDOS, e o freado não some calado.
//
// Antes, só as listas tinham freio. O pedido do filtro (tipo 4), o de drops
// (tipo 3) e toda mudança (0x0F73) rodavam sem limite dentro do laço do jogo.
func TestFadasFreioCobreDropsFiltroEMudanca(t *testing.T) {
	srv, c := mesaDaLixeira(t)
	le := func() (protocol.Header, []byte, bool) { return readMaybeHeaderRaw(t, c) }

	var depois []int16
	naContaDoRelogio(t, srv, 7, func(w *world.World, d *Dispatcher, s *world.Session, e *world.Entity) {
		e.Equip[fairyEquipSlot] = world.Item{Index: fadaAzul7Dias}
		// Duas mudanças no mesmo instante: só a primeira vale.
		d.fadasMuda(w, s, protocol.Header{}, corpoDoMuda(protocol.FadasMudaPoe, itemFadaTesteA))
		d.fadasMuda(w, s, protocol.Header{}, corpoDoMuda(protocol.FadasMudaPoe, itemFadaTesteB))
		depois = slices.Clone(e.FadaFiltro)
		// Dois pedidos do filtro e dois de drops, também no mesmo instante.
		d.fadasPede(w, s, protocol.Header{}, corpoDoPede(protocol.FadasPedeFiltro))
		d.fadasPede(w, s, protocol.Header{}, corpoDoPede(protocol.FadasPedeFiltro))
		d.fadasPede(w, s, protocol.Header{}, corpoDoPede(protocol.FadasPedeDrops))
		d.fadasPede(w, s, protocol.Header{}, corpoDoPede(protocol.FadasPedeDrops))
		// A sentinela.
		busca := corpoDoPede(protocol.FadasPedeBusca)
		copy(busca[8:], "zzzznenhum")
		d.fadasPede(w, s, protocol.Header{}, busca)
	})
	if !slices.Equal(depois, []int16{itemFadaTesteA}) {
		t.Fatalf("lista depois das duas mudanças = %v; a segunda devia ter sido freada", depois)
	}

	tipos, corpos := quadrosAteASentinela(t, le)
	querTipos := []protocol.Type{
		protocol.MsgFadasFiltro, protocol.MsgFadasFaixas, // a mudança aceita
		protocol.MsgFadasFiltro,                          // a mudança freada: o estado, sem as faixas
		protocol.MsgFadasFiltro, protocol.MsgFadasFaixas, // o pedido do filtro aceito
		protocol.MsgFadasFiltro, // o pedido do filtro freado: só o estado
		protocol.MsgFadasDrops,  // o pedido de drops aceito (versão velha: só o 0x0F72); o freado não responde
	}
	if !slices.Equal(tipos, querTipos) {
		t.Fatalf("respostas = %04x; queria %04x", tipos, querTipos)
	}
	// A mudança freada responde o ESTADO QUE VALE (um item, o A), com o motivo próprio.
	freada := corpos[2]
	if freada[2] != protocol.FadasMotivoDevagar || freada[3] != 1 ||
		int16(binary.LittleEndian.Uint16(freada[4:])) != itemFadaTesteA {
		t.Errorf("resposta da mudança freada = % x; queria motivo %d e a lista com o item A",
			freada, protocol.FadasMotivoDevagar)
	}

	// Passado o intervalo, a mudança entra.
	naContaDoRelogio(t, srv, 7, func(w *world.World, d *Dispatcher, s *world.Session, e *world.Entity) {
		s.FadasMudaEm = s.FadasMudaEm.Add(-fadasCliqueIntervalo)
		d.fadasMuda(w, s, protocol.Header{}, corpoDoMuda(protocol.FadasMudaPoe, itemFadaTesteB))
		depois = slices.Clone(e.FadaFiltro)
	})
	if len(depois) != 2 {
		t.Errorf("lista depois do intervalo = %v; queria os dois itens", depois)
	}
}

func TestFadasFreia(t *testing.T) {
	var ultimo time.Time
	t0 := time.Date(2026, 10, 3, 12, 0, 0, 0, time.UTC)
	if fadasFreia(&ultimo, t0, time.Second) {
		t.Error("freou o primeiro pedido")
	}
	if !fadasFreia(&ultimo, t0.Add(999*time.Millisecond), time.Second) {
		t.Error("não freou dentro do intervalo")
	}
	if !ultimo.Equal(t0) {
		t.Error("o pedido freado empurrou a marca: uma rajada nunca mais passaria")
	}
	if fadasFreia(&ultimo, t0.Add(time.Second), time.Second) {
		t.Error("freou no fim do intervalo")
	}
	// O relógio que andou para trás não pode prender o jogador.
	if fadasFreia(&ultimo, t0.Add(-time.Hour), time.Second) {
		t.Error("freou com o relógio para trás")
	}
}

// A FAIXA LARGA É CALCULADA UMA VEZ. Ela passa por todos os monstros que dão o
// item, e o pedido do filtro a faz para até 60 itens.
func TestFadasFaixaLargaFicaGuardada(t *testing.T) {
	d, w, _ := mundoDasFadas(t, 0,
		blocoDaFada("Bicho", moldeDaFada("Bicho", 150, map[int]int16{3: itemFadaTesteA})),
		blocoDaFada("Outro", moldeDaFada("Outro", 200, map[int]int16{3: itemFadaTesteA})),
	)
	agora := time.Date(2026, 10, 3, 12, 0, 0, 0, time.UTC)
	d.now = func() time.Time { return agora }
	c := d.fadasCatalogo(w)

	d.fadasFaixaLarga(w, c, itemFadaTesteA)
	if _, ok := c.largas[itemFadaTesteA]; !ok {
		t.Fatal("a faixa larga não ficou guardada")
	}
	// A prova de que a segunda chamada não refaz a conta: devolve o que está guardado.
	marca := []protocol.FadasFaixa{{Efeito: 250, Min: 1, Max: 2}}
	c.largas[itemFadaTesteA] = marca
	if got := d.fadasFaixaLarga(w, c, itemFadaTesteA); !slices.Equal(got, marca) {
		t.Errorf("a segunda chamada refez a conta: %v", got)
	}
	// Item que ninguém dá não entra no guardado (o índice vem do cliente).
	d.fadasFaixaLarga(w, c, 6000)
	if _, ok := c.largas[6000]; ok {
		t.Error("guardou a faixa de um item que nenhum monstro dá")
	}
	// Vencido o prazo, refaz.
	agora = agora.Add(fadasLargasValidade)
	if got := d.fadasFaixaLarga(w, c, itemFadaTesteA); slices.Equal(got, marca) {
		t.Error("depois do prazo ainda devolveu a faixa guardada")
	}
	// Catálogo refeito, guardado zerado.
	c.largas[itemFadaTesteA] = marca
	w.RegisterGenerators([]*world.Generator{blocoDaFada("Bicho", moldeDaFada("Bicho", 150, map[int]int16{3: itemFadaTesteA}))})
	agora = agora.Add(fadasCatalogoValidade)
	c = d.fadasCatalogo(w)
	if got := d.fadasFaixaLarga(w, c, itemFadaTesteA); slices.Equal(got, marca) {
		t.Error("o catálogo foi refeito e a faixa guardada ficou")
	}
}

// O QUE NÃO PODE SER PROTEGIDO NUNCA É DESCARTADO. A Mesa pode dar o 454, que o
// painel mostra e fadaAplica recusa pôr na lista.
func TestFadaFiltroNaoDescartaOQueNaoPodeSerProtegido(t *testing.T) {
	const semDefesa int16 = 454
	if fadaItemValido(semDefesa) {
		t.Fatal("o 454 passou a ser protegível: o teste precisa de outro item")
	}
	d, w, killer := mundoDasFadas(t, fadaAzul7Dias, bichoDoFiltro())
	d.dropRules = droprule.NewTable([]droprule.Rule{
		{Mob: "Bicho", Item: semDefesa, Chance: droprule.MaxChance},
		{Mob: "Bicho", Item: itemFadaTesteB, Chance: droprule.MaxChance},
	})
	killer.FadaFiltroLigado, killer.FadaFiltro = true, []int16{itemFadaTesteC}

	c := d.fadasCatalogo(w)
	if !slices.Contains(d.dropsVisiveis(w, &c.lista[c.porMolde[droprule.Canonical("Bicho")]]), semDefesa) {
		t.Fatal("o painel não mostra o 454: o teste não mede o caso")
	}
	mataMonstroDaFada(t, d, w, killer, 0)
	if _, ok := carryHas(killer, semDefesa); !ok {
		t.Error("o filtro descartou um item que o jogador não tinha como proteger")
	}
	if _, ok := carryHas(killer, itemFadaTesteB); ok {
		t.Error("o item fora da lista entrou: o filtro parou de funcionar")
	}
}

// O QUE NÃO COUBE NO 0x0F72 NUNCA É DESCARTADO: o cliente não o recebeu, o
// jogador não o viu e não pôde protegê-lo.
func TestFadaFiltroNaoDescartaOQueFicouAlemDoCorte(t *testing.T) {
	const primeiro int16 = 1000
	const quantos = protocol.FadasDropsMax + 10
	regras := make([]droprule.Rule, 0, quantos)
	for i := range quantos {
		regras = append(regras, droprule.Rule{Mob: "Bicho", Item: primeiro + int16(i), Chance: 1})
	}
	d, w, killer := mundoDasFadas(t, fadaAzul7Dias, blocoDaFada("Bicho", moldeDaFada("Bicho", 30, nil)))
	d.dropRules = droprule.NewTable(regras)
	killer.FadaFiltroLigado, killer.FadaFiltro = true, []int16{itemFadaTesteC}
	g := w.GeneratorAt(0)
	mob := w.Entity(w.SpawnMobAt(world.MobSpawn{Template: g.LeaderTmpl, TemplateName: "Bicho", X: 6, Y: 5}))

	c := d.fadasCatalogo(w)
	m := &c.lista[c.porMolde[droprule.Canonical("Bicho")]]
	if n := len(d.dropsVisiveis(w, m)); n != quantos {
		t.Fatalf("o monstro tem %d drops; o teste precisa de %d", n, quantos)
	}
	noPainel := d.fadasDropsDoPainel(w, m)
	if len(noPainel) != protocol.FadasDropsMax {
		t.Fatalf("a lista do painel tem %d itens; queria %d", len(noPainel), protocol.FadasDropsMax)
	}
	ultimoMostrado, primeiroCortado := noPainel[len(noPainel)-1], primeiro+int16(protocol.FadasDropsMax)
	if !d.fadaDescarta(w, killer, mob, ultimoMostrado) {
		t.Error("não descartou o último item que o painel mostra")
	}
	if d.fadaDescarta(w, killer, mob, primeiroCortado) {
		t.Error("descartou um item que ficou além do corte do 0x0F72")
	}
}

// O DESCARTE DEIXA RASTRO, e a linha da Mesa não diz que o item caiu quando ele
// foi descartado.
func TestFadaFiltroDescarteDeixaRastro(t *testing.T) {
	d, w, killer := mundoDasFadas(t, fadaAzul7Dias, bichoDoFiltro())
	var saida bytes.Buffer
	d.log = slog.New(slog.NewTextHandler(&saida, nil))
	d.dropRules = mesaDoFiltro()
	killer.FadaFiltroLigado, killer.FadaFiltro = true, []int16{itemFadaTesteA}

	mataMonstroDaFada(t, d, w, killer, 0)

	var descarte, mesa string
	for _, linha := range strings.Split(saida.String(), "\n") {
		switch {
		case strings.Contains(linha, "saque descartado"):
			descarte = linha
		case strings.Contains(linha, "drop table hit"):
			mesa = linha
		}
	}
	for _, quer := range []string{"char=Heroi", "conta=", "mob=Bicho", "item=2316"} {
		if !strings.Contains(descarte, quer) {
			t.Errorf("a linha do descarte não tem %q: %q", quer, descarte)
		}
	}
	if !strings.Contains(mesa, "item not delivered") || !strings.Contains(mesa, "delivered=0") {
		t.Errorf("a linha da Mesa diz que o item caiu, e ele foi descartado: %q", mesa)
	}

	// Sem filtro, a linha da Mesa é a de sempre, e não há linha de descarte.
	saida.Reset()
	killer.FadaFiltroLigado = false
	mataMonstroDaFada(t, d, w, killer, 0)
	if s := saida.String(); strings.Contains(s, "saque descartado") || strings.Contains(s, "not delivered") ||
		!strings.Contains(s, `msg="drop table hit"`) || !strings.Contains(s, "delivered=1") {
		t.Errorf("sem filtro, o log ficou: %q", s)
	}
}

// A GRAVAÇÃO QUE FALHA DEVOLVE A MEMÓRIA AO QUE O BANCO TEM, e o jogador recebe
// esse estado com o motivo 5. Antes, a memória ficava com o estado novo e o
// banco com o antigo: o que a tela mostrava não era o que valia no relogin.
func TestFadasGravacaoQueFalhaVoltaAoEstadoDoBanco(t *testing.T) {
	srv, c := mesaDaLixeira(t)
	espera := func(quer func([]byte) bool) []byte {
		t.Helper()
		_, corpo, ok := quadroAte(t, c, 3*time.Second, func(h protocol.Header, p []byte) bool {
			return h.Type == protocol.MsgFadasFiltro && quer(p)
		})
		if !ok {
			t.Fatal("o 0x0F74 esperado não chegou")
		}
		return corpo
	}

	// Uma mudança que grava: o item A fica protegido, no banco também.
	naContaDoRelogio(t, srv, 7, func(w *world.World, d *Dispatcher, s *world.Session, e *world.Entity) {
		e.Equip[fairyEquipSlot] = world.Item{Index: fadaAzul7Dias}
		d.fadasMuda(w, s, protocol.Header{}, corpoDoMuda(protocol.FadasMudaPoe, itemFadaTesteA))
	})
	espera(func(p []byte) bool { return p[2] == protocol.FadasMotivoNenhum && p[3] == 1 })
	srv.noLaco(t, func(w *world.World, _ *Dispatcher) { w.WaitSaves() })

	// O banco cai, e o jogador liga o filtro.
	naContaDoRelogio(t, srv, 7, func(w *world.World, d *Dispatcher, s *world.Session, _ *world.Entity) {
		db := w.Persistence().(*fakeDB)
		db.mu.Lock()
		db.fadaFiltroErr = errors.New("banco fora")
		db.mu.Unlock()
		s.FadasMudaEm = time.Time{}
		d.fadasMuda(w, s, protocol.Header{}, corpoDoMuda(protocol.FadasMudaLiga, 0))
	})
	// Primeiro a resposta do pedido (ligado), depois o aviso da falha (desligado).
	espera(func(p []byte) bool { return p[0] == 1 && p[2] == protocol.FadasMotivoNenhum })
	aviso := espera(func(p []byte) bool { return p[2] == protocol.FadasMotivoNaoGravou })
	if aviso[0] != 0 || aviso[3] != 1 {
		t.Errorf("aviso da falha = % x; queria desligado e um item (o estado do banco)", aviso)
	}
	var ligado bool
	var itens []int16
	naContaDoRelogio(t, srv, 7, func(_ *world.World, _ *Dispatcher, _ *world.Session, e *world.Entity) {
		ligado, itens = e.FadaFiltroLigado, slices.Clone(e.FadaFiltro)
	})
	if ligado || !slices.Equal(itens, []int16{itemFadaTesteA}) {
		t.Errorf("memória depois da falha: ligado %v, itens %v; queria o estado do banco", ligado, itens)
	}
}

// bancoQueSegura segura a gravação do filtro até o teste soltar.
type bancoQueSegura struct {
	world.NopPersistence
	solta chan struct{}
}

func (b bancoQueSegura) SaveFadaFiltro(context.Context, int64, int, bool, []int16) error {
	<-b.solta
	return nil
}

// QUEM SAI E VOLTA antes de a mudança chegar ao banco entra com o estado que
// ainda está a caminho, e não com o que o banco devolveu.
func TestFadaFiltroDoLoginUsaOQueEstaACaminho(t *testing.T) {
	b := bancoQueSegura{solta: make(chan struct{})}
	w := world.New(world.Config{GridDim: 16}, slog.New(slog.NewTextHandler(io.Discard, nil)), b, nil)
	defer func() { close(b.solta); w.WaitSaves() }()

	// Sem nada a caminho, vale o banco (limpo como sempre).
	if ligado, itens := fadaFiltroDoLogin(w, 7, 1, true, []int16{itemFadaTesteA, 100}); !ligado ||
		!slices.Equal(itens, []int16{itemFadaTesteA}) {
		t.Fatalf("sem fila: ligado %v itens %v", ligado, itens)
	}

	// O jogador desligou e saiu; a gravação ainda não voltou.
	antes := world.FadaFiltroSalvo{Conta: 7, Slot: 1, Ligado: true, Itens: []int16{itemFadaTesteA}}
	novo := world.FadaFiltroSalvo{Conta: 7, Slot: 1, Ligado: false, Itens: []int16{itemFadaTesteA}}
	w.GravaFadaFiltro(antes, novo, nil)

	// O banco ainda diz "ligado"; o login tem de entrar desligado.
	if ligado, itens := fadaFiltroDoLogin(w, 7, 1, true, []int16{itemFadaTesteA}); ligado ||
		!slices.Equal(itens, []int16{itemFadaTesteA}) {
		t.Errorf("com a gravação a caminho: ligado %v itens %v; queria desligado", ligado, itens)
	}
	// Outro personagem da mesma conta não tem nada com isso.
	if ligado, _ := fadaFiltroDoLogin(w, 7, 2, true, []int16{itemFadaTesteA}); !ligado {
		t.Error("a fila de um personagem mudou o filtro de outro")
	}
}
