package handler

import (
	"hash/fnv"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/jeanluca/w2pp-openwyd/internal/campotreino"
	"github.com/jeanluca/w2pp-openwyd/internal/droprule"
	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/protocol"
	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/world"
)

// O PAINEL DE DROP DAS FADAS (pedido da dona em 02/10/2026): o jogador vê os
// monstros perto dele, ou procura um pelo nome, e vê o que cada um pode dar. É a
// vitrine do filtro da fada (fadas_filtro.go): o que ele clica aqui vira Item
// Protegido.
//
// A CHANCE NÃO SAI DAQUI. Decisão da dona: o painel mostra O QUE cai, nunca
// QUANTO. Por isso dropsVisiveis devolve só índices, em ordem de índice — a casa
// do molde é a chance (loot.EffectiveDropRate), e a ordem das casas a entregaria.

// fadaMonstro é uma linha do catálogo: um molde de monstro que algum bloco do
// NPCGener cria.
type fadaMonstro struct {
	molde string // nome do ARQUIVO do molde, canônico: a chave, como na Mesa de Drops
	nome  string // o nome que o jogo mostra, com espaço no lugar de '_'
	busca string // o nome sem acento e em minúsculas, para a pesquisa
	nivel uint16

	// De onde ler os bytes do molde na hora de listar o saque. Não se guarda o
	// vetor: a ficha de /monstros o troca no lugar (Generator.Rev não se mexe).
	bloco    int
	seguidor bool

	// Saques que dependem de ONDE o monstro nasce, e não só do molde.
	noCampo   bool // algum bloco dele nasce no campo de treino (Repletion A)
	noColiseu bool // algum bloco dele é do Coliseu {N}
}

// fadaCatalogo é a lista de todos os monstros que o mundo cria, em ordem de nome.
// O número de um monstro é a posição dele aqui, e só vale para esta versão.
type fadaCatalogo struct {
	assinatura uint64
	versao     uint16
	lista      []fadaMonstro
	porMolde   map[string]int
	conferido  time.Time
}

// fadasCatalogoValidade é de quanto em quanto o catálogo é conferido contra os
// blocos. A conferência passa por todos os blocos, e o descarte do filtro
// (fadas_filtro.go) consulta o catálogo a cada item: sem este prazo, seria uma
// volta no NPCGener inteiro por drop.
const fadasCatalogoValidade = 5 * time.Second

// fadasCatalogo devolve o catálogo em dia, refazendo-o quando os blocos mudaram
// (receita trocada no painel, bloco ligado ou desligado).
func (d *Dispatcher) fadasCatalogo(w *world.World) *fadaCatalogo {
	c := &d.fadas
	agora := d.now()
	if c.porMolde != nil && !agora.Before(c.conferido) && agora.Sub(c.conferido) < fadasCatalogoValidade {
		return c
	}
	c.conferido = agora
	if sig := fadasAssinatura(w); c.porMolde == nil || sig != c.assinatura {
		c.assinatura = sig
		c.lista = fadasMontaCatalogo(w)
		c.porMolde = make(map[string]int, len(c.lista))
		for i := range c.lista {
			c.porMolde[c.lista[i].molde] = i
		}
		c.versao++
		if c.versao == 0 {
			c.versao = 1
		}
		d.log.Info("painel das fadas: catálogo de monstros", "versao", c.versao, "monstros", len(c.lista))
	}
	return c
}

// fadasAssinatura resume o que o catálogo lê dos blocos. Mudou, refaz.
func fadasAssinatura(w *world.World) uint64 {
	h := fnv.New64a()
	var n [7]byte
	for i := 0; i < w.GeneratorCount(); i++ {
		g := w.GeneratorAt(i)
		if g == nil {
			continue
		}
		n[0], n[1] = byte(i), byte(i>>8)
		n[2], n[3], n[4], n[5] = byte(g.Rev), byte(g.Rev>>8), byte(g.Rev>>16), byte(g.Rev>>24)
		n[6] = 0
		if g.Off {
			n[6] |= 1
		}
		if g.FollowerTmpl != nil {
			n[6] |= 2
		}
		if g.LeaderTmpl != nil {
			n[6] |= 4
		}
		_, _ = h.Write(n[:])
		_, _ = h.Write([]byte(g.LeaderName))
		_, _ = h.Write([]byte{0})
		_, _ = h.Write([]byte(g.FollowerName))
		_, _ = h.Write([]byte{0})
	}
	return h.Sum64()
}

