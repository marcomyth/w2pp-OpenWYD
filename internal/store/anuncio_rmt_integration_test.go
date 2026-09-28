//go:build integration

// Testes de integração da abertura de anúncio em dinheiro real.
//
//	W2PP_TEST_DSN=postgres://postgres:dev@localhost:5432/postgres go test -tags=integration ./internal/store/
package store

import (
	"context"
	"errors"
	"testing"
)

func doisItens() []ItemAnunciado {
	return []ItemAnunciado{
		{CargoSlot: 0, ItemIndex: 1030, Eff1: 3, EffV1: 9, PrecoCentavos: 5000},
		{CargoSlot: 4, ItemIndex: 1040, Eff2: 7, EffV2: 11, PrecoCentavos: 12000},
	}
}

// Sem chave Pix não nasce anúncio nenhum, e o que importa é o "nenhum": a
// conferência está DENTRO da transação, então ela não pode deixar metade.
func TestAbrirAnunciosRecusaSemChavePix(t *testing.T) {
	s, ctx := freshStore(t)
	conta := contaPix(ctx, t, s, "sem_chave")

	_, err := s.AbrirAnunciosRMT(ctx, conta, "Vendedor", doisItens())

	if !errors.Is(err, ErrSemChavePix) {
		t.Fatalf("erro = %v, quero ErrSemChavePix", err)
	}
	if n := anunciosDaConta(ctx, t, s, conta); n != 0 {
		t.Errorf("nasceram %d anuncio(s) para quem nao tem onde receber", n)
	}
}

// Com chave, os dois nascem, ativos, com a fotografia e o preço em centavos — e
// os ids voltam NA ORDEM, que é como quem chamou liga cada um ao slot do baú.
func TestAbrirAnunciosGravaAFotografia(t *testing.T) {
	s, ctx := freshStore(t)
	conta := contaPix(ctx, t, s, "com_chave")
	if err := s.SalvarChavePix(ctx, conta, "11111111111", ChavePixCPF, "11144477735"); err != nil {
		t.Fatal(err)
	}

	ids, err := s.AbrirAnunciosRMT(ctx, conta, "Vendedor", doisItens())
	if err != nil {
		t.Fatalf("abrindo: %v", err)
	}

	if len(ids) != 2 || ids[0] == 0 || ids[1] == 0 {
		t.Fatalf("ids = %v", ids)
	}
	var slot, indice, eff1, effv1 int16
	var preco int64
	var status int16
	if err := s.pool.QueryRow(ctx, `
		SELECT cargo_slot, item_index, eff1, effv1, preco_centavos, status
		  FROM rmt_anuncio WHERE id = $1`, ids[0]).
		Scan(&slot, &indice, &eff1, &effv1, &preco, &status); err != nil {
		t.Fatal(err)
	}
	if slot != 0 || indice != 1030 || eff1 != 3 || effv1 != 9 || preco != 5000 {
		t.Errorf("primeiro anuncio = slot %d item %d eff %d/%d preco %d", slot, indice, eff1, effv1, preco)
	}
	if status != anuncioAtivo {
		t.Errorf("status = %d, quero ATIVO (%d)", status, anuncioAtivo)
	}
	// A ordem: o segundo id tem de ser o do slot 4, senão quem marcar o baú marca
	// o slot errado — e o item errado fica preso enquanto o certo fica solto.
	if err := s.pool.QueryRow(ctx,
		`SELECT cargo_slot FROM rmt_anuncio WHERE id = $1`, ids[1]).Scan(&slot); err != nil {
		t.Fatal(err)
	}
	if slot != 4 {
		t.Errorf("o segundo id aponta o slot %d, e o segundo item era o do slot 4", slot)
	}
}

