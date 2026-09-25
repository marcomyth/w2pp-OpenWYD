//go:build integration

// Testes de integração da ABERTURA e do CANCELAMENTO da cobrança.
//
//	W2PP_TEST_DSN=postgres://postgres:dev@localhost:5432/postgres go test -tags=integration ./internal/store/
package store

import (
	"context"
	"testing"
	"time"
)

// anuncioPronto monta o estado completo de um item à venda: vendedor com chave
// Pix, anúncio ativo e o cadeado no slot.
func anuncioPronto(ctx context.Context, t *testing.T, s *Store, nome string) (vendedor, comprador, anuncio int64) {
	t.Helper()
	vendedor = contaPix(ctx, t, s, "vendedor_"+nome)
	comprador = contaPix(ctx, t, s, "comprador_"+nome)
	if err := s.SalvarChavePix(ctx, vendedor, "11111111111", ChavePixCPF, "11144477735"); err != nil {
		t.Fatal(err)
	}
	anuncio = anuncioAtivoSimples(ctx, t, s, vendedor, 0)
	itemMarcado(ctx, t, s, vendedor, 0, anuncio)
	return vendedor, comprador, anuncio
}

// A cobrança nasce com prazo, valor do anúncio e a chave de destino.
//
// A chave vem da mesma transação de propósito: lida antes, uma troca no
// meio-tempo geraria um QR para a conta errada e o dinheiro cairia no lugar
// errado sem ninguém ter errado nada.
func TestAbrirCobrancaNasceComPrazoEDestino(t *testing.T) {
	s, ctx := freshStore(t)
	vendedor, comprador, anuncio := anuncioPronto(ctx, t, s, "abre")

	res, cob, err := s.AbrirCobrancaRMT(ctx, anuncio, comprador, "ref-abre-1", 0)
	if err != nil {
		t.Fatalf("abrindo: %v", err)
	}

	if res != CobrancaAbertaOK {
		t.Fatalf("resultado = %d, quero CobrancaAbertaOK(%d)", res, CobrancaAbertaOK)
	}
	if cob.CobrancaID == 0 {
		t.Error("a cobranca nasceu sem id")
	}
	if cob.VendedorConta != vendedor {
		t.Errorf("vendedor = %d, quero %d", cob.VendedorConta, vendedor)
	}
	if cob.ValorCentavos != 5000 {
		t.Errorf("valor = %d centavos, quero 5000 — o preco vem do ANUNCIO", cob.ValorCentavos)
	}
	if cob.ChavePixDestino != "11111111111" {
		t.Errorf("chave de destino = %q; sem ela nao ha QR", cob.ChavePixDestino)
	}
	if faltam := time.Until(cob.ExpiraEm); faltam <= 0 || faltam > JanelaPadraoCobranca+time.Minute {
		t.Errorf("prazo = %v, quero perto de %v", faltam, JanelaPadraoCobranca)
	}
}

// A IDEMPOTÊNCIA: a mesma referência externa encontra a MESMA linha.
//
// É o caminho normal de um pedido repetido — a tela recarregou, a rede engasgou,
// o app tentou de novo. Sem isto, cada repetição criaria outra cobrança e o
// índice de uma aberta por anúncio recusaria, transformando um repique normal em
// erro na cara do comprador.
func TestAbrirCobrancaComAMesmaReferenciaNaoCriaOutra(t *testing.T) {
	s, ctx := freshStore(t)
	_, comprador, anuncio := anuncioPronto(ctx, t, s, "idem")

	_, primeira, err := s.AbrirCobrancaRMT(ctx, anuncio, comprador, "ref-idem-1", 0)
	if err != nil {
		t.Fatal(err)
	}
	res, segunda, err := s.AbrirCobrancaRMT(ctx, anuncio, comprador, "ref-idem-1", 0)
	if err != nil {
		t.Fatalf("repetindo: %v", err)
	}

	if res != CobrancaJaExistia {
		t.Errorf("resultado = %d, quero CobrancaJaExistia(%d)", res, CobrancaJaExistia)
	}
	if segunda.CobrancaID != primeira.CobrancaID {
		t.Errorf("a repeticao criou a cobranca %d; a primeira era %d",
			segunda.CobrancaID, primeira.CobrancaID)
	}
	var quantas int
	if err := s.pool.QueryRow(ctx,
		`SELECT count(*) FROM rmt_cobranca WHERE anuncio_id = $1`, anuncio).Scan(&quantas); err != nil {
		t.Fatal(err)
	}
	if quantas != 1 {
		t.Errorf("existem %d cobrancas contra o mesmo anuncio, quero 1", quantas)
	}
}

