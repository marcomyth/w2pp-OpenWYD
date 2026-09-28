package handler

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/protocol"
	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/world"
)

// O Painel de Guilda: as três abas e o que as alimenta.
//
// Quase tudo o que o painel mostra já existia e só não tinha onde aparecer. O
// nome, a fama, o clã e os cargos estão no banco desde a 0012_guild_system; a
// aliança e a guerra, em guild_relation; quem domina cada cidade e quanto ela
// cobra de imposto, em guild_zone; e a convocação é o /convocar de sempre
// (summonGuild, em guild.go). O recado, o teto de membros e a linha de status do
// membro são da 0079_painel_de_guilda, e esses três são novos de verdade — o
// legado não tinha janela de guilda, só comandos de chat.
//
// A divisão do trabalho é a que o protocolo descreve: a aba Informações e a aba
// Buffs saem da memória deste processo e são baratas; a aba Membros lê a guilda
// inteira do banco, porque este processo só conhece quem está conectado.

const (
	// guildaQuadroTTL é por quanto tempo uma leitura do quadro de membros serve
	// para responder sem voltar ao banco.
	//
	// Trinta segundos porque o quadro muda devagar — entrar e sair de guilda são
	// eventos raros — e porque o painel pede a mesma lista duas vezes seguidas
	// por desenho: uma para contar na aba Informações, outra ao abrir a aba
	// Membros. Sem a janela, abrir o painel custaria duas consultas à guilda
	// inteira.
	guildaQuadroTTL = 30 * time.Second

	// guildaPedidoMinIntervalo é o quanto um jogador tem de esperar entre dois
	// pedidos de painel que vão ao banco.
	//
	// Sem isto, um cliente remendado pede o quadro em laço e transforma uma tecla
	// numa consulta por quadro de vídeo. O painel humano pede quando abre e
	// quando troca de aba; meio segundo não é sentido por ninguém.
	guildaPedidoMinIntervalo = 500 * time.Millisecond

	// As abas, como o cliente as numera em MsgGuildaPede.
	guildaAbaInfo     = 0
	guildaAbaMembros  = 1
	guildaAbaBuffs    = 2
	guildaAbaLista    = 3
	guildaAbaEsquadra = 4
)

// quadroDeGuilda é o quadro de membros lido do banco, com a hora da leitura.
type quadroDeGuilda struct {
	membros []world.GuildMemberRecord
	// A escalação das cinco cidades, indexada pela zona. Vem na MESMA ida ao
	// banco que o quadro: as duas alimentam a mesma tela, e separá-las seria
	// pagar dois round trips para desenhar um painel.
	esquadras [protocol.GuildaCidades][]string
	lidoEm    time.Time
}

// guildaPede atende MsgGuildaPede: o cliente quer uma aba do painel.
func (d *Dispatcher) guildaPede(w *world.World, s *world.Session, _ protocol.Header, payload []byte) {
	e := w.Entity(s.Conn)
	if e == nil || s.Mode != world.UserPlay {
		return
	}
	corpo, err := protocol.DecodeGuildaPede(payload)
	if err != nil {
		return
	}
	// Sem guilda, um PEDIDO DE DADOS não é um erro: o painel abre no menu e
	// precisa perguntar "eu tenho guilda?" para saber se a segunda porta diz
	// "Guild" ou "Criar sua Guild". A resposta é a aba Informações vazia, e o
	// id zero nela é o que significa "nenhuma".
	//
	// Antes isto mandava uma linha de chat dizendo que o jogador não estava em
	// guilda, e ela aparecia TODA vez que o painel abria — uma reclamação em
	// resposta a uma pergunta. As recusas por falta de guilda ficam nas AÇÕES
	// (convocar, recado, status, buff), onde há mesmo algo sendo negado.
	if e.Guild == 0 {
		if corpo.Alvo == guildaAbaLista {
			d.guildaMandaLista(w, s, e)
			return
		}
		w.Send(s, protocol.MsgGuildaAbre, (&protocol.GuildaAbreBody{}).Encode())
		return
	}
	switch corpo.Alvo {
	case guildaAbaInfo:
		d.guildaMandaInfo(w, s, e)
	case guildaAbaMembros:
		d.guildaMandaMembros(w, s, e, corpo.Pagina)
	case guildaAbaBuffs:
		d.guildaMandaBuffs(w, s, e)
	case guildaAbaLista:
		d.guildaMandaLista(w, s, e)
	case guildaAbaEsquadra:
		// A pagina carrega a ZONA aqui: a escalacao e por cidade, e nao ha
		// paginacao para ela - sessenta nomes cabem num pacote so.
		d.guildaMandaEsquadra(w, s, e, corpo.Pagina)
	}
}

