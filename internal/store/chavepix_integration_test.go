//go:build integration

// Testes de integração da chave Pix de recebimento. Precisam de banco de verdade:
//
//	W2PP_TEST_DSN=postgres://postgres:dev@localhost:5432/postgres go test -tags=integration ./internal/store/
//
// O que está aqui NÃO cabe no teste de unidade: a recusa com cobrança aberta
// atravessa duas tabelas e uma transação, e o rastro só prova alguma coisa se a
// linha estiver no banco depois que a gravação voltou.
package store

import (
	"context"
	"errors"
	"testing"
)

// contaPix cria uma conta e devolve o id.
func contaPix(ctx context.Context, t *testing.T, s *Store, nome string) int64 {
	t.Helper()
	var id int64
	if err := s.pool.QueryRow(ctx,
		`INSERT INTO account (name, pass_hash) VALUES ($1, 'x') RETURNING id`, nome).Scan(&id); err != nil {
		t.Fatalf("criando a conta %s: %v", nome, err)
	}
	return id
}

// anuncioComCobrancaAberta monta o estado que a trava precisa enxergar: um
// anúncio deste vendedor com uma cobrança em aberto pendurada nele.
func anuncioComCobrancaAberta(ctx context.Context, t *testing.T, s *Store, vendedor, comprador int64, ref string) int64 {
	t.Helper()
	var anuncio int64
	if err := s.pool.QueryRow(ctx, `
		INSERT INTO rmt_anuncio (vendedor_conta, cargo_slot, item_index, preco_centavos, status)
		VALUES ($1, 0, 1100, 5000, 1) RETURNING id`, vendedor).Scan(&anuncio); err != nil {
		t.Fatalf("criando o anuncio: %v", err)
	}
	if _, err := s.pool.Exec(ctx, `
		INSERT INTO rmt_cobranca (anuncio_id, comprador_conta, referencia_externa, valor_centavos, metodo, status, expira_em)
		VALUES ($1, $2, $3, 5000, 1, 1, now() + interval '30 minutes')`, anuncio, comprador, ref); err != nil {
		t.Fatalf("criando a cobranca: %v", err)
	}
	return anuncio
}

// A trava que importa: com cobrança aberta a chave NÃO muda.
//
// Sem ela o roteiro é de dois cliques — anunciar, esperar o comprador abrir o QR,
// trocar a chave, receber numa conta diferente da que valia quando a venda
// começou. O teste confere as duas metades: que a chamada recusa, e que o banco
// continua com a chave antiga (recusar e gravar assim mesmo seria pior do que
// não recusar).
func TestSalvarChavePixRecusaComCobrancaAberta(t *testing.T) {
	s, ctx := freshStore(t)
	vendedor := contaPix(ctx, t, s, "vendedor_pix")
	comprador := contaPix(ctx, t, s, "comprador_pix")

	if err := s.SalvarChavePix(ctx, vendedor, "11111111111", ChavePixCPF, "11144477735"); err != nil {
		t.Fatalf("primeira gravacao: %v", err)
	}
	anuncioComCobrancaAberta(ctx, t, s, vendedor, comprador, "ref-aberta-1")

	err := s.SalvarChavePix(ctx, vendedor, "22222222222", ChavePixCPF, "11144477735")

	if !errors.Is(err, ErrVendaEmCurso) {
		t.Fatalf("erro = %v, quero ErrVendaEmCurso", err)
	}
	r, err := s.LerChavePix(ctx, vendedor)
	if err != nil {
		t.Fatalf("lendo de volta: %v", err)
	}
	if r.ChaveMascarada != "***1111" {
		t.Errorf("a chave virou %q; a recusa nao devia ter gravado nada", r.ChaveMascarada)
	}
}

