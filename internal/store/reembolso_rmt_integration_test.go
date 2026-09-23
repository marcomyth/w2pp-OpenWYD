//go:build integration

// Testes de integração das transições do reembolso — as duas saídas do estado
// RECUSADO, que é onde dinheiro de gente fica parado.
//
//	W2PP_TEST_DSN=postgres://postgres:dev@localhost:5432/postgres go test -tags=integration ./internal/store/
package store

import (
	"context"
	"errors"
	"testing"
)

func reembolsoDe(ctx context.Context, t *testing.T, s *Store, id int64) (int16, string) {
	t.Helper()
	var st *int16
	var erro *string
	if err := s.pool.QueryRow(ctx,
		`SELECT reembolso_status, reembolso_erro FROM rmt_cobranca WHERE id = $1`, id).
		Scan(&st, &erro); err != nil {
		t.Fatalf("lendo o reembolso da cobranca %d: %v", id, err)
	}
	var n int16
	if st != nil {
		n = *st
	}
	var e string
	if erro != nil {
		e = *erro
	}
	return n, e
}

func auditoriaDe(ctx context.Context, t *testing.T, s *Store, acao string) (int, int64, string) {
	t.Helper()
	var quantas int
	var alvo *int64
	var papel string
	if err := s.pool.QueryRow(ctx, `
		SELECT count(*), max(target_account_id), coalesce(max(actor_role), '')
		  FROM admin_audit_log WHERE action = $1`, acao).Scan(&quantas, &alvo, &papel); err != nil {
		t.Fatalf("lendo a auditoria de %s: %v", acao, err)
	}
	var id int64
	if alvo != nil {
		id = *alvo
	}
	return quantas, id, papel
}

// staffDeTeste cria a conta de quem age. A chave estrangeira da auditoria exige
// que o ator exista, e ela está certa em exigir: linha de auditoria apontando para
// uma conta que não existe é linha que não responde "quem fez".
func staffDeTeste(ctx context.Context, t *testing.T, s *Store, nome string) AtorDaStaff {
	t.Helper()
	return AtorDaStaff{ContaID: contaPix(ctx, t, s, nome), Papel: "admin"}
}

// recusadoNaFila monta uma cobrança com o reembolso RECUSADO pela processadora.
func recusadoNaFila(ctx context.Context, t *testing.T, s *Store, nome, codigo string) (comprador, cobranca int64) {
	t.Helper()
	vendedor := contaPix(ctx, t, s, "vendedor_"+nome)
	comprador = contaPix(ctx, t, s, "comprador_"+nome)
	if err := s.SalvarChavePix(ctx, vendedor, "11111111111", ChavePixCPF); err != nil {
		t.Fatal(err)
	}
	anuncio := anuncioComFoto(ctx, t, s, vendedor, "Mercador", 0, 0, 1)
	itemMarcado(ctx, t, s, vendedor, 0, anuncio)
	ref := "ref-" + nome
	if _, _, err := s.AbrirCobrancaRMT(ctx, anuncio, comprador, ref, 0); err != nil {
		t.Fatal(err)
	}
	if err := s.pool.QueryRow(ctx, `
		UPDATE rmt_cobranca SET status = $2, encerrada_em = now(), paga_em = now(),
			identifier_syncpay = $3
		 WHERE referencia_externa = $1 RETURNING id`,
		ref, cobrancaPagaSemItem, "id-deles-"+nome).Scan(&cobranca); err != nil {
		t.Fatal(err)
	}
	if err := s.MarcarReembolsoRecusado(ctx, cobranca, codigo); err != nil {
		t.Fatal(err)
	}
	return comprador, cobranca
}

// "TENTAR DE NOVO" põe o reembolso de volta na fila, e registra quem mandou.
//
// Volta para PENDENTE e não para PEDIDO porque pedir é falar com a processadora,
// e esta função não fala com ninguém. Marcar PEDIDO aqui faria a página do
// comprador dizer "em análise" sobre um pedido que nunca saiu.
func TestTentarDeNovoVoltaParaPendenteEAudita(t *testing.T) {
	s, ctx := freshStore(t)
	comprador, cobranca := recusadoNaFila(ctx, t, s, "retry", "403 nao habilitado")

	err := s.ReabrirReembolsoRecusado(ctx, cobranca, staffDeTeste(ctx, t, s, "staff_retry"))
	if err != nil {
		t.Fatalf("tentando de novo: %v", err)
	}

	st, erro := reembolsoDe(ctx, t, s, cobranca)
	if st != reembolsoPendente {
		t.Errorf("estado = %d, quero pendente(%d): pedir e falar com a processadora, "+
			"e esta funcao nao fala com ninguem", st, reembolsoPendente)
	}
	if erro != "" {
		t.Errorf("o erro antigo %q ficou; ele e da tentativa que ja passou", erro)
	}
	quantas, alvo, papel := auditoriaDe(ctx, t, s, "rmt_reembolso_tentar_de_novo")
	if quantas != 1 {
		t.Errorf("a auditoria tem %d linha(s), quero 1", quantas)
	}
	if alvo != comprador {
		t.Errorf("o alvo da auditoria e %d, quero o COMPRADOR %d: o dinheiro e dele",
			alvo, comprador)
	}
	if papel != "admin" {
		t.Errorf("papel = %q", papel)
	}
}