// guildaMandaInfo monta e envia a aba Informações.
//
// O total de membros é a única coisa aqui que não está na memória, então a aba
// sai em duas etapas: o quadro é garantido primeiro (do banco ou da janela de
// 30s), e só então a linha é montada. Se o banco não responder, o painel ainda
// abre — com o total que este processo consegue provar, que é o número de
// conectados. Mostrar a guilda sem o total é melhor do que não mostrar nada.
func (d *Dispatcher) guildaMandaInfo(w *world.World, s *world.Session, e *world.Entity) {
	d.comQuadroDeGuilda(w, s, e.Guild, func(w *world.World, s *world.Session, quadro []world.GuildMemberRecord) {
		e := w.Entity(s.Conn)
		if e == nil || e.Guild == 0 {
			return
		}
		corpo := d.montaInfoDaGuilda(w, e, quadro, presencaDaGuilda(w, e.Guild))
		w.Send(w.Session(s.Conn), protocol.MsgGuildaAbre, corpo.Encode())
	})
}

// presencaDeGuilda é quem da guilda está jogando agora, e onde.
//
// Uma varredura só responde as duas perguntas do painel — quantos estão online e
// quantos estão em cada cidade -, e é por isso que elas viajam juntas: percorrer
// as sessões duas vezes seria contar a mesma gente duas vezes.
type presencaDeGuilda struct {
	online     int16
	porCidade  [protocol.GuildaCidades]int16
	conectados map[string]bool // nome em minúsculas -> está online
}

// presencaDaGuilda percorre as sessões uma vez e responde tudo o que o painel
// precisa saber sobre quem está conectado.
func presencaDaGuilda(w *world.World, guilda uint16) presencaDeGuilda {
	p := presencaDeGuilda{conectados: map[string]bool{}}
	if guilda == 0 {
		return p
	}
	w.ForEachPlaying(-1, func(_ *world.Session, te *world.Entity) {
		if te.Guild != guilda {
			return
		}
		p.online++
		p.conectados[strings.ToLower(te.Name)] = true
		if v := world.Village(te.X, te.Y); v >= 0 && v < protocol.GuildaCidades {
			p.porCidade[v]++
		}
	})
	return p
}

