package handler

import (
	"context"
	"fmt"
	"time"

	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/protocol"
	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/world"
)

// Os buffs de guilda: quatro vantagens que valem para a guilda inteira enquanto
// o relógio corre.
//
// Nada disto existe no legado — é conteúdo novo, pedido em 21/09/2026 junto com
// o painel.
//
// DUAS DECISÕES DE DESENHO, as duas deliberadas:
//
//  1. NÃO são afetos. Passar por SetAffect seria o caminho curto e está errado
//     por dois motivos: os afetos de um personagem são poucos e disputados, e um
//     buff de guilda comeria a casa de um buff de skill que o jogador pagou de
//     mana; e este servidor corta a duração dos afetos a dez minutos
//     (-affect-max-minutes), o que faria um buff de duas horas durar dez. O buff
//     de guilda entra onde entram os bônus de equipamento, em refreshScore, e o
//     relógio dele é da GUILDA, não do personagem.
//
//  2. Não há pacote que ligue um buff. Quem liga é um item de cash, usado pelo
//     caminho comum de usar item. Assim não existe um "me dê um buff" para um
//     cliente remendado mandar, e o custo é cobrado onde já se cobra por cash.
//
// O buff pertence à guilda e segue quem entrar nela: quem se juntar no meio da
// hora pega o resto da hora. É a consequência de o relógio ser da guilda, e é a
// leitura certa — o buff é uma vantagem da guilda, não um presente para os
// nomes que estavam online no momento em que alguém clicou.

// Os quatro buffs. O número é o que viaja no pacote e o que o item nomeia, então
// ele é estável: acrescentar é seguro, reordenar troca o que cada item faz.
const (
	buffGuildaVida   uint8 = 1 // vida máxima
	buffGuildaDefesa uint8 = 2 // defesa
	buffGuildaDano   uint8 = 3 // dano físico E mágico, os dois juntos
	buffGuildaDrop   uint8 = 4 // chance de drop
)

// receitaDeBuff é o que um buff faz e por quanto tempo.
//
// Os números são de partida e mexem com equilíbrio, então vivem juntos, num
// lugar só, em vez de espalhados pelo cálculo de score.
type receitaDeBuff struct {
	Tipo uint8
	Nome string

	VidaPc int // soma em HpAddPct: vida máxima, em porcento
	ACPc   int // soma percentual na defesa
	DanoPc int // soma em DanoFisicoPct E DanoMagicoPct, os dois
	Drop   int // soma em EquipDropBonus, na escala das fadas
}

// receitasDeBuff é a tabela dos quatro, na ordem do painel.
//
// Os números são de partida e mexem com equilíbrio, então estão todos aqui, à
// vista um do outro. Duas notas sobre a escala:
//
// A vida é percentual sobre a vida máxima já somada, pelo mesmo caminho do
// EF_HPADD dos equipamentos — é o campo que o jogo já sabe aplicar, e usá-lo faz
// o buff se comportar como um item de vida, que é o que ele é.
//
// O drop NÃO é percentual, porque o número que o jogo usa não é uma
// percentagem: é uma escala própria onde uma Fada Vermelha vale 16 e cada peça
// de grau 5 vale 8 (drop_bonus.go). Vinte põe o buff um pouco acima de uma fada,
// que é o tamanho certo para uma vantagem de guilda inteira.
var receitasDeBuff = [...]receitaDeBuff{
	{Tipo: buffGuildaVida, Nome: "Buff de Vida", VidaPc: 15},
	{Tipo: buffGuildaDefesa, Nome: "Buff de Defesa", ACPc: 18},
	{Tipo: buffGuildaDano, Nome: "Buff de Dano", DanoPc: 15},
	{Tipo: buffGuildaDrop, Nome: "Buff de Drop", Drop: 20},
}

