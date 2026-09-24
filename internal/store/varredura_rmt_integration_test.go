//go:build integration

// A varredura que confere antes de vencer, e a fila das devoluções.
//
//	W2PP_TEST_DSN=postgres://postgres:dev@localhost:5432/postgres go test -tags=integration ./internal/store/
package store

import (
	"context"
	"errors"
	"testing"
	"time"
)

// poeIdentifier finge o que a criação do código Pix faz: grava o id da
// processadora na cobrança. Sem ele não há código, e sem código ninguém paga.
func poeIdentifier(ctx context.Context, t *testing.T, s *Store, ref, identifier string) {
	t.Helper()
	if _, err := s.pool.Exec(ctx,
		`UPDATE rmt_cobranca SET identifier_syncpay = $2 WHERE referencia_externa = $1`,
		ref, identifier); err != nil {
		t.Fatalf("gravando o identifier: %v", err)
	}
}

// venceOPrazo empurra o prazo para trás, que é o que o relógio faria.
func venceOPrazo(ctx context.Context, t *testing.T, s *Store, ref string) {
	t.Helper()
	if _, err := s.pool.Exec(ctx,
		`UPDATE rmt_cobranca SET expira_em = now() - interval '1 minute'
		  WHERE referencia_externa = $1`, ref); err != nil {
		t.Fatalf("vencendo o prazo: %v", err)
	}
}

// idDaCobranca acha a linha pela referência.
func idDaCobranca(ctx context.Context, t *testing.T, s *Store, ref string) int64 {
	t.Helper()
	var id int64
	if err := s.pool.QueryRow(ctx,
		`SELECT id FROM rmt_cobranca WHERE referencia_externa = $1`, ref).Scan(&id); err != nil {
		t.Fatalf("lendo a cobranca %s: %v", ref, err)
	}
	return id
}

