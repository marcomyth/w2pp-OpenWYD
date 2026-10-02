package handler

import (
	"encoding/binary"
	"slices"
	"testing"
	"time"

	"github.com/jeanluca/w2pp-openwyd/internal/droprule"
	"github.com/jeanluca/w2pp-openwyd/internal/regiao"
	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/content"
	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/protocol"
	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/refine"
	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/world"
)

const (
	itemFadaTesteA int16 = 2405 // Âmago de Andaluz B
	itemFadaTesteB int16 = 2316 // Ovo de Fenrir
	itemFadaTesteC int16 = 412  // Poeira de Oriharucon
)

// moldeDaFada monta um molde de monstro com nome, nível e os itens nas casas dadas.
func moldeDaFada(nome string, nivel int32, casas map[int]int16) []byte {
	b := expMobTemplate(nivel, 0, 0)
	clear(b[0:16])
	copy(b[0:15], nome)
	for casa, idx := range casas {
		binary.LittleEndian.PutUint16(b[268+casa*8:], uint16(idx))
	}
	return b
}

// mundoDasFadas é o mundo de mobKilledWorld com blocos registrados, para o
// catálogo ter de onde ler. Devolve também quem mata, já com a fada pedida.
func mundoDasFadas(t *testing.T, fada int16, gens ...*world.Generator) (*Dispatcher, *world.World, *world.Entity) {
	t.Helper()
	d, w, killer := mobKilledWorld(t)
	w.RegisterGenerators(gens)
	killer.Equip[fairyEquipSlot] = world.Item{Index: fada}
	return d, w, killer
}

func blocoDaFada(arquivo string, tmpl []byte) *world.Generator {
	return &world.Generator{Name: arquivo, LeaderName: arquivo, LeaderTmpl: tmpl, SegX: [5]int16{6}, SegY: [5]int16{5}}
}

// mataDoBloco cria um monstro do bloco idx e o mata.
func mataMonstroDaFada(t *testing.T, d *Dispatcher, w *world.World, killer *world.Entity, idx int) {
	t.Helper()
	g := w.GeneratorAt(idx)
	id := w.SpawnMobAt(world.MobSpawn{Template: g.LeaderTmpl, TemplateName: g.LeaderName, X: 6, Y: 5, GenIndex: int16(idx)})
	if id < 0 {
		t.Fatal("SpawnMobAt falhou")
	}
	d.mobKilled(w, killer, w.Entity(id))
}

// O catálogo lista só monstro, sem repetir molde, em ordem de nome, e deixa de
// fora o bloco desligado e o NPC de loja (cujo Carry é estoque, e não saque).
func TestFadasCatalogoSoMonstroLigado(t *testing.T) {
	loja := moldeDaFada("Vendedor", 1, map[int]int16{0: itemFadaTesteA})
	loja[92+12] = 1 // CurrentScore.Merchant
	desligado := blocoDaFada("Sumido", moldeDaFada("Sumido", 9, nil))
	desligado.Off = true
	par := blocoDaFada("Zumbi", moldeDaFada("Zumbi", 20, nil))
	par.FollowerName, par.FollowerTmpl = "Aranha_Negra", moldeDaFada("Aranha_Negra", 12, nil)

	d, w, _ := mundoDasFadas(t, 0,
		par,
		blocoDaFada("Zumbi", moldeDaFada("Zumbi", 20, nil)),
		blocoDaFada("Vendedor", loja),
		desligado,
	)
	cat := d.fadasCatalogo(w)
	var nomes []string
	for _, m := range cat.lista {
		nomes = append(nomes, m.nome)
	}
	if want := []string{"Aranha Negra", "Zumbi"}; !slices.Equal(nomes, want) {
		t.Fatalf("catálogo = %v, quero %v", nomes, want)
	}
	if cat.lista[0].nivel != 12 || cat.lista[1].nivel != 20 {
		t.Errorf("níveis = %d e %d, quero 12 e 20", cat.lista[0].nivel, cat.lista[1].nivel)
	}
	versao := cat.versao

	// Ligar o bloco muda a assinatura: o catálogo se refaz com versão nova.
	desligado.Off = false
	d.fadas.conferido = d.fadas.conferido.Add(-2 * fadasCatalogoValidade)
	cat = d.fadasCatalogo(w)
	if len(cat.lista) != 3 || cat.versao == versao {
		t.Errorf("depois de ligar o bloco: %d monstros, versão %d (era %d)", len(cat.lista), cat.versao, versao)
	}
}

