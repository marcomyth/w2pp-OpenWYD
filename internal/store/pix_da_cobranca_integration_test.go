//go:build integration

// A CORRIDA DAS DUAS ABAS, que é a segunda corrida deste sistema que custa
// dinheiro de verdade — e diferente da outra, esta custa uma cobrança a MAIS na
// processadora, não uma venda a menos.
//
//	W2PP_TEST_DSN=postgres://postgres:dev@localhost:5432/postgres go test -tags=integration ./internal/store/
package store

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// cobrancaAbertaParaPix cria uma cobrança aberta, sem código, para os testes daqui.
//
// E PÕE A MARCA DO ESCROW no item, que é o passo que a primeira versão deste ajudante
// esqueceu. Sem ela o anúncio está ativo e o item NÃO está preso a ele, e o banco
// recusa a cobrança com ItemNaoEstaPreso — porque as duas coisas discordam e
// discordância em linha de dinheiro se resolve não cobrando.
//
// O estado sem marca NÃO EXISTE NO JOGO: o item é marcado na mesma transação em que o
// anúncio nasce. Montá-lo aqui era testar uma situação que o sistema não produz, e os
// nove testes deste arquivo falharam todos pelo mesmo motivo — que era meu, e não do
// código sob teste.
func cobrancaAbertaParaPix(ctx context.Context, t *testing.T, s *Store, ref string) int64 {
	t.Helper()
	vendedor := contaPix(ctx, t, s, "vendedor_"+ref)
	comprador := contaPix(ctx, t, s, "comprador_"+ref)
	anuncio := anuncioAtivoSimples(ctx, t, s, vendedor, 0)
	itemMarcado(ctx, t, s, vendedor, 0, anuncio)

	res, cob, err := s.AbrirCobrancaRMT(ctx, anuncio, comprador, ref, 5*time.Minute)
	if err != nil {
		t.Fatalf("abrindo a cobranca: %v", err)
	}
	if res != CobrancaAbertaOK {
		t.Fatalf("abertura = %d, queria aberta", res)
	}
	return cob.CobrancaID
}

// A PROVA DA CORRIDA: duas leituras ao mesmo tempo produzem UMA chamada à
// processadora, e as duas devolvem o MESMO código.
//
// O que aconteceria sem a trava, e é por isso que este teste existe: a página do
// comprador consulta a cada cinco segundos, então duas abas ou um refresh fazem
// duas leituras chegarem juntas. As duas veriam codigo_pix nulo, as duas chamariam
// a processadora, e nasceriam DUAS cobranças lá. A aba que perdesse mostraria o
// código que ela mesma criou — um identifier que não está gravado em lugar nenhum.
// O comprador paga esse, o aviso chega com um identifier que não acha cobrança, e o
// dinheiro dele fica sem venda.
//
// O portão é o que força a ordem. Sem ele a primeira chamada terminaria antes de a
// segunda começar, e o teste passaria por sorte numa máquina rápida — provando
// nada. Com ele, a primeira chamada SEGURA a transação aberta, e a segunda tem de
// esperar o lock da linha. Que ela está esperando é lido do pg_stat_activity e não
// suposto por um sleep.
func TestDuasLeiturasJuntasCriamUmPixSo(t *testing.T) {
	s, ctx := freshStore(t)
	cobranca := cobrancaAbertaParaPix(ctx, t, s, "ref-duas-abas")

	var chamadas atomic.Int32
	entrou := make(chan struct{}, 2)
	libera := make(chan struct{})

	criar := func(context.Context, string, int64) (string, string, error) {
		n := chamadas.Add(1)
		entrou <- struct{}{}
		<-libera
		// Código E identifier DIFERENTES por chamada, de propósito: se a função
		// devolvesse o que acabou de voltar da ponte em vez do que ficou gravado,
		// as duas abas mostrariam códigos diferentes e o teste veria isso.
		return fmt.Sprintf("pix-%d", n), fmt.Sprintf("ident-%d", n), nil
	}

	var wg sync.WaitGroup
	saidas := make([]PixDaCobranca, 2)
	erros := make([]error, 2)
	for i := range saidas {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			saidas[i], erros[i] = s.CriarPixSeFaltar(ctx, cobranca, time.Minute, criar)
		}(i)
	}

	// Uma das duas entrou na chamada à ponte; a outra tem de estar presa no lock.
	<-entrou
	esperaBloquearCobranca(ctx, t, s)
	close(libera)
	wg.Wait()

	for i := range erros {
		if erros[i] != nil {
			t.Fatalf("leitura %d falhou: %v", i, erros[i])
		}
	}
	if n := chamadas.Load(); n != 1 {
		t.Errorf("a processadora foi chamada %d vezes, queria 1: nasceu cobranca a mais la", n)
	}
	if saidas[0].CodigoPix != saidas[1].CodigoPix {
		t.Errorf("as duas abas veem codigos diferentes: %q e %q",
			saidas[0].CodigoPix, saidas[1].CodigoPix)
	}
	if saidas[0].CodigoPix != "pix-1" {
		t.Errorf("codigo = %q, queria o da unica chamada", saidas[0].CodigoPix)
	}
	// Uma criou, a outra leu o gravado. Qual delas ganhou não importa; que
	// exatamente uma tenha criado, importa.
	if saidas[0].Criado == saidas[1].Criado {
		t.Errorf("as duas dizem Criado=%v; exatamente uma devia ter criado", saidas[0].Criado)
	}

	// E o que ficou GRAVADO é o da chamada que ganhou, com o identifier dela.
	var codigo, ident string
	if err := s.pool.QueryRow(ctx, `
		SELECT coalesce(codigo_pix, ''), coalesce(identifier_syncpay, '')
		  FROM rmt_cobranca WHERE id = $1`, cobranca).Scan(&codigo, &ident); err != nil {
		t.Fatal(err)
	}
	if codigo != "pix-1" || ident != "ident-1" {
		t.Errorf("gravado: codigo=%q ident=%q", codigo, ident)
	}
}