// fadasMontaCatalogo percorre os blocos e junta os moldes de monstro.
//
// SÓ MONSTRO: num NPC de loja o Carry é o ESTOQUE, e mostrá-lo como saque seria
// erro. Bloco desligado fica de fora, ou o painel anunciaria monstro que não existe.
func fadasMontaCatalogo(w *world.World) []fadaMonstro {
	porMolde := map[string]*fadaMonstro{}
	junta := func(bloco int, g *world.Generator, tmpl []byte, arquivo string, seguidor bool) {
		if len(tmpl) < fadasTamanhoDoMolde || arquivo == "" {
			return
		}
		x, y := int(g.SegX[0]), int(g.SegY[0])
		b := protocol.ParseMobBasics(tmpl)
		if world.MoldeNaoCombate(b, arquivo, bloco, g.SegX[0], g.SegY[0]) {
			return
		}
		noCampo := campotreino.Contem(x, y)
		noColiseu := bloco >= coliseuNPrimeiroBloco && bloco <= coliseuNUltimoBloco
		chave := droprule.Canonical(arquivo)
		if m := porMolde[chave]; m != nil {
			m.noCampo = m.noCampo || noCampo
			m.noColiseu = m.noColiseu || noColiseu
			return
		}
		nome := strings.ReplaceAll(b.Name, "_", " ")
		porMolde[chave] = &fadaMonstro{
			molde: chave, nome: nome, busca: fadasSemAcento(nome),
			nivel: uint16(max(0, min(int(b.Level), 0xFFFF))),
			bloco: bloco, seguidor: seguidor,
			noCampo: noCampo, noColiseu: noColiseu,
		}
	}
	for i := 0; i < w.GeneratorCount(); i++ {
		g := w.GeneratorAt(i)
		if g == nil || g.Off {
			continue
		}
		junta(i, g, g.LeaderTmpl, g.LeaderName, false)
		junta(i, g, g.FollowerTmpl, g.FollowerName, true)
	}
	lista := make([]fadaMonstro, 0, len(porMolde))
	for _, m := range porMolde {
		lista = append(lista, *m)
	}
	slices.SortFunc(lista, func(a, b fadaMonstro) int {
		if c := strings.Compare(a.busca, b.busca); c != 0 {
			return c
		}
		if a.nivel != b.nivel {
			return int(a.nivel) - int(b.nivel)
		}
		return strings.Compare(a.molde, b.molde)
	})
	return lista
}

// fadasTamanhoDoMolde é o STRUCT_MOB canônico, de 816 bytes, que os blocos guardam.
const fadasTamanhoDoMolde = 816

// fadasSemAcento põe um nome em minúsculas e sem acento, para a busca não
// depender de como o jogador digita. Aceita os dois jeitos de o texto chegar:
// UTF-8 (o que o Go escreve) e os bytes CP1252 do cliente e dos moldes.
func fadasSemAcento(s string) string {
	out := make([]byte, 0, len(s))
	poe := func(r rune) {
		switch {
		case r >= 'A' && r <= 'Z':
			r += 'a' - 'A'
		case r >= 0xC0 && r <= 0xFF:
			r = fadasBaseDoAcento[r-0xC0]
		case r == '_':
			r = ' '
		}
		if r < 0x80 {
			out = append(out, byte(r))
		}
	}
	if utf8.ValidString(s) {
		for _, r := range s {
			poe(r)
		}
	} else {
		for i := 0; i < len(s); i++ {
			poe(rune(s[i]))
		}
	}
	return strings.TrimSpace(string(out))
}

// fadasBaseDoAcento leva cada letra de 0xC0 a 0xFF (igual em Latin-1 e CP1252) à
// letra sem acento, em minúscula. O que não é letra vira 0x80, que a busca descarta.
var fadasBaseDoAcento = func() [64]rune {
	const de = "ÀÁÂÃÄÅÆÇÈÉÊËÌÍÎÏÐÑÒÓÔÕÖ×ØÙÚÛÜÝÞßàáâãäåæçèéêëìíîïðñòóôõö÷øùúûüýþÿ"
	const para = "aaaaaaaceeeeiiiidnooooo?ouuuuy?saaaaaaaceeeeiiiidnooooo?ouuuuy?y"
	var t [64]rune
	i := 0
	for _, r := range de {
		c := rune(para[i])
		if c == '?' {
			c = 0x80
		}
		t[r-0xC0] = c
		i++
	}
	return t
}()