// A lista de drops junta o molde e a Mesa, tira o que a Mesa governa a 0%, e sai
// em ORDEM DE ÍNDICE: a ordem das casas entregaria a raridade.
func TestDropsVisiveisJuntaMoldeEMesaEmOrdemDeIndice(t *testing.T) {
	tmpl := moldeDaFada("Bicho", 30, map[int]int16{
		11: itemFadaTesteA, // sempre cai
		20: itemFadaTesteC, // raríssimo, e de índice MENOR
		3:  100,            // índice interno: fora
		5:  454,            // suprimido: fora
		7:  itemFadaTesteA, // repetido
		9:  777,            // a Mesa tira a 0%
	})
	d, w, _ := mundoDasFadas(t, 0, blocoDaFada("Bicho", tmpl))
	d.dropRules = droprule.NewTable([]droprule.Rule{
		{Mob: "Bicho", Item: 777, Chance: 0},
		{Mob: "Bicho", Item: itemFadaTesteB, Chance: 1},
		{Mob: "Outro", Item: 3000, Chance: droprule.MaxChance},
	})
	cat := d.fadasCatalogo(w)
	got := d.dropsVisiveis(w, &cat.lista[0])
	if want := []int16{itemFadaTesteC, itemFadaTesteB, itemFadaTesteA}; !slices.Equal(got, want) {
		t.Fatalf("dropsVisiveis = %v, quero %v", got, want)
	}
}

// O saque de chefe escrito em código aparece na lista do chefe.
func TestDropsVisiveisTemOSaqueDeChefe(t *testing.T) {
	d, w, _ := mundoDasFadas(t, 0,
		blocoDaFada(reiTrollZumbiTemplate, moldeDaFada("Rei Troll", 200, nil)),
		blocoDaFada(bossManticoraTemplate, moldeDaFada("Boss Manticora", 300, nil)),
	)
	cat := d.fadasCatalogo(w)
	troll := d.dropsVisiveis(w, &cat.lista[cat.porMolde[droprule.Canonical(reiTrollZumbiTemplate)]])
	for _, p := range reiTrollZumbiPacote {
		if !slices.Contains(troll, p.item) {
			t.Errorf("Rei Troll Zumbi: falta o item %d do pacote", p.item)
		}
	}
	mant := d.dropsVisiveis(w, &cat.lista[cat.porMolde[droprule.Canonical(bossManticoraTemplate)]])
	for _, p := range bossManticoraPremios {
		if !slices.Contains(mant, p.itemN) || (p.itemB != 0 && !slices.Contains(mant, p.itemB)) {
			t.Errorf("Boss Mantícora: falta o prêmio %q", p.nome)
		}
	}
}

// TUDO QUE mobKilled ENTREGA TEM DE ESTAR NA LISTA. Se a lista mostrasse menos do
// que cai, o jogador não teria como proteger o item. Mata muitas vezes cada
// monstro, sem filtro, e confere cada item que chegou à mochila.
func TestDropsVisiveisCobreOQueMobKilledEntrega(t *testing.T) {
	casos := map[string][]byte{
		"Bicho": moldeDaFada("Bicho", 30, map[int]int16{
			11: itemFadaTesteA, 0: itemFadaTesteC, 12: 2441, 30: 2442, 40: 419,
		}),
		reiTrollZumbiTemplate:  moldeDaFada("Rei Troll", 200, map[int]int16{11: itemFadaTesteA}),
		bossManticoraTemplate:  moldeDaFada("Boss Manticora", 300, nil),
		"Tauron_Agmo":          moldeDaFada("Tauron Agmo", 250, nil),
		taronTiranoMolde:       moldeDaFada("Taron Tirano", 250, nil),
		bossConjuradorTemplate: moldeDaFada("Boss Conjurador", 250, nil),
	}
	for arquivo, tmpl := range casos {
		t.Run(arquivo, func(t *testing.T) {
			d, w, killer := mundoDasFadas(t, 0, blocoDaFada(arquivo, tmpl))
			d.dropRules = droprule.NewTable([]droprule.Rule{
				{Mob: "Bicho", Item: itemFadaTesteB, Chance: droprule.MaxChance},
			})
			cat := d.fadasCatalogo(w)
			lista := d.dropsVisiveis(w, &cat.lista[0])
			for range 300 {
				mataMonstroDaFada(t, d, w, killer, 0)
				for i := range killer.Carry {
					if idx := killer.Carry[i].Index; idx != 0 && !slices.Contains(lista, idx) {
						t.Fatalf("mobKilled entregou o item %d, que a lista %v não mostra", idx, lista)
					}
					killer.Carry[i] = world.Item{}
				}
			}
		})
	}
}

