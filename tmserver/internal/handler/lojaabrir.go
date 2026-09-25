package handler

import (
	"context"

	"github.com/jeanluca/w2pp-openwyd/internal/store"
	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/protocol"
	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/world"
)

// Montar a barraca pelo painel da Loja do Servidor.
//
// A janela de barraca do cliente está aposentada: ela só sabia de ouro, e o
// preço e a escolha dos itens vinham dela. Agora quem monta é o painel, e por
// isso precisa de duas coisas que só existem aqui: ver o cofre da conta e
// mandar montar.
//
// O pedido de montar NÃO leva o item, só a posição no cofre. Era daí que vinha
// metade das checagens do fluxo antigo (o memcmp anti-troca): se o servidor lê o
// item do próprio cofre, não há o que conferir contra o cliente. O resto das
// regras continua igual à de sempre — cidade, itens proibidos, EF_NOTRADE, teto
// de preço e pelo menos um item à venda.

// lojaCargo atende MsgLojaCargo: manda o cofre da conta, só os slots ocupados.
func (d *Dispatcher) lojaCargo(w *world.World, s *world.Session, _ protocol.Header, _ []byte) {
	e := w.Entity(s.Conn)
	if e == nil || s.Mode != world.UserPlay {
		return
	}
	cargo := w.Cargo(s.AccountID)
	if cargo == nil {
		return
	}
	var corpo protocol.LojaCargoListaBody
	for pos := 0; pos < world.MaxCargo && corpo.Qtd < protocol.LojaCargoMax; pos++ {
		it := cargo.Items[pos]
		if it.Empty() {
			continue
		}
		corpo.Itens[corpo.Qtd] = protocol.LojaCargoItem{
			Slot:   int16(pos),
			Indice: it.Index,
			Refino: lojaRefino(it),
			Qtd:    lojaQuantidade(it),
		}
		corpo.Qtd++
	}
	w.SendTo(s, protocol.Header{Type: protocol.MsgLojaCargoLista, ID: protocol.IDScene},
		corpo.Encode())
}