// fadasMolde devolve os bytes do molde de um monstro do catálogo, ou nil se o
// bloco mudou debaixo dele (o catálogo se refaz na próxima conferência).
func fadasMolde(w *world.World, m *fadaMonstro) []byte {
	g := w.GeneratorAt(m.bloco)
	if g == nil {
		return nil
	}
	tmpl, arquivo := g.LeaderTmpl, g.LeaderName
	if m.seguidor {
		tmpl, arquivo = g.FollowerTmpl, g.FollowerName
	}
	if len(tmpl) < fadasTamanhoDoMolde || droprule.Canonical(arquivo) != m.molde {
		return nil
	}
	return tmpl
}

// dropsVisiveis é tudo que um monstro pode entregar pelo saque, só os índices, sem
// repetir e em ordem de índice. Junta as três fontes de mobKilled:
//
//   - as casas do molde, com as mesmas exclusões do laço de lá;
//   - as regras da Mesa de Drops com chance acima de zero;
//   - os saques escritos em código (fadasSaqueEspecial).
//
// É também a rede de segurança do filtro: um item que não está aqui nunca é
// descartado (fadaDescarta), então esquecer uma fonte nesta lista deixa o item
// passar, em vez de fazê-lo sumir sem o jogador ter como protegê-lo.
func (d *Dispatcher) dropsVisiveis(w *world.World, m *fadaMonstro) []int16 {
	var out []int16
	if tmpl := fadasMolde(w, m); tmpl != nil {
		for _, c := range protocol.MobCarry(tmpl) {
			idx := int16(c.Index)
			if idx <= 390 || int(idx) >= maxItemList || idx == 454 {
				continue
			}
			if d.dropRules.Governs(m.molde, idx) {
				continue
			}
			out = append(out, idx)
		}
	}
	for _, r := range d.dropRules.Rolls(m.molde) {
		out = append(out, r.Item)
	}
	for _, idx := range fadasSaqueEspecial(m) {
		// O saque de chefe respeita a Mesa: item governado por ela não sai dele.
		if !d.dropRules.Governs(m.molde, idx) {
			out = append(out, idx)
		}
	}
	slices.Sort(out)
	return slices.Compact(out)
}

// fadasSaqueEspecial lista o que os saques escritos em código podem entregar para
// um molde: os chefes e os lugares que mobKilled trata fora do molde e da Mesa.
//
// QUEM ACRESCENTAR UM SAQUE NOVO em mobKilled acrescenta o molde aqui. Se
// esquecer, o item cai para todo mundo, com filtro ou sem (ver dropsVisiveis).
func fadasSaqueEspecial(m *fadaMonstro) []int16 {
	mob := &world.Entity{TemplateName: m.molde}
	var out []int16
	premios := func(tabela []bossManticoraPremio) {
		for _, p := range tabela {
			out = append(out, p.itemN)
			if p.itemB != 0 {
				out = append(out, p.itemB)
			}
		}
	}
	if m.noCampo {
		out = append(out, itemRepletionA)
	}
	if m.noColiseu {
		out = append(out, coliseuNItens[:]...)
	}
	if isAgmo(mob) {
		for _, a := range agmoAmagos {
			out = append(out, a.item)
		}
	}
	if isBossManticora(mob) {
		premios(bossManticoraPremios)
	}
	if isBossDragaoLich(mob) || isBossHidraDourada(mob) {
		premios(bossDragaoLichPremios)
	}
	if isChefeDaLava(mob) {
		if m.molde == droprule.Canonical(bossGolemFogoTemplate) {
			premios(golemFogoPremios)
		} else {
			premios(lavaChefePremios)
		}
	}
	if isCiclopeTirano(mob) {
		out = append(out, itemBarraPrata10Mi, itemOvoCavaloLeveN, itemOvoCavaloLeveB)
		out = append(out, armasDDoTirano...)
	}
	if isTaronTirano(mob) {
		out = append(out, itemBarraPrata10Mi, itemOvoCavaloLeveN, itemOvoCavaloLeveB)
		out = append(out, armasDDoTaronTirano...)
	}
	if isBossConjurador(mob) || isGargulaSabioChefe(mob) {
		out = append(out, armasCFisicas...)
		out = append(out, armasCMagicas...)
	}
	if isReiTrollZumbi(mob) {
		for _, p := range reiTrollZumbiPacote {
			out = append(out, p.item)
		}
	}
	if alma := geloChefeAlma(mob); alma != 0 {
		for _, p := range geloChefePremios {
			if p.item != 0 {
				out = append(out, p.item)
			}
		}
		out = append(out, alma)
	}
	return out
}

