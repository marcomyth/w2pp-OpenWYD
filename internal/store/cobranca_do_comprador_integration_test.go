//go:build integration

// Testes de integração da leitura da cobrança pelo lado de QUEM PAGA.
//
//	W2PP_TEST_DSN=postgres://postgres:dev@localhost:5432/postgres go test -tags=integration ./internal/store/
package store

import (
	"context"
	"testing"
	"time"
)

// personagemDe cria um personagem para a conta, que é de onde sai o nome do
// vendedor que o comprador vê.
func personagemDe(ctx context.Context, t *testing.T, s *Store, conta int64, slot int, nome string) {
	t.Helper()
	if _, err := s.pool.Exec(ctx, `
		INSERT INTO character (account_id, slot, name, class, level)
		VALUES ($1, $2, $3, 1, 50)`, conta, slot, nome); err != nil {
		t.Fatalf("criando o personagem %s: %v", nome, err)
	}
}

// anuncioComFoto cria um anúncio com refino e quantidade na fotografia, que é o
// que a página mostra ao lado do item.
func anuncioComFoto(ctx context.Context, t *testing.T, s *Store, vendedor int64,
	personagem string, slot int16, refino, qtd int,
) int64 {
	t.Helper()
	var id int64
	if err := s.pool.QueryRow(ctx, `
		INSERT INTO rmt_anuncio (vendedor_conta, vendedor_personagem, cargo_slot, item_index,
			eff1, effv1, eff2, effv2, preco_centavos, status)
		VALUES ($1, $2, $3, 1030, $4, $5, $6, $7, 12345, 1) RETURNING id`,
		vendedor, personagem, slot, efeitoRefino, refino, efeitoQuantidade, qtd).Scan(&id); err != nil {
		t.Fatalf("criando o anuncio: %v", err)
	}
	return id
}

// A cobrança aberta chega com tudo o que a página precisa.
//
// O refino e a quantidade saem da FOTOGRAFIA do anúncio, e não de colunas
// próprias: é o que mantém uma fonte só, e é a única que funciona com o vendedor
// fora do jogo.
func TestCobrancaAbertaChegaComAFotografia(t *testing.T) {
	s, ctx := freshStore(t)
	vendedor := contaPix(ctx, t, s, "vendedor_leitura")
	comprador := contaPix(ctx, t, s, "comprador_leitura")
	if err := s.SalvarChavePix(ctx, vendedor, "11111111111", ChavePixCPF); err != nil {
		t.Fatal(err)
	}
	personagemDe(ctx, t, s, vendedor, 0, "Mercador")
	anuncio := anuncioComFoto(ctx, t, s, vendedor, "Mercador", 0, 9, 3)
	itemMarcado(ctx, t, s, vendedor, 0, anuncio)
	if _, _, err := s.AbrirCobrancaRMT(ctx, anuncio, comprador, "ref-leitura-1", 0); err != nil {
		t.Fatal(err)
	}
	if _, err := s.pool.Exec(ctx,
		`UPDATE rmt_cobranca SET codigo_pix = $2 WHERE referencia_externa = $1`,
		"ref-leitura-1", "00020126BR.GOV.BCB.PIX6304ABCD"); err != nil {
		t.Fatal(err)
	}

	tem, cob, err := s.CobrancaAtualDoComprador(ctx, comprador, 0)
	if err != nil {
		t.Fatalf("lendo: %v", err)
	}

	if !tem {
		t.Fatal("nao achou a cobranca aberta do comprador")
	}
	if cob.CodigoPix != "00020126BR.GOV.BCB.PIX6304ABCD" {
		t.Errorf("codigo = %q", cob.CodigoPix)
	}
	if cob.ValorCentavos != 12345 {
		t.Errorf("valor = %d, quero 12345 — o preco vem do ANUNCIO", cob.ValorCentavos)
	}
	if cob.Estado != EstadoCobrancaAberta {
		t.Errorf("estado = %d, quero aberta(%d)", cob.Estado, EstadoCobrancaAberta)
	}
	if cob.ItemIndex != 1030 || cob.Refino != 9 || cob.Quantidade != 3 {
		t.Errorf("item = %d refino = %d qtd = %d; quero 1030/9/3",
			cob.ItemIndex, cob.Refino, cob.Quantidade)
	}
	if cob.VendedorNome != "Mercador" {
		t.Errorf("vendedor = %q, quero o nome do PERSONAGEM", cob.VendedorNome)
	}
	if time.Until(cob.ExpiraEm) <= 0 {
		t.Errorf("prazo = %v, ja no passado", cob.ExpiraEm)
	}
}