// Cobrança FECHADA não tranca. A trava é contra desviar um pagamento em curso, e
// não contra quem vendeu uma vez na vida trocar de banco.
func TestSalvarChavePixLiberaComCobrancaFechada(t *testing.T) {
	s, ctx := freshStore(t)
	vendedor := contaPix(ctx, t, s, "vendedor_pix2")
	comprador := contaPix(ctx, t, s, "comprador_pix2")

	if err := s.SalvarChavePix(ctx, vendedor, "11111111111", ChavePixCPF, "11144477735"); err != nil {
		t.Fatalf("primeira gravacao: %v", err)
	}
	anuncio := anuncioComCobrancaAberta(ctx, t, s, vendedor, comprador, "ref-fechada-1")
	if _, err := s.pool.Exec(ctx, `UPDATE rmt_cobranca SET status = 2 WHERE anuncio_id = $1`, anuncio); err != nil {
		t.Fatalf("fechando a cobranca: %v", err)
	}

	if err := s.SalvarChavePix(ctx, vendedor, "22222222222", ChavePixCPF, "11144477735"); err != nil {
		t.Fatalf("com a cobranca paga devia deixar trocar: %v", err)
	}
	r, _ := s.LerChavePix(ctx, vendedor)
	if r.ChaveMascarada != "***2222" {
		t.Errorf("chave = %q, quero ***2222", r.ChaveMascarada)
	}
}

// E a cobrança aberta de OUTRO vendedor não tranca esta conta. É o erro fácil de
// cometer na consulta: contar cobranças abertas sem amarrar no dono do anúncio
// travaria o servidor inteiro assim que alguém abrisse um QR.
func TestSalvarChavePixNaoTravaPorCobrancaDeOutro(t *testing.T) {
	s, ctx := freshStore(t)
	eu := contaPix(ctx, t, s, "eu_pix")
	outro := contaPix(ctx, t, s, "outro_pix")
	comprador := contaPix(ctx, t, s, "comprador_pix3")

	anuncioComCobrancaAberta(ctx, t, s, outro, comprador, "ref-do-outro")

	if err := s.SalvarChavePix(ctx, eu, "33333333333", ChavePixCPF, "11144477735"); err != nil {
		t.Fatalf("a venda do outro travou a minha chave: %v", err)
	}
}

// O rastro: cada troca deixa uma linha, mascarada dos dois lados, e o primeiro
// cadastro tem o "de onde" nulo — nulo ali significa "não havia chave antes", e
// não "não sei qual era".
func TestSalvarChavePixDeixaRastro(t *testing.T) {
	s, ctx := freshStore(t)
	conta := contaPix(ctx, t, s, "rastro_pix")

	if err := s.SalvarChavePix(ctx, conta, "11111111111", ChavePixCPF, "11144477735"); err != nil {
		t.Fatalf("cadastro: %v", err)
	}
	if err := s.SalvarChavePix(ctx, conta, "jogador@exemplo.com", ChavePixEmail, "11144477735"); err != nil {
		t.Fatalf("troca: %v", err)
	}

	rows, err := s.pool.Query(ctx, `
		SELECT tipo_antigo, chave_antiga_mascarada, tipo_novo, chave_nova_mascarada
		  FROM rmt_recebedor_historico WHERE account_id = $1 ORDER BY criado_em, id`, conta)
	if err != nil {
		t.Fatalf("lendo o historico: %v", err)
	}
	defer rows.Close()

	type linha struct {
		tipoAnt  *int16
		chaveAnt *string
		tipoNovo int16
		chaveNov string
	}
	var achadas []linha
	for rows.Next() {
		var l linha
		if err := rows.Scan(&l.tipoAnt, &l.chaveAnt, &l.tipoNovo, &l.chaveNov); err != nil {
			t.Fatalf("scan: %v", err)
		}
		achadas = append(achadas, l)
	}
	if len(achadas) != 2 {
		t.Fatalf("linhas de historico = %d, quero 2", len(achadas))
	}
	if achadas[0].tipoAnt != nil || achadas[0].chaveAnt != nil {
		t.Errorf("o primeiro cadastro veio com 'de onde': %+v", achadas[0])
	}
	if achadas[0].chaveNov != "***1111" {
		t.Errorf("primeira linha, chave nova = %q", achadas[0].chaveNov)
	}
	if achadas[1].chaveAnt == nil || *achadas[1].chaveAnt != "***1111" {
		t.Errorf("a troca nao registrou de onde veio: %+v", achadas[1])
	}
	if achadas[1].chaveNov != "j***@exemplo.com" {
		t.Errorf("segunda linha, chave nova = %q", achadas[1].chaveNov)
	}
	// E o que o rastro NÃO pode ter: a chave inteira, em nenhuma das colunas.
	var vazou int
	if err := s.pool.QueryRow(ctx, `
		SELECT count(*) FROM rmt_recebedor_historico
		 WHERE chave_antiga_mascarada = '11111111111' OR chave_nova_mascarada = '11111111111'
		    OR chave_nova_mascarada = 'jogador@exemplo.com'`).Scan(&vazou); err != nil {
		t.Fatalf("conferindo vazamento: %v", err)
	}
	if vazou != 0 {
		t.Errorf("%d linha(s) do historico guardaram a chave inteira", vazou)
	}
}