// fadasPedeIntervalo é o mínimo entre dois pedidos de lista do mesmo jogador. A
// busca "em tempo real" manda um a cada pausa de digitação.
const fadasPedeIntervalo = 150 * time.Millisecond

// fadasPede atende o 0x0F70.
func (d *Dispatcher) fadasPede(w *world.World, s *world.Session, _ protocol.Header, payload []byte) {
	e := w.Entity(s.Conn)
	if e == nil || s.Mode != world.UserPlay {
		return
	}
	corpo, err := protocol.DecodeFadasPede(payload)
	if err != nil {
		d.log.Info("painel das fadas: pedido recusado", "conn", s.Conn, "erro", err)
		return
	}
	switch corpo.Tipo {
	case protocol.FadasPedeProximos, protocol.FadasPedeBusca:
		agora := d.now()
		if !s.FadasPedidoEm.IsZero() && !agora.Before(s.FadasPedidoEm) && agora.Sub(s.FadasPedidoEm) < fadasPedeIntervalo {
			return
		}
		s.FadasPedidoEm = agora
		d.fadasMandaMonstros(w, s, corpo)
	case protocol.FadasPedeDrops:
		d.fadasMandaDrops(w, s, corpo)
	case protocol.FadasPedeFiltro:
		d.fadasMandaFiltro(w, s, e, protocol.FadasMotivoNenhum)
	}
}

// fadasBuscaMin é o menor texto que a busca aceita: com uma letra só a resposta
// seria quase o catálogo inteiro.
const fadasBuscaMin = 2

// fadasMandaMonstros responde com uma página de monstros: os que estão em volta
// do jogador, ou os que batem com o texto da busca.
func (d *Dispatcher) fadasMandaMonstros(w *world.World, s *world.Session, corpo protocol.FadasPedeBody) {
	cat := d.fadasCatalogo(w)
	var achados []int
	if corpo.Tipo == protocol.FadasPedeProximos {
		visto := map[int]bool{}
		w.ForEachMobInView(s.Conn, func(m *world.Entity) {
			if m.Summoner != 0 || m.TemplateName == "" || m.HP <= 0 {
				return
			}
			if i, ok := cat.porMolde[droprule.Canonical(m.TemplateName)]; ok && !visto[i] {
				visto[i] = true
				achados = append(achados, i)
			}
		})
		slices.Sort(achados)
	} else if texto := fadasSemAcento(corpo.Texto); len(texto) >= fadasBuscaMin {
		for i := range cat.lista {
			if strings.Contains(cat.lista[i].busca, texto) {
				achados = append(achados, i)
			}
		}
	}
	resp := protocol.FadasMonstrosBody{
		Versao: cat.versao, Tipo: corpo.Tipo, Pagina: corpo.Pagina,
		Total: uint16(min(len(achados), 0xFFFF)),
	}
	// Página além do fim volta para a última: a lista de próximos encolhe sozinha.
	if ultima := max(0, (len(achados)-1)/protocol.FadasMonstrosPorPagina); int(resp.Pagina) > ultima {
		resp.Pagina = uint8(ultima)
	}
	ini := min(int(resp.Pagina)*protocol.FadasMonstrosPorPagina, len(achados))
	fim := min(ini+protocol.FadasMonstrosPorPagina, len(achados))
	for _, i := range achados[ini:fim] {
		m := &cat.lista[i]
		resp.Monstros = append(resp.Monstros, protocol.FadasMonstro{Numero: uint16(i), Nivel: m.nivel, Nome: m.nome})
	}
	w.Send(s, protocol.MsgFadasMonstros, resp.Encode())
}

// fadasSemMonstro é o número que o 0x0F72 leva quando o pedido veio com a versão
// velha do catálogo ou com um número que não existe: o cliente pede a lista de novo.
const fadasSemMonstro = 0xFFFF

// fadasMandaDrops responde com os itens que um monstro do catálogo pode dar.
func (d *Dispatcher) fadasMandaDrops(w *world.World, s *world.Session, corpo protocol.FadasPedeBody) {
	cat := d.fadasCatalogo(w)
	if corpo.Versao != cat.versao || int(corpo.Monstro) >= len(cat.lista) {
		w.Send(s, protocol.MsgFadasDrops, protocol.EncodeFadasDrops(cat.versao, fadasSemMonstro, nil))
		return
	}
	itens := d.dropsVisiveis(w, &cat.lista[corpo.Monstro])
	w.Send(s, protocol.MsgFadasDrops, protocol.EncodeFadasDrops(cat.versao, corpo.Monstro, itens))
}