// O NOME VEM DA FOTOGRAFIA, E NÃO DE UM PERSONAGEM QUALQUER DA CONTA.
//
// O vendedor tem dois personagens e quem montou a barraca foi o SEGUNDO. Procurar
// um personagem da conta — o de menor slot, por exemplo — erraria de duas formas
// ao mesmo tempo: mostraria ao comprador um nome que ele nunca viu na cidade, e
// exporia o nome de outro personagem, que não tem nada a ver com a venda.
//
// É o caso que a busca por conta passaria em silêncio: ela devolveria um nome, só
// que o errado.
func TestONomeEODoPersonagemQueMontouABarraca(t *testing.T) {
	s, ctx := freshStore(t)
	vendedor := contaPix(ctx, t, s, "vendedor_dois_chars")
	comprador := contaPix(ctx, t, s, "comprador_dois_chars")
	if err := s.SalvarChavePix(ctx, vendedor, "11111111111", ChavePixCPF); err != nil {
		t.Fatal(err)
	}
	personagemDe(ctx, t, s, vendedor, 0, "PrimeiroSlot")
	personagemDe(ctx, t, s, vendedor, 1, "QuemVendeu")

	// A barraca é do SEGUNDO, e é esse nome que entra na fotografia.
	ids, err := s.AbrirAnunciosRMT(ctx, vendedor, "QuemVendeu",
		[]ItemAnunciado{{CargoSlot: 0, ItemIndex: 1030, PrecoCentavos: 5000}})
	if err != nil {
		t.Fatal(err)
	}
	itemMarcado(ctx, t, s, vendedor, 0, ids[0])
	if _, _, err := s.AbrirCobrancaRMT(ctx, ids[0], comprador, "ref-dois-chars", 0); err != nil {
		t.Fatal(err)
	}

	_, cob, err := s.CobrancaAtualDoComprador(ctx, comprador, 0)
	if err != nil {
		t.Fatalf("lendo: %v", err)
	}

	if cob.VendedorNome != "QuemVendeu" {
		t.Errorf("vendedor = %q, quero QuemVendeu — PrimeiroSlot nao montou barraca "+
			"nenhuma e o nome dele nao tem por que aparecer", cob.VendedorNome)
	}
}

// Conta sem cobrança nenhuma: nada, e sem erro.
func TestSemCobrancaNaoAchaENaoFalha(t *testing.T) {
	s, ctx := freshStore(t)
	ninguem := contaPix(ctx, t, s, "conta_limpa")

	tem, _, err := s.CobrancaAtualDoComprador(ctx, ninguem, 0)
	if err != nil {
		t.Fatalf("erro numa conta sem cobranca: %v", err)
	}
	if tem {
		t.Error("achou cobranca numa conta que nunca comprou nada")
	}
}

// A COBRANÇA DE OUTRO COMPRADOR NÃO APARECE. É a trava mais simples do arquivo e
// a que mais custaria: o código Pix de outra pessoa na tela é o pagamento dela na
// mão de quem não devia.
func TestNaoVeACobrancaDeOutroComprador(t *testing.T) {
	s, ctx := freshStore(t)
	vendedor := contaPix(ctx, t, s, "vendedor_privado")
	dono := contaPix(ctx, t, s, "comprador_dono")
	bisbilhoteiro := contaPix(ctx, t, s, "comprador_curioso")
	if err := s.SalvarChavePix(ctx, vendedor, "11111111111", ChavePixCPF); err != nil {
		t.Fatal(err)
	}
	anuncio := anuncioComFoto(ctx, t, s, vendedor, "Mercador", 0, 0, 1)
	itemMarcado(ctx, t, s, vendedor, 0, anuncio)
	if _, _, err := s.AbrirCobrancaRMT(ctx, anuncio, dono, "ref-privada-1", 0); err != nil {
		t.Fatal(err)
	}

	tem, _, err := s.CobrancaAtualDoComprador(ctx, bisbilhoteiro, 0)
	if err != nil {
		t.Fatalf("lendo: %v", err)
	}
	if tem {
		t.Error("devolveu a cobranca de outra pessoa")
	}
}