// Trocar a chave zera a verificação: a chave nova não é a que foi verificada.
// Deixar a marca de pé seria dizer que conferimos o que não conferimos.
func TestTrocarChaveZeraAVerificacao(t *testing.T) {
	s, ctx := freshStore(t)
	conta := contaPix(ctx, t, s, "verif_pix")

	if err := s.SalvarChavePix(ctx, conta, "11111111111", ChavePixCPF, "11144477735"); err != nil {
		t.Fatalf("cadastro: %v", err)
	}
	if _, err := s.pool.Exec(ctx,
		`UPDATE rmt_recebedor SET verificada_em = now() WHERE account_id = $1`, conta); err != nil {
		t.Fatalf("marcando como verificada: %v", err)
	}
	if r, _ := s.LerChavePix(ctx, conta); !r.Verificada {
		t.Fatal("o preparo do teste nao marcou a verificacao")
	}

	if err := s.SalvarChavePix(ctx, conta, "22222222222", ChavePixCPF, "11144477735"); err != nil {
		t.Fatalf("troca: %v", err)
	}

	r, _ := s.LerChavePix(ctx, conta)
	if r.Verificada {
		t.Error("a chave nova continuou marcada como verificada")
	}
}

// Conta inexistente é ErrNotFound, e não uma linha órfã em rmt_recebedor.
func TestSalvarChavePixContaInexistente(t *testing.T) {
	s, ctx := freshStore(t)

	err := s.SalvarChavePix(ctx, 999999, "11111111111", ChavePixCPF, "11144477735")

	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("erro = %v, quero ErrNotFound", err)
	}
	var n int
	if err := s.pool.QueryRow(ctx, `SELECT count(*) FROM rmt_recebedor`).Scan(&n); err != nil {
		t.Fatalf("contando: %v", err)
	}
	if n != 0 {
		t.Errorf("gravou %d linha(s) para uma conta que nao existe", n)
	}
}

// TemChavePixValida é o que o lojaAbrir pergunta antes de aceitar um anúncio em
// dinheiro real — a recusa tem de ser na hora de ANUNCIAR, e não com o comprador
// já com o QR aberto.
func TestTemChavePixValida(t *testing.T) {
	s, ctx := freshStore(t)
	conta := contaPix(ctx, t, s, "tem_pix")

	if tem, err := s.TemChavePixValida(ctx, conta); err != nil || tem {
		t.Fatalf("antes de cadastrar: tem=%v err=%v", tem, err)
	}
	if err := s.SalvarChavePix(ctx, conta, "11111111111", ChavePixCPF, "11144477735"); err != nil {
		t.Fatalf("cadastro: %v", err)
	}
	if tem, err := s.TemChavePixValida(ctx, conta); err != nil || !tem {
		t.Fatalf("depois de cadastrar: tem=%v err=%v", tem, err)
	}
}

// O DOCUMENTO É OBRIGATÓRIO, e a recusa acontece ANTES de qualquer escrita.
//
// Exigir aqui é o que impede a conta de chegar ao dia do repasse sem ele. A ponte
// exige documento; sem ele o dinheiro do vendedor fica parado sem caminho de saída, e
// a pessoa descobre isso depois de já ter vendido — no pior momento possível.
func TestSalvarChavePixExigeDocumentoValido(t *testing.T) {
	s, ctx := freshStore(t)
	conta := contaPix(ctx, t, s, "doc_obrigatorio")

	ruins := []struct{ nome, doc string }{
		{"vazio", ""},
		{"curto", "1114447773"},
		{"onze iguais, que fecham a conta", "11111111111"},
		{"um digito trocado", "11144477734"},
	}
	for _, c := range ruins {
		if err := s.SalvarChavePix(ctx, conta, "11111111111", ChavePixCPF, c.doc); !errors.Is(err, ErrDocumentoInvalido) {
			t.Errorf("%s (%q): erro = %v, quero ErrDocumentoInvalido", c.nome, c.doc, err)
		}
	}

	// E NADA foi gravado por causa das tentativas recusadas: a chave também não.
	// Recusar depois de escrever deixaria a conta com chave e sem documento, que é
	// justamente o estado que esta validação existe para impedir.
	r, err := s.LerChavePix(ctx, conta)
	if err != nil {
		t.Fatal(err)
	}
	if r.TemChave {
		t.Errorf("gravou a chave apesar de recusar o documento: %+v", r)
	}
}

