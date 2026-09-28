package handler

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/combine"
	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/protocol"
	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/world"
)

// A Loja de Honra: o God of War trocando itens pelos pontos que a lojinha
// aberta rende (shoppoints.go, 3 por quinze minutos e 7 com Fada Azul).
//
// Pedida em 2026-09-20. Até aqui os pontos só entravam: eram creditados, o
// /pontos os mostrava, e não havia onde gastá-los. Esta é a saída.
//
// O NPC é reaproveitado, não criado. O God of War já está de pé em Armia
// (NPCGener #[6063], em 2130,2088) com Merchant 104, e 104 não cai em nenhum
// ramo do roteamento de clique em misc.go — clicar nele não fazia absolutamente
// nada. É a mesma situação de onde saiu a loja do Unicórnio Puro
// (loja_de_emblema.go), e o truque é o mesmo: o servidor continua vendo 104, que
// é como reconhece a loja, e anuncia Merchant 1 ao cliente para que o clique
// chegue como _MSG_REQShopList.
//
// A diferença é o que responde a esse clique. A loja de emblema deixa o cliente
// abrir a janela de loja dele; aqui o servidor NÃO manda MSG_ShopList — manda
// MsgHonraAbre, e quem desenha é o nosso painel (client/gamepatch/honra.cpp). O
// motivo é o preço: a janela do cliente só sabe escrever preço com o cifrão do
// ouro, e o que se paga aqui é ponto. Ver protocol/lojahonra.go.

const (
	// merchantLojaDeHonra é o Merchant que SÓ esta loja tem, e ele é nosso: nenhum
	// template do jogo usa 201.
	//
	// A primeira versão reconhecia a loja pelo 104 do template God_of_War, como a
	// loja de emblema faz com o 110 do Unicórnio Puro. Não serve: medido em
	// 21/09/2026, três NPCs carregam 104 — o God_of_War (#590), o Treinador2 (#457)
	// e o Uxmal (#564) —, e os três viravam loja de honra. O 110 do Unicórnio é
	// dele sozinho; o 104 não é de ninguém.
	//
	// Quem identifica a loja é o TEMPLATE, e a tradução de template para Merchant
	// acontece uma vez, no nascimento do NPC (marcaLojaDeHonra, em npcconfig.go).
	// Daí para frente tudo aqui olha só este número, como o resto do servidor faz.
	merchantLojaDeHonra = 201

	// templateDaLojaDeHonra é o arquivo de template do NPC que abre a loja. É por
	// ele, e não pelo nome exibido, que a loja é reconhecida: o nome é editável
	// pelo painel (e virou "Honor Store" em 21/09/2026), o template não.
	templateDaLojaDeHonra = "God_of_War"

	// tilesDaLojaDeHonra é a distância em que a loja ainda está de pé: 20 tiles em
	// cada eixo. O jogo usa 33 (VIEWGRID, uma tela) para as lojas dele; a Josiel
	// achou solto demais e pediu 40% menos, em 21/09/2026 — 33 menos 40% é 19,8, e
	// 20 é o inteiro ao lado.
	//
	// Divergir do jogo aqui é de propósito e só aqui: as lojas de NPC do cliente
	// continuam com o 33 delas, que não passa por este código.
	tilesDaLojaDeHonra = 20

	// honraMotivo é o que aparece no extrato de shop_points_audit.
	honraMotivo = "loja de honra"
)

// msgHonraItemSaiu responde à compra de uma vaga que deixou de vender entre a
// abertura do painel e o clique.
const msgHonraItemSaiu = "Este item não está mais à venda. Abra a loja de novo."

// msgHonraAtualizada é o aviso a quem estava com o painel aberto quando a equipe
// mudou o estoque.
const msgHonraAtualizada = "A Loja de Honra foi atualizada. Abra a loja de novo para ver os preços novos."