// PRAZO VENCIDO VIRA EXPIRADA SEM ESPERAR A VARREDURA.
//
// A varredura roda de minuto em minuto e a página pode ser aberta no meio desse
// minuto. Dizer "pague" para um código que já não vale seria mandar a pessoa
// mandar dinheiro para uma cobrança morta — e o dinheiro chegaria, porque o código
// continua pagável na processadora.
func TestPrazoVencidoJaViraExpiradaEEsconderCodigo(t *testing.T) {
	s, ctx := freshStore(t)
	vendedor := contaPix(ctx, t, s, "vendedor_vencido")
	comprador := contaPix(ctx, t, s, "comprador_vencido")
	if err := s.SalvarChavePix(ctx, vendedor, "11111111111", ChavePixCPF); err != nil {
		t.Fatal(err)
	}
	anuncio := anuncioComFoto(ctx, t, s, vendedor, "Mercador", 0, 0, 1)
	itemMarcado(ctx, t, s, vendedor, 0, anuncio)
	if _, _, err := s.AbrirCobrancaRMT(ctx, anuncio, comprador, "ref-vencida-1", 0); err != nil {
		t.Fatal(err)
	}
	// O prazo para trás e o status AINDA aberto: é exatamente o estado entre o
	// vencimento e a varredura.
	if _, err := s.pool.Exec(ctx, `
		UPDATE rmt_cobranca SET expira_em = now() - interval '1 second', codigo_pix = 'CODIGO'
		 WHERE referencia_externa = 'ref-vencida-1'`); err != nil {
		t.Fatal(err)
	}

	tem, cob, err := s.CobrancaAtualDoComprador(ctx, comprador, 0)
	if err != nil {
		t.Fatalf("lendo: %v", err)
	}

	if !tem {
		t.Fatal("esvaziou a resposta; a pessoa merece saber que o tempo acabou")
	}
	if cob.Estado != EstadoCobrancaExpirada {
		t.Errorf("estado = %d, quero expirada(%d) sem esperar a varredura",
			cob.Estado, EstadoCobrancaExpirada)
	}
	if cob.CodigoPix != "" {
		t.Errorf("mostrou o codigo %q de uma cobranca vencida; ele continua pagavel "+
			"na processadora", cob.CodigoPix)
	}
}

// A COBRANÇA RECÉM-FECHADA CONTINUA APARECENDO, e é por isso que a leitura se
// chama "atual" e não "aberta".
//
// O pior instante para a página esvaziar é o segundo seguinte ao pagamento. Quem
// acabou de mandar dinheiro e vê a tela em branco abre chamado, e está certo.
func TestCobrancaRecemFechadaAparecePorAlgunsMinutos(t *testing.T) {
	s, ctx := freshStore(t)
	vendedor := contaPix(ctx, t, s, "vendedor_recente")
	comprador := contaPix(ctx, t, s, "comprador_recente")
	if err := s.SalvarChavePix(ctx, vendedor, "11111111111", ChavePixCPF); err != nil {
		t.Fatal(err)
	}
	anuncio := anuncioComFoto(ctx, t, s, vendedor, "Mercador", 0, 0, 1)
	itemMarcado(ctx, t, s, vendedor, 0, anuncio)
	if _, _, err := s.AbrirCobrancaRMT(ctx, anuncio, comprador, "ref-recente-1", 0); err != nil {
		t.Fatal(err)
	}
	if _, err := s.pool.Exec(ctx, `
		UPDATE rmt_cobranca
		   SET status = $1, encerrada_em = now(), paga_em = now(), codigo_pix = 'CODIGO'
		 WHERE referencia_externa = 'ref-recente-1'`, cobrancaPaga); err != nil {
		t.Fatal(err)
	}

	tem, cob, err := s.CobrancaAtualDoComprador(ctx, comprador, 0)
	if err != nil {
		t.Fatalf("lendo: %v", err)
	}
	if !tem || cob.Estado != EstadoCobrancaPaga {
		t.Errorf("tem = %v estado = %d; quero a paga ainda visivel", tem, cob.Estado)
	}
	if cob.CodigoPix != "" {
		t.Error("mostrou o codigo de uma cobranca ja paga")
	}

	// E PASSADA A JANELA, some. Sem esta metade, um "devolve sempre a última"
	// passaria no teste de cima e encheria a página de gente para sempre.
	if _, err := s.pool.Exec(ctx, `
		UPDATE rmt_cobranca SET encerrada_em = now() - interval '2 hours'
		 WHERE referencia_externa = 'ref-recente-1'`); err != nil {
		t.Fatal(err)
	}
	tem, _, err = s.CobrancaAtualDoComprador(ctx, comprador, 0)
	if err != nil {
		t.Fatalf("lendo: %v", err)
	}
	if tem {
		t.Error("a cobranca de duas horas atras continua na pagina")
	}
}