// receitaDoBuff acha a receita de um tipo.
func receitaDoBuff(tipo uint8) (receitaDeBuff, bool) {
	for _, r := range receitasDeBuff {
		if r.Tipo == tipo {
			return r, true
		}
	}
	return receitaDeBuff{}, false
}

// buffsDaGuilda é o relógio dos quatro buffs de uma guilda: quando cada um
// termina. Tempo zero significa desligado.
type buffsDaGuilda struct {
	expira [protocol.GuildaBuffs]time.Time
}

// ligado diz se o buff daquela posição ainda vale em "agora".
func (b *buffsDaGuilda) ligado(i int, agora time.Time) bool {
	return !b.expira[i].IsZero() && agora.Before(b.expira[i])
}

// bonusDeGuilda é o que os buffs ligados somam ao score de um membro.
type bonusDeGuilda struct {
	vidaPc int
	acPc   int
	danoPc int
	drop   int
}

func (b bonusDeGuilda) vazio() bool { return b == bonusDeGuilda{} }

// bonusDeBuffDeGuilda soma o que os buffs ligados da guilda de e valem agora.
//
// Chamada de dentro de refreshScore, que roda em toda troca de equipamento e em
// todo golpe que mexe no score — por isso ela não aloca e não vai ao banco: é um
// laço de quatro posições sobre um mapa pequeno.
func (d *Dispatcher) bonusDeBuffDeGuilda(e *world.Entity) bonusDeGuilda {
	var soma bonusDeGuilda
	if e == nil || e.Guild == 0 || !world.IsPlayer(e.ID) {
		return soma
	}
	b := d.guildaBuffs[e.Guild]
	if b == nil {
		return soma
	}
	agora := d.now()
	for i, r := range receitasDeBuff {
		if !b.ligado(i, agora) {
			continue
		}
		soma.vidaPc += r.VidaPc
		soma.acPc += r.ACPc
		soma.danoPc += r.DanoPc
		soma.drop += r.Drop
	}
	return soma
}

// ligaBuffDeGuilda acende um buff para a guilda e devolve quando ele termina.
//
// Usar um item com um buff JÁ ligado soma o tempo em vez de reiniciá-lo: o item
// foi pago, e um jogador que clica dois minutos cedo não deve perder uma hora
// por isso. O teto existe para o tempo não virar estoque — acumular vinte itens
// e ficar um dia inteiro buffado é outra coisa, e não é esta.
func (d *Dispatcher) ligaBuffDeGuilda(w *world.World, guilda uint16, tipo uint8, dura time.Duration) (time.Time, bool) {
	if _, ok := receitaDoBuff(tipo); !ok || guilda == 0 || dura <= 0 {
		return time.Time{}, false
	}
	i := int(tipo) - 1
	if i < 0 || i >= protocol.GuildaBuffs {
		return time.Time{}, false
	}
	b := d.guildaBuffs[guilda]
	if b == nil {
		b = &buffsDaGuilda{}
		d.guildaBuffs[guilda] = b
	}
	agora := d.now()
	base := agora
	if b.ligado(i, agora) {
		base = b.expira[i]
	}
	fim := base.Add(dura)
	if teto := agora.Add(buffGuildaTetoAcumulado); fim.After(teto) {
		fim = teto
	}
	b.expira[i] = fim
	d.gravaBuffDeGuilda(w, guilda, tipo, fim)

	// O score de quem está online muda AGORA: sem isto o buff só apareceria no
	// próximo refresh — uma troca de equipamento, um level -, e o jogador veria
	// "ativo" no painel sem sentir diferença nenhuma.
	d.refrescaGuilda(w, guilda)
	return fim, true
}

