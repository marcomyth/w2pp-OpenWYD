//go:build integration

// O ajuste de valor de um repasse recusado, feito pela staff.
//
//	W2PP_TEST_DSN=postgres://postgres:dev@localhost:5432/postgres go test -tags=integration ./internal/store/
package store

import (
	"context"
	"errors"
	"testing"
)

// repasseRecusadoParaAjuste monta o cenário REAL que criou esta função: venda paga,
// repasse aberto pelo valor cheio, e a processadora recusando o saque.
func repasseRecusadoParaAjuste(ctx context.Context, t *testing.T, s *Store, sufixo string) (repasseID int64) {
	t.Helper()
	v := montaVenda(ctx, t, s, sufixo)
	if _, _, err := s.ConfirmarCobrancaRMT(ctx, v.ref, dentroDoPrazo(), HoraDaProcessadora,
		precoEmCentavos, taxaZero()); err != nil {
		t.Fatal(err)
	}
	if err := s.pool.QueryRow(ctx,
		`SELECT id FROM rmt_repasse WHERE vendedor_conta = $1`, v.vendedor).Scan(&repasseID); err != nil {
		t.Fatal(err)
	}
	http := int32(403)
	if err := s.MarcarRepasseRecusado(ctx, repasseID, &http, "",
		"SyncPay recusou 403: This endpoint does not accept this token type."); err != nil {
		t.Fatal(err)
	}
	return repasseID
}

func valorEAjuste(ctx context.Context, t *testing.T, s *Store, id int64) (valor int64, de *int64, nota, por *string) {
	t.Helper()
	if err := s.pool.QueryRow(ctx, `
		SELECT valor_centavos, ajuste_de_centavos, ajuste_nota, ajustado_por
		  FROM rmt_repasse WHERE id = $1`, id).Scan(&valor, &de, &nota, &por); err != nil {
		t.Fatal(err)
	}
	return valor, de, nota, por
}

// O AJUSTE GRAVA O VALOR NOVO E GUARDA O ANTIGO NA PRÓPRIA LINHA.
//
// O antigo na linha, e não só na auditoria, porque a auditoria é escrita DEPOIS da
// mudança neste sistema — o painel admite isso no próprio código. Para um número
// sobrescrito não basta: sem o antigo em algum lugar que a mesma transação garanta,
// ninguém responde "quanto era" no dia em que a auditoria falhar.
func TestAjusteGravaONovoEGuardaOAntigo(t *testing.T) {
	s, ctx := freshStore(t)
	id := repasseRecusadoParaAjuste(ctx, t, s, "ajuste_ok")

	antigo, err := s.AjustarValorDoRepasse(ctx, id, 20,
		AtorDoRepasse{Nome: "hanna"}, "ordem da Hanna, liquido medido, net_amount 0.2")
	if err != nil {
		t.Fatal(err)
	}
	if antigo != precoEmCentavos {
		t.Errorf("devolveu antigo = %d, queria %d", antigo, precoEmCentavos)
	}

	valor, de, nota, por := valorEAjuste(ctx, t, s, id)
	if valor != 20 {
		t.Errorf("valor gravado = %d, queria 20", valor)
	}
	if de == nil || *de != precoEmCentavos {
		t.Errorf("ajuste_de_centavos = %v, queria %d", de, precoEmCentavos)
	}
	if nota == nil || *nota == "" {
		t.Error("a nota nao foi gravada")
	}
	if por == nil || *por != "hanna" {
		t.Errorf("ajustado_por = %v, queria hanna", por)
	}
}

// E A LINHA CONTINUA RECUSADA, que é a metade que vale mais.
//
// Se o ajuste devolvesse à fila, a varredura mandaria o valor novo imediatamente — e no
// caso real a recusa era de CREDENCIAL, então ela tomaria o mesmo 403 de dois em dois
// minutos, queimando uma referência por rodada.
func TestAjusteNaoDevolveParaAFilaDePagar(t *testing.T) {
	s, ctx := freshStore(t)
	id := repasseRecusadoParaAjuste(ctx, t, s, "ajuste_fila")

	if _, err := s.AjustarValorDoRepasse(ctx, id, 20, AtorDoRepasse{Nome: "hanna"}, "nota"); err != nil {
		t.Fatal(err)
	}

	fila, err := s.RepassesAPagar(ctx, 50)
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range fila {
		if r.ID == id {
			t.Fatalf("o repasse ajustado voltou para a fila de pagar, por %d centavos", r.ValorCentavos)
		}
	}
}