// itemDeHonra é uma troca oferecida pela loja: uma vaga do estoque do God of War.
//
// Efeitos vai no item entregue como está: é por ele que uma pilha sai com
// EF_AMOUNT (61 N) e uma fada sai com o prazo dela (106 1 = 24 horas, que só
// começa a correr quando ela é equipada — timeditem.go).
type itemDeHonra struct {
	Slot    int16 // a vaga no painel de NPC (0..26), e é por ela que a compra aponta o item
	Indice  int16
	Preco   int32 // em pontos de lojinha, por compra (a pilha inteira)
	Cat     uint8 // a aba do painel: protocol.HonraCat*
	Efeitos [3]world.Effect
}

// estoqueDeHonra é o que o God of War vende: as vagas da loja dele que têm preço
// em pontos, na ordem das vagas.
//
// O estoque mora no BANCO desde 26/09/2026, nas vagas do NPC (npc_shop_item, com
// price_points da 0078), e é editado no painel como qualquer lojista. Antes
// morava numa tabela neste arquivo, e mudar um preço exigia deploy — que reinicia
// o servidor e derruba todo mundo. Agora a recarga das lojas de NPC (npcconfig.go,
// a cada 15 s) reescreve as vagas do God of War em npc.Carry e os preços em
// npc.ShopPointPrice (applyShop), e esta função só lê o que está no NPC. Não há
// cópia do estoque em lugar nenhum para ficar velha.
//
// Uma vaga cobrada em OURO não entra: esta loja só sabe cobrar ponto, e mostrar o
// item com um preço que ela não cobra seria pior que escondê-lo. Preço zero
// também não: item de graça na loja de honra é engano de cadastro, e não uma
// promoção. Os dois casos são avisados no log a cada recarga
// (auditaEstoqueDeHonra), para quem cadastrou descobrir por que o item sumiu.
func (d *Dispatcher) estoqueDeHonra(npc *world.Entity) []itemDeHonra {
	if !ehLojaDeHonra(npc) {
		return nil
	}
	var estoque []itemDeHonra
	for slot := 0; slot < maxShopSlots && len(estoque) < protocol.HonraMaxItens; slot++ {
		pos := protocol.ShopSlot(slot)
		it := npc.Carry[pos]
		if it.Index <= 0 {
			continue
		}
		preco, emPontos := npc.ShopPointPrice[pos]
		if !emPontos || preco <= 0 {
			continue
		}
		estoque = append(estoque, itemDeHonra{
			Slot:    int16(slot),
			Indice:  it.Index,
			Preco:   preco,
			Cat:     d.categoriaDeHonra(it.Index),
			Efeitos: it.Effects,
		})
	}
	return estoque
}

// itemDeHonraNaVaga é a troca da vaga slot, se o God of War vende alguma ali.
func (d *Dispatcher) itemDeHonraNaVaga(npc *world.Entity, slot int) (itemDeHonra, bool) {
	for _, it := range d.estoqueDeHonra(npc) {
		if int(it.Slot) == slot {
			return it, true
		}
	}
	return itemDeHonra{}, false
}

// categoriaDeHonra é a aba do item, tirada da casa de equipar dele no ItemList
// (nPos): arma vai para Armas, peça de armadura para Set, e o resto — material,
// consumível, fada, montaria — para Consumo.
//
// Deduzida, e não cadastrada, porque o painel de NPC não tem onde escrever a aba:
// ele só conhece item, quantidade e preço. A classificação é a mesma que o
// compositor usa (combine.SlotKindForPos), e é estreita de propósito: só as cinco
// casas de armadura (2..32) e as três de arma (64, 128, 192) saem de Consumo.
// Fada (casa 16384 no ItemList) e montaria caem em Consumo, que é onde as abas
// antigas, escritas à mão, já as punham.
//
// Sem catálogo (tmServer sem -content) tudo vai para Consumo, e continua
// aparecendo na aba Todos.
func (d *Dispatcher) categoriaDeHonra(indice int16) uint8 {
	switch combine.SlotKindForPos(int32(d.combineCatalog.Pos[int(indice)])) {
	case combine.SlotWeapon:
		return protocol.HonraCatArmas
	case combine.SlotArmour:
		return protocol.HonraCatSet
	default:
		return protocol.HonraCatConsumo
	}
}