// TODOS OU NENHUM. Um preço inválido no segundo item derruba o primeiro junto:
// meia barraca seria uma vitrine que promete o que não pode cumprir, e o vendedor
// não teria como saber quais metades.
func TestAbrirAnunciosENenhumSeUmFalhar(t *testing.T) {
	s, ctx := freshStore(t)
	conta := contaPix(ctx, t, s, "meio_caminho")
	if err := s.SalvarChavePix(ctx, conta, "11111111111", ChavePixCPF, "11144477735"); err != nil {
		t.Fatal(err)
	}
	itens := doisItens()
	itens[1].PrecoCentavos = 0 // o CHECK da 0105 exige > 0

	if _, err := s.AbrirAnunciosRMT(ctx, conta, "Vendedor", itens); err == nil {
		t.Fatal("aceitou preco zero")
	}
	if n := anunciosDaConta(ctx, t, s, conta); n != 0 {
		t.Errorf("sobraram %d anuncio(s) da transacao que falhou", n)
	}
}

// Cancelar tira da vitrine — e só o que ainda está ATIVO.
//
// O segundo caso é o que importa: um anúncio já VENDIDO não pode ser fechado por
// um caminho de desistência, porque isso apagaria a prova de uma venda que
// aconteceu. Quem garante é o WHERE, e não a ordem em que as coisas acontecem.
func TestCancelarSoFechaOQueEstaAtivo(t *testing.T) {
	s, ctx := freshStore(t)
	conta := contaPix(ctx, t, s, "cancelar")
	if err := s.SalvarChavePix(ctx, conta, "11111111111", ChavePixCPF, "11144477735"); err != nil {
		t.Fatal(err)
	}
	ids, err := s.AbrirAnunciosRMT(ctx, conta, "Vendedor", doisItens())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.pool.Exec(ctx,
		`UPDATE rmt_anuncio SET status = $2 WHERE id = $1`, ids[1], anuncioVendido); err != nil {
		t.Fatal(err)
	}

	if err := s.CancelarAnunciosRMT(ctx, ids); err != nil {
		t.Fatalf("cancelando: %v", err)
	}

	var a, b int16
	if err := s.pool.QueryRow(ctx, `SELECT status FROM rmt_anuncio WHERE id = $1`, ids[0]).Scan(&a); err != nil {
		t.Fatal(err)
	}
	if err := s.pool.QueryRow(ctx, `SELECT status FROM rmt_anuncio WHERE id = $1`, ids[1]).Scan(&b); err != nil {
		t.Fatal(err)
	}
	if a != anuncioCancelado {
		t.Errorf("o ativo ficou no status %d, quero CANCELADO (%d)", a, anuncioCancelado)
	}
	if b != anuncioVendido {
		t.Errorf("o vendido virou %d; cancelar apagou a prova de uma venda", b)
	}
}

// Dois anúncios ativos sobre o MESMO slot não podem existir: a marca do escrow
// guarda UM id, então o segundo nasceria órfão. A invariante é do banco (0105) e
// este teste guarda que ela continua de pé.
func TestDoisAtivosNoMesmoSlotNaoEntram(t *testing.T) {
	s, ctx := freshStore(t)
	conta := contaPix(ctx, t, s, "slot_dobrado")
	if err := s.SalvarChavePix(ctx, conta, "11111111111", ChavePixCPF, "11144477735"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.AbrirAnunciosRMT(ctx, conta, "Vendedor", doisItens()[:1]); err != nil {
		t.Fatal(err)
	}

	if _, err := s.AbrirAnunciosRMT(ctx, conta, "Vendedor", doisItens()[:1]); err == nil {
		t.Fatal("o banco aceitou dois anuncios ativos no mesmo slot")
	}
	if n := anunciosDaConta(ctx, t, s, conta); n != 1 {
		t.Errorf("anuncios = %d, quero 1", n)
	}
}

func anunciosDaConta(ctx context.Context, t *testing.T, s *Store, conta int64) int {
	t.Helper()
	var n int
	if err := s.pool.QueryRow(ctx,
		`SELECT count(*) FROM rmt_anuncio WHERE vendedor_conta = $1`, conta).Scan(&n); err != nil {
		t.Fatalf("contando anuncios: %v", err)
	}
	return n
}