// esperaBloquearCobranca espera até que alguém esteja de fato esperando o lock de
// uma linha de cobrança.
//
// Ler o pg_stat_activity é o único jeito honesto: dormir um tempo fixo faria o
// teste passar por sorte numa máquina rápida, e um teste de corrida que passa por
// sorte é pior do que nenhum — ele diz que a trava funciona quando ninguém sabe.
func esperaBloquearCobranca(ctx context.Context, t *testing.T, s *Store) {
	t.Helper()
	for range 300 {
		var esperando int
		if err := s.pool.QueryRow(ctx, `
			SELECT count(*) FROM pg_stat_activity
			 WHERE wait_event_type = 'Lock' AND query LIKE '%rmt_cobranca%'`).Scan(&esperando); err != nil {
			t.Fatalf("lendo pg_stat_activity: %v", err)
		}
		if esperando > 0 {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("ninguem ficou esperando o lock; a trava da linha nao esta la, " +
		"ou o teste nao forcou a ordem que queria")
}

// A SEGUNDA LEITURA NÃO CHAMA A PROCESSADORA. É o caso mais comum de todos — a
// página relê a cada cinco segundos —, e chamar de novo seria uma cobrança paga a
// cada cinco segundos por comprador.
func TestSegundaLeituraNaoChamaAProcessadora(t *testing.T) {
	s, ctx := freshStore(t)
	cobranca := cobrancaAbertaParaPix(ctx, t, s, "ref-releitura")

	var chamadas int
	criar := func(context.Context, string, int64) (string, string, error) {
		chamadas++
		return "pix-1", "ident-1", nil
	}

	primeira, err := s.CriarPixSeFaltar(ctx, cobranca, time.Minute, criar)
	if err != nil {
		t.Fatal(err)
	}
	segunda, err := s.CriarPixSeFaltar(ctx, cobranca, time.Minute, criar)
	if err != nil {
		t.Fatal(err)
	}

	if chamadas != 1 {
		t.Errorf("chamou a processadora %d vezes em duas leituras", chamadas)
	}
	if !primeira.Criado || segunda.Criado {
		t.Errorf("primeira.Criado=%v segunda.Criado=%v", primeira.Criado, segunda.Criado)
	}
	if segunda.CodigoPix != "pix-1" || segunda.Identifier != "ident-1" {
		t.Errorf("a releitura devolveu %+v", segunda)
	}
}

// FALHA DA PONTE NÃO GRAVA NADA, e é isso que faz a resposta INCERTA ser segura de
// repetir: a leitura seguinte tenta com a MESMA referencia_externa, e a ponte
// reconhece a repetição em vez de criar uma segunda cobrança lá.
//
// Se a falha gravasse um código vazio, a linha ficaria para sempre num estado que a
// trava considera resolvido, e a página mostraria "gerando" até o prazo vencer.
func TestFalhaDaPonteNaoGravaNadaEPermiteTentarDeNovo(t *testing.T) {
	s, ctx := freshStore(t)
	cobranca := cobrancaAbertaParaPix(ctx, t, s, "ref-incerta")

	quebrada := func(context.Context, string, int64) (string, string, error) {
		return "", "", errors.New("a chamada saiu e a resposta nao voltou")
	}
	if _, err := s.CriarPixSeFaltar(ctx, cobranca, time.Minute, quebrada); err == nil {
		t.Fatal("a falha da ponte nao virou erro")
	}

	var codigo *string
	if err := s.pool.QueryRow(ctx,
		`SELECT codigo_pix FROM rmt_cobranca WHERE id = $1`, cobranca).Scan(&codigo); err != nil {
		t.Fatal(err)
	}
	if codigo != nil {
		t.Fatalf("gravou %q depois de a ponte falhar", *codigo)
	}

	boa := func(context.Context, string, int64) (string, string, error) {
		return "pix-2", "ident-2", nil
	}
	out, err := s.CriarPixSeFaltar(ctx, cobranca, time.Minute, boa)
	if err != nil {
		t.Fatalf("a segunda tentativa falhou: %v", err)
	}
	if out.CodigoPix != "pix-2" || !out.Criado {
		t.Errorf("segunda tentativa = %+v", out)
	}
}

// CÓDIGO VAZIO DA PONTE É RECUSADO. Gravar um vazio é indistinguível de nunca ter
// chamado para quem lê, e distinguível para a trava — a pior combinação possível.
func TestCodigoVazioDaPonteERecusado(t *testing.T) {
	s, ctx := freshStore(t)
	cobranca := cobrancaAbertaParaPix(ctx, t, s, "ref-vazio")

	semCodigo := func(context.Context, string, int64) (string, string, error) {
		return "", "ident-x", nil
	}
	if _, err := s.CriarPixSeFaltar(ctx, cobranca, time.Minute, semCodigo); !errors.Is(err, ErrPonteSemCodigo) {
		t.Fatalf("erro = %v, queria ErrPonteSemCodigo", err)
	}

	var codigo, ident *string
	if err := s.pool.QueryRow(ctx,
		`SELECT codigo_pix, identifier_syncpay FROM rmt_cobranca WHERE id = $1`,
		cobranca).Scan(&codigo, &ident); err != nil {
		t.Fatal(err)
	}
	if codigo != nil || ident != nil {
		t.Errorf("gravou algo: codigo=%v ident=%v", codigo, ident)
	}
}

// IDENTIFIER VAZIO DA PONTE É RECUSADO, igual ao código vazio.
//
// Sem o identifier, o aviso de pagamento não acha a cobrança, a consulta do fim da
// janela não tem o que consultar, e o reembolso não tem referência. Um código gravado
// sem ele daria um Pix pagável cujo pagamento ninguém liga de volta à venda — e
// devolver o dinheiro também ficaria sem por onde.
func TestIdentifierVazioDaPonteERecusado(t *testing.T) {
	s, ctx := freshStore(t)
	cobranca := cobrancaAbertaParaPix(ctx, t, s, "ref-sem-ident")

	semIdent := func(context.Context, string, int64) (string, string, error) {
		return "pix-1", "", nil
	}
	if _, err := s.CriarPixSeFaltar(ctx, cobranca, time.Minute, semIdent); !errors.Is(err, ErrPonteSemIdentifier) {
		t.Fatalf("erro = %v, queria ErrPonteSemIdentifier", err)
	}

	// NADA gravado: nem o código. Mostrar o código sem o identifier seria o pior dos
	// dois mundos — pagável e não rastreável.
	var codigo, ident *string
	if err := s.pool.QueryRow(ctx,
		`SELECT codigo_pix, identifier_syncpay FROM rmt_cobranca WHERE id = $1`,
		cobranca).Scan(&codigo, &ident); err != nil {
		t.Fatal(err)
	}
	if codigo != nil || ident != nil {
		t.Errorf("gravou algo: codigo=%v ident=%v", codigo, ident)
	}
}

// GravarIdentifierSeFaltar só preenche o NULO, e nunca sobrescreve.
//
// O primeiro caso é a corrida no nascimento: a confirmação aconteceu pela referência
// que a processadora devolveu, e a cobrança ficou paga sem o id deles. O segundo é a
// proteção: dois identifiers diferentes na mesma cobrança é caso para uma pessoa, e
// não algo para esta função resolver em silêncio.
func TestGravarIdentifierSoPreencheONulo(t *testing.T) {
	s, ctx := freshStore(t)
	cobranca := cobrancaAbertaParaPix(ctx, t, s, "ref-completa")

	if err := s.GravarIdentifierSeFaltar(ctx, "ref-completa", "ident-atrasado"); err != nil {
		t.Fatal(err)
	}
	var ident string
	if err := s.pool.QueryRow(ctx,
		`SELECT coalesce(identifier_syncpay, '') FROM rmt_cobranca WHERE id = $1`,
		cobranca).Scan(&ident); err != nil {
		t.Fatal(err)
	}
	if ident != "ident-atrasado" {
		t.Fatalf("identifier = %q", ident)
	}

	// Segunda chamada com OUTRO id não muda nada.
	if err := s.GravarIdentifierSeFaltar(ctx, "ref-completa", "ident-outro"); err != nil {
		t.Fatal(err)
	}
	if err := s.pool.QueryRow(ctx,
		`SELECT coalesce(identifier_syncpay, '') FROM rmt_cobranca WHERE id = $1`,
		cobranca).Scan(&ident); err != nil {
		t.Fatal(err)
	}
	if ident != "ident-atrasado" {
		t.Errorf("sobrescreveu para %q", ident)
	}

	// Referência que não existe não é erro: o caminho que chama isto já sabe que a
	// confirmação encontrou a cobrança, e um erro aqui só faria ruído.
	if err := s.GravarIdentifierSeFaltar(ctx, "ref-que-nao-existe", "x"); err != nil {
		t.Errorf("referencia inexistente virou erro: %v", err)
	}
	// E identifier vazio não faz nada.
	if err := s.GravarIdentifierSeFaltar(ctx, "ref-completa", ""); err != nil {
		t.Errorf("identifier vazio virou erro: %v", err)
	}
}

// PRAZO NO FIM NÃO CRIA, e nada é gravado. Um Pix com dez segundos de vida é
// reembolso fabricado: a pessoa paga, o dinheiro cai numa cobrança vencida, o item
// já foi solto, e os dois lados perdem — ela sem item e a gente pagando taxa.
func TestPrazoNoFimNaoCria(t *testing.T) {
	s, ctx := freshStore(t)
	cobranca := cobrancaAbertaParaPix(ctx, t, s, "ref-prazo")

	// Encurta o prazo para dez segundos, que é menos do que o mínimo pedido.
	if _, err := s.pool.Exec(ctx,
		`UPDATE rmt_cobranca SET expira_em = now() + interval '10 seconds' WHERE id = $1`,
		cobranca); err != nil {
		t.Fatal(err)
	}

	var chamadas int
	criar := func(context.Context, string, int64) (string, string, error) {
		chamadas++
		return "pix-1", "ident-1", nil
	}

	out, err := s.CriarPixSeFaltar(ctx, cobranca, time.Minute, criar)
	if err != nil {
		t.Fatal(err)
	}
	if !out.SemPrazo || out.CodigoPix != "" {
		t.Errorf("saida = %+v, queria SemPrazo sem codigo", out)
	}
	if chamadas != 0 {
		t.Errorf("chamou a processadora %d vezes com o prazo no fim", chamadas)
	}
}

// IDENTIFIER QUE JÁ É DE OUTRA COBRANÇA É RECUSADO, e não sobrescrito. Duas linhas
// nossas apontando para o mesmo pagamento significa pedir dois reembolsos do mesmo
// dinheiro — e é o índice único da 0115 que impede, não a boa memória de alguém.
func TestIdentifierDeOutraCobrancaERecusado(t *testing.T) {
	s, ctx := freshStore(t)
	primeira := cobrancaAbertaParaPix(ctx, t, s, "ref-ident-a")
	segunda := cobrancaAbertaParaPix(ctx, t, s, "ref-ident-b")

	mesmoIdent := func(context.Context, string, int64) (string, string, error) {
		return "pix-qualquer", "ident-repetido", nil
	}
	if _, err := s.CriarPixSeFaltar(ctx, primeira, time.Minute, mesmoIdent); err != nil {
		t.Fatalf("a primeira falhou: %v", err)
	}
	_, err := s.CriarPixSeFaltar(ctx, segunda, time.Minute, mesmoIdent)
	if !errors.Is(err, ErrIdentifierDeOutraCobranca) {
		t.Fatalf("erro = %v, queria ErrIdentifierDeOutraCobranca", err)
	}

	// E a segunda continua SEM código: melhor sem Pix, e alguém olhando, do que com
	// um Pix que aponta para o pagamento de outra pessoa.
	var codigo *string
	if err := s.pool.QueryRow(ctx,
		`SELECT codigo_pix FROM rmt_cobranca WHERE id = $1`, segunda).Scan(&codigo); err != nil {
		t.Fatal(err)
	}
	if codigo != nil {
		t.Errorf("a segunda ficou com o codigo %q", *codigo)
	}
}

// A fila dos pagamentos órfãos: repetição COMPLETA a linha em vez de criar dez, e
// não reabre o que uma pessoa já resolveu.
func TestPagamentoOrfaoRepetidoCompletaENaoDuplica(t *testing.T) {
	s, ctx := freshStore(t)

	// Primeiro aviso: só o identifier e o motivo. É o que chega quando a
	// processadora ainda não sabe o resto.
	if err := s.RegistrarPagamentoOrfao(ctx, PagamentoOrfao{
		Identifier: "sp-orfao", Motivo: MotivoOrfaoSemCobranca,
	}); err != nil {
		t.Fatal(err)
	}
	// Segundo aviso, já com valor e hora.
	valor := int64(5000)
	quando := time.Now().UTC().Truncate(time.Second)
	if err := s.RegistrarPagamentoOrfao(ctx, PagamentoOrfao{
		Identifier: "sp-orfao", Motivo: MotivoOrfaoSemCobranca,
		ValorCentavos: &valor, PagoEm: &quando, ReferenciaVista: "ref-x",
	}); err != nil {
		t.Fatal(err)
	}

	fila, err := s.PagamentosOrfaos(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(fila) != 1 {
		t.Fatalf("fila tem %d linhas, queria 1: o webhook repetido duplicou", len(fila))
	}
	if fila[0].ValorCentavos == nil || *fila[0].ValorCentavos != valor {
		t.Errorf("valor = %v, o segundo aviso nao completou a linha", fila[0].ValorCentavos)
	}
	if fila[0].ReferenciaVista != "ref-x" {
		t.Errorf("referencia = %q", fila[0].ReferenciaVista)
	}

	// Um TERCEIRO aviso, mais pobre, não apaga o que já se sabia.
	if err := s.RegistrarPagamentoOrfao(ctx, PagamentoOrfao{
		Identifier: "sp-orfao", Motivo: MotivoOrfaoSemCobranca,
	}); err != nil {
		t.Fatal(err)
	}
	fila, err = s.PagamentosOrfaos(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(fila) != 1 || fila[0].ValorCentavos == nil || *fila[0].ValorCentavos != valor {
		t.Errorf("um aviso pobre apagou o que se sabia: %+v", fila)
	}

	// Resolvido sai da fila, e um aviso atrasado NÃO o traz de volta.
	if err := s.ResolverPagamentoOrfao(ctx, fila[0].ID, "hanna", "devolvido na mao"); err != nil {
		t.Fatal(err)
	}
	if err := s.RegistrarPagamentoOrfao(ctx, PagamentoOrfao{
		Identifier: "sp-orfao", Motivo: MotivoOrfaoSemCobranca,
	}); err != nil {
		t.Fatal(err)
	}
	fila, err = s.PagamentosOrfaos(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(fila) != 0 {
		t.Errorf("um aviso atrasado reabriu o que uma pessoa ja resolveu: %+v", fila)
	}
}

// Pagamento órfão sem identifier é ERRO e não linha vazia: sem o id deles, ninguém
// acha o pagamento lá, e uma linha que não aponta para nada é pior do que nenhuma.
func TestPagamentoOrfaoSemIdentifierEErro(t *testing.T) {
	s, ctx := freshStore(t)
	if err := s.RegistrarPagamentoOrfao(ctx, PagamentoOrfao{
		Motivo: MotivoOrfaoSemCobranca,
	}); err == nil {
		t.Error("aceitou um orfao sem identifier")
	}
}

// CobrancaDoIdentifier acha pelo id DELES, e não acha é resposta normal.
func TestCobrancaDoIdentifier(t *testing.T) {
	s, ctx := freshStore(t)
	cobranca := cobrancaAbertaParaPix(ctx, t, s, "ref-busca")
	criar := func(context.Context, string, int64) (string, string, error) {
		return "pix-1", "ident-busca", nil
	}
	if _, err := s.CriarPixSeFaltar(ctx, cobranca, time.Minute, criar); err != nil {
		t.Fatal(err)
	}

	ref, achou, err := s.CobrancaDoIdentifier(ctx, "ident-busca")
	if err != nil {
		t.Fatal(err)
	}
	if !achou || ref != "ref-busca" {
		t.Errorf("achou=%v ref=%q", achou, ref)
	}

	if _, achou, err := s.CobrancaDoIdentifier(ctx, "ident-que-nao-existe"); err != nil || achou {
		t.Errorf("identifier desconhecido: achou=%v err=%v", achou, err)
	}
	// Vazio não vira consulta, e não vira erro.
	if _, achou, err := s.CobrancaDoIdentifier(ctx, ""); err != nil || achou {
		t.Errorf("identifier vazio: achou=%v err=%v", achou, err)
	}
}