// A VARREDURA CEGA NÃO ENCOSTA MAIS NA COBRANÇA QUE ALGUÉM PODE TER PAGO.
//
// É a regressão que este trabalho existe para impedir. Antes, a varredura do
// dbserver vencia qualquer cobrança no prazo sem perguntar nada à processadora — e
// a reconciliação então soltava o item do vendedor. Um Pix pago no último segundo,
// com o aviso atrasado, virava pagamento sem item: a pessoa pagava, não recebia, e
// o item já tinha voltado.
func TestVarreduraCegaNaoVenceQuemTemCodigo(t *testing.T) {
	s, ctx := freshStore(t)
	v := montaVenda(ctx, t, s, "com-codigo")
	poeIdentifier(ctx, t, s, v.ref, "tx-com-codigo")
	venceOPrazo(ctx, t, s, v.ref)

	anuncios, err := s.ExpirarCobrancasRMT(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(anuncios) != 0 {
		t.Errorf("venceu %v as cegas; o item do vendedor sairia de cima de um Pix possivel", anuncios)
	}
	if st := statusDaCobranca(ctx, t, s, v.ref); st != cobrancaAberta {
		t.Errorf("status = %d, quero aberta(%d)", st, cobrancaAberta)
	}
}

// E CONTINUA VENCENDO A QUE NINGUÉM PODERIA TER PAGO.
//
// Sem identifier não há código Pix, e sem código não há pagamento possível. Estas
// vencem sem consultar ninguém — e é importante que continuem vencendo: se
// parassem, todo anúncio cujo comprador desistiu ficaria preso esperando uma
// consulta que não tem o que consultar.
func TestVarreduraCegaVenceQuemNaoTemCodigo(t *testing.T) {
	s, ctx := freshStore(t)
	v := montaVenda(ctx, t, s, "sem-codigo")
	venceOPrazo(ctx, t, s, v.ref)

	anuncios, err := s.ExpirarCobrancasRMT(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(anuncios) != 1 {
		t.Fatalf("venceu %d, quero 1", len(anuncios))
	}
	if st := statusDaCobranca(ctx, t, s, v.ref); st != cobrancaExpirada {
		t.Errorf("status = %d, quero expirada(%d)", st, cobrancaExpirada)
	}
}

// A FILA DE CONFERIR TRAZ SÓ AS COM CÓDIGO, E AS MAIS PERTO DE VENCER PRIMEIRO.
//
// A ordem importa quando o limite corta a lista: o que fica de fora tem de ser o que
// ainda tem tempo.
func TestFilaDeConferirTrazSoAsComCodigoEPorPrazo(t *testing.T) {
	s, ctx := freshStore(t)
	longe := montaVenda(ctx, t, s, "longe")
	perto := montaVenda(ctx, t, s, "perto")
	// Uma terceira venda sem código, que não pode aparecer na fila.
	montaVenda(ctx, t, s, "sem-codigo-na-fila")
	poeIdentifier(ctx, t, s, longe.ref, "tx-longe")
	poeIdentifier(ctx, t, s, perto.ref, "tx-perto")
	if _, err := s.pool.Exec(ctx,
		`UPDATE rmt_cobranca SET expira_em = now() + interval '1 minute'
		  WHERE referencia_externa = $1`, perto.ref); err != nil {
		t.Fatal(err)
	}

	fila, err := s.CobrancasParaConferir(ctx, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(fila) != 2 {
		t.Fatalf("fila = %d, quero 2 (a sem codigo nao entra)", len(fila))
	}
	if fila[0].Identifier != "tx-perto" {
		t.Errorf("a primeira e %q; a mais perto de vencer tem de vir na frente", fila[0].Identifier)
	}
	for _, c := range fila {
		if c.Identifier == "" {
			t.Error("entrou linha sem identifier: nao haveria o que consultar")
		}
	}
}

// VENCER A CONFERIDA SÓ VALE PARA QUEM ESTÁ ABERTA E VENCIDA.
//
// As guardas não são desconfiança: entre a consulta à processadora e este UPDATE
// cabe um pagamento chegando pelo webhook. A linha que mudou nesse intervalo não
// pode ser fechada por uma decisão tomada sobre o estado antigo.
func TestVencerAConferidaRespeitaOEstadoDeAgora(t *testing.T) {
	t.Run("aberta e vencida vence", func(t *testing.T) {
		s, ctx := freshStore(t)
		v := montaVenda(ctx, t, s, "vence-ok")
		poeIdentifier(ctx, t, s, v.ref, "tx-vence-ok")
		venceOPrazo(ctx, t, s, v.ref)
		id := idDaCobranca(ctx, t, s, v.ref)

		anuncio, err := s.VencerCobrancaConferida(ctx, id)
		if err != nil {
			t.Fatal(err)
		}
		if anuncio == 0 {
			t.Error("nao venceu a que estava vencida")
		}
	})

	t.Run("ainda no prazo nao vence", func(t *testing.T) {
		s, ctx := freshStore(t)
		v := montaVenda(ctx, t, s, "no-prazo")
		poeIdentifier(ctx, t, s, v.ref, "tx-no-prazo")
		id := idDaCobranca(ctx, t, s, v.ref)

		anuncio, err := s.VencerCobrancaConferida(ctx, id)
		if err != nil {
			t.Fatal(err)
		}
		if anuncio != 0 {
			t.Error("venceu uma cobranca que ainda tinha prazo")
		}
	})

	t.Run("divergente nao vence", func(t *testing.T) {
		s, ctx := freshStore(t)
		v := montaVenda(ctx, t, s, "divergente")
		poeIdentifier(ctx, t, s, v.ref, "tx-divergente")
		venceOPrazo(ctx, t, s, v.ref)
		if _, err := s.pool.Exec(ctx,
			`UPDATE rmt_cobranca SET valor_divergente_centavos = 100
			  WHERE referencia_externa = $1`, v.ref); err != nil {
			t.Fatal(err)
		}
		id := idDaCobranca(ctx, t, s, v.ref)

		anuncio, err := s.VencerCobrancaConferida(ctx, id)
		if err != nil {
			t.Fatal(err)
		}
		if anuncio != 0 {
			// Nela o dinheiro JÁ ENTROU, com o valor errado, e ela espera uma pessoa.
			t.Error("venceu a divergente: o item sairia de cima de um pagamento recebido")
		}
	})

	t.Run("confirmada no meio do caminho nao vence", func(t *testing.T) {
		s, ctx := freshStore(t)
		v := montaVenda(ctx, t, s, "confirmou-antes")
		poeIdentifier(ctx, t, s, v.ref, "tx-confirmou-antes")
		venceOPrazo(ctx, t, s, v.ref)
		id := idDaCobranca(ctx, t, s, v.ref)
		if _, _, err := s.ConfirmarCobrancaRMT(ctx, v.ref, time.Now().UTC(),
			HoraDaProcessadora, precoEmCentavos); err != nil {
			t.Fatal(err)
		}

		anuncio, err := s.VencerCobrancaConferida(ctx, id)
		if err != nil {
			t.Fatal(err)
		}
		if anuncio != 0 {
			t.Error("venceu por cima de uma confirmacao")
		}
	})
}

// A FILA DE DEVOLVER TRAZ SÓ O PENDENTE COM CÓDIGO, E CONTA O QUE FICOU DE FORA.
//
// Uma fila que exclui linhas caladamente faz o log dizer "nada a fazer" enquanto o
// dinheiro de alguém está parado. Foi o que aconteceu com o repasse, e o contador
// existe para não acontecer de novo.
func TestFilaDeDevolverTrazPendenteComCodigoEContaOResto(t *testing.T) {
	s, ctx := freshStore(t)
	comCodigo := montaVenda(ctx, t, s, "devolver-com")
	semCodigo := montaVenda(ctx, t, s, "devolver-sem")
	poeIdentifier(ctx, t, s, comCodigo.ref, "tx-devolver")
	for _, v := range []vendaMontada{comCodigo, semCodigo} {
		if err := s.MarcarReembolsoPendente(ctx, idDaCobranca(ctx, t, s, v.ref)); err != nil {
			t.Fatal(err)
		}
	}

	fila, err := s.ReembolsosParaPedir(ctx, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(fila) != 1 {
		t.Fatalf("fila = %d, quero 1", len(fila))
	}
	if fila[0].Identifier != "tx-devolver" || fila[0].Referencia != comCodigo.ref {
		t.Errorf("linha = %+v", fila[0])
	}

	n, err := s.ReembolsosSemIdentifier(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Errorf("sem identifier = %d, quero 1: a linha sumiria de todo contador", n)
	}
}

// O INCERTO SAI DA FILA E NÃO VOLTA SOZINHO.
//
// É a trava que impede o comprador de receber duas vezes. Se o incerto continuasse
// pendente, a rodada seguinte pediria de novo — e o pedido anterior pode ter sido
// criado.
func TestReembolsoIncertoSaiDaFilaENaoVolta(t *testing.T) {
	s, ctx := freshStore(t)
	v := montaVenda(ctx, t, s, "incerto")
	poeIdentifier(ctx, t, s, v.ref, "tx-incerto")
	id := idDaCobranca(ctx, t, s, v.ref)
	if err := s.MarcarReembolsoPendente(ctx, id); err != nil {
		t.Fatal(err)
	}
	if err := s.MarcarReembolsoIncerto(ctx, id, "a chamada nao voltou"); err != nil {
		t.Fatal(err)
	}

	fila, err := s.ReembolsosParaPedir(ctx, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(fila) != 0 {
		t.Fatalf("o incerto continua na fila: seria pedido de novo")
	}

	// E ele não anda para frente sozinho: nem para pedido, nem para recusado. Só
	// uma pessoa que abrir o painel da processadora resolve.
	if err := s.MarcarReembolsoPedido(ctx, id); err == nil {
		t.Error("o incerto virou pedido sozinho")
	}
	if err := s.MarcarReembolsoRecusado(ctx, id, "X"); err == nil {
		t.Error("o incerto virou recusado sozinho")
	}
}

// A FILA DAS VENCIDAS SEM CONFERIR É O ALARME DE QUE NINGUÉM ESTÁ VARRENDO.
//
// Com o webserver no ar ela fica em zero. Crescendo, ela diz que as cobranças
// pararam de ser conferidas — e o efeito visível é o item do vendedor continuar
// preso depois do prazo.
func TestVencidasSemConferirContaAsQuePassaramDoPrazo(t *testing.T) {
	s, ctx := freshStore(t)
	v := montaVenda(ctx, t, s, "alarme")
	poeIdentifier(ctx, t, s, v.ref, "tx-alarme")
	venceOPrazo(ctx, t, s, v.ref)

	n, err := s.CobrancasVencidasSemConferir(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Errorf("alarme = %d, quero 1", n)
	}

	if _, err := s.VencerCobrancaConferida(ctx, idDaCobranca(ctx, t, s, v.ref)); err != nil {
		t.Fatal(err)
	}
	if n, err = s.CobrancasVencidasSemConferir(ctx); err != nil || n != 0 {
		t.Errorf("depois de vencer: alarme = %d, err = %v", n, err)
	}
}

// A COBRANÇA VENCIDA CONTINUA NA LISTA POR UM TEMPO, e sai dela depois.
//
// O código Pix não morre com a cobrança: não há como cancelá-lo na processadora. Se
// ninguém mais perguntar, um pagamento atrasado cujo aviso se perdeu não vira linha
// nenhuma — nem pagamento sem item, nem órfão — e o dinheiro entra sem destino.
//
// A janela é chute até alguém medir quanto tempo o código vive; o que este teste
// amarra é que ela EXISTE nos dois sentidos: a recém-vencida entra, e a velha sai.
func TestACobrancaVencidaFicaNaListaEnquantoOCodigoPodeSerPago(t *testing.T) {
	s, ctx := freshStore(t)
	nova := montaVenda(ctx, t, s, "morta-nova")
	velha := montaVenda(ctx, t, s, "morta-velha")
	poeIdentifier(ctx, t, s, nova.ref, "tx-morta-nova")
	poeIdentifier(ctx, t, s, velha.ref, "tx-morta-velha")
	for _, v := range []vendaMontada{nova, velha} {
		venceOPrazo(ctx, t, s, v.ref)
		if _, err := s.VencerCobrancaConferida(ctx, idDaCobranca(ctx, t, s, v.ref)); err != nil {
			t.Fatal(err)
		}
	}
	// A velha venceu há três dias, fora de qualquer janela razoável.
	if _, err := s.pool.Exec(ctx,
		`UPDATE rmt_cobranca SET expira_em = now() - interval '3 days'
		  WHERE referencia_externa = $1`, velha.ref); err != nil {
		t.Fatal(err)
	}

	lista, err := s.CobrancasMortasParaConferir(ctx, JanelaDaCobrancaMorta, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(lista) != 1 {
		t.Fatalf("lista = %d, quero 1 (so a recem-vencida)", len(lista))
	}
	if lista[0].Identifier != "tx-morta-nova" {
		t.Errorf("veio %q", lista[0].Identifier)
	}
}

// O CAMINHO INTEIRO DO PAGAMENTO QUE CHEGOU DEPOIS DE VENCER.
//
// Vence sem pagamento; dez minutos depois o dinheiro aparece; a confirmação põe a
// linha em PAGA_SEM_ITEM e a devolução em PENDENTE; e ela entra na fila de pedir.
//
// É o buraco que a planejadora viu: sem a conferência das vencidas, ninguém chega a
// chamar o ConfirmarCobrancaRMT, e esse dinheiro não existe em lugar nenhum do
// nosso lado.
func TestPagamentoDepoisDeVencerVaiPararNaFilaDeDevolucao(t *testing.T) {
	s, ctx := freshStore(t)
	v := montaVenda(ctx, t, s, "pagou-tarde")
	poeIdentifier(ctx, t, s, v.ref, "tx-pagou-tarde")
	venceOPrazo(ctx, t, s, v.ref)
	id := idDaCobranca(ctx, t, s, v.ref)
	if _, err := s.VencerCobrancaConferida(ctx, id); err != nil {
		t.Fatal(err)
	}

	// Ela continua sendo perguntada.
	lista, err := s.CobrancasMortasParaConferir(ctx, JanelaDaCobrancaMorta, 10)
	if err != nil || len(lista) != 1 {
		t.Fatalf("lista = %d, err = %v", len(lista), err)
	}

	// E a consulta acha o pagamento, dez minutos depois do prazo.
	res, venda, err := s.ConfirmarCobrancaRMT(ctx, v.ref, time.Now().UTC(),
		HoraDaProcessadora, precoEmCentavos)
	if err != nil {
		t.Fatal(err)
	}
	if res != CobrancaPagaSemItem {
		t.Fatalf("resultado = %v, quero paga sem item", res)
	}
	if err := s.MarcarReembolsoPendente(ctx, venda.CobrancaID); err != nil {
		t.Fatal(err)
	}

	fila, err := s.ReembolsosParaPedir(ctx, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(fila) != 1 || fila[0].CobrancaID != id {
		t.Fatalf("fila de devolucao = %+v; o dinheiro ficaria sem destino", fila)
	}
}

// A SAÍDA DO VALOR DIVERGENTE DESTRAVA AS DUAS PONTAS, e este teste existe porque a
// primeira versão da tela não tinha saída nenhuma.
//
// A cobrança divergente fica ABERTA. Enquanto ela estiver, o comprador não consegue
// abrir outra — o índice de uma aberta por comprador o recusa — e o item do vendedor
// fica marcado, porque o anúncio continua esperando uma cobrança que nunca fecha. A
// staff resolveria o dinheiro por fora e as duas pessoas continuariam presas para
// sempre, porque nada no sistema saberia que acabou.
//
// A sabotagem que o teste faz é a que a planejadora pediu: DEPOIS da saída, o
// comprador consegue abrir outra cobrança e o cadeado do vendedor sai.
func TestASaidaDoDivergenteDestravaAsDuasPontas(t *testing.T) {
	s, ctx := freshStore(t)
	v := montaVenda(ctx, t, s, "divergente-preso")
	id := idDaCobranca(ctx, t, s, v.ref)
	if _, err := s.pool.Exec(ctx,
		`UPDATE rmt_cobranca SET valor_divergente_centavos = 700 WHERE id = $1`, id); err != nil {
		t.Fatal(err)
	}

	// Antes: ela está na fila, e o comprador está travado.
	fila, err := s.ValoresDivergentes(ctx)
	if err != nil || len(fila) != 1 {
		t.Fatalf("fila = %d, err = %v", len(fila), err)
	}

	ator := AtorDaStaff{ContaID: 1, Papel: "admin"}
	if err := s.ResolverDivergenteDevolvido(ctx, id, ator, "devolvi pelo painel"); err != nil {
		t.Fatalf("a saida falhou: %v", err)
	}

	// A cobrança saiu da fila, e a coluna da divergência FICA como registro.
	if fila, err = s.ValoresDivergentes(ctx); err != nil || len(fila) != 0 {
		t.Fatalf("depois da saida: fila = %d, err = %v", len(fila), err)
	}
	var divergente *int64
	if err := s.pool.QueryRow(ctx,
		`SELECT valor_divergente_centavos FROM rmt_cobranca WHERE id = $1`, id).Scan(&divergente); err != nil {
		t.Fatal(err)
	}
	if divergente == nil {
		t.Error("a saida apagou o registro de que houve divergencia")
	}

	// E O ESTADO É PAGA_SEM_ITEM, que é a verdade: o dinheiro entrou, o item não saiu.
	if st := statusDaCobranca(ctx, t, s, v.ref); st != cobrancaPagaSemItem {
		t.Errorf("status = %d, quero paga sem item(%d)", st, cobrancaPagaSemItem)
	}

	// O COMPRADOR VOLTA A COMPRAR: nada mais o segura.
	var abertas int
	if err := s.pool.QueryRow(ctx,
		`SELECT count(*) FROM rmt_cobranca WHERE comprador_conta = $1 AND status = $2`,
		v.comprador, cobrancaAberta).Scan(&abertas); err != nil {
		t.Fatal(err)
	}
	if abertas != 0 {
		t.Errorf("o comprador continua com %d cobranca(s) aberta(s); nao compraria nada", abertas)
	}

	// E O ITEM DO VENDEDOR SAI, pelo caminho de sempre: a reconciliação do login.
	slots, err := s.ReconciliarEscrowRMT(ctx, v.vendedor)
	if err != nil {
		t.Fatal(err)
	}
	if len(slots) != 1 || slots[0] != 3 {
		t.Errorf("cadeados a soltar = %v, quero [3]: o item ficaria preso no bau", slots)
	}
}

// E ELA SÓ VALE UMA VEZ, e só sobre uma cobrança que é mesmo divergente.
//
// Dois cliques fechariam duas vezes, e sem a conferência da divergência esta saída
// viraria um jeito de fechar qualquer cobrança aberta pela tela errada.
func TestASaidaDoDivergenteSoValeUmaVezESoNaDivergente(t *testing.T) {
	s, ctx := freshStore(t)
	ator := AtorDaStaff{ContaID: 1, Papel: "admin"}

	t.Run("duas vezes nao", func(t *testing.T) {
		v := montaVenda(ctx, t, s, "div-duas-vezes")
		id := idDaCobranca(ctx, t, s, v.ref)
		if _, err := s.pool.Exec(ctx,
			`UPDATE rmt_cobranca SET valor_divergente_centavos = 700 WHERE id = $1`, id); err != nil {
			t.Fatal(err)
		}
		if err := s.ResolverDivergenteDevolvido(ctx, id, ator, "primeira"); err != nil {
			t.Fatal(err)
		}
		if err := s.ResolverDivergenteDevolvido(ctx, id, ator, "segunda"); !errors.Is(err, ErrDivergenteNaoEstaAberta) {
			t.Errorf("erro = %v, quero ErrDivergenteNaoEstaAberta", err)
		}
	})

	t.Run("cobranca sem divergencia nao", func(t *testing.T) {
		v := montaVenda(ctx, t, s, "div-sem-divergencia")
		id := idDaCobranca(ctx, t, s, v.ref)
		if err := s.ResolverDivergenteDevolvido(ctx, id, ator, "nao devia passar"); !errors.Is(err, ErrDivergenteNaoEstaAberta) {
			t.Errorf("erro = %v, quero ErrDivergenteNaoEstaAberta", err)
		}
		if st := statusDaCobranca(ctx, t, s, v.ref); st != cobrancaAberta {
			t.Errorf("status = %d: fechou uma cobranca comum pela tela errada", st)
		}
	})
}