// auditaEstoqueDeHonra avisa, no log, das vagas do God of War que ele não vai
// vender: cobradas em ouro ou a preço zero. Roda a cada recarga das lojas
// (npcconfig.go), que é quando o estoque muda.
func (d *Dispatcher) auditaEstoqueDeHonra(npc *world.Entity) {
	if !ehLojaDeHonra(npc) {
		return
	}
	var emOuro, deGraca []int
	for slot := 0; slot < maxShopSlots; slot++ {
		pos := protocol.ShopSlot(slot)
		if npc.Carry[pos].Index <= 0 {
			continue
		}
		preco, emPontos := npc.ShopPointPrice[pos]
		switch {
		case !emPontos:
			emOuro = append(emOuro, slot)
		case preco <= 0:
			deGraca = append(deGraca, slot)
		}
	}
	if len(emOuro) > 0 || len(deGraca) > 0 {
		d.log.Warn("loja de honra: vagas fora da venda, cadastre o preço em pontos",
			"npc", npc.ID, "vagas_em_ouro", emOuro, "vagas_a_zero", deGraca)
	}
}

// ehLojaDeHonra diz se npc é a loja de honra.
func ehLojaDeHonra(npc *world.Entity) bool {
	return npc != nil && npc.Merchant == merchantLojaDeHonra
}

// marcaLojaDeHonra põe o Merchant da loja no NPC que nasceu do template dela.
// Chamada no nascimento, das duas formas que um NPC do painel nasce
// (npcconfig.go), e é o único lugar que liga o template ao Merchant.
//
// O template chega pelo npc_definition e não pela entidade: a entidade nasce sem
// TemplateName quando o NPC é gerido pelo painel — o bloco vai de propósito sem
// nome, para não herdar as regras de drop de outro monstro (ver npcconfig.go).
func marcaLojaDeHonra(e *world.Entity, templateName string) {
	if e == nil || !strings.EqualFold(strings.TrimSpace(templateName), templateDaLojaDeHonra) {
		return
	}
	e.Merchant = merchantLojaDeHonra
}

// itensDeHonraParaOPainel monta o estoque na forma da linha.
func itensDeHonraParaOPainel(estoque []itemDeHonra) []protocol.HonraItem {
	itens := make([]protocol.HonraItem, 0, len(estoque))
	for _, it := range estoque {
		itens = append(itens, protocol.HonraItem{
			Slot:      it.Slot,
			Indice:    it.Indice,
			Qtd:       uint8(itemAmount(world.Item{Index: it.Indice, Effects: it.Efeitos})),
			Preco:     it.Preco,
			Categoria: it.Cat,
		})
	}
	return itens
}

// abrirLojaDeHonra responde ao clique no God of War: manda o estoque e o saldo.
//
// O saldo vem do banco, e não de um número guardado na sessão, pelo mesmo motivo
// do /pontos: o ponto é da CONTA, e os outros três personagens dela podem ter
// somado enquanto este estava andando. Leitura fora do laço, como toda leitura
// de banco.
func (d *Dispatcher) abrirLojaDeHonra(w *world.World, s *world.Session, npc *world.Entity) {
	// Guardar qual NPC abriu a loja é o que faz a compra poder exigir presença:
	// sem isto, um cliente remendado compraria de qualquer lugar do mundo.
	s.LojaHonraNPC = npc.ID
	itens := itensDeHonraParaOPainel(d.estoqueDeHonra(npc))
	// O quanto a lojinha rende vai junto, para o painel poder dizer de onde sai a
	// moeda que ele cobra. É lido agora, e para ESTE jogador: quem está com uma
	// Fada Azul ganha mais, e o painel não teria como saber disso sozinho.
	porJanela := shopPointsPerWindow(w.Entity(s.Conn))
	accountID := s.AccountID
	p := w.Persistence()
	d.log.Info("loja de honra aberta", "conn", s.Conn, "npc", npc.ID, "itens", len(itens))
	w.Go(s, func() func(*world.World, *world.Session) {
		saldo, err := p.ShopPoints(context.Background(), accountID)
		return func(w *world.World, s *world.Session) {
			if err != nil {
				d.log.Warn("loja de honra: leitura de saldo falhou", "conta", accountID, "err", err)
				sendClientMessage(w, s, "Não foi possível consultar seus pontos agora.")
				return
			}
			corpo := (&protocol.HonraAbreBody{
				Saldo:         saldo,
				PorJanela:     int16(porJanela),
				MinutosJanela: shopPointsWindowMs / 60000,
				Itens:         itens,
			}).Encode()
			w.Send(s, protocol.MsgHonraAbre, corpo)
		}
	})
}