// gravaBuffDeGuilda manda a expiração para o banco, fora do laço.
//
// A memória já foi mudada quando isto roda, e é de propósito: o jogador não
// espera o banco para o buff valer. O que o banco garante é o RESTART — um item
// de 30 dias comprado com cash não pode morrer numa manutenção.
func (d *Dispatcher) gravaBuffDeGuilda(w *world.World, guilda uint16, tipo uint8, fim time.Time) {
	p := w.Persistence()
	if p == nil {
		return
	}
	w.GoDetached(func() func(*world.World) {
		ctx, cancel := context.WithTimeout(context.Background(), guildStateFetchTimeout)
		defer cancel()
		err := p.SaveGuildBuff(ctx, world.GuildBuffRecord{GuildID: guilda, Type: tipo, ExpiresAt: fim})
		return func(*world.World) {
			if err != nil {
				// Não desfaz a memória: o buff foi pago e vale nesta sessão. O que
				// se perde é a travessia do restart, e isso o log tem de dizer.
				d.log.Error("buff de guilda não foi gravado; não sobrevive a um restart",
					"guilda", guilda, "buff", tipo, "err", err)
			}
		}
	})
}

// apagaBuffDeGuilda tira do banco a linha de um buff que venceu.
func (d *Dispatcher) apagaBuffDeGuilda(w *world.World, guilda uint16, tipo uint8) {
	p := w.Persistence()
	if p == nil {
		return
	}
	w.GoDetached(func() func(*world.World) {
		ctx, cancel := context.WithTimeout(context.Background(), guildStateFetchTimeout)
		defer cancel()
		err := p.DeleteGuildBuff(ctx, guilda, tipo)
		return func(*world.World) {
			if err != nil {
				// Uma linha vencida que sobrou é inofensiva: a leitura do boot só
				// traz o que ainda vale. Fica no log para não virar lixo calado.
				d.log.Warn("buff de guilda vencido não foi apagado",
					"guilda", guilda, "buff", tipo, "err", err)
			}
		}
	})
}

// restauraBuffsDeGuilda repõe na memória os buffs que o banco diz que ainda
// valem. Chamada no boot, antes de qualquer jogador entrar.
func (d *Dispatcher) restauraBuffsDeGuilda(buffs []world.GuildBuffRecord) {
	agora := d.now()
	for _, b := range buffs {
		i := int(b.Type) - 1
		if i < 0 || i >= protocol.GuildaBuffs || b.GuildID == 0 {
			continue
		}
		// O banco já filtra o vencido, mas o relógio dele não é o deste processo:
		// conferir de novo é o que impede um buff morto de voltar à vida por causa
		// de alguns segundos de diferença entre as duas máquinas.
		if !b.ExpiresAt.After(agora) {
			continue
		}
		g := d.guildaBuffs[b.GuildID]
		if g == nil {
			g = &buffsDaGuilda{}
			d.guildaBuffs[b.GuildID] = g
		}
		g.expira[i] = b.ExpiresAt
	}
}

// buffGuildaTetoAcumulado é o máximo de tempo que um buff pode ter à frente.
//
// Sessenta dias, ou dois itens de 30 empilhados. O teto existe para o tempo não
// virar estoque — comprar vinte itens e deixar a guilda buffada por um ano é
// outro produto -, mas ele tem de caber com folga em cima do maior item, senão
// o segundo item que alguém compra já é jogado fora pela metade.
const buffGuildaTetoAcumulado = 60 * 24 * time.Hour

// refrescaGuilda recalcula o score de todo membro conectado da guilda e manda o
// novo para a tela dele.
func (d *Dispatcher) refrescaGuilda(w *world.World, guilda uint16) {
	if guilda == 0 {
		return
	}
	w.ForEachPlaying(-1, func(ts *world.Session, te *world.Entity) {
		if te.Guild != guilda {
			return
		}
		d.refreshScore(te)
		d.sendScore(w, ts, te)
	})
}