// Com duas fechadas dentro da janela, ganha a MAIS NOVA — que é a que a pessoa
// está olhando.
func TestEntreDuasFechadasGanhaAMaisNova(t *testing.T) {
	s, ctx := freshStore(t)
	vendedor := contaPix(ctx, t, s, "vendedor_duas")
	comprador := contaPix(ctx, t, s, "comprador_duas")
	if err := s.SalvarChavePix(ctx, vendedor, "11111111111", ChavePixCPF); err != nil {
		t.Fatal(err)
	}
	anuncio := anuncioComFoto(ctx, t, s, vendedor, "Mercador", 0, 0, 1)
	itemMarcado(ctx, t, s, vendedor, 0, anuncio)

	// Primeira: cancelada há pouco, criada antes.
	if _, _, err := s.AbrirCobrancaRMT(ctx, anuncio, comprador, "ref-velha", 0); err != nil {
		t.Fatal(err)
	}
	if _, err := s.pool.Exec(ctx, `
		UPDATE rmt_cobranca SET status = $1, encerrada_em = now(),
			criada_em = now() - interval '3 minutes'
		 WHERE referencia_externa = 'ref-velha'`, cobrancaCancelada); err != nil {
		t.Fatal(err)
	}
	// Segunda: paga, criada depois.
	if _, _, err := s.AbrirCobrancaRMT(ctx, anuncio, comprador, "ref-nova", 0); err != nil {
		t.Fatal(err)
	}
	if _, err := s.pool.Exec(ctx, `
		UPDATE rmt_cobranca SET status = $1, encerrada_em = now(), paga_em = now()
		 WHERE referencia_externa = 'ref-nova'`, cobrancaPaga); err != nil {
		t.Fatal(err)
	}

	tem, cob, err := s.CobrancaAtualDoComprador(ctx, comprador, 0)
	if err != nil {
		t.Fatalf("lendo: %v", err)
	}
	if !tem || cob.Estado != EstadoCobrancaPaga {
		t.Errorf("tem = %v estado = %d; quero a mais nova, que e a paga", tem, cob.Estado)
	}
}

// Anúncio sem nome na fotografia não derruba a leitura.
//
// A coluna é NULL nos anúncios criados antes da 0113. Não existe nenhum em
// produção hoje, mas a leitura não pode explodir por isso: o nome vem vazio e a
// página mostra o resto — melhor do que a página não abrir.
func TestAnuncioSemNomeNaFotografiaNaoDerrubaALeitura(t *testing.T) {
	s, ctx := freshStore(t)
	vendedor := contaPix(ctx, t, s, "vendedor_sem_char")
	comprador := contaPix(ctx, t, s, "comprador_sem_char")
	if err := s.SalvarChavePix(ctx, vendedor, "11111111111", ChavePixCPF); err != nil {
		t.Fatal(err)
	}
	anuncio := anuncioComFoto(ctx, t, s, vendedor, "", 0, 0, 1)
	itemMarcado(ctx, t, s, vendedor, 0, anuncio)
	if _, _, err := s.AbrirCobrancaRMT(ctx, anuncio, comprador, "ref-sem-char", 0); err != nil {
		t.Fatal(err)
	}

	tem, cob, err := s.CobrancaAtualDoComprador(ctx, comprador, 0)
	if err != nil {
		t.Fatalf("lendo: %v", err)
	}
	if !tem {
		t.Fatal("nao achou a cobranca")
	}
	if cob.VendedorNome != "" {
		t.Errorf("nome = %q, quero vazio", cob.VendedorNome)
	}
	if cob.ValorCentavos != 12345 {
		t.Error("perdeu o resto da cobranca por falta de um nome")
	}
}