// lojaAbrir atende MsgLojaAbrir: monta a barraca com os itens, preços e moedas
// que o painel escolheu.
func (d *Dispatcher) lojaAbrir(w *world.World, s *world.Session, _ protocol.Header, payload []byte) {
	e := w.Entity(s.Conn)
	if e == nil || e.HP <= 0 || s.Mode != world.UserPlay {
		return
	}
	// Não se monta barraca no meio de uma troca, nem com uma já aberta.
	if s.Trade.Active || s.TradeMode != 0 {
		d.notify(w, s, NoticeCantAutoTrade)
		return
	}
	village := world.Village(e.X, e.Y)
	if village < 0 || village > 4 || inAutoTradeForbiddenRect(e.X, e.Y) {
		d.notify(w, s, NoticeOnlyVillage)
		return
	}
	var pedido protocol.LojaAbrirBody
	if err := pedido.Decode(payload); err != nil {
		return
	}
	cargo := w.Cargo(s.AccountID)
	if cargo == nil {
		return
	}

	// Sem título, a barraca leva o nome do dono: no painel não há onde digitar —
	// o cliente lê o teclado por conta dele, e cada tecla é um atalho do jogo.
	titulo := pedido.Titulo
	if titulo == "" {
		titulo = e.Name
	}

	// Valida tudo antes de mexer em qualquer coisa.
	barraca := &world.AutoTradeState{Title: titulo, Tax: world.CityTax(village)}
	usados := map[int]bool{}
	for i := 0; i < protocol.MaxAutoTradeWire; i++ {
		p := pedido.Slots[i]
		barraca.Slots[i].CargoPos = -1
		if p.CargoPos < 0 {
			continue
		}
		pos := int(p.CargoPos)
		if pos >= world.MaxCargo {
			return
		}
		// Duas prateleiras apontando para o mesmo slot venderiam o item duas
		// vezes; a segunda venda sairia do vazio.
		if usados[pos] {
			return
		}
		item := cargo.Items[pos]
		if item.Empty() {
			return
		}
		if p.Preco <= 0 || p.Preco > autoTradePriceMax {
			return
		}
		if p.Moeda > protocol.LojaMoedaRMT {
			return
		}
		// PILHA NÃO VAI A DINHEIRO REAL.
		//
		// O anúncio guarda a fotografia de UM item e a marca do escrow guarda UM
		// id no slot do baú (0104/0105). Uma pilha de dez é um slot só, então não
		// há onde escrever "vendi três": ou sai a pilha inteira, ou a cobrança
		// fica sem par. E a venda parcial é justamente o que o comprador espera
		// de uma pilha, porque é assim que ela funciona em ouro.
		//
		// A recusa é aqui, na montagem, e não na compra: é o vendedor que pode
		// consertar — separando a pilha — e ele está na frente da tela agora.
		// O MERCADO EM DINHEIRO REAL PODE ESTAR FECHADO, e esta é a primeira pergunta
		// entre as de dinheiro real: antes dela, recusar por pilha ou por preço mínimo
		// explicaria uma regra de um mercado que nem está aberto.
		//
		// A RECUSA É AQUI, na montagem da barraca. O jogador ainda não anunciou nada, não
		// há cobrança, não há escrow, e ele descobre agora em vez de descobrir quando
		// alguém tentar comprar.
		if p.Moeda == protocol.LojaMoedaRMT && !d.podeUsarRMT(s) {
			sendClientMessage(w, s, msgRMTNaoEstaAberta)
			return
		}
		if p.Moeda == protocol.LojaMoedaRMT && itemAmount(item) > 1 {
			sendClientMessage(w, s, msgPilhaNaoVaiRMT)
			return
		}
		// O PREÇO MÍNIMO EM DINHEIRO REAL, decisão da Hanna de 24/09/2026.
		//
		// Um centavo não é preço: a processadora cobra taxa por cobrança, então um
		// item de R$ 0,01 custa mais para vender do que rende, e o repasse ao
		// vendedor sairia negativo. O cliente novo também trava abaixo disso, mas
		// QUEM MANDA É O SERVIDOR — um cliente remendado não pode criar anúncio de
		// um centavo.
		//
		// A recusa é AQUI, na montagem, pelo mesmo motivo da pilha: é o vendedor que
		// pode consertar, e ele está na frente da tela agora. Recusar só na compra
		// puniria o comprador por um preço que ele não escolheu.
		if p.Moeda == protocol.LojaMoedaRMT && int64(p.Preco) < store.PrecoMinimoRMTCentavos {
			sendClientMessage(w, s, msgPrecoMinimoRMT)
			return
		}
		// E O TETO, pelo mesmo motivo e no mesmo lugar.
		//
		// Ele protege gente diferente do mínimo. O mínimo impede o vendedor de vender
		// de graça; o TETO impede o comprador de pagar uma fortuna por um erro de
		// digitação — R$ 50.000 num campo de preço é um engano plausível, e do outro
		// lado dele sai um Pix de verdade da conta de alguém.
		if p.Moeda == protocol.LojaMoedaRMT && int64(p.Preco) > store.TetoDaVendaRMTCentavos {
			sendClientMessage(w, s, msgPrecoMaximoRMT)
			return
		}
		// A RECONCILIAÇÃO DO LOGIN AINDA ESTÁ NO BANCO.
		//
		// Ela cancela todo anúncio ativo da conta, supondo que quem acabou de
		// entrar não tem barraca de pé. Deixar montar agora criaria um anúncio que
		// a volta dela cancelaria — barraca nova de pé e venda desfeita em
		// silêncio, o mesmo defeito do login duplicado por outra porta.
		//
		// A trava é aqui e não numa espera: o tempo que o jogador leva para andar
		// até a cidade não é garantia de nada, e com o banco lento a janela é de
		// até dez segundos.
		//
		// Só o dinheiro real é recusado. Barraca de ouro não cria anúncio, então a
		// reconciliação não tem o que desfazer nela.
		if p.Moeda == protocol.LojaMoedaRMT && s.ReconciliandoEscrow {
			sendClientMessage(w, s, msgEscrowSincronizando)
			return
		}
		// ITEM JÁ PRESO NUM ANÚNCIO NÃO VOLTA PARA A PRATELEIRA.
		//
		// A marca do escrow diz que existe um anúncio em dinheiro real vivo sobre
		// este slot. Deixar montar de novo — em ouro, inclusive — venderia pela
		// segunda vez o que já está prometido: quem pagasse o Pix pagaria por nada,
		// e o servidor não teria de onde tirar o segundo item.
		//
		// A armadilha do `itemSlot` não cobre esta porta: a montagem lê o baú
		// direto, como a compra do painel.
		if item.AnuncioRMT != 0 {
			sendClientMessage(w, s, msgItemJaAnunciado)
			return
		}
		if autoTradeBlacklist[item.Index] {
			d.notify(w, s, NoticeCantAutoTrade)
			return
		}
		if d.itemAbility(item, efNoTrade) != 0 {
			d.notify(w, s, NoticeCantMoveItem)
			return
		}
		usados[pos] = true
		barraca.Slots[i].Item = item
		barraca.Slots[i].CargoPos = pos
		barraca.Slots[i].Price = p.Preco
		barraca.Moeda[i] = p.Moeda
	}
	if !shopStocked(barraca) {
		d.notify(w, s, NoticeCantAutoTrade)
		return
	}

	// As prateleiras em dinheiro real precisam de linha no banco ANTES de a
	// barraca subir: é o anúncio que dá identidade ao que está à venda, e é o id
	// dele que vira a marca do escrow no slot do baú. Subir a barraca antes
	// abriria uma janela em que alguém compra uma oferta sem anúncio atrás.
	if anuncios, posicoes := prateleirasEmRMT(barraca); len(anuncios) > 0 {
		d.abreAnunciosESobe(w, s, barraca, anuncios, posicoes)
		return
	}
	d.sobeBarraca(w, s, e, barraca)
}