// montaInfoDaGuilda monta a aba Informações.
//
// A presença entra por parâmetro em vez de ser varrida aqui dentro: assim a
// montagem é uma função pura da guilda, do quadro e de quem está online, e pode
// ser conferida sem um servidor de pé.
func (d *Dispatcher) montaInfoDaGuilda(w *world.World, e *world.Entity, quadro []world.GuildMemberRecord, pres presencaDeGuilda) *protocol.GuildaAbreBody {
	info, _ := w.GuildInfo(e.Guild)
	corpo := &protocol.GuildaAbreBody{
		GuildaID:   e.Guild,
		Nome:       guildDisplayName(w, e.Guild),
		Fama:       info.Fame,
		Capacidade: int16(guildaCapacidade(info.MemberCap)),
		Recado:     info.Notice,
		RecadoPor:  info.NoticeBy,
		MeuCargo:   e.GuildLevel,
		Membros:    int16(len(quadro)),
	}
	if !info.NoticeAt.IsZero() {
		corpo.RecadoEm = info.NoticeAt.Unix()
	}
	corpo.Lider = guildaLider(quadro)

	// Aliada e guerra saem resolvidas em NOME: o cliente não tem registro de
	// guildas e não teria como transformar um id em texto.
	if id, ok := d.guildAllies[e.Guild]; ok && id != 0 {
		corpo.Aliada = guildDisplayName(w, id)
	}
	if id, ok := d.guildWars[e.Guild]; ok && id != 0 {
		corpo.Guerra = guildDisplayName(w, id)
	}

	corpo.Online = pres.online
	// Quem está online mas não consta do quadro do banco ainda assim existe: o
	// total nunca pode sair menor que o número de gente conectada na guilda, ou o
	// painel diria "42 online de 0 membros".
	if corpo.Membros < pres.online {
		corpo.Membros = pres.online
	}

	// Convocados é quem está DESIGNADO para a cidade, e não quem está lá agora.
	// Mudou em 21/09/2026, junto com a escalação: a pergunta que a aba responde
	// deixou de ser "quanta gente tenho ali" e passou a ser "quem eu escalei".
	guardado := d.guildaQuadro[e.Guild]
	for i := range corpo.Cidades {
		z := d.guildZones[i]
		corpo.Cidades[i] = protocol.GuildaCidade{
			Zona:       uint8(i),
			Dona:       z.ChargeGuild == e.Guild,
			Imposto:    z.CityTax,
			Convocados: int16(len(guardado.esquadras[i])),
		}
	}
	return corpo
}

// guildaLider é o nome de quem lidera, tirado do quadro.
//
// O quadro já vem ordenado por cargo (ListGuildMembers ordena guild_level DESC),
// então o líder é a primeira linha — mas só se ela de fato for cargo 9: uma
// guilda cujo líder foi apagado do banco tem primeira linha sem ser líder, e
// dizer que um sublíder é o líder é pior do que não dizer nada.
func guildaLider(quadro []world.GuildMemberRecord) string {
	if len(quadro) == 0 || quadro[0].Level != protocol.GuildaCargoLider {
		return ""
	}
	return quadro[0].Name
}

// guildaCapacidade protege contra uma guilda gravada antes da 0079, cuja coluna
// member_cap poderia chegar aqui como zero se alguém escrevesse na tabela à mão.
// Zero na tela seria "249 / 0".
func guildaCapacidade(teto int) int {
	if teto <= 0 {
		return guildaCapacidadePadrao
	}
	return teto
}

// guildaCapacidadePadrao espelha o DEFAULT 250 da coluna member_cap (0079).
const guildaCapacidadePadrao = 250

// guildaMandaMembros envia uma página do quadro de membros.
func (d *Dispatcher) guildaMandaMembros(w *world.World, s *world.Session, e *world.Entity, pagina uint8) {
	d.comQuadroDeGuilda(w, s, e.Guild, func(w *world.World, s *world.Session, quadro []world.GuildMemberRecord) {
		e := w.Entity(s.Conn)
		if e == nil || e.Guild == 0 {
			return
		}
		corpo := guildaPaginaDeMembros(presencaDaGuilda(w, e.Guild), quadro, pagina)
		w.Send(w.Session(s.Conn), protocol.MsgGuildaMembros, corpo.Encode())
	})
}

// guildaPaginaDeMembros recorta uma página do quadro e marca quem está online.
//
// Quem está conectado vem da presença, num mapa por nome: com 250 membros e 40
// linhas por página, perguntar às sessões linha por linha percorreria a lista de
// jogadores quarenta vezes para desenhar uma tela.
//
// O casamento é por NOME em minúsculas porque os dois lados vêm de lugares
// diferentes — o quadro do banco, a presença deste processo — e o nome é a única
// coisa que os dois têm em comum.
func guildaPaginaDeMembros(pres presencaDeGuilda, quadro []world.GuildMemberRecord, pagina uint8) *protocol.GuildaMembrosBody {
	corpo := &protocol.GuildaMembrosBody{Pagina: pagina, Total: int16(len(quadro))}
	inicio := int(pagina) * protocol.GuildaMembrosPorPagina
	if inicio >= len(quadro) {
		return corpo
	}
	fim := min(inicio+protocol.GuildaMembrosPorPagina, len(quadro))
	for _, m := range quadro[inicio:fim] {
		linha := protocol.GuildaMembro{
			Nome:   m.Name,
			Cargo:  m.Level,
			Online: pres.conectados[strings.ToLower(m.Name)],
			Status: m.Status,
		}
		if !m.LastSeen.IsZero() {
			linha.VistoEm = m.LastSeen.Unix()
		}
		corpo.Membros = append(corpo.Membros, linha)
	}
	return corpo
}