// honraFecha atende MsgHonraFecha: o painel fechou, então a loja não está mais
// aberta e a compra volta a ser recusada.
func (d *Dispatcher) honraFecha(_ *world.World, s *world.Session, _ protocol.Header, _ []byte) {
	s.LojaHonraNPC = 0
}

// naVistaDaLoja diz se a loja continua de pé: a caixa de tilesDaLojaDeHonra em
// volta do NPC, nos dois eixos.
//
// A FORMA é a do legado — GetInView (GetFunc.cpp:764) mede assim, por caixa e não
// por raio, e é por ela que o _MSG_Buy responde a uma compra de longe com
// _MSG_CloseShop (_MSG_Buy.cpp:60). O TAMANHO é nosso, e menor: ver
// tilesDaLojaDeHonra.
//
// O mesmo número serve para fechar o painel e para recusar a compra, de propósito.
// Com dois limites existiria uma faixa em que o painel fica aberto e a compra é
// recusada sem o jogador entender por quê.
func naVistaDaLoja(e, npc *world.Entity) bool {
	if e == nil || npc == nil {
		return false
	}
	dx := int(npc.X) - int(e.X)
	dy := int(npc.Y) - int(e.Y)
	if dx < 0 {
		dx = -dx
	}
	if dy < 0 {
		dy = -dy
	}
	return dx <= tilesDaLojaDeHonra && dy <= tilesDaLojaDeHonra
}

// fechaLojaDeHonra derruba o painel: esquece o NPC e manda o cliente fechar.
func (d *Dispatcher) fechaLojaDeHonra(w *world.World, s *world.Session) {
	if s.LojaHonraNPC == 0 {
		return
	}
	s.LojaHonraNPC = 0
	w.Send(s, protocol.MsgHonraFechou, nil)
}

// afastouDaLojaDeHonra fecha o painel quando o jogador anda para fora da vista do
// NPC. Chamada a cada passo aceito (movement.go).
//
// Andar com a loja aberta é permitido de propósito: é o que o jogo faz com as
// lojas de NPC dele. O que não pode é continuar comprando de longe, e o painel
// aberto do outro lado do mapa seria isso — ou pareceria isso, que dá no mesmo
// para quem está olhando a tela.
func (d *Dispatcher) afastouDaLojaDeHonra(w *world.World, s *world.Session, e *world.Entity) {
	if s.LojaHonraNPC == 0 {
		return
	}
	npc := w.Entity(s.LojaHonraNPC)
	if ehLojaDeHonra(npc) && naVistaDaLoja(e, npc) {
		return
	}
	d.log.Info("loja de honra fechada: o NPC saiu de vista", "conn", s.Conn, "npc", s.LojaHonraNPC)
	d.fechaLojaDeHonra(w, s)
}

