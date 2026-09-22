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
		// A marca na barra entra e sai junto com o buff. Só manda o vetor quando
		// ele mudou: um MsgSendAffect por refresh seria um pacote de 268 bytes
		// para dizer que nada aconteceu.
		if d.sincronizaAfetosDeGuilda(te) {
			d.sendAffect(w, ts, te)
		}
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
		if te.Guild != guilda {
			return
		}
		sendClientMessage(w, ts, msgGuildaBuffVenceu)
		// Tira a marca da barra. O refrescaGuilda que roda junto ja costuma
		// fazer isso; isto cobre qualquer caminho que o esqueca.
		if d.sincronizaAfetosDeGuilda(te) {
			d.sendAffect(w, ts, te)
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

// --- os buffs na barra do personagem: POR QUE NAO PELO VETOR DE AFETOS ------
//
// TENTADO E DESFEITO em 22/09/2026, e a nota fica para ninguem tentar de novo.
//
// O cliente desenha a barra a partir de um vetor de 32 posicoes que viaja no
// pacote de score, empacotado como (tipo << 8) | tempo. Escrever ali os buffs de
// guilda parecia de graca: nao mexe na lista de afetos do personagem, nao
// duplica efeito, nao gasta casa de buff de skill.
//
// So que o TIPO do afeto nao e so um numero de icone para o cliente. Ele tambem
// escolhe o efeito VISUAL que o personagem recebe. Os tipos 16, 17, 18 e 20
// foram escolhidos por estarem fora da lista que applyAffectScore trata - livres
// do lado do SERVIDOR -, e o resultado em tela foi o personagem inteiro pintado
// de verde, como quem esta envenenado.
//
// A licao: "o servidor nao trata" nao quer dizer "o cliente nao trata". Achar um
// tipo livre dos dois lados exige ler o cliente, e mesmo assim o icone seria o
// que ele ja tem, nao o que a guilda quer mostrar.
//
// O caminho que sobra, e que da controle total sobre a arte: desenhar os quatro
// icones NOS, numa camada propria, ao lado da barra do jogo - do mesmo jeito que
// o painel de alvos e a Loja de Honra desenham o que desenham. A tira de icones
// do cliente (UI/mainparts.wyt) ja foi decifrada e serve de molde.

// --- os buffs na barra do personagem ---------------------------------------
//
// PELO MESMO CAMINHO DA POÇÃO DIVINA, que é o que o jogo já usa para um buff de
// 30 dias aparecer ali com o tempo certo. Rastreado em 22/09/2026:
//
//   O item põe um afeto no vetor do personagem com Time = 2.000.000.000, um
//   valor sentinela que quer dizer "infinito" e não uma duração. O prazo de
//   verdade mora FORA do afeto, num campo próprio (DivineEnd). Na hora de
//   enviar, sendAffect vê a sentinela e manda (prazo − agora) / 8 no lugar.
//
// O afeto é só a MARCA; quem sabe a duração é outro lugar. Os nossos buffs já
// têm esse outro lugar — a tabela guild_buff -, então falta só a marca.
//
// Por que não pelo vetor de ícones do pacote de score, que foi a primeira
// tentativa: aquele campo tem UM BYTE de tempo, e o cliente nem o usa para a
// barra. Quem alimenta a barra é o MSG_SendAffect (0x03B9), com 4 bytes de
// tempo por casa — mais de mil anos de alcance.

// tipoNaBarra é o tipo de afeto que representa cada buff de guilda na barra.
//
// Os quatro são o fim da faixa que o cliente aceita (1..49), escolhidos por
// dois motivos: nenhum deles é tratado por applyAffectScore, então nenhum efeito
// do legado se liga a eles; e são os últimos do catálogo do cliente, a parte
// menos usada — o 49 nem nome tem na tabela de nomes dele (0x61E240).
//
// O NÚMERO IMPORTA, e isso custou uma rodada: cada tipo tem identidade no
// cliente. A primeira tentativa usou o 16, que é o Tick Life, e o personagem
// ficou com a aura verde de regeneração. Se algum destes quatro trouxer efeito
// visual, o conserto é trocar o número — não o mecanismo.
var tipoNaBarra = [protocol.GuildaBuffs]uint8{46, 47, 48, 49}

// buffDeGuildaSentinela é o Time guardado no afeto: grande o bastante para a
// varredura de afetos nunca o zerar, e é o sinal de que o tempo de verdade vem
// de outro lugar. Mesmo papel do divineAffectTime da Poção Divina.
const buffDeGuildaSentinela = 2000000000

// tipoDeBuffDeGuilda diz qual buff um tipo de afeto representa, ou 0.
func tipoDeBuffDeGuilda(tipo uint8) uint8 {
	for i, t := range tipoNaBarra {
		if t == tipo {
			return receitasDeBuff[i].Tipo
		}
	}
	return 0
}

// sincronizaAfetosDeGuilda põe (e tira) as marcas dos buffs no vetor de afetos.
//
// Chamada de onde o estado muda: ao acender um buff, quando um vence, e no
// login. Não é chamada por quadro — ela mexe no vetor de afetos, e mexer nele à
// toa faria o servidor mandar atualização de buff sem nada ter mudado.
func (d *Dispatcher) sincronizaAfetosDeGuilda(e *world.Entity) bool {
	if e == nil || !world.IsPlayer(e.ID) {
		return false
	}
	b := d.guildaBuffs[e.Guild]
	agora := d.now()
	mudou := false
	for i := range receitasDeBuff {
		ligado := e.Guild != 0 && b != nil && b.ligado(i, agora)
		tipo := tipoNaBarra[i]
		casa := -1
		for k := range e.Affect {
			if e.Affect[k].Type == tipo {
				casa = k
				break
			}
		}
		if ligado && casa < 0 {
			livre := e.EmptyAffect(tipo)
			if livre < 0 {
				continue // o vetor encheu; os afetos do jogador vêm primeiro
			}
			e.Affect[livre] = world.Affect{Type: tipo, Level: 1, Time: buffDeGuildaSentinela}
			mudou = true
		} else if !ligado && casa >= 0 {
			e.Affect[casa] = world.Affect{}
			mudou = true
		}
	}
	return mudou
}

// tempoDoBuffNaBarra devolve o tempo que deve ir no pacote para um afeto nosso,
// em ticks de 8 segundos, ou false quando o tipo não é nosso.
//
// É o espelho exato do que sendAffect faz com a Poção Divina: a sentinela no
// vetor é ignorada e o tempo sai do prazo de verdade.
func (d *Dispatcher) tempoDoBuffNaBarra(e *world.Entity, tipo uint8) (uint32, bool) {
	if e == nil || e.Guild == 0 {
		return 0, false
	}
	for i, t := range tipoNaBarra {
		if t != tipo {
			continue
		}
		b := d.guildaBuffs[e.Guild]
		if b == nil || !b.ligado(i, d.now()) {
			return 0, true
		}
		return uint32(b.expira[i].Sub(d.now()) / (8 * time.Second)), true
	}
	return 0, false
}