// comQuadroDeGuilda entrega o quadro de membros ao chamador, do cache quando ele
// está fresco e do banco quando não está.
//
// É o único caminho até o banco neste arquivo, de propósito: assim a janela de
// 30 segundos e o freio por jogador valem para todas as abas, e não há como uma
// aba nova esquecer de um dos dois.
func (d *Dispatcher) comQuadroDeGuilda(w *world.World, s *world.Session, guilda uint16,
	entao func(*world.World, *world.Session, []world.GuildMemberRecord)) {

	if q, ok := d.guildaQuadro[guilda]; ok && d.now().Sub(q.lidoEm) < guildaQuadroTTL {
		entao(w, s, q.membros)
		return
	}
	agora := d.now()
	if !s.GuildaPedidoEm.IsZero() && agora.Sub(s.GuildaPedidoEm) < guildaPedidoMinIntervalo {
		// Recusa silenciosa: o painel humano nunca chega aqui, e quem chega está
		// pedindo em laço. Uma linha de chat por quadro de vídeo seria pior do que
		// o próprio pedido.
		return
	}
	s.GuildaPedidoEm = agora

	p := w.Persistence()
	if p == nil {
		entao(w, s, nil)
		return
	}
	w.Go(s, func() func(*world.World, *world.Session) {
		ctx, cancel := context.WithTimeout(context.Background(), guildStateFetchTimeout)
		defer cancel()
		quadro, err := p.ListGuildMembers(ctx, guilda)
		esquadras, errEsq := p.ListGuildSquads(ctx, guilda)
		return func(w *world.World, s *world.Session) {
			if errEsq != nil {
				// A escalação falhar não impede o painel: ela é uma coluna a
				// menos, e o resto da aba continua valendo.
				d.log.Warn("painel de guilda: escalações não vieram", "guilda", guilda, "err", errEsq)
			}
			if err != nil {
				d.log.Warn("painel de guilda: leitura do quadro falhou", "guilda", guilda, "err", err)
				// Segue com o que der: a aba Informações ainda sabe contar quem
				// está online, e a aba Membros aparece vazia em vez de não aparecer.
				entao(w, s, nil)
				return
			}
			// A ordem vem do banco (cargo decrescente, depois nome), mas ela é
			// reafirmada aqui porque a paginação depende dela: duas páginas pedidas
			// em ordens diferentes mostrariam o mesmo membro duas vezes.
			sort.SliceStable(quadro, func(i, j int) bool {
				if quadro[i].Level != quadro[j].Level {
					return quadro[i].Level > quadro[j].Level
				}
				return strings.ToLower(quadro[i].Name) < strings.ToLower(quadro[j].Name)
			})
			guardado := quadroDeGuilda{membros: quadro, lidoEm: d.now()}
			for _, sq := range esquadras {
				if sq.Zone >= 0 && sq.Zone < protocol.GuildaCidades {
					guardado.esquadras[sq.Zone] = sq.Names
				}
			}
			d.guildaQuadro[guilda] = guardado
			entao(w, s, quadro)
		}
	})
}

// guildaEsqueceQuadro joga fora o quadro guardado de uma guilda, para a próxima
// leitura ir ao banco.
//
// Chamado por quem MUDA o quadro — entrar, sair, ser expulso, ser promovido -,
// porque a janela de 30 segundos existe para poupar consulta repetida, não para
// mostrar uma guilda que já não é aquela. Sem isto, um expulso continuaria na
// lista por meio minuto, que é exatamente o meio minuto em que alguém vai olhar.
func (d *Dispatcher) guildaEsqueceQuadro(guilda uint16) {
	if guilda != 0 {
		delete(d.guildaQuadro, guilda)
	}
}