// ACIMA DO VALOR DA COBRANÇA NÃO PASSA: seria pagar mais do que entrou, que é dinheiro
// saindo do nada. E o teto é lido da cobrança aqui dentro, não recebido de fora — um
// limite que quem chama informa não é limite.
func TestAjusteAcimaDaCobrancaNaoPassa(t *testing.T) {
	s, ctx := freshStore(t)
	id := repasseRecusadoParaAjuste(ctx, t, s, "ajuste_teto")

	_, err := s.AjustarValorDoRepasse(ctx, id, precoEmCentavos+1, AtorDoRepasse{Nome: "hanna"}, "nota")
	if !errors.Is(err, ErrAjusteInvalido) {
		t.Fatalf("erro = %v, queria ErrAjusteInvalido", err)
	}
	if valor, _, _, _ := valorEAjuste(ctx, t, s, id); valor != precoEmCentavos {
		t.Errorf("o valor mudou para %d mesmo com o ajuste recusado", valor)
	}
}

// ZERO E NEGATIVO NÃO SÃO PAGAMENTO. Zero manda a ponte transferir nada; negativo seria
// um saque ao contrário.
func TestAjusteZeroOuNegativoNaoPassa(t *testing.T) {
	s, ctx := freshStore(t)
	id := repasseRecusadoParaAjuste(ctx, t, s, "ajuste_zero")

	for _, v := range []int64{0, -1} {
		if _, err := s.AjustarValorDoRepasse(ctx, id, v, AtorDoRepasse{Nome: "hanna"}, "nota"); !errors.Is(err, ErrAjusteInvalido) {
			t.Errorf("valor %d: erro = %v, queria ErrAjusteInvalido", v, err)
		}
	}
}

// SEM NOTA NÃO AJUSTA, e a regra mora no banco e não só na tela: outra tela amanhã
// chamaria esta função sem saber dela.
func TestAjusteSemNotaNaoPassa(t *testing.T) {
	s, ctx := freshStore(t)
	id := repasseRecusadoParaAjuste(ctx, t, s, "ajuste_nota")

	if _, err := s.AjustarValorDoRepasse(ctx, id, 20, AtorDoRepasse{Nome: "hanna"}, ""); !errors.Is(err, ErrAjusteSemNota) {
		t.Fatalf("erro = %v, queria ErrAjusteSemNota", err)
	}
	if valor, _, _, _ := valorEAjuste(ctx, t, s, id); valor != precoEmCentavos {
		t.Errorf("o valor mudou para %d sem nota", valor)
	}
}

// SÓ MEXE NO RECUSADO. Num repasse PENDENTE a fila de pagar pode lê-lo entre a leitura e
// a escrita, e a ponte receberia uma ordem com o valor velho.
func TestAjusteSoMexeNoRecusado(t *testing.T) {
	s, ctx := freshStore(t)
	v := montaVenda(ctx, t, s, "ajuste_pendente")
	if _, _, err := s.ConfirmarCobrancaRMT(ctx, v.ref, dentroDoPrazo(), HoraDaProcessadora,
		precoEmCentavos, taxaZero()); err != nil {
		t.Fatal(err)
	}
	var id int64
	if err := s.pool.QueryRow(ctx,
		`SELECT id FROM rmt_repasse WHERE vendedor_conta = $1`, v.vendedor).Scan(&id); err != nil {
		t.Fatal(err)
	}

	_, err := s.AjustarValorDoRepasse(ctx, id, 20, AtorDoRepasse{Nome: "hanna"}, "nota")
	if !errors.Is(err, ErrRepasseInexistente) {
		t.Fatalf("erro = %v, queria ErrRepasseInexistente num repasse PENDENTE", err)
	}
	if valor, _, _, _ := valorEAjuste(ctx, t, s, id); valor != precoEmCentavos {
		t.Errorf("mexeu num repasse pendente: valor = %d", valor)
	}
}

// DOIS AJUSTES SEGUIDOS NÃO APAGAM DE ONDE A LINHA PARTIU.
//
// O ajuste_de_centavos guarda o PRIMEIRO valor, e não o da vez anterior: quem for
// conferir a linha depois quer saber de quanto ela nasceu, e um campo que se sobrescreve
// a cada correção esconde justamente o número que originou a dúvida.
func TestDoisAjustesGuardamOPrimeiroValor(t *testing.T) {
	s, ctx := freshStore(t)
	id := repasseRecusadoParaAjuste(ctx, t, s, "ajuste_dois")

	if _, err := s.AjustarValorDoRepasse(ctx, id, 50, AtorDoRepasse{Nome: "hanna"}, "primeira"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.AjustarValorDoRepasse(ctx, id, 20, AtorDoRepasse{Nome: "hanna"}, "segunda"); err != nil {
		t.Fatal(err)
	}

	valor, de, nota, _ := valorEAjuste(ctx, t, s, id)
	if valor != 20 {
		t.Errorf("valor = %d, queria 20", valor)
	}
	if de == nil || *de != precoEmCentavos {
		t.Errorf("ajuste_de_centavos = %v, queria o PRIMEIRO valor %d", de, precoEmCentavos)
	}
	if nota == nil || *nota != "segunda" {
		t.Errorf("nota = %v, queria a mais recente", nota)
	}
}