// prateleirasEmRMT separa as prateleiras em dinheiro real, e devolve junto a
// posição de cada uma na barraca — é por ela que o id que volta do banco acha o
// slot do baú que vai marcar.
func prateleirasEmRMT(barraca *world.AutoTradeState) ([]world.AnuncioRMT, []int) {
	var anuncios []world.AnuncioRMT
	var posicoes []int
	for i := range barraca.Slots {
		if barraca.Moeda[i] != protocol.LojaMoedaRMT || barraca.Slots[i].CargoPos < 0 {
			continue
		}
		anuncios = append(anuncios, world.AnuncioRMT{
			CargoSlot: int16(barraca.Slots[i].CargoPos),
			Item:      barraca.Slots[i].Item,
			// O preço em dinheiro real é em CENTAVOS. É o mesmo campo do preço em
			// ouro no pedido do painel, e a moeda é quem diz como lê-lo — como no
			// resto do sistema, onde o método é coluna e não nome de tabela.
			PrecoCentavos: int64(barraca.Slots[i].Price),
		})
		posicoes = append(posicoes, i)
	}
	return anuncios, posicoes
}

// abreAnunciosESobe vai ao banco FORA do laço criar os anúncios, volta, marca os
// slots do baú com os ids e sobe a barraca.
//
// Usa GoDetached e não Go de propósito. O Go descarta a volta quando a sessão
// morre no meio, e aqui a volta não é só cosmética: se os anúncios nasceram e a
// barraca não vai subir, ELES TÊM DE SER CANCELADOS. Um anúncio ativo esquecido
// prende o slot para sempre, pelo índice de um ativo por slot.
//
// E revalida tudo na volta. Entre a ida e a volta o jogador pode ter morrido,
// saído da cidade, aberto outra barraca, ou mexido no baú — e a validação da ida
// não vale mais. Barato demais para não refazer.
func (d *Dispatcher) abreAnunciosESobe(w *world.World, s *world.Session,
	barraca *world.AutoTradeState, anuncios []world.AnuncioRMT, posicoes []int,
) {
	conta, conn := s.AccountID, s.Conn
	// O nome vai junto porque a fotografia é montada agora: quando alguém
	// perguntar "quem me vendeu isto", a barraca já terá descido.
	personagem := ""
	if e := w.Entity(conn); e != nil {
		personagem = e.Name
	}
	persist := w.Persistence()
	w.GoDetached(func() func(*world.World) {
		ids, semChave, err := persist.OpenRmtListings(context.Background(), conta, personagem, anuncios)
		return func(w *world.World) {
			sess := sessaoDaConexao(w, conn, conta)
			if err != nil {
				d.log.Warn("loja: nao consegui abrir os anuncios", "conn", conn, "err", err)
				if sess != nil {
					sendClientMessage(w, sess, msgAnuncioNaoSaiu)
				}
				return
			}
			if semChave {
				if sess != nil {
					sendClientMessage(w, sess, msgSemChavePix)
				}
				return
			}
			if !d.podeSubirAinda(w, sess, anuncios) {
				// Nasceram e não vão valer: tira da vitrine, senão prendem os
				// slots para sempre.
				d.cancelaAnuncios(w, ids)
				if sess != nil {
					sendClientMessage(w, sess, msgAnuncioNaoSaiu)
				}
				return
			}
			cargo := w.Cargo(sess.AccountID)
			for k, pos := range posicoes {
				slot := barraca.Slots[pos].CargoPos
				cargo.Items[slot].AnuncioRMT = ids[k]
				barraca.Slots[pos].Item = cargo.Items[slot]
			}
			// Salva o baú agora, e não no próximo save: entre a criação do
			// anúncio e a gravação da marca existe um instante em que o anúncio
			// está ativo e o item não está preso. Encurtar esse instante é o que
			// dá para fazer daqui.
			w.SalvaCargo(sess.AccountID)
			d.sobeBarraca(w, sess, w.Entity(sess.Conn), barraca)
			d.log.Info("loja: anuncios em dinheiro real abertos",
				"conn", conn, "conta", conta, "anuncios", len(ids))
		}
	})
}