// tickBuffsDeGuilda apaga os buffs que venceram e devolve o score de quem os
// tinha.
//
// Roda a cada tick e quase sempre não faz nada: o laço é sobre as guildas que
// TÊM buff, que são poucas, e só mexe em alguém no tick exato em que um buff
// vence. Sem ele o bônus sumiria do cálculo na hora certa (bonusDeBuffDeGuilda
// confere o relógio), mas o número na tela do jogador só se corrigiria no
// próximo refresh — ele andaria com a barra dizendo uma coisa e o servidor
// calculando outra.
func (d *Dispatcher) tickBuffsDeGuilda(w *world.World) {
	if len(d.guildaBuffs) == 0 {
		return
	}
	agora := d.now()
	for guilda, b := range d.guildaBuffs {
		venceu := false
		algumLigado := false
		for i := range b.expira {
			switch {
			case b.expira[i].IsZero():
			case agora.Before(b.expira[i]):
				algumLigado = true
			default:
				b.expira[i] = time.Time{}
				venceu = true
				d.apagaBuffDeGuilda(w, guilda, uint8(i+1))
			}
		}
		if venceu {
			d.refrescaGuilda(w, guilda)
			d.avisaBuffVencido(w, guilda)
		}
		if !algumLigado && !venceu {
			// Nada ligado e nada vencendo: a guilda não precisa mais de linha no
			// mapa. Limpar aqui é o que impede o mapa de crescer com uma entrada
			// por guilda que já usou um buff algum dia.
			delete(d.guildaBuffs, guilda)
		}
	}
}

// avisaBuffVencido conta aos conectados da guilda que um buff caiu.
func (d *Dispatcher) avisaBuffVencido(w *world.World, guilda uint16) {
	w.ForEachPlaying(-1, func(ts *world.Session, te *world.Entity) {
		if te.Guild == guilda {
			sendClientMessage(w, ts, msgGuildaBuffVenceu)
		}
	})
}

// guildaMandaBuffs envia a aba Buffs: o estado dos quatro e os itens da mochila.
func (d *Dispatcher) guildaMandaBuffs(w *world.World, s *world.Session, e *world.Entity) {
	corpo := d.corpoDeBuffs(e.Guild)
	corpo.Itens = itensDeBuffNaMochila(e)
	w.Send(s, protocol.MsgGuildaBuffs, corpo.Encode())
}

// itensDeBuffNaMochila lista os Guild Buff que o jogador carrega.
func itensDeBuffNaMochila(e *world.Entity) []protocol.GuildaItemDeBuff {
	var out []protocol.GuildaItemDeBuff
	for i := range e.Carry {
		dura := duracaoDoItemDeBuff(e.Carry[i].Index)
		if dura <= 0 {
			continue
		}
		if len(out) >= protocol.GuildaItensDeBuffMax {
			break
		}
		out = append(out, protocol.GuildaItemDeBuff{
			Slot:   int16(i),
			Indice: e.Carry[i].Index,
			Dias:   int16(dura / (24 * time.Hour)),
		})
	}
	return out
}

// corpoDeBuffs monta a aba Buffs. Separada do envio para ter teste sem rede.
func (d *Dispatcher) corpoDeBuffs(guilda uint16) *protocol.GuildaBuffsBody {
	corpo := &protocol.GuildaBuffsBody{}
	b := d.guildaBuffs[guilda]
	agora := d.now()
	for i, r := range receitasDeBuff {
		corpo.Buffs[i] = protocol.GuildaBuff{Tipo: r.Tipo}
		if b == nil || !b.ligado(i, agora) {
			continue
		}
		// Segundos arredondados para CIMA: um buff com 400 milissegundos de vida
		// mandado como 0 seria desenhado como desligado, e o painel piscaria
		// "inativo" um tique antes de o servidor concordar.
		restam := b.expira[i].Sub(agora)
		corpo.Buffs[i].Ativo = true
		corpo.Buffs[i].Restam = int32((restam + time.Second - 1) / time.Second)
	}
	return corpo
}