// O pacote de drops leva só índices: nem chance, nem casa.
func TestFadasDropsNaoLevaChanceNemCasa(t *testing.T) {
	b := protocol.EncodeFadasDrops(7, 3, []int16{412, 2316, 2405})
	if len(b) != 8+2*3 {
		t.Fatalf("pacote com %d bytes, quero %d: só o cabeçalho e 2 bytes por item", len(b), 8+2*3)
	}
	if v, m, n := binary.LittleEndian.Uint16(b), binary.LittleEndian.Uint16(b[2:]), binary.LittleEndian.Uint16(b[4:]); v != 7 || m != 3 || n != 3 {
		t.Errorf("cabeçalho = versão %d, monstro %d, n %d", v, m, n)
	}
}

// A busca não depende de maiúscula, acento ou '_', venha o texto em UTF-8 ou nos
// bytes CP1252 do cliente.
func TestFadasSemAcento(t *testing.T) {
	casos := map[string]string{
		"Dragão_Vermelho":      "dragao vermelho",
		"Drag\xe3o Vermelho":   "dragao vermelho", // CP1252
		"  ÁGUIA ":             "aguia",
		"Cav._Lugefer":         "cav. lugefer",
		"G\xe1rgula S\xe1bio.": "gargula sabio.",
	}
	for in, want := range casos {
		if got := fadasSemAcento(in); got != want {
			t.Errorf("fadasSemAcento(%q) = %q, quero %q", in, got, want)
		}
	}
}

// --- o filtro na entrega ---------------------------------------------------

func bichoDoFiltro() *world.Generator {
	// As duas casas que sempre caem: 11 (protegido nos testes) e 0? Não: só a 11
	// é garantida, então o segundo item vem da Mesa, a 100%.
	return blocoDaFada("Bicho", moldeDaFada("Bicho", 30, map[int]int16{11: itemFadaTesteA}))
}

func mesaDoFiltro() droprule.Table {
	return droprule.NewTable([]droprule.Rule{{Mob: "Bicho", Item: itemFadaTesteB, Chance: droprule.MaxChance}})
}

func TestFadaFiltroSoEntraOProtegido(t *testing.T) {
	casos := []struct {
		nome       string
		fada       int16
		ligado     bool
		protegidos []int16
		querA      bool // o item do molde entra?
		querB      bool // o item da Mesa entra?
	}{
		{"desligado: tudo entra", fadaAzul7Dias, false, []int16{itemFadaTesteA}, true, true},
		{"Azul ligada: só o protegido", fadaAzul7Dias, true, []int16{itemFadaTesteA}, true, false},
		{"Vermelha ligada: só o protegido", fadaVermelha3Dias, true, []int16{itemFadaTesteB}, false, true},
		{"os dois protegidos", fadaAzul3Dias, true, []int16{itemFadaTesteB, itemFadaTesteA}, true, true},
		{"sem fada: tudo entra", 0, true, []int16{itemFadaTesteA}, true, true},
		{"Fada do Vale não filtra", fadaDoVale7Dias, true, []int16{itemFadaTesteA}, true, true},
		{"Fada Verde não filtra", 3900, true, []int16{itemFadaTesteA}, true, true},
		{"ligado com lista vazia não descarta", fadaAzul7Dias, true, nil, true, true},
	}
	for _, c := range casos {
		t.Run(c.nome, func(t *testing.T) {
			d, w, killer := mundoDasFadas(t, c.fada, bichoDoFiltro())
			d.dropRules = mesaDoFiltro()
			killer.FadaFiltroLigado = c.ligado
			killer.FadaFiltro = slices.Sorted(slices.Values(c.protegidos))
			mataMonstroDaFada(t, d, w, killer, 0)
			if _, ok := carryHas(killer, itemFadaTesteA); ok != c.querA {
				t.Errorf("item do molde na mochila = %v, quero %v", ok, c.querA)
			}
			if _, ok := carryHas(killer, itemFadaTesteB); ok != c.querB {
				t.Errorf("item da Mesa na mochila = %v, quero %v", ok, c.querB)
			}
		})
	}
}