// honraCompra atende MsgHonraCompra: troca pontos por um item do estoque.
//
// A ordem importa e é esta: confere tudo o que o laço sabe, DEBITA no banco, e
// só depois entrega o item. Debitar primeiro é o que impede a entrega dupla —
// dois pedidos seguidos não podem virar dois itens com saldo para um. Se entre o
// débito e a entrega a mochila encheu, os pontos VOLTAM: o estorno é a única
// forma de o jogador não pagar por nada.
func (d *Dispatcher) honraCompra(w *world.World, s *world.Session, _ protocol.Header, payload []byte) {
	e := w.Entity(s.Conn)
	if e == nil || e.HP <= 0 || s.Mode != world.UserPlay {
		return
	}
	if s.TradeMode != 0 || s.Trade.Active {
		return // quem está vendendo ou negociando não compra
	}
	// Um pedido por vez. Sem esta trava dois cliques rápidos viram dois débitos
	// no ar ao mesmo tempo, e o segundo pode passar com o saldo do primeiro.
	if s.HonraCobrando {
		return
	}
	var pedido protocol.HonraCompraBody
	if err := pedido.Decode(payload); err != nil {
		return
	}
	npc := w.Entity(s.LojaHonraNPC)
	if !ehLojaDeHonra(npc) {
		return // painel fechado, ou nunca esteve aberto
	}
	// Presença: a loja é do NPC, não do jogador. O limite é o mesmo do jogo — ver
	// naVistaDaLoja —, e é o mesmo que fecha o painel, para não existir uma faixa
	// em que ele está aberto e a compra é recusada.
	if !naVistaDaLoja(e, npc) {
		sendClientMessage(w, s, "Você está longe demais da loja.")
		d.fechaLojaDeHonra(w, s)
		return
	}
	// A vaga é lida do NPC AGORA, e não do que o painel mostrou: se a equipe
	// trocou o estoque entre a abertura e o clique, vale o estoque de agora. Uma
	// vaga que deixou de vender responde e fecha o painel, em vez de sumir calada
	// — o jogador está olhando para um item que clicou e precisa saber por que
	// não veio.
	item, ok := d.itemDeHonraNaVaga(npc, int(pedido.Slot))
	if !ok {
		sendClientMessage(w, s, msgHonraItemSaiu)
		d.fechaLojaDeHonra(w, s)
		return
	}
	if firstFreeTradeSlot(e) < 0 {
		d.notify(w, s, NoticeNoSpaceToTrade)
		return
	}

	s.HonraCobrando = true
	accountID := s.AccountID
	nome := e.Name
	preco := item.Preco
	indice := item.Indice
	efeitos := item.Efeitos
	p := w.Persistence()
	d.log.Info("loja de honra: cobrando", "conn", s.Conn, "conta", accountID,
		"item", indice, "pontos", preco)
	w.Go(s, func() func(*world.World, *world.Session) {
		saldo, err := p.AddShopPoints(context.Background(), accountID, -preco, nome, honraMotivo)
		return func(w *world.World, s *world.Session) {
			s.HonraCobrando = false
			if errors.Is(err, world.ErrPontosInsuficientes) {
				sendClientMessage(w, s, fmt.Sprintf(
					"Você precisa de %d pontos de lojinha para trocar por este item.", preco))
				return
			}
			if err != nil {
				d.log.Error("loja de honra: débito falhou", "conta", accountID,
					"pontos", preco, "err", err)
				sendClientMessage(w, s, "A loja de honra está indisponível agora. Tente de novo.")
				return
			}
			d.entregaDeHonra(w, s, accountID, nome, indice, efeitos, preco, saldo)
		}
	})
}