// itemDoBuffDeGuilda é o item de cash que acende os buffs da guilda: UM item
// para os quatro.
//
// É o Ticket_Serviço(24hrs), reaproveitado. O item já existe no catálogo
// (Release/Common/ItemList.csv:5243) com EF_VOLATILE 197, e o 197 é um volátil
// MORTO: não é tratado em nenhum lugar do TMSrv legado nem deste servidor —
// conferido varrendo Source/Code/TMSrv/ e handler/ inteiros em 21/09/2026. Usar
// um Ticket hoje não faz nada, então reaproveitá-lo não tira função de ninguém.
//
// Reaproveitar em vez de criar um índice novo poupa o outro lado: um item novo
// precisaria de linha no ItemList do SERVIDOR e no do CLIENTE, mais arte, e um
// cliente desatualizado desenharia um quadrado vazio na mochila.
//
// SAO DOIS, e a diferenca entre eles e so o tempo: o 3439 (que era o Ticket de
// 24 horas) vale 15 dias, e o 3440 (que era o de 7 dias) vale 30. Os dois
// acendem os mesmos quatro buffs.
var itensDeBuffDeGuilda = map[int16]time.Duration{
	3439: 15 * 24 * time.Hour,
	3440: 30 * 24 * time.Hour,
}

// acendeUmBuff usa o item da casa dada para acender UM buff.
//
// Um item, um buff, escolhido pelo jogador. Começou diferente - um item acendia
// os quatro -, e a Josiel trocou em 21/09/2026 ao ver o resultado: acender tudo
// de uma vez tira a escolha, e a escolha é o que faz o item valer a pena guardar.
//
// A conferência é dupla e é o ponto do desenho: o TIPO tem de existir e a CASA
// tem de ter mesmo um Guild Buff. Um cliente remendado que mande uma casa vazia,
// ou um tipo inventado, não acende nada.
func (d *Dispatcher) acendeUmBuff(w *world.World, s *world.Session, e *world.Entity, src int, tipo uint8) {
	if e.Guild == 0 {
		sendClientMessage(w, s, msgGuildaSemGuilda)
		return
	}
	if src < 0 || src >= len(e.Carry) {
		return
	}
	dura := duracaoDoItemDeBuff(e.Carry[src].Index)
	if dura <= 0 {
		sendClientMessage(w, s, msgGuildaSemItemDeBuff)
		return
	}
	r, ok := receitaDoBuff(tipo)
	if !ok {
		return
	}
	fim, ok := d.ligaBuffDeGuilda(w, e.Guild, tipo, dura)
	if !ok {
		return
	}
	consumeOneItem(&e.Carry[src])
	d.sendSlot(w, s, world.ItemPlaceCarry, src, e.Carry[src])

	// O aviso vai para a guilda inteira, e não só para quem gastou: o item é
	// caro e o efeito é de todos, então quem pagou merece que os outros saibam.
	linha := msgGuildaBuffLigado(r.Nome, e.Name, fim.Sub(d.now()))
	w.ForEachPlaying(-1, func(ts *world.Session, te *world.Entity) {
		if te.Guild == e.Guild {
			sendClientMessage(w, ts, linha)
		}
	})
	d.log.Info("buff de guilda ligado",
		"conn", s.Conn, "guilda", e.Guild, "buff", r.Nome, "resta", fim.Sub(d.now()).String())
}

// useBuffDeGuilda atende o uso do item DIRETO da mochila.
//
// Ele não acende nada, e isso é deliberado: desde que cada item acende UM buff
// escolhido, usar o item no inventário não diz qual. Escolher um por conta
// própria gastaria o item do jogador numa decisão que não foi dele.
func (d *Dispatcher) useBuffDeGuilda(w *world.World, s *world.Session, e *world.Entity, src int) {
	sendClientMessage(w, s, msgGuildaUseOPainel)
	d.sendSlot(w, s, world.ItemPlaceCarry, src, e.Carry[src])
}