// O FILTRO NÃO MUDA OS SORTEIOS. Quem decide é a entrega, depois de tudo
// sorteado: a sequência de w.Rand() com filtro é a mesma de sem filtro.
func TestFadaFiltroNaoMexeNaSequenciaDeSorteios(t *testing.T) {
	proximo := func(ligado bool) int {
		d, w, killer := mundoDasFadas(t, fadaAzul7Dias, blocoDaFada("Bicho", moldeDaFada("Bicho", 30, map[int]int16{
			11: itemFadaTesteA, 12: 2441, 0: itemFadaTesteC, 30: 2442,
		})))
		d.dropRules = mesaDoFiltro()
		killer.FadaFiltroLigado, killer.FadaFiltro = ligado, []int16{itemFadaTesteC}
		for range 20 {
			mataMonstroDaFada(t, d, w, killer, 0)
		}
		return w.Rand().Intn(1 << 30)
	}
	if sem, com := proximo(false), proximo(true); sem != com {
		t.Errorf("o filtro mudou a sequência de sorteios: %d sem, %d com", sem, com)
	}
}

// O pet mata, o prêmio é do dono, e vale o filtro do dono.
func TestFadaFiltroValeParaODonoDoPet(t *testing.T) {
	d, w, dono := mundoDasFadas(t, fadaAzul7Dias, bichoDoFiltro())
	d.dropRules = mesaDoFiltro()
	dono.FadaFiltroLigado, dono.FadaFiltro = true, []int16{itemFadaTesteB}
	g := w.GeneratorAt(0)
	id := w.SpawnMobAt(world.MobSpawn{Template: g.LeaderTmpl, TemplateName: g.LeaderName, X: 6, Y: 5})
	if d.fadaDescarta(w, dono, w.Entity(id), itemFadaTesteB) {
		t.Error("descartou o item protegido do dono")
	}
	if !d.fadaDescarta(w, dono, w.Entity(id), itemFadaTesteA) {
		t.Error("não descartou o item fora da lista do dono")
	}
}

// A REDE DE SEGURANÇA: o que o painel não mostra para o monstro nunca some. Vale
// para o monstro sem bloco (um /gm criar) e para o item que não está na lista dele.
func TestFadaFiltroNaoDescartaOQueOPainelNaoMostra(t *testing.T) {
	d, w, killer := mundoDasFadas(t, fadaAzul7Dias, bichoDoFiltro())
	killer.FadaFiltroLigado, killer.FadaFiltro = true, []int16{itemFadaTesteC}
	g := w.GeneratorAt(0)

	doBloco := w.Entity(w.SpawnMobAt(world.MobSpawn{Template: g.LeaderTmpl, TemplateName: "Bicho", X: 6, Y: 5}))
	if d.fadaDescarta(w, killer, doBloco, 3000) {
		t.Error("descartou um item que não está na lista do monstro")
	}
	semBloco := w.Entity(w.SpawnMobAt(world.MobSpawn{Template: g.LeaderTmpl, TemplateName: "Criado_Na_Mao", X: 7, Y: 5}))
	if d.fadaDescarta(w, killer, semBloco, itemFadaTesteA) {
		t.Error("descartou o saque de um monstro que não está no catálogo")
	}
	// Sem monstro (item que não é saque) também não.
	if d.fadaDescarta(w, killer, nil, itemFadaTesteA) {
		t.Error("descartou um item que não veio de monstro")
	}
	if !d.putMobDrop(w, killer, nil, world.Item{Index: itemFadaTesteA}) {
		t.Error("putMobDrop sem monstro não entregou")
	}
}