// podeSubirAinda refaz, na volta do banco, as perguntas que a ida já tinha feito.
//
// A última é a que importa de verdade: o item de cada slot tem de ser o MESMO que
// foi fotografado no anúncio. Se ele mudou, o anúncio promete uma coisa e o baú
// tem outra — e o comprador pagaria pelo que está escrito.
func (d *Dispatcher) podeSubirAinda(w *world.World, s *world.Session,
	anuncios []world.AnuncioRMT,
) bool {
	if s == nil || s.Mode != world.UserPlay || s.AutoTrade != nil || s.TradeMode != 0 {
		return false
	}
	e := w.Entity(s.Conn)
	if e == nil || e.HP <= 0 {
		return false
	}
	village := world.Village(e.X, e.Y)
	if village < 0 || village > 4 || inAutoTradeForbiddenRect(e.X, e.Y) {
		return false
	}
	cargo := w.Cargo(s.AccountID)
	if cargo == nil {
		return false
	}
	for _, a := range anuncios {
		if a.CargoSlot < 0 || int(a.CargoSlot) >= world.MaxCargo {
			return false
		}
		atual := cargo.Items[a.CargoSlot]
		if atual.Index != a.Item.Index || atual.Effects != a.Item.Effects || atual.AnuncioRMT != 0 {
			return false
		}
	}
	return true
}

// cancelaAnuncios tira da vitrine os anúncios que não chegaram a valer. Roda fora
// do laço porque fala com o banco, e não tem volta: se falhar, a linha fica ativa
// e a reconciliação do órfão é quem resolve.
func (d *Dispatcher) cancelaAnuncios(w *world.World, ids []int64) {
	persist := w.Persistence()
	w.GoDetached(func() func(*world.World) {
		err := persist.CancelRmtListings(context.Background(), ids)
		return func(*world.World) {
			if err != nil {
				d.log.Warn("loja: nao consegui cancelar os anuncios orfaos", "ids", ids, "err", err)
			}
		}
	})
}

// sessaoDaConexao acha a sessão de uma conexão dentro do laço. Ela é procurada de
// novo, e não carregada de fora, porque entre a ida ao banco e a volta o jogador
// pode ter caído — e um ponteiro guardado apontaria para o que o mundo já largou.
// A CONTA FAZ PARTE DA PERGUNTA, e não é zelo excessivo: o número da conexão é
// REAPROVEITADO. Se o vendedor cai durante a ida ao banco e outra pessoa entra no
// mesmo número, procurar só pelo conn devolve a sessão DELA. A volta então
// conferiria o baú da conta errada e, se por azar o item batesse, marcaria o baú
// do segundo com o anúncio do primeiro e subiria a barraca de um na sessão do
// outro.
//
// Sessão de outra conta é tratada como sessão que sumiu, que é o que ela é para
// este pedido: os anúncios são cancelados e ninguém é avisado, porque quem
// pediu já não está aqui.
func sessaoDaConexao(w *world.World, conn int, conta int64) *world.Session {
	var achada *world.Session
	w.ForEachSession(func(s *world.Session, _ *world.Entity) {
		if s != nil && s.Conn == conn && s.AccountID == conta {
			achada = s
		}
	})
	return achada
}