// As linhas que o painel e o chat mostram.
const (
	msgGuildaSemGuilda           = "Você não está em uma guilda."
	msgGuildaConvocaForaDaCidade = "A convocação só funciona dentro de uma cidade."
	msgGuildaRecadoFalhou        = "Não foi possível gravar o recado agora."
	msgGuildaBuffVenceu          = "Um buff da guilda terminou."
	msgGuildaListaFalhou         = "Não foi possível listar as guildas agora."
	msgGuildaNomeVazio           = "Escolha um nome para a guilda."
	msgGuildaSemItemDeBuff       = "Você não tem um Guild Buff na mochila."
	msgGuildaUseOPainel          = "Abra o Painel de Guild (G), aba Buffs, e escolha qual ativar."
	msgGuildaEscalaFalhou        = "Não foi possível gravar a escalação agora."
)

func msgGuildaRecadoNovo(autor string) string {
	return fmt.Sprintf("%s escreveu um novo recado da guilda.", autor)
}

func msgGuildaBuffLigado(buff, autor string, resta time.Duration) string {
	return fmt.Sprintf("%s ativou o %s: %s.", autor, buff, duracaoEmTexto(resta))
}

// duracaoEmTexto escreve um tempo do jeito que um jogador o leria.
//
// Um buff destes vale semanas, e "21600 minuto(s)" nao diz nada a ninguem. A
// unidade acompanha a grandeza: dias enquanto houver dias, horas enquanto
// houver horas, minutos no fim.
func duracaoEmTexto(d time.Duration) string {
	switch {
	case d >= 48*time.Hour:
		return fmt.Sprintf("%d dias", int(d.Hours())/24)
	case d >= 24*time.Hour:
		return "1 dia"
	case d >= time.Hour:
		return fmt.Sprintf("%d hora(s)", int(d.Hours()))
	default:
		return fmt.Sprintf("%d minuto(s)", max(int(d.Minutes()), 1))
	}
}

// aplicaBuffDeGuilda soma o bônus dos buffs ao score já calculado.
//
// Percentuais sobre o que existe, e não valores fixos: um buff que somasse
// pontos de defesa valeria muito para quem tem pouco e quase nada para quem tem
// muito, e o buff é da guilda inteira. Percentual dá a mesma vantagem relativa
// para o novato e para o veterano.
//
// O drop é a exceção e soma direto, porque o número que o jogo usa ali não é uma
// percentagem — é a escala própria das fadas e dos itens de grau 5
// (drop_bonus.go). Somar "18%" a ela não significaria nada.
//
// Tem de rodar DEPOIS de refreshScore ter escrito EquipDropBonus, senão a soma
// do drop é sobrescrita no mesmo instante em que é feita.
func aplicaBuffDeGuilda(e *world.Entity, b bonusDeGuilda) {
	if b.vazio() || e == nil {
		return
	}
	// A vida entra pelo campo que o jogo já usa para o EF_HPADD dos
	// equipamentos: ele é lido na hora de calcular a vida máxima efetiva
	// (effectiveMaxHP), e não no total guardado, então somar aqui basta.
	e.HpAddPct += int32(b.vidaPc)

	// Os dois percentuais de dano já SÃO percentuais no score (o do Hércules e
	// afins), então somam-se ao que estiver lá em vez de multiplicar. O buff de
	// dano paga os dois de uma vez, que é o que ele promete.
	e.DanoFisicoPct += int32(b.danoPc)
	e.DanoMagicoPct += int32(b.danoPc)

	if b.acPc != 0 {
		e.AC += e.AC * int32(b.acPc) / 100
	}
	e.EquipDropBonus += int32(b.drop)
}

// duracaoDoItemDeBuff diz por quanto tempo este item acende os buffs, ou zero
// quando ele nao e um item de buff.
//
// O casamento e por INDICE e nao por volatil porque os dois itens dividem o
// EF_VOLATILE 197 e valem tempos diferentes: pelo volatil nao haveria como
// distinguir 15 dias de 30.
func duracaoDoItemDeBuff(index int16) time.Duration {
	return itensDeBuffDeGuilda[index]
}