// As mudanças da lista: pôr, tirar, ligar, desligar, e as recusas.
func TestFadaAplica(t *testing.T) {
	e := &world.Entity{}
	muda := func(acao uint8, item int16) (uint8, bool) {
		return fadaAplica(e, protocol.FadasMudaBody{Acao: acao, Item: item})
	}

	if m, _ := muda(protocol.FadasMudaLiga, 0); m != protocol.FadasMotivoSemFada {
		t.Errorf("ligar sem fada: motivo %d, quero SemFada", m)
	}
	e.Equip[fairyEquipSlot] = world.Item{Index: fadaVermelha5Dias}
	if m, _ := muda(protocol.FadasMudaLiga, 0); m != protocol.FadasMotivoListaVazia || e.FadaFiltroLigado {
		t.Errorf("ligar com lista vazia: motivo %d, ligado %v", m, e.FadaFiltroLigado)
	}
	if m, _ := muda(protocol.FadasMudaPoe, 100); m != protocol.FadasMotivoItemRuim {
		t.Errorf("pôr índice interno: motivo %d, quero ItemRuim", m)
	}
	for _, idx := range []int16{2405, 412, 2316} {
		if m, mudou := muda(protocol.FadasMudaPoe, idx); m != 0 || !mudou {
			t.Fatalf("pôr %d: motivo %d, mudou %v", idx, m, mudou)
		}
	}
	if _, mudou := muda(protocol.FadasMudaPoe, 412); mudou {
		t.Error("pôr duas vezes o mesmo item contou como mudança")
	}
	if want := []int16{412, 2316, 2405}; !slices.Equal(e.FadaFiltro, want) {
		t.Fatalf("lista = %v, quero %v (em ordem, sem repetir)", e.FadaFiltro, want)
	}
	if m, mudou := muda(protocol.FadasMudaLiga, 0); m != 0 || !mudou || !e.FadaFiltroLigado {
		t.Fatalf("ligar: motivo %d, mudou %v, ligado %v", m, mudou, e.FadaFiltroLigado)
	}
	muda(protocol.FadasMudaTira, 412)
	muda(protocol.FadasMudaTira, 2316)
	if !e.FadaFiltroLigado {
		t.Error("desligou antes de a lista esvaziar")
	}
	// Tirar o último desliga: ligado com lista vazia descartaria tudo.
	muda(protocol.FadasMudaTira, 2405)
	if e.FadaFiltroLigado || len(e.FadaFiltro) != 0 {
		t.Errorf("tirar o último: ligado %v, lista %v", e.FadaFiltroLigado, e.FadaFiltro)
	}

	// O teto.
	for i := range protocol.FadasFiltroMax {
		muda(protocol.FadasMudaPoe, int16(1000+i))
	}
	if m, _ := muda(protocol.FadasMudaPoe, 3000); m != protocol.FadasMotivoListaCheia || len(e.FadaFiltro) != protocol.FadasFiltroMax {
		t.Errorf("acima do teto: motivo %d, %d itens", m, len(e.FadaFiltro))
	}
	muda(protocol.FadasMudaLiga, 0)
	if _, mudou := muda(protocol.FadasMudaTiraTudo, 0); !mudou || e.FadaFiltroLigado || len(e.FadaFiltro) != 0 {
		t.Errorf("tirar tudo: ligado %v, %d itens", e.FadaFiltroLigado, len(e.FadaFiltro))
	}
}

// O que vem do banco no login é limpo: fora da faixa e repetido saem, e ligado
// sem item vira desligado.
func TestFadaFiltroDoBanco(t *testing.T) {
	ligado, itens := fadaFiltroDoBanco(true, []int16{2405, 100, 412, 2405, -3, 454})
	if !ligado || !slices.Equal(itens, []int16{412, 2405}) {
		t.Errorf("= %v %v, quero true [412 2405]", ligado, itens)
	}
	if ligado, itens := fadaFiltroDoBanco(true, []int16{100}); ligado || len(itens) != 0 {
		t.Errorf("ligado sem item válido = %v %v, quero desligado e vazio", ligado, itens)
	}
}