// guildaConvoca atende MsgGuildaConvoca: o botão Convocar de uma cidade.
//
// Reaproveita o /convocar inteiro (summonGuild): as regras de quem pode e de
// onde pode são as mesmas, e ter duas cópias delas seria ter duas respostas para
// a mesma pergunta. O botão é outro caminho até o mesmo comando, não um comando
// novo.
func (d *Dispatcher) guildaConvoca(w *world.World, s *world.Session, _ protocol.Header, payload []byte) {
	e := w.Entity(s.Conn)
	if e == nil || s.Mode != world.UserPlay {
		return
	}
	if _, err := protocol.DecodeGuildaPede(payload); err != nil {
		return
	}
	if e.Guild == 0 {
		sendClientMessage(w, s, msgGuildaSemGuilda)
		return
	}
	// A cidade do pedido é ignorada de propósito, e isto não é um esquecimento:
	// convocar puxa a guilda para ONDE QUEM CHAMOU ESTÁ, e quem chamou está numa
	// cidade só. Aceitar a zona do cliente seria deixá-lo teleportar a guilda
	// para uma cidade onde ninguém está — a lista de cidades do painel é para
	// mostrar quanta gente há em cada uma, não para escolher o destino.
	if world.Village(e.X, e.Y) < 0 {
		sendClientMessage(w, s, msgGuildaConvocaForaDaCidade)
		return
	}
	d.summonGuild(w, s)
}

// guildaRecado atende MsgGuildaRecado: o botão Escrever do quadro de avisos.
func (d *Dispatcher) guildaRecado(w *world.World, s *world.Session, _ protocol.Header, payload []byte) {
	e := w.Entity(s.Conn)
	if e == nil || s.Mode != world.UserPlay || e.Guild == 0 {
		return
	}
	corpo, err := protocol.DecodeGuildaTexto(payload, protocol.GuildaRecadoMax)
	if err != nil {
		return
	}
	texto := limpaLinhaDeTexto(corpo.Texto)
	guilda, autor := e.Guild, e.Name

	// A memória muda agora para o painel de quem escreveu responder no ato; o
	// banco confirma depois. Se o banco recusar, a memória volta atrás — é o que
	// evita um recado que aparece para quem escreveu e some no próximo login.
	info, _ := w.GuildInfo(guilda)
	anterior := info
	w.SetGuildNotice(guilda, texto, autor, d.now())

	p := w.Persistence()
	if p == nil {
		return
	}
	w.Go(s, func() func(*world.World, *world.Session) {
		ctx, cancel := context.WithTimeout(context.Background(), guildStateFetchTimeout)
		defer cancel()
		err := p.SaveGuildNotice(ctx, guilda, texto, autor)
		return func(w *world.World, s *world.Session) {
			if err != nil {
				d.log.Warn("painel de guilda: gravar recado falhou", "guilda", guilda, "err", err)
				w.SetGuildNotice(guilda, anterior.Notice, anterior.NoticeBy, anterior.NoticeAt)
				sendClientMessage(w, s, msgGuildaRecadoFalhou)
				return
			}
			d.guildaAvisaMembros(w, guilda, autor)
		}
	})
}

// guildaAvisaMembros conta aos conectados que o recado mudou.
//
// Só a quem está NA guilda, e por mensagem de chat em vez de reabrir o painel de
// todo mundo: quem está com a janela aberta pede de novo quando quiser, e quem
// não está não tem por que ver uma janela pular na tela.
func (d *Dispatcher) guildaAvisaMembros(w *world.World, guilda uint16, autor string) {
	w.ForEachPlaying(-1, func(ts *world.Session, te *world.Entity) {
		if te.Guild == guilda {
			sendClientMessage(w, ts, msgGuildaRecadoNovo(autor))
		}
	})
}