// sobeBarraca é o fim comum dos dois caminhos: com e sem dinheiro real.
func (d *Dispatcher) sobeBarraca(w *world.World, s *world.Session, e *world.Entity,
	barraca *world.AutoTradeState,
) {
	if s == nil || e == nil {
		return
	}
	s.AutoTrade = barraca
	s.TradeMode = 1
	barraca.OpenedAt = w.Now()
	barraca.PaidUntil = barraca.OpenedAt
	// A barraca sobe primeiro: é ela que dá o id pelo qual os outros compram.
	d.raiseShopStall(w, s, e)
	if barraca.CloneID >= world.MaxUser {
		// A barraca fica de pé sozinha e o dono volta a jogar (decisão do Marco,
		// 17/09). O cliente fecha a janela de loja ao receber isto; a nossa nem
		// abre mais, mas manter o aviso custa nada e cobre um cliente antigo que
		// ainda tenha aquela janela na tela.
		w.Send(s, protocol.MsgQuitTrade, nil)
	}
	// O painel espera este aviso para fechar a tela de montagem e já mostrar a
	// barraca na vitrine. O id é o mesmo pelo qual os outros compram.
	// O aviso aos outros sai depois de a barraca estar de pe e com id: antes
	// disso a vitrine que eles receberiam nao teria as ofertas novas.
	d.mercadoMudou(w)
	aviso := protocol.LojaAbriuBody{Barraca: int32(shopStallID(s))}
	w.SendTo(s, protocol.Header{Type: protocol.MsgLojaAbriu, ID: protocol.IDScene}, aviso.Encode())
	// Avisa na hora em que ele abre, e não só no /pontos: quem abre a segunda
	// barraca para dobrar o prêmio precisa saber que não dobra antes de passar
	// quinze minutos esperando um crédito que não vem.
	if precedidaNoMundo(w, s) {
		sendClientMessage(w, s, msgOutraLojaRende)
	}
	d.log.Info("loja: barraca montada pelo painel", "conn", s.Conn, "titulo", barraca.Title,
		"imposto", barraca.Tax, "clone", barraca.CloneID)
}

// msgPilhaNaoVaiRMT é o que o vendedor lê quando põe uma pilha à venda por
// dinheiro real. Diz o que fazer, e não só que não deu.
// msgPrecoMinimoRMT é a recusa do preço abaixo do mínimo.
//
// Diz o VALOR, e não só "muito baixo": o vendedor precisa saber para quanto subir, e
// uma recusa que não diz o número obriga a tentativa e erro.
const msgPrecoMinimoRMT = "O preço mínimo em dinheiro real é R$ 5,00."

// msgPrecoMaximoRMT é a recusa acima do teto.
//
// A FRASE É A DA HANNA, palavra por palavra, e ela manda falar com o suporte porque
// quem esbarra no teto ou errou a digitação ou tem um caso que a regra não previu — e
// os dois precisam de gente, não de outra tentativa.
//
// MEDIDA: 73 bytes em Windows-1252, dentro dos 94 que o painel corta. Os acentos de
// "Preço" e "máximo" custam um byte cada.
const msgPrecoMaximoRMT = "Preço máximo em RMT: R$ 500,00. Para valores maiores, fale com o suporte."

const msgPilhaNaoVaiRMT = "Pilha não pode ser vendida por dinheiro real. Separe uma unidade e anuncie ela."

// msgSemChavePix é a terceira recusa do dinheiro real, e a que o vendedor
// resolve sozinho.
const msgSemChavePix = "Cadastre sua chave Pix no site antes de vender por dinheiro real."

// msgAnuncioNaoSaiu cobre os dois jeitos de a montagem falhar depois de ir ao
// banco: o banco recusou, ou o mundo mudou enquanto a resposta vinha. Uma
// mensagem só porque, para quem está na frente da tela, as duas pedem a mesma
// coisa — tentar de novo.
// msgEscrowSincronizando é o que o vendedor lê quando monta uma barraca em
// dinheiro real antes de a reconciliação do login voltar do banco. É espera de
// segundos, e o pedido não se perde — ele monta de novo.
const msgEscrowSincronizando = "Aguarde um instante e tente de novo."

// msgItemJaAnunciado é a recusa de quem tenta pôr de novo à venda um item que
// já está preso num anúncio em dinheiro real — quase sempre porque a barraca
// anterior caiu e o anúncio dela continua de pé.
const msgItemJaAnunciado = "Esse item já está anunciado por dinheiro real. Cancele o anúncio antes de vendê-lo de novo."

const msgAnuncioNaoSaiu = "Não deu para montar a barraca em dinheiro real. Tente de novo."