// --- pelo fio --------------------------------------------------------------

// O SERVIDOR RESPONDE SEMPRE ao 0x0F73, com o estado depois do pedido, e ao
// 0x0F70 de monstros e de drops.
func TestFadasRespondePeloFio(t *testing.T) {
	srv, c := mesaDaLixeira(t)

	espera := func(tipo protocol.Type) []byte {
		t.Helper()
		_, corpo, ok := quadroAte(t, c, 2*time.Second, func(h protocol.Header, _ []byte) bool { return h.Type == tipo })
		if !ok {
			t.Fatalf("o servidor não respondeu o 0x%04X", uint16(tipo))
		}
		return corpo
	}
	muda := func(acao uint8, item int16) []byte {
		b := make([]byte, 4)
		b[0] = acao
		binary.LittleEndian.PutUint16(b[2:], uint16(item))
		return b
	}

	naContaDoRelogio(t, srv, 7, func(w *world.World, d *Dispatcher, s *world.Session, e *world.Entity) {
		e.Equip[fairyEquipSlot] = world.Item{Index: fadaAzul7Dias}
		d.fadasMuda(w, s, protocol.Header{}, muda(protocol.FadasMudaPoe, itemFadaTesteA))
	})
	if b := espera(protocol.MsgFadasFiltro); b[0] != 0 || b[1] != 1 || b[2] != 0 || b[3] != 1 ||
		int16(binary.LittleEndian.Uint16(b[4:])) != itemFadaTesteA {
		t.Errorf("depois de pôr: % x", b)
	}

	naContaDoRelogio(t, srv, 7, func(w *world.World, d *Dispatcher, s *world.Session, _ *world.Entity) {
		d.fadasMuda(w, s, protocol.Header{}, muda(protocol.FadasMudaLiga, 0))
	})
	if b := espera(protocol.MsgFadasFiltro); b[0] != 1 || b[2] != 0 {
		t.Errorf("depois de ligar: % x", b)
	}

	// Sem a fada, ligar é recusado com o motivo.
	naContaDoRelogio(t, srv, 7, func(w *world.World, d *Dispatcher, s *world.Session, _ *world.Entity) {
		d.fadasMuda(w, s, protocol.Header{}, muda(protocol.FadasMudaDesliga, 0))
	})
	espera(protocol.MsgFadasFiltro)
	naContaDoRelogio(t, srv, 7, func(w *world.World, d *Dispatcher, s *world.Session, e *world.Entity) {
		e.Equip[fairyEquipSlot] = world.Item{}
		d.fadasMuda(w, s, protocol.Header{}, muda(protocol.FadasMudaLiga, 0))
	})
	if b := espera(protocol.MsgFadasFiltro); b[0] != 0 || b[1] != 0 || b[2] != protocol.FadasMotivoSemFada {
		t.Errorf("ligar sem fada: % x", b)
	}

	// A busca: uma página de monstros, com a versão do catálogo.
	pede := make([]byte, 8+protocol.FadasNome)
	pede[0] = protocol.FadasPedeBusca
	copy(pede[8:], "zzzznenhum")
	naContaDoRelogio(t, srv, 7, func(w *world.World, d *Dispatcher, s *world.Session, _ *world.Entity) {
		d.fadasPede(w, s, protocol.Header{}, pede)
	})
	if b := espera(protocol.MsgFadasMonstros); b[2] != protocol.FadasPedeBusca || b[6] != 0 {
		t.Errorf("busca sem resultado: % x", b)
	}

	// Drops com a versão errada: o servidor manda "peça a lista de novo".
	pede[0] = protocol.FadasPedeDrops
	binary.LittleEndian.PutUint16(pede[2:], 0xBEEF)
	naContaDoRelogio(t, srv, 7, func(w *world.World, d *Dispatcher, s *world.Session, e *world.Entity) {
		d.fadasPede(w, s, protocol.Header{}, pede)
	})
	if b := espera(protocol.MsgFadasDrops); binary.LittleEndian.Uint16(b[2:]) != fadasSemMonstro {
		t.Errorf("drops com versão velha: % x", b)
	}
}