// DOIS COMPRADORES, UM ITEM: o segundo é recusado.
//
// A invariante que impede duas pessoas de pagarem pelo mesmo item e as duas
// terem razão. Ela mora no índice único da 0105, e é por isso que ela vale mesmo
// quando os dois cliques chegam no mesmo instante.
func TestSegundoCompradorNaoAbreCobrancaNoMesmoAnuncio(t *testing.T) {
	s, ctx := freshStore(t)
	_, comprador, anuncio := anuncioPronto(ctx, t, s, "dois")
	outro := contaPix(ctx, t, s, "outro_comprador")

	if _, _, err := s.AbrirCobrancaRMT(ctx, anuncio, comprador, "ref-dois-1", 0); err != nil {
		t.Fatal(err)
	}
	res, _, err := s.AbrirCobrancaRMT(ctx, anuncio, outro, "ref-dois-2", 0)
	if err != nil {
		t.Fatalf("segundo comprador: %v", err)
	}

	if res != AnuncioComOutraCobranca {
		t.Errorf("resultado = %d, quero AnuncioComOutraCobranca(%d)", res, AnuncioComOutraCobranca)
	}
}

// As três recusas restantes, lado a lado. O valor está na DIFERENÇA: separadas,
// cada uma passaria com uma função que sempre recusa.
func TestAsRecusasDaAbertura(t *testing.T) {
	s, ctx := freshStore(t)

	t.Run("comprar de si mesmo", func(t *testing.T) {
		vendedor, _, anuncio := anuncioPronto(ctx, t, s, "eumesmo")
		res, _, err := s.AbrirCobrancaRMT(ctx, anuncio, vendedor, "ref-eu-1", 0)
		if err != nil {
			t.Fatal(err)
		}
		// Não há ganho no item — ele volta para o mesmo baú. O custo é a CHAMADA:
		// cada cobrança bate na processadora, que cobra por isso.
		if res != CompradorEOVendedor {
			t.Errorf("resultado = %d, quero CompradorEOVendedor(%d)", res, CompradorEOVendedor)
		}
	})

	t.Run("anuncio sem cadeado", func(t *testing.T) {
		vendedor := contaPix(ctx, t, s, "vendedor_solto")
		comprador := contaPix(ctx, t, s, "comprador_solto")
		anuncio := anuncioAtivoSimples(ctx, t, s, vendedor, 7)
		// Nenhum itemMarcado: o anúncio está ativo e nada segura o item.
		res, _, err := s.AbrirCobrancaRMT(ctx, anuncio, comprador, "ref-solto-1", 0)
		if err != nil {
			t.Fatal(err)
		}
		if res != ItemNaoEstaPreso {
			t.Errorf("resultado = %d, quero ItemNaoEstaPreso(%d): cobrar aqui produziria "+
				"dinheiro entrando sem nada para entregar", res, ItemNaoEstaPreso)
		}
	})

	t.Run("anuncio que nao existe", func(t *testing.T) {
		comprador := contaPix(ctx, t, s, "comprador_fantasma")
		res, _, err := s.AbrirCobrancaRMT(ctx, 999999, comprador, "ref-fantasma-1", 0)
		if err != nil {
			t.Fatal(err)
		}
		if res != AnuncioNaoDisponivel {
			t.Errorf("resultado = %d, quero AnuncioNaoDisponivel(%d)", res, AnuncioNaoDisponivel)
		}
	})
}