// guildaAtiva atende MsgGuildaAtiva: o botao "Ativar" da aba Buffs.
//
// ELE NAO DA BUFF. O pacote diz QUAL buff e COM QUAL item da mochila, e o
// servidor confere as duas coisas antes de gastar. Quem nao tem o item nao ganha
// nada - era a razao de eu nao ter posto botao nenhum no comeco, e e o que
// permite ter um agora.
func (d *Dispatcher) guildaAtiva(w *world.World, s *world.Session, _ protocol.Header, payload []byte) {
	e := w.Entity(s.Conn)
	if e == nil || s.Mode != world.UserPlay {
		return
	}
	corpo, err := protocol.DecodeGuildaAtiva(payload)
	if err != nil {
		return
	}
	d.acendeUmBuff(w, s, e, int(corpo.Slot), corpo.Tipo)
}

// --- os buffs na barra do personagem ---------------------------------------
//
// O cliente desenha a barra de buffs a partir de um vetor de 32 posições que
// viaja no pacote de score, cada uma empacotada como (tipo << 8) | tempo. Os
// buffs de guilda entram AÍ, e não na lista de afetos do personagem.
//
// A diferença importa: a lista de afetos é varrida a cada tick, tem efeito
// próprio no cálculo de score e é apagada quando o tempo acaba. Um buff de
// guilda que entrasse nela seria aplicado DUAS vezes — uma por aqui e outra pelo
// afeto — e ainda disputaria uma das poucas casas com os buffs de skill que o
// jogador pagou de mana. Escrever só no vetor do pacote resolve o desenho sem
// tocar em nada disso.
//
// OS TIPOS SÃO ESCOLHIDOS ENTRE OS QUE O SERVIDOR NÃO TRATA. applyAffectScore
// age sobre 1-15, 19, 25, 27, 28, 29, 34, 35, 36, 37 e 42; estes quatro estão
// fora dessa lista de propósito, para que nenhum efeito do legado se ligue a
// eles por engano.
//
// Se o cliente não tiver ícone para um deles, a barra mostra um espaço vazio —
// e aí a resposta é criar a arte na tira de estados do mainparts.wyt, cujo
// formato já está decifrado.
var tipoNaBarra = [protocol.GuildaBuffs]uint8{16, 17, 18, 20}

// marcaBuffsDeGuildaNaBarra acrescenta os buffs ligados ao vetor de ícones.
//
// Escreve nas ÚLTIMAS casas livres, de trás para frente, para não empurrar os
// afetos de verdade do jogador: os dele vêm primeiro na barra, que é onde ele
// espera encontrá-los.
func (d *Dispatcher) marcaBuffsDeGuildaNaBarra(e *world.Entity, sc *protocol.ScoreData) {
	if e == nil || e.Guild == 0 || !world.IsPlayer(e.ID) {
		return
	}
	b := d.guildaBuffs[e.Guild]
	if b == nil {
		return
	}
	agora := d.now()
	casa := len(sc.Affect) - 1
	for i := range receitasDeBuff {
		if !b.ligado(i, agora) {
			continue
		}
		for casa >= 0 && sc.Affect[casa] != 0 {
			casa--
		}
		if casa < 0 {
			return // a barra encheu; os afetos do jogador ficam
		}
		// O tempo do pacote é um byte, então ele satura em 255 — e é só o
		// relógio do ícone, não a duração de verdade, que o painel mostra.
		restam := b.expira[i].Sub(agora) / (8 * time.Second)
		if restam > 255 {
			restam = 255
		}
		if restam < 1 {
			restam = 1
		}
		sc.Affect[casa] = protocol.PackAffect(protocol.AffectData{
			Type: tipoNaBarra[i], Time: uint32(restam),
		})
		casa--
	}
}