// cobrancaPagaSemItemDe monta o caso do pagamento atrasado: cobrança fechada como
// PAGA_SEM_ITEM, com o reembolso no estado pedido.
func cobrancaPagaSemItemDe(ctx context.Context, t *testing.T, s *Store,
	vendedor, comprador int64, ref string, reembolso int16,
) int64 {
	t.Helper()
	anuncio := anuncioComFoto(ctx, t, s, vendedor, "Mercador", 0, 0, 1)
	itemMarcado(ctx, t, s, vendedor, 0, anuncio)
	if _, _, err := s.AbrirCobrancaRMT(ctx, anuncio, comprador, ref, 0); err != nil {
		t.Fatal(err)
	}
	var id int64
	if err := s.pool.QueryRow(ctx, `
		UPDATE rmt_cobranca
		   SET status = $2, encerrada_em = now() - interval '3 hours',
		       paga_em = now() - interval '3 hours',
		       reembolso_status = $3, reembolso_pedido_em = now() - interval '3 hours'
		 WHERE referencia_externa = $1 RETURNING id`,
		ref, cobrancaPagaSemItem, reembolso).Scan(&id); err != nil {
		t.Fatal(err)
	}
	// O anúncio sai do caminho para o próximo poder usar o mesmo slot.
	if _, err := s.pool.Exec(ctx,
		`UPDATE rmt_anuncio SET status = $2 WHERE id = $1`, anuncio, anuncioVendido); err != nil {
		t.Fatal(err)
	}
	if _, err := s.pool.Exec(ctx, `
		DELETE FROM item WHERE owner_kind = 'account_cargo' AND account_id = $1 AND slot = 0`,
		vendedor); err != nil {
		t.Fatal(err)
	}
	return id
}

// O PAGAMENTO ATRASADO NÃO ENVELHECE ENQUANTO O REEMBOLSO NÃO TERMINA.
//
// O dinheiro de alguém está parado. Uma página que o esquece depois de dez
// minutos deixa a pessoa sem nada para olhar e sem a quem perguntar — e ela pagou
// de verdade.
//
// A cobrança do teste fechou há TRÊS HORAS, muito além da janela recente.
func TestPagamentoAtrasadoNaoEnvelheceEnquantoOReembolsoNaoTermina(t *testing.T) {
	s, ctx := freshStore(t)
	vendedor := contaPix(ctx, t, s, "vendedor_atrasado")
	comprador := contaPix(ctx, t, s, "comprador_atrasado")
	if err := s.SalvarChavePix(ctx, vendedor, "11111111111", ChavePixCPF); err != nil {
		t.Fatal(err)
	}
	cobrancaPagaSemItemDe(ctx, t, s, vendedor, comprador, "ref-atrasado", reembolsoPedido)

	tem, cob, err := s.CobrancaAtualDoComprador(ctx, comprador, 0)
	if err != nil {
		t.Fatalf("lendo: %v", err)
	}

	if !tem {
		t.Fatal("esqueceu um pagamento atrasado de tres horas atras; o dinheiro esta parado")
	}
	if cob.Estado != EstadoCobrancaPagaSemItem {
		t.Errorf("estado = %d, quero paga-sem-item(%d)", cob.Estado, EstadoCobrancaPagaSemItem)
	}
	if cob.Reembolso != ReembolsoPedido {
		t.Errorf("reembolso = %d, quero pedido(%d)", cob.Reembolso, ReembolsoPedido)
	}
	if cob.ReembolsoPedidoEm.IsZero() {
		t.Error("sem a data do pedido a pagina nao consegue contar os dois dias uteis")
	}
}

// O RECUSADO TAMBÉM NÃO SAI DA PÁGINA. Ele espera uma pessoa, e enquanto espera a
// pessoa que pagou merece ver que alguém sabe do dinheiro dela.
func TestReembolsoRecusadoNaoSaiDaPagina(t *testing.T) {
	s, ctx := freshStore(t)
	vendedor := contaPix(ctx, t, s, "vendedor_recusado")
	comprador := contaPix(ctx, t, s, "comprador_recusado")
	if err := s.SalvarChavePix(ctx, vendedor, "11111111111", ChavePixCPF); err != nil {
		t.Fatal(err)
	}
	cobrancaPagaSemItemDe(ctx, t, s, vendedor, comprador, "ref-recusado", reembolsoRecusado)

	tem, cob, err := s.CobrancaAtualDoComprador(ctx, comprador, 0)
	if err != nil {
		t.Fatalf("lendo: %v", err)
	}
	if !tem || cob.Reembolso != ReembolsoRecusado {
		t.Errorf("tem = %v reembolso = %d; quero o recusado ainda visivel", tem, cob.Reembolso)
	}
}