// ANÚNCIO CUJA BARRACA CAIU NÃO ACEITA COBRANÇA NOVA, mesmo estando ATIVO.
//
// "Ativo" não basta, e a distinção é a razão de a coluna `barraca_caiu` existir.
// Um anúncio nesse estado continua ativo de propósito, para o Pix atrasado DAQUELA
// cobrança ainda encontrar o que entregar — mas ele não está mais à venda para
// ninguém. Cobrar contra ele venderia uma vitrine que não existe: o comprador
// pagaria por um item que o vendedor já recolheu da praça.
func TestCobrancaNaoNasceContraAnuncioSemBarraca(t *testing.T) {
	s, ctx := freshStore(t)
	_, comprador, anuncio := anuncioPronto(ctx, t, s, "sembarraca")
	primeiro := contaPix(ctx, t, s, "primeiro_comprador_sb")
	// Uma cobrança aberta é o que faz o anúncio sobreviver ao fechamento da
	// barraca, em vez de ser cancelado.
	if _, _, err := s.AbrirCobrancaRMT(ctx, anuncio, primeiro, "ref-sb-0", 0); err != nil {
		t.Fatal(err)
	}
	if _, err := s.EncerrarAnunciosRMT(ctx, []int64{anuncio}); err != nil {
		t.Fatal(err)
	}
	if st, caiu := statusDoAnuncio(ctx, t, s, anuncio); st != anuncioAtivo || !caiu {
		t.Fatalf("o cenario nao se montou: status=%d barraca_caiu=%v", st, caiu)
	}
	// E a primeira cobrança sai do caminho, para a recusa abaixo não poder ser
	// creditada ao índice de uma aberta por anúncio.
	if _, err := s.CancelarCobrancaRMT(ctx, "ref-sb-0"); err != nil {
		t.Fatal(err)
	}

	res, _, err := s.AbrirCobrancaRMT(ctx, anuncio, comprador, "ref-sb-1", 0)
	if err != nil {
		t.Fatalf("abrindo: %v", err)
	}

	if res != AnuncioNaoDisponivel {
		t.Errorf("resultado = %d, quero AnuncioNaoDisponivel(%d): a barraca ja tinha descido",
			res, AnuncioNaoDisponivel)
	}
}

// UMA COBRANÇA ABERTA POR COMPRADOR, e esta é a que impede travar o mercado
// inteiro de graça.
//
// Sem ela, uma conta clica em comprar em TODAS as prateleiras em dinheiro real do
// mercado. Cada clique prende o item de um vendedor por cinco minutos. Ninguém
// pagou nada, nenhum item mudou de mão, e todo o estoque fica indisponível —
// repetindo a cada cinco minutos, para sempre.
//
// Dois VENDEDORES diferentes de propósito: a invariante que já existia é por
// ANÚNCIO, e com um vendedor só ela poderia ser a responsável pela recusa. A prova
// só vale com anúncios que não têm nada em comum além do comprador.
func TestUmaCobrancaAbertaPorComprador(t *testing.T) {
	s, ctx := freshStore(t)
	comprador := contaPix(ctx, t, s, "comprador_ganancioso")

	primeiro := contaPix(ctx, t, s, "vendedor_um")
	segundo := contaPix(ctx, t, s, "vendedor_dois")
	for _, v := range []int64{primeiro, segundo} {
		if err := s.SalvarChavePix(ctx, v, "11111111111", ChavePixCPF, "11144477735"); err != nil {
			t.Fatal(err)
		}
	}
	anuncioA := anuncioComFoto(ctx, t, s, primeiro, "Um", 0, 0, 1)
	itemMarcado(ctx, t, s, primeiro, 0, anuncioA)
	anuncioB := anuncioComFoto(ctx, t, s, segundo, "Dois", 0, 0, 1)
	itemMarcado(ctx, t, s, segundo, 0, anuncioB)

	if res, _, err := s.AbrirCobrancaRMT(ctx, anuncioA, comprador, "ref-ganancia-1", 0); err != nil {
		t.Fatal(err)
	} else if res != CobrancaAbertaOK {
		t.Fatalf("a primeira nao abriu: resultado %d", res)
	}

	res, _, err := s.AbrirCobrancaRMT(ctx, anuncioB, comprador, "ref-ganancia-2", 0)
	if err != nil {
		t.Fatalf("segunda cobranca: %v", err)
	}

	if res != CompradorJaTemCobranca {
		t.Errorf("resultado = %d, quero CompradorJaTemCobranca(%d): com outro resultado "+
			"uma conta prende o mercado inteiro de graca", res, CompradorJaTemCobranca)
	}
	// E o item do SEGUNDO vendedor continua livre para outra pessoa comprar.
	outro := contaPix(ctx, t, s, "outro_comprador_livre")
	if res, _, err := s.AbrirCobrancaRMT(ctx, anuncioB, outro, "ref-ganancia-3", 0); err != nil {
		t.Fatal(err)
	} else if res != CobrancaAbertaOK {
		t.Errorf("o item do segundo vendedor ficou preso: resultado %d", res)
	}
}

