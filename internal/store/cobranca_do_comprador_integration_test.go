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
	slot int16, refino, qtd int,
) int64 {
	t.Helper()
	var id int64
	if err := s.pool.QueryRow(ctx, `
		INSERT INTO rmt_anuncio (vendedor_conta, cargo_slot, item_index,
			eff1, effv1, eff2, effv2, preco_centavos, status)
		VALUES ($1, $2, 1030, $3, $4, $5, $6, 12345, 1) RETURNING id`,
		vendedor, slot, efeitoRefino, refino, efeitoQuantidade, qtd).Scan(&id); err != nil {
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
	anuncio := anuncioComFoto(ctx, t, s, vendedor, 0, 9, 3)
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
	anuncio := anuncioComFoto(ctx, t, s, vendedor, 0, 0, 1)
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
	anuncio := anuncioComFoto(ctx, t, s, vendedor, 0, 0, 1)
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
	anuncio := anuncioComFoto(ctx, t, s, vendedor, 0, 0, 1)
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
	anuncio := anuncioComFoto(ctx, t, s, vendedor, 0, 0, 1)
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

// Vendedor sem personagem nenhum não derruba a leitura.
//
// Acontece de verdade: conta criada pelo site, item posto à venda por outro
// personagem que foi apagado. O nome vem vazio e a página mostra o resto — melhor
// do que a página não abrir.
func TestVendedorSemPersonagemNaoDerrubaALeitura(t *testing.T) {
	s, ctx := freshStore(t)
	vendedor := contaPix(ctx, t, s, "vendedor_sem_char")
	comprador := contaPix(ctx, t, s, "comprador_sem_char")
	if err := s.SalvarChavePix(ctx, vendedor, "11111111111", ChavePixCPF); err != nil {
		t.Fatal(err)
	}
	anuncio := anuncioComFoto(ctx, t, s, vendedor, 0, 0, 1)
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