// entregaDeHonra põe o item comprado na mochila, já com os pontos debitados. Uma
// entrega que não cabe mais devolve os pontos.
//
// Roda no laço, na volta do débito — e por isso tudo é conferido de novo: entre o
// pedido e esta linha o jogador pode ter morrido, deslogado e voltado, ou enchido
// a mochila.
func (d *Dispatcher) entregaDeHonra(w *world.World, s *world.Session, accountID int64,
	nome string, indice int16, efeitos [3]world.Effect, preco, saldo int32) {
	e := w.Entity(s.Conn)
	destino := -1
	if e != nil && s.Mode == world.UserPlay {
		destino = firstFreeTradeSlot(e)
	}
	if destino < 0 {
		// Estorno. O crédito pode falhar por sua vez, e aí o log é a única
		// testemunha: é dinheiro de alguém, então ele grita.
		p := w.Persistence()
		d.log.Warn("loja de honra: sem espaço na volta do débito, estornando",
			"conta", accountID, "pontos", preco)
		w.GoDetached(func() func(*world.World) {
			if _, err := p.AddShopPoints(context.Background(), accountID, preco, nome,
				honraMotivo+": estorno"); err != nil {
				d.log.Error("loja de honra: estorno falhou", "conta", accountID,
					"pontos", preco, "err", err)
			}
			return nil
		})
		d.notify(w, s, NoticeNoSpaceToTrade)
		return
	}
	item := world.Item{Index: indice, Effects: efeitos}
	e.Carry[destino] = item
	d.sendSlot(w, s, world.ItemPlaceCarry, destino, item)
	w.Send(s, protocol.MsgHonraSaldo, protocol.EncodeHonraSaldo(saldo))
	d.log.Info("loja de honra: entregue", "conn", s.Conn, "conta", accountID,
		"item", indice, "pontos", preco, "saldo", saldo, "slot", destino)
}

// honraAberta é o que um painel aberto mostrava antes de uma recarga das lojas:
// de qual NPC gerido ele era (o slug, que sobrevive à recarga) e o estoque.
type honraAberta struct {
	slug    string
	estoque []itemDeHonra
}

// guardaLojasDeHonraAbertas anota, antes de a recarga tirar os NPCs do mundo, o
// que cada painel de honra aberto estava mostrando. A chave é o id do God of War
// de antes da recarga, que é o que as sessões guardam em LojaHonraNPC.
//
// Loop-only; devolve nil quando ninguém está com o painel aberto, que é o caso
// comum.
func (d *Dispatcher) guardaLojasDeHonraAbertas(w *world.World) map[int]honraAberta {
	var antes map[int]honraAberta
	w.ForEachSession(func(s *world.Session, _ *world.Entity) {
		if s.LojaHonraNPC == 0 {
			return
		}
		if antes == nil {
			antes = make(map[int]honraAberta)
		}
		if _, ja := antes[s.LojaHonraNPC]; ja {
			return
		}
		for slug, id := range d.managedNPCs {
			if id == s.LojaHonraNPC {
				antes[id] = honraAberta{slug: slug, estoque: d.estoqueDeHonra(w.Entity(id))}
				break
			}
		}
	})
	return antes
}

// religaLojasDeHonraAbertas acerta os painéis abertos depois da recarga.
//
// A recarga tira todo NPC gerido do mundo e o põe de volta, e o God of War volta
// com OUTRO id. Um painel que continuasse apontando para o id antigo recusaria
// toda compra (o NPC não existe mais, ou é outro). Então: se o estoque do God of
// War é o mesmo de antes — a equipe mexeu em outra loja —, o painel passa a
// apontar para o id novo e o jogador nem percebe. Se mudou, o painel fecha com um
// aviso, porque os preços que ele mostra não são mais os que a loja cobra.
func (d *Dispatcher) religaLojasDeHonraAbertas(w *world.World, antes map[int]honraAberta) {
	w.ForEachSession(func(s *world.Session, _ *world.Entity) {
		if s.LojaHonraNPC == 0 {
			return
		}
		if a, ok := antes[s.LojaHonraNPC]; ok {
			if id, ok := d.managedNPCs[a.slug]; ok {
				npc := w.Entity(id)
				if ehLojaDeHonra(npc) && slices.Equal(d.estoqueDeHonra(npc), a.estoque) {
					s.LojaHonraNPC = id
					return
				}
			}
		}
		d.fechaLojaDeHonra(w, s)
		sendClientMessage(w, s, msgHonraAtualizada)
	})
}