// E DEPOIS DE FECHAR A PRIMEIRA, ELE COMPRA DE NOVO. Sem esta metade, uma trava
// que recusasse sempre passaria no teste de cima — e o comprador ficaria preso
// para sempre à primeira compra que fez.
func TestDepoisDeFecharOCompradorAbreOutra(t *testing.T) {
	s, ctx := freshStore(t)
	_, comprador, anuncio := anuncioPronto(ctx, t, s, "denovo")

	if _, _, err := s.AbrirCobrancaRMT(ctx, anuncio, comprador, "ref-denovo-1", 0); err != nil {
		t.Fatal(err)
	}
	if _, err := s.CancelarCobrancaRMT(ctx, "ref-denovo-1"); err != nil {
		t.Fatal(err)
	}

	res, _, err := s.AbrirCobrancaRMT(ctx, anuncio, comprador, "ref-denovo-2", 0)
	if err != nil {
		t.Fatalf("segunda tentativa: %v", err)
	}
	if res != CobrancaAbertaOK {
		t.Errorf("resultado = %d, quero aberta: a primeira ja fechou", res)
	}
}

// Cancelar fecha a cobrança aberta, e SÓ a aberta.
func TestCancelarCobrancaSoFechaAAberta(t *testing.T) {
	s, ctx := freshStore(t)
	_, comprador, anuncio := anuncioPronto(ctx, t, s, "cancela")
	if _, _, err := s.AbrirCobrancaRMT(ctx, anuncio, comprador, "ref-canc-1", 0); err != nil {
		t.Fatal(err)
	}

	fechou, err := s.CancelarCobrancaRMT(ctx, "ref-canc-1")
	if err != nil {
		t.Fatalf("cancelando: %v", err)
	}
	if !fechou {
		t.Error("nao fechou a cobranca aberta")
	}
	// A segunda vez não faz nada, e isso importa: o cancelamento chega por evento
	// de rede e chega repetido.
	if fechou, err = s.CancelarCobrancaRMT(ctx, "ref-canc-1"); err != nil {
		t.Fatal(err)
	} else if fechou {
		t.Error("cancelou de novo uma cobranca ja fechada")
	}
}

// O COMPRADOR SAIU DO JOGO, A COBRANÇA FOI CANCELADA, E O PIX DELE ENTREGA.
//
// Regra da Hanna: quem pagou DENTRO do prazo recebe, e o estado da nossa linha não
// muda isso. Ele saiu do jogo — talvez para pagar no celular, que é o movimento
// natural — e o cancelamento é consequência disso, não do pagamento.
//
// E NÃO É "pago com atraso": esta coluna agora conta uma coisa só, o pagamento
// feito depois do prazo. Antes ela era ligada pelo estado da nossa linha, e por
// isso marcava também quem pagou em dia com o aviso atrasado — o número ficava
// inflado justamente na pergunta que ele existe para responder, que é se a janela
// de cinco minutos está curta demais.
func TestCompradorQueSaiuMasPagouNoPrazoRecebe(t *testing.T) {
	s, ctx := freshStore(t)
	_, comprador, anuncio := anuncioPronto(ctx, t, s, "atraso")
	if _, _, err := s.AbrirCobrancaRMT(ctx, anuncio, comprador, "ref-atraso-1", 0); err != nil {
		t.Fatal(err)
	}
	if _, err := s.CancelarCobrancaRMT(ctx, "ref-atraso-1"); err != nil {
		t.Fatal(err)
	}

	res, venda, err := s.ConfirmarCobrancaRMT(ctx, "ref-atraso-1", dentroDoPrazo(), HoraDaProcessadora, 0)
	if err != nil {
		t.Fatalf("confirmando tarde: %v", err)
	}

	if res != CobrancaConfirmada {
		t.Fatalf("resultado = %d, quero CobrancaConfirmada(%d): quem pagou direito recebe",
			res, CobrancaConfirmada)
	}
	if venda.EntregaID == 0 {
		t.Error("nao enfileirou a entrega")
	}
	// A COLUNA CONTA O LARGO, e a suíte CONTINHA AS DUAS REGRAS antes de este job
	// existir: este teste exigia falso e o TestPagamentoAtrasadoComItemAindaMarcadoEntrega
	// exigia verdadeiro, no MESMO cenário. Nenhum dos dois rodava, então ninguém
	// podia ver a contradição.
	//
	// Ficou o largo, que é o que a 0105 escreveu: "a confirmação que chegou DEPOIS do
	// cancelamento ou da expiração". A razão é a pergunta que a coluna existe para
	// responder — "o prazo está errado?" — e este caso É um sintoma disso: com uma
	// janela maior, a corrida entre o pagamento e a nossa varredura não teria
	// acontecido. Contar só o pagamento genuinamente tardio esconderia a corrida, que
	// é o que mais aparece na prática.
	//
	// A regra ESTREITA continua existindo, com outro nome: `foraDoPrazo`, que é quem
	// decide a entrega. Ela não virou coluna porque o pagamento tardio de verdade já
	// é achável pelo status PAGA_SEM_ITEM.
	if !venda.PagoComAtraso {
		t.Error("nao contou a confirmacao que chegou depois do cancelamento")
	}
}