// E O CONCLUÍDO ENVELHECE COMO QUALQUER OUTRO. Sem esta metade, um "atrasado nunca
// sai" encheria a página para sempre depois de o dinheiro já ter voltado.
func TestReembolsoConcluidoEnvelheceNormalmente(t *testing.T) {
	s, ctx := freshStore(t)
	vendedor := contaPix(ctx, t, s, "vendedor_concluido")
	comprador := contaPix(ctx, t, s, "comprador_concluido")
	if err := s.SalvarChavePix(ctx, vendedor, "11111111111", ChavePixCPF); err != nil {
		t.Fatal(err)
	}
	cobrancaPagaSemItemDe(ctx, t, s, vendedor, comprador, "ref-concluido", reembolsoConcluido)

	tem, _, err := s.CobrancaAtualDoComprador(ctx, comprador, 0)
	if err != nil {
		t.Fatalf("lendo: %v", err)
	}
	if tem {
		t.Error("o reembolso concluido ha tres horas continua na pagina para sempre")
	}
}

// A COBRANÇA ABERTA TEM PRECEDÊNCIA sobre o atrasado, e o atrasado VOLTA quando
// ela fechar.
//
// As duas metades no mesmo teste porque a segunda é o que impede o conserto
// preguiçoso: esconder o atrasado atrás da aberta seria perdê-lo, e o valor está
// em ele estar só na fila.
func TestAbertaTemPrecedenciaEOAtrasadoVolta(t *testing.T) {
	s, ctx := freshStore(t)
	vendedor := contaPix(ctx, t, s, "vendedor_fila")
	comprador := contaPix(ctx, t, s, "comprador_fila")
	if err := s.SalvarChavePix(ctx, vendedor, "11111111111", ChavePixCPF); err != nil {
		t.Fatal(err)
	}
	// A ABERTA NASCE PRIMEIRO E O ATRASADO DEPOIS, de propósito: assim o atrasado
	// é o MAIS NOVO, e uma ordenação só por data escolheria ele. É o que isola a
	// regra de precedência — com a aberta sendo também a mais nova, os dois
	// critérios dariam a mesma resposta e o teste não provaria nada.
	aberto := anuncioComFoto(ctx, t, s, vendedor, "Mercador", 5, 0, 1)
	itemMarcado(ctx, t, s, vendedor, 5, aberto)
	if _, _, err := s.AbrirCobrancaRMT(ctx, aberto, comprador, "ref-fila-nova", 0); err != nil {
		t.Fatal(err)
	}
	atrasada := cobrancaPagaSemItemDe(ctx, t, s, vendedor, comprador, "ref-fila-velha", reembolsoPedido)
	if _, err := s.pool.Exec(ctx, `
		UPDATE rmt_cobranca SET criada_em = now() + interval '1 minute'
		 WHERE referencia_externa = 'ref-fila-velha'`); err != nil {
		t.Fatal(err)
	}

	_, cob, err := s.CobrancaAtualDoComprador(ctx, comprador, 0)
	if err != nil {
		t.Fatalf("lendo: %v", err)
	}
	if cob.Estado != EstadoCobrancaAberta {
		t.Fatalf("estado = %d, quero aberta: e a unica em que a pessoa pode agir", cob.Estado)
	}

	// A aberta fecha, e o atrasado volta sozinho.
	if _, err := s.CancelarCobrancaRMT(ctx, "ref-fila-nova"); err != nil {
		t.Fatal(err)
	}
	// Empurra a cancelada para fora da janela recente, senão é ELA a mais nova.
	if _, err := s.pool.Exec(ctx, `
		UPDATE rmt_cobranca SET encerrada_em = now() - interval '3 hours'
		 WHERE referencia_externa = 'ref-fila-nova'`); err != nil {
		t.Fatal(err)
	}

	_, cob, err = s.CobrancaAtualDoComprador(ctx, comprador, 0)
	if err != nil {
		t.Fatalf("lendo: %v", err)
	}
	if cob.CobrancaID != atrasada {
		t.Errorf("voltou a cobranca %d, quero a atrasada %d: ela estava so na fila",
			cob.CobrancaID, atrasada)
	}
}