// O documento é guardado em DÍGITOS e volta MASCARADO, mostrando só o fim.
//
// Normalizar na escrita: "111.444.777-35" e "11144477735" são a mesma pessoa, e
// guardar os dois formatos faria uma busca por documento achar metade das linhas.
func TestDocumentoENormalizadoEVoltaMascarado(t *testing.T) {
	s, ctx := freshStore(t)
	conta := contaPix(ctx, t, s, "doc_mascara")

	if err := s.SalvarChavePix(ctx, conta, "fulano@exemplo.com", ChavePixEmail, "111.444.777-35"); err != nil {
		t.Fatalf("salvando: %v", err)
	}

	var guardado string
	if err := s.pool.QueryRow(ctx,
		`SELECT documento FROM rmt_recebedor WHERE account_id = $1`, conta).Scan(&guardado); err != nil {
		t.Fatal(err)
	}
	if guardado != "11144477735" {
		t.Errorf("guardado = %q, quero so os digitos", guardado)
	}

	r, err := s.LerChavePix(ctx, conta)
	if err != nil {
		t.Fatal(err)
	}
	if r.DocumentoMascarado != "***.***.***-35" {
		t.Errorf("mascara = %q", r.DocumentoMascarado)
	}
	// O DOCUMENTO INTEIRO NÃO ATRAVESSA ESTA CAMADA, pelo mesmo motivo da chave: o
	// site é público, e o que chega no navegador vai para o histórico e para o cache.
	if r.DocumentoMascarado == guardado {
		t.Error("a leitura devolveu o documento inteiro")
	}
}

// Quem cadastrou a chave ANTES de a coluna existir não tem documento, e a leitura
// disso não pode quebrar nem inventar máscara sobre coisa nenhuma.
func TestRecebedorSemDocumentoVoltaMascaraVazia(t *testing.T) {
	s, ctx := freshStore(t)
	conta := contaPix(ctx, t, s, "doc_antigo")

	// Escrito direto, sem passar pela validação, que é o que a migração encontrou.
	if _, err := s.pool.Exec(ctx, `
		INSERT INTO rmt_recebedor (account_id, chave, tipo, documento, updated_at)
		VALUES ($1, '11111111111', 1, NULL, now())`, conta); err != nil {
		t.Fatal(err)
	}

	r, err := s.LerChavePix(ctx, conta)
	if err != nil {
		t.Fatalf("a leitura quebrou com documento nulo: %v", err)
	}
	if !r.TemChave {
		t.Error("nao achou a chave que existe")
	}
	if r.DocumentoMascarado != "" {
		t.Errorf("mascara = %q, quero vazio: nao ha documento para mascarar", r.DocumentoMascarado)
	}
}

// O banco recusa documento fora de forma mesmo por escrita direta. O CHECK existe
// porque invariante de dado pessoal que depende de alguém lembrar não é invariante.
func TestBancoRecusaDocumentoForaDeForma(t *testing.T) {
	s, ctx := freshStore(t)
	conta := contaPix(ctx, t, s, "doc_check")

	for _, ruim := range []string{"111.444.777-35", "1114447773", "abcdefghijk"} {
		if _, err := s.pool.Exec(ctx, `
			INSERT INTO rmt_recebedor (account_id, chave, tipo, documento, updated_at)
			VALUES ($1, '11111111111', 1, $2, now())
			ON CONFLICT (account_id) DO UPDATE SET documento = EXCLUDED.documento`,
			conta, ruim); err == nil {
			t.Errorf("o banco aceitou o documento %q", ruim)
		}
	}
}