// A EXPIRAÇÃO fecha o que venceu, e só o que venceu.
func TestExpirarFechaSoOQueVenceu(t *testing.T) {
	s, ctx := freshStore(t)
	_, comprador, anuncio := anuncioPronto(ctx, t, s, "vence")
	if _, _, err := s.AbrirCobrancaRMT(ctx, anuncio, comprador, "ref-vence-1", 0); err != nil {
		t.Fatal(err)
	}

	// Nada venceu ainda.
	if venceram, err := s.ExpirarCobrancasRMT(ctx); err != nil {
		t.Fatal(err)
	} else if len(venceram) != 0 {
		t.Errorf("expirou %v antes da hora", venceram)
	}

	// Empurra o prazo para trás, que é o que o relógio faria.
	if _, err := s.pool.Exec(ctx,
		`UPDATE rmt_cobranca SET expira_em = now() - interval '1 second'
		  WHERE referencia_externa = 'ref-vence-1'`); err != nil {
		t.Fatal(err)
	}

	venceram, err := s.ExpirarCobrancasRMT(ctx)
	if err != nil {
		t.Fatalf("expirando: %v", err)
	}
	if len(venceram) != 1 || venceram[0] != anuncio {
		t.Errorf("expirou %v, quero [%d]", venceram, anuncio)
	}
	if st := statusDaCobranca(ctx, t, s, "ref-vence-1"); st != cobrancaExpirada {
		t.Errorf("status = %d, quero expirada(%d)", st, cobrancaExpirada)
	}
}