// --- tipo do lugar, região e faixas de adicional (fase 2) ----------------------

func blocoDaFadaEm(arquivo string, tmpl []byte, x, y int16) *world.Generator {
	g := blocoDaFada(arquivo, tmpl)
	g.SegX[0], g.SegY[0] = x, y
	return g
}

// Todo monstro do catálogo tem tipo e região. Com mais de um lugar vale o que
// tem mais blocos, e no empate o primeiro em ordem de bloco. As quests de corrida
// são reconhecidas pelo molde.
func TestFadasTipoERegiaoDoMonstro(t *testing.T) {
	tmpl := moldeDaFada("X", 10, nil)
	d, w, _ := mundoDasFadas(t, 0,
		blocoDaFadaEm("Maioria", tmpl, 1200, 1700), // Deserto
		blocoDaFadaEm("Maioria", tmpl, 1100, 3500), // Água Normal
		blocoDaFadaEm("Maioria", tmpl, 1110, 3510), // Água Normal
		blocoDaFadaEm("Empate", tmpl, 2300, 2100),  // Armia, primeiro
		blocoDaFadaEm("Empate", tmpl, 1200, 1700),  // Deserto
		blocoDaFadaEm("Arena", tmpl, 2400, 2100),   // Coveiro, dentro de Armia
		blocoDaFadaEm("COrc_Guarda", tmpl, 2300, 2100),
		blocoDaFadaEm("Perdido", tmpl, 5, 5),
	)
	cat := d.fadasCatalogo(w)
	quero := map[string]regiao.Lugar{
		"maioria":     {Tipo: regiao.Masmorra, Nome: "Água Normal"},
		"empate":      {Tipo: regiao.MapaAberto, Nome: "Armia"},
		"arena":       {Tipo: regiao.Quest, Nome: "Coveiro"},
		"corc_guarda": {Tipo: regiao.Quest, Nome: "Castelo Orc"},
		"perdido":     regiao.Padrao,
	}
	if len(cat.lista) != len(quero) {
		t.Fatalf("catálogo com %d monstros, quero %d", len(cat.lista), len(quero))
	}
	for _, m := range cat.lista {
		if m.lugar != quero[m.molde] {
			t.Errorf("%s: lugar %+v, quero %+v", m.molde, m.lugar, quero[m.molde])
		}
	}
}

// A lista de monstros leva o tipo e a região, em CP1252, e não leva o nível.
func TestFadasMonstrosLevaTipoERegiao(t *testing.T) {
	b := (&protocol.FadasMonstrosBody{Versao: 3, Tipo: protocol.FadasPedeProximos, Total: 1,
		Monstros: []protocol.FadasMonstro{{Numero: 9, Tipo: uint8(regiao.Masmorra), Nome: "Troll", Regiao: "Água Normal"}},
	}).Encode()
	if len(b) != 8+4+protocol.FadasNome+protocol.FadasRegiao {
		t.Fatalf("pacote com %d bytes", len(b))
	}
	linha := b[8:]
	if binary.LittleEndian.Uint16(linha) != 9 || linha[2] != uint8(regiao.Masmorra) {
		t.Errorf("número ou tipo errado: % x", linha[:4])
	}
	if got := string(linha[4:9]); got != "Troll" {
		t.Errorf("nome = %q", got)
	}
	// "Água" em CP1252: Á é um byte só, 0xC1.
	if reg := linha[4+protocol.FadasNome:]; reg[0] != 0xC1 || string(reg[1:11]) != "gua Normal" || reg[11] != 0 {
		t.Errorf("região = % x", reg[:12])
	}
}