// "RESOLVIDO NA MÃO" é para quando a staff devolveu pelo painel deles.
//
// O dinheiro voltou de verdade; o que falta é o nosso registro saber. Sem esta
// ação a linha ficaria em RECUSADO para sempre e a página do comprador continuaria
// dizendo que há algo pendente que já não há.
func TestResolvidoNaMaoConcluiEAudita(t *testing.T) {
	s, ctx := freshStore(t)
	_, cobranca := recusadoNaFila(ctx, t, s, "namao", "403 nao habilitado")

	err := s.ResolverReembolsoNaMao(ctx, cobranca, staffDeTeste(ctx, t, s, "staff_namao"))
	if err != nil {
		t.Fatalf("resolvendo na mao: %v", err)
	}

	if st, _ := reembolsoDe(ctx, t, s, cobranca); st != reembolsoConcluido {
		t.Errorf("estado = %d, quero concluido(%d)", st, reembolsoConcluido)
	}
	if quantas, _, _ := auditoriaDe(ctx, t, s, "rmt_reembolso_resolvido_na_mao"); quantas != 1 {
		t.Errorf("a auditoria tem %d linha(s), quero 1", quantas)
	}
}

// AS DUAS AÇÕES SÓ VALEM SOBRE UM RECUSADO.
//
// Reabrir um que está em ANÁLISE criaria um segundo pedido sobre o mesmo
// dinheiro. Marcar como devolvido um que está em análise faria a página mentir, e
// depois chegaria a devolução de verdade sobre um registro que já dizia concluído.
func TestAsAcoesDaStaffSoValemSobreORecusado(t *testing.T) {
	s, ctx := freshStore(t)
	_, cobranca := recusadoNaFila(ctx, t, s, "emanalise", "erro")
	// Volta para "em análise", que é o estado em que ninguém deve mexer.
	if err := s.MarcarReembolsoPedido(ctx, cobranca); err != nil {
		t.Fatal(err)
	}
	ator := staffDeTeste(ctx, t, s, "staff_analise")

	if err := s.ReabrirReembolsoRecusado(ctx, cobranca, ator); !errors.Is(err, ErrReembolsoNaoEstaRecusado) {
		t.Errorf("reabrir devolveu %v; quero a recusa prevista", err)
	}
	if err := s.ResolverReembolsoNaMao(ctx, cobranca, ator); !errors.Is(err, ErrReembolsoNaoEstaRecusado) {
		t.Errorf("resolver devolveu %v; quero a recusa prevista", err)
	}

	if st, _ := reembolsoDe(ctx, t, s, cobranca); st != reembolsoPedido {
		t.Errorf("o estado mudou para %d apesar das recusas", st)
	}
	// E NADA foi para a auditoria: registrar uma ação que não aconteceu é pior do
	// que não registrar, porque manda quem for auditar procurar um efeito que não
	// existe.
	if quantas, _, _ := auditoriaDe(ctx, t, s, "rmt_reembolso_tentar_de_novo"); quantas != 0 {
		t.Errorf("a auditoria registrou %d acao(oes) que nao aconteceram", quantas)
	}
}

// A FILA DA STAFF traz o que uma pessoa precisa para resolver: o identifier deles
// e o código de erro deles.
//
// Sem os dois a tela só consegue dizer "deu erro": o identifier é o que acha a
// venda no painel da processadora, e o código é o que diz o que fazer a respeito.
func TestAFilaDaStaffTrazOIdentifierEOErro(t *testing.T) {
	s, ctx := freshStore(t)
	comprador, cobranca := recusadoNaFila(ctx, t, s, "fila", "403 reembolso nao habilitado")

	fila, err := s.ReembolsosRecusados(ctx)
	if err != nil {
		t.Fatalf("lendo a fila: %v", err)
	}

	if len(fila) != 1 {
		t.Fatalf("a fila tem %d linha(s), quero 1", len(fila))
	}
	f := fila[0]
	if f.CobrancaID != cobranca || f.CompradorConta != comprador {
		t.Errorf("linha = %+v", f)
	}
	if f.Identifier != "id-deles-fila" {
		t.Errorf("identifier = %q; sem ele nao da para achar a venda no painel deles", f.Identifier)
	}
	if f.Erro != "403 reembolso nao habilitado" {
		t.Errorf("erro = %q; o codigo deles vai inteiro, nao resumido", f.Erro)
	}
	if f.ValorCentavos != 12345 {
		t.Errorf("valor = %d", f.ValorCentavos)
	}

	// Resolvido, sai da fila. Sem esta metade, um "lista tudo" passaria acima.
	if err := s.ResolverReembolsoNaMao(ctx, cobranca, staffDeTeste(ctx, t, s, "staff_namao")); err != nil {
		t.Fatal(err)
	}
	fila, err = s.ReembolsosRecusados(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(fila) != 0 {
		t.Errorf("a fila ainda tem %d linha(s) depois de resolvida", len(fila))
	}
}