// guildaStatus atende MsgGuildaStatus: a linha que o membro escreve para si.
func (d *Dispatcher) guildaStatus(w *world.World, s *world.Session, _ protocol.Header, payload []byte) {
	e := w.Entity(s.Conn)
	if e == nil || s.Mode != world.UserPlay || e.Guild == 0 {
		return
	}
	corpo, err := protocol.DecodeGuildaTexto(payload, protocol.GuildaStatusMax)
	if err != nil {
		return
	}
	// O status mora no quadro, que é lido do banco: mudá-lo aqui sem invalidar o
	// cache faria o próprio autor ver o texto antigo por até trinta segundos.
	d.guildaEsqueceQuadro(e.Guild)
	d.log.Info("painel de guilda: status",
		"conn", s.Conn, "guilda", e.Guild, "tamanho", len(corpo.Texto))
	// A gravação em si entra junto com a aba Membros editável; por ora o texto é
	// aceito, limpo e descartado, e é por isso que este caminho não grava nada.
	// Deixá-lo aqui mudo seria pior: o cliente já manda o pacote, e um pacote sem
	// tratador vira log de erro a cada digitação.
	_ = limpaLinhaDeTexto(corpo.Texto)
}

// limpaLinhaDeTexto tira o que não pode viajar num texto que outros vão ler: as
// pontas em branco e os bytes de controle, incluindo a quebra de linha.
//
// A quebra de linha sai porque a linha de chat do jogo termina no primeiro \n, e
// um recado de duas linhas chegaria pela metade para quem o lesse pelo aviso.
func limpaLinhaDeTexto(s string) string {
	limpo := strings.Map(func(r rune) rune {
		if r < 0x20 || r == 0x7F {
			return ' '
		}
		return r
	}, s)
	return strings.TrimSpace(limpo)
}

// guildaMandaLista atende a tela "Guilds do Server".
//
// Vai ao banco toda vez, e não tem cache como o quadro de membros: a lista é
// aberta raramente (é uma tela de curiosidade, não de trabalho) e uma guilda
// nova que não aparecesse nela seria estranho justo para quem acabou de criá-la.
//
// O freio por jogador é o mesmo do quadro, e pelo mesmo motivo.
func (d *Dispatcher) guildaMandaLista(w *world.World, s *world.Session, e *world.Entity) {
	agora := d.now()
	if !s.GuildaPedidoEm.IsZero() && agora.Sub(s.GuildaPedidoEm) < guildaPedidoMinIntervalo {
		return
	}
	s.GuildaPedidoEm = agora

	p := w.Persistence()
	if p == nil {
		w.Send(s, protocol.MsgGuildaLista, (&protocol.GuildaListaBody{MinhaGuilda: e.Guild}).Encode())
		return
	}
	minha := e.Guild
	w.Go(s, func() func(*world.World, *world.Session) {
		ctx, cancel := context.WithTimeout(context.Background(), guildStateFetchTimeout)
		defer cancel()
		guildas, err := p.ListGuildSummaries(ctx, protocol.GuildaListaMax)
		return func(w *world.World, s *world.Session) {
			if err != nil {
				d.log.Warn("painel de guilda: lista falhou", "err", err)
				sendClientMessage(w, s, msgGuildaListaFalhou)
				return
			}
			corpo := &protocol.GuildaListaBody{MinhaGuilda: minha}
			for _, g := range guildas {
				corpo.Guildas = append(corpo.Guildas, protocol.GuildaDaLista{
					ID:      g.ID,
					Nome:    g.Name,
					Lider:   g.Leader,
					Membros: int16(g.Members),
					Fama:    g.Fame,
				})
			}
			w.Send(s, protocol.MsgGuildaLista, corpo.Encode())
		}
	})
}

// guildaCria atende MsgGuildaCria: o botão "Criar sua Guild" do painel.
//
// Reaproveita o /create inteiro (createGuild, em guild.go), e isso é a decisão
// importante: as regras de criar guilda são sete — nível, clã, cidadania, dia da
// semana, nome válido, nome livre e o custo em ouro -, e tê-las em dois lugares
// seria garantir que um dia elas discordassem. O botão é outra porta para o
// mesmo comando, não um comando novo.
func (d *Dispatcher) guildaCria(w *world.World, s *world.Session, _ protocol.Header, payload []byte) {
	e := w.Entity(s.Conn)
	if e == nil || s.Mode != world.UserPlay {
		return
	}
	corpo, err := protocol.DecodeGuildaTexto(payload, protocol.GuildaNomeMaxCriar)
	if err != nil {
		return
	}
	nome := limpaLinhaDeTexto(corpo.Texto)
	if nome == "" {
		sendClientMessage(w, s, msgGuildaNomeVazio)
		return
	}
	d.createGuild(w, s, []byte(nome))
}