// E depois de a cobrança vencer, a reconciliação do vendedor SOLTA o item.
//
// É o ciclo inteiro fechando: anunciou, alguém abriu o QR e sumiu, o prazo
// acabou, e o item volta. Sem esta última perna o vendedor ficaria preso ao
// arrependimento de um comprador.
func TestDepoisDeExpirarOItemVoltaAoVendedor(t *testing.T) {
	s, ctx := freshStore(t)
	vendedor, comprador, anuncio := anuncioPronto(ctx, t, s, "ciclo")
	if _, _, err := s.AbrirCobrancaRMT(ctx, anuncio, comprador, "ref-ciclo-1", 0); err != nil {
		t.Fatal(err)
	}
	// A barraca desce com a cobrança aberta: o anúncio fica de pé esperando.
	if _, err := s.EncerrarAnunciosRMT(ctx, []int64{anuncio}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.pool.Exec(ctx,
		`UPDATE rmt_cobranca SET expira_em = now() - interval '1 second'
		  WHERE referencia_externa = 'ref-ciclo-1'`); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ExpirarCobrancasRMT(ctx); err != nil {
		t.Fatal(err)
	}

	slots, err := s.ReconciliarEscrowRMT(ctx, vendedor)
	if err != nil {
		t.Fatalf("reconciliando: %v", err)
	}

	if len(slots) != 1 || slots[0] != 0 {
		t.Errorf("cadeados a soltar = %v, quero [0]: o prazo acabou e ninguem pagou", slots)
	}
	if st, _ := statusDoAnuncio(ctx, t, s, anuncio); st != anuncioCancelado {
		t.Errorf("o anuncio ficou no status %d; o slot continuaria preso pelo indice", st)
	}
}

func statusDaCobranca(ctx context.Context, t *testing.T, s *Store, ref string) int16 {
	t.Helper()
	var st int16
	if err := s.pool.QueryRow(ctx,
		`SELECT status FROM rmt_cobranca WHERE referencia_externa = $1`, ref).Scan(&st); err != nil {
		t.Fatalf("lendo a cobranca %s: %v", ref, err)
	}
	return st
}

// ANÚNCIO ABAIXO DO MÍNIMO NÃO ABRE COBRANÇA.
//
// O mínimo de R$ 1,00 nasceu em 24/09/2026, e quando nasceu JÁ HAVIA anúncio de um
// centavo gravado, de um teste da Hanna. A trava principal é na montagem da barraca,
// no jogo, onde o vendedor pode consertar — mas ela não alcança o que já está no
// banco. Esta é a segunda trava, e é a que cobre esse caso.
//
// Sem ela, o anúncio velho continuaria vendendo por um centavo: a processadora cobra
// taxa por cobrança, então o repasse ao vendedor sairia negativo e a casa pagaria
// para vender.
//
// O anúncio é gravado DIRETO NO BANCO de propósito, porque é assim que ele existe na
// produção — passar pelo jogo seria testar a outra trava, não esta.
func TestCobrancaRecusaAnuncioAbaixoDoMinimo(t *testing.T) {
	s, ctx := freshStore(t)
	vendedor := contaPix(ctx, t, s, "vendedor_minimo")
	comprador := contaPix(ctx, t, s, "comprador_minimo")
	if err := s.SalvarChavePix(ctx, vendedor, "11111111111", ChavePixCPF, "11144477735"); err != nil {
		t.Fatal(err)
	}

	// Um centavo, como o Elmo do teste que ficou no banco.
	var anuncio int64
	if err := s.pool.QueryRow(ctx, `
		INSERT INTO rmt_anuncio (vendedor_conta, cargo_slot, item_index, preco_centavos, status)
		VALUES ($1, 0, 1100, 1, 1) RETURNING id`, vendedor).Scan(&anuncio); err != nil {
		t.Fatal(err)
	}
	itemMarcado(ctx, t, s, vendedor, 0, anuncio)

	res, _, err := s.AbrirCobrancaRMT(ctx, anuncio, comprador, "ref-minimo-1", 0)
	if err != nil {
		t.Fatal(err)
	}
	if res != AnuncioNaoDisponivel {
		t.Errorf("resultado = %v, queria AnuncioNaoDisponivel: um anuncio de 1 centavo abriu cobranca", res)
	}

	// E O MÍNIMO EXATO ABRE, que é o outro lado da linha. Sem este caso, a trava
	// poderia estar recusando 100 também e o teste de cima passaria igual.
	var noMinimo int64
	if err := s.pool.QueryRow(ctx, `
		INSERT INTO rmt_anuncio (vendedor_conta, cargo_slot, item_index, preco_centavos, status)
		VALUES ($1, 1, 1100, $2, 1) RETURNING id`, vendedor, PrecoMinimoRMTCentavos).Scan(&noMinimo); err != nil {
		t.Fatal(err)
	}
	itemMarcado(ctx, t, s, vendedor, 1, noMinimo)

	res, cob, err := s.AbrirCobrancaRMT(ctx, noMinimo, comprador, "ref-minimo-2", 0)
	if err != nil {
		t.Fatal(err)
	}
	if res != CobrancaAbertaOK {
		t.Fatalf("no minimo exato o resultado foi %v, queria CobrancaAbertaOK", res)
	}
	if cob.ValorCentavos != PrecoMinimoRMTCentavos {
		t.Errorf("valor = %d, queria %d", cob.ValorCentavos, PrecoMinimoRMTCentavos)
	}
}