// As faixas de adicional: só para o item que passa pelo sorteio comum. O saque de
// chefe e o item da Mesa num lugar com tabela própria ficam sem faixa.
func TestFadasFaixasSoDoSorteioComum(t *testing.T) {
	const espada, luva int16 = 2000, 2001
	prepara := func(d *Dispatcher) {
		armaC := int(armasCFisicas[0])
		d.itemPos = map[int]int{int(espada): 64, int(luva): 16, armaC: 64}
		d.itemUnique = map[int]int{int(espada): 41, armaC: 41}
		d.itemReqs = map[int]content.ItemReq{int(espada): {Lvl: 100}, int(luva): {Lvl: 100}}
	}
	mesa := droprule.NewTable([]droprule.Rule{
		{Mob: "Bicho", Item: luva, Chance: 1},
		{Mob: "COrc_Guarda", Item: luva, Chance: 1},
	})
	d, w, _ := mundoDasFadas(t, 0,
		blocoDaFada("Bicho", moldeDaFada("Bicho", 150, map[int]int16{3: espada, 4: itemFadaTesteC})),
		blocoDaFada("COrc_Guarda", moldeDaFada("Orc", 150, map[int]int16{3: espada})),
		blocoDaFada(bossConjuradorTemplate, moldeDaFada("Conjurador", 300, nil)),
	)
	prepara(d)
	d.dropRules = mesa
	cat := d.fadasCatalogo(w)
	faixasDe := func(molde string) map[int16][]protocol.FadasFaixa {
		out := map[int16][]protocol.FadasFaixa{}
		for _, f := range d.fadasFaixas(w, &cat.lista[cat.porMolde[droprule.Canonical(molde)]]) {
			out[f.Item] = f.Faixas
		}
		return out
	}

	bicho := faixasDe("Bicho")
	if len(bicho[espada]) == 0 || len(bicho[luva]) == 0 {
		t.Errorf("monstro comum: espada %v, luva %v; as duas deviam ter faixa", bicho[espada], bicho[luva])
	}
	if _, tem := bicho[itemFadaTesteC]; tem {
		t.Error("a poeira (material) veio com faixa de adicional")
	}
	orc := faixasDe("COrc_Guarda")
	if len(orc[espada]) == 0 {
		t.Error("Castelo Orc: a espada do molde devia ter a faixa do sorteio comum")
	}
	if _, tem := orc[luva]; tem {
		t.Error("Castelo Orc: o item da Mesa recebe a tabela do lugar e não pode levar a faixa comum")
	}
	if chefe := faixasDe(bossConjuradorTemplate); len(chefe) != 0 {
		t.Errorf("saque de chefe veio com faixa: %v", chefe)
	}

	// E a faixa é a do sorteio: uma espada de monstro 50 níveis acima.
	ref := refine.NovoPossiveis(d.dropBonus).Do(refine.Base{Unique: 41, ReqLvl: 100, Pos: 64, Indice: int(espada)}, 150)
	if len(ref) != len(bicho[espada]) {
		t.Fatalf("faixas da espada: %v, o sorteio dá %v", bicho[espada], ref)
	}
	for i, f := range ref {
		if g := bicho[espada][i]; g.Efeito != f.Efeito || g.Min != f.Min || g.Max != f.Max {
			t.Errorf("faixa %d: %+v, o sorteio dá %+v", i, g, f)
		}
	}
}

// O 0x0F76 só leva efeito, mínimo e máximo: três bytes por faixa, nenhuma chance.
func TestFadasFaixasNoFio(t *testing.T) {
	b := protocol.EncodeFadasFaixas(4, 2, []protocol.FadasFaixasItem{
		{Item: 2000, Faixas: []protocol.FadasFaixa{{Efeito: 2, Min: 9, Max: 54}, {Efeito: 74, Min: 3, Max: 18}}},
		{Item: 2001},
	})
	if len(b) != 8+4+2*3 {
		t.Fatalf("pacote com %d bytes, quero %d", len(b), 8+4+2*3)
	}
	if n := binary.LittleEndian.Uint16(b[4:]); n != 1 {
		t.Errorf("n = %d: o item sem faixa não devia ir", n)
	}
	if !slices.Equal(b[8:], []byte{0xD0, 0x07, 2, 0, 2, 9, 54, 74, 3, 18}) {
		t.Errorf("corpo = % x", b[8:])
	}
}