// --- a escalação de cidade (0081_convocacao_de_guilda) ----------------------
//
// A aba Cidades deixou de ser só informação: o líder escolhe, de dentro da
// guilda, quem defende cada cidade. A escalação vive no banco e é lida quando a
// aba abre.

// guildaMandaEsquadra envia quem está designado para uma cidade.
func (d *Dispatcher) guildaMandaEsquadra(w *world.World, s *world.Session, e *world.Entity, zona uint8) {
	if int(zona) >= protocol.GuildaCidades {
		return
	}
	p := w.Persistence()
	if p == nil {
		w.Send(s, protocol.MsgGuildaEsquadra, (&protocol.GuildaEsquadraBody{Zona: zona}).Encode())
		return
	}
	guilda := e.Guild
	w.Go(s, func() func(*world.World, *world.Session) {
		ctx, cancel := context.WithTimeout(context.Background(), guildStateFetchTimeout)
		defer cancel()
		esquadras, err := p.ListGuildSquads(ctx, guilda)
		return func(w *world.World, s *world.Session) {
			if err != nil {
				d.log.Warn("painel de guilda: escalação falhou", "guilda", guilda, "err", err)
				return
			}
			corpo := &protocol.GuildaEsquadraBody{Zona: zona}
			for _, sq := range esquadras {
				if sq.Zone == int(zona) {
					corpo.Nomes = sq.Names
					break
				}
			}
			w.Send(s, protocol.MsgGuildaEsquadra, corpo.Encode())
		}
	})
}

// guildaDesigna atende MsgGuildaDesigna: trocar a escalação de uma cidade.
//
// Os nomes são conferidos contra o QUADRO da guilda antes de gravar, e é o ponto
// do desenho: sem isso, um cliente remendado escalaria qualquer nome do servidor
// — inclusive de outra guilda — para a cidade dele. Só entra quem é membro.
func (d *Dispatcher) guildaDesigna(w *world.World, s *world.Session, _ protocol.Header, payload []byte) {
	e := w.Entity(s.Conn)
	if e == nil || s.Mode != world.UserPlay || e.Guild == 0 {
		return
	}
	corpo, err := protocol.DecodeGuildaEsquadra(payload)
	if err != nil {
		return
	}
	if int(corpo.Zona) >= protocol.GuildaCidades {
		return
	}
	guilda, zona := e.Guild, int(corpo.Zona)
	pedidos := corpo.Nomes

	d.comQuadroDeGuilda(w, s, guilda, func(w *world.World, s *world.Session, quadro []world.GuildMemberRecord) {
		daGuilda := make(map[string]string, len(quadro))
		for _, m := range quadro {
			daGuilda[strings.ToLower(m.Name)] = m.Name
		}
		var nomes []string
		for _, n := range pedidos {
			if canonico, ok := daGuilda[strings.ToLower(n)]; ok && len(nomes) < protocol.GuildaEsquadraMax {
				nomes = append(nomes, canonico)
			}
		}
		p := w.Persistence()
		if p == nil {
			return
		}
		w.Go(s, func() func(*world.World, *world.Session) {
			ctx, cancel := context.WithTimeout(context.Background(), guildStateFetchTimeout)
			defer cancel()
			err := p.SetGuildSquad(ctx, guilda, zona, nomes)
			return func(w *world.World, s *world.Session) {
				if err != nil {
					d.log.Warn("painel de guilda: gravar escalação falhou",
						"guilda", guilda, "zona", zona, "err", err)
					sendClientMessage(w, s, msgGuildaEscalaFalhou)
					return
				}
				d.log.Info("escalação de cidade gravada",
					"guilda", guilda, "zona", zona, "nomes", len(nomes))
				if e := w.Entity(s.Conn); e != nil {
					d.guildaMandaEsquadra(w, s, e, uint8(zona))
					d.guildaMandaInfo(w, s, e)
				}
			}
		})
	})
}

// guildaAcao atende MsgGuildaAcao: promover, passar a liderança, expulsar e sair.
//
// NENHUMA REGRA PRÓPRIA AQUI, e isso é o desenho: cada ação chama o comando nativo
// que o chat já chamava, com o nome no lugar do argumento. Quem decide se pode é o
// comando — ele é que sabe de cargo, de guilda, de cidade e de tudo o que veio do
// legado. Uma segunda cópia dessas regras aqui divergiria no dia em que só uma fosse
// corrigida, e a divergência seria alguém expulsando quem não podia.
func (d *Dispatcher) guildaAcao(w *world.World, s *world.Session, _ protocol.Header, payload []byte) {
	e := w.Entity(s.Conn)
	if e == nil || s.Mode != world.UserPlay {
		return
	}
	corpo, err := protocol.DecodeGuildaAcao(payload)
	if err != nil {
		return
	}
	if e.Guild == 0 {
		sendClientMessage(w, s, msgGuildaSemGuilda)
		return
	}
	// SOBRE SI MESMO SÓ VALE O "SAIR". Promover-se, passar a liderança para si ou
	// expulsar-se são pedidos que o painel não oferece — e que, chegando mesmo
	// assim, encontrariam comandos escritos para agir sobre OUTRO. Recusar aqui é
	// mais barato do que descobrir o que cada um faz consigo.
	if corpo.Acao != protocol.GuildaAcaoSai && strings.EqualFold(corpo.Nome, e.Name) {
		return
	}
	// O quadro de membros muda em qualquer uma das quatro, então a página guardada
	// tem de sair ANTES: senão o próximo pedido do painel devolve a lista velha, com
	// quem já saiu.
	d.guildaEsqueceQuadro(e.Guild)
	switch corpo.Acao {
	case protocol.GuildaAcaoPromove:
		d.subcreate(w, s, []byte(corpo.Nome))
	case protocol.GuildaAcaoLideranca:
		d.handoverGuild(w, s, []byte(corpo.Nome))
	case protocol.GuildaAcaoExpulsa:
		d.kickGuild(w, s, []byte(corpo.Nome))
	case protocol.GuildaAcaoSai:
		d.leaveGuild(w, s)
	}
}

// guildaImposto atende MsgGuildaImposto: a taxa da cidade que a guilda domina.
//
// A ZONA DO PEDIDO É IGNORADA DE PROPÓSITO, pelo mesmo motivo do Convocar: a guilda
// cobra de UMA cidade, e é o servidor que sabe qual. Aceitar a zona do cliente seria
// deixá-lo mexer no imposto da cidade dos outros. Ela viaja no corpo só para o log
// poder dizer sobre o que era o pedido.
//
// E a linha é montada e entregue ao guildTax INTEIRO, em vez de repetir as regras:
// precisa ser líder, precisa haver cidade cobrada, a taxa vai de 0 a 30 e só muda uma
// vez por dia. São quatro regras, e é exatamente por serem quatro que elas ficam num
// lugar só.
func (d *Dispatcher) guildaImposto(w *world.World, s *world.Session, _ protocol.Header, payload []byte) {
	e := w.Entity(s.Conn)
	if e == nil || s.Mode != world.UserPlay {
		return
	}
	corpo, err := protocol.DecodeGuildaImposto(payload)
	if err != nil {
		return
	}
	if e.Guild == 0 {
		sendClientMessage(w, s, msgGuildaSemGuilda)
		return
	}
	d.log.Info("guilda: pedido de imposto", "conn", s.Conn, "guilda", e.Guild,
		"zona_pedida", corpo.Zona, "taxa", corpo.Ticks)
	d.guildTax(w, s, fmt.Sprintf("guildtax %d", corpo.Ticks))
}
