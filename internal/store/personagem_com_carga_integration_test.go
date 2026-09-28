//go:build integration

// Testes de integração do par personagem+carga: as duas metades vão ao banco na
// MESMA transação, ou nenhuma vai.
//
//	W2PP_TEST_DSN=postgres://postgres:dev@localhost:5432/postgres go test -tags=integration ./internal/store/
package store

import (
	"context"
	"errors"
	"testing"

	"github.com/jeanluca/w2pp-openwyd/internal/domain"
)

// contaComPersonagem monta a conta, o personagem e a carga que os testes daqui
// usam, e devolve o id da conta.
func contaComPersonagem(ctx context.Context, t *testing.T, s *Store, nome string, ouroPersonagem, ouroCarga int32) int64 {
	t.Helper()
	conta := contaPix(ctx, t, s, nome)
	if _, err := s.pool.Exec(ctx, `
		INSERT INTO character (account_id, slot, name, class, level, coin)
		VALUES ($1, 0, $2, 1, 50, $3)`, conta, nome+"_p", ouroPersonagem); err != nil {
		t.Fatalf("criando o personagem: %v", err)
	}
	if _, err := s.pool.Exec(ctx,
		`UPDATE account SET cargo_coin = $2 WHERE id = $1`, conta, ouroCarga); err != nil {
		t.Fatalf("pondo ouro na carga: %v", err)
	}
	return conta
}

// leOuro devolve o ouro do personagem e o da carga como estão NO BANCO.
func leOuro(ctx context.Context, t *testing.T, s *Store, conta int64) (personagem, carga int32) {
	t.Helper()
	if err := s.pool.QueryRow(ctx, `
		SELECT c.coin, a.cargo_coin
		  FROM character c JOIN account a ON a.id = c.account_id
		 WHERE c.account_id = $1 AND c.slot = 0`, conta).Scan(&personagem, &carga); err != nil {
		t.Fatalf("lendo o ouro: %v", err)
	}
	return personagem, carga
}

// contaItens conta as cópias de um item nos dois lados, que é a pergunta que o
// dupe responde errado.
func contaItens(ctx context.Context, t *testing.T, s *Store, conta int64, indice int16) int {
	t.Helper()
	var n int
	if err := s.pool.QueryRow(ctx, `
		SELECT count(*) FROM item
		 WHERE item_index = $2
		   AND (account_id = $1 OR character_id IN (SELECT id FROM character WHERE account_id = $1))`,
		conta, indice).Scan(&n); err != nil {
		t.Fatalf("contando o item: %v", err)
	}
	return n
}

// personagemDoPar monta o domain.Character mínimo que o save exige.
func personagemDoPar(nome string, ouro int32, carry []domain.Item) domain.Character {
	return domain.Character{Slot: 0, Name: nome, Level: 50, Coin: ouro, Carry: carry}
}

// TestParGravaAsDuasMetadesJuntas: o saque tira da carga e põe na mochila, e as
// duas metades só fazem sentido juntas. Depois de recarregar, a soma tem de ser a
// mesma e o item tem de existir UMA vez.
func TestParGravaAsDuasMetadesJuntas(t *testing.T) {
	s, ctx := freshStore(t)
	const item = int16(1030)

	t.Run("saque: a soma fecha e o item fica uma vez so", func(t *testing.T) {
		conta := contaComPersonagem(ctx, t, s, "par_saque", 0, 1000)
		// O item começa na carga.
		if err := s.SaveCargo(ctx, conta, 1000, []domain.Item{{Slot: 0, Index: item}}); err != nil {
			t.Fatal(err)
		}
		// O saque como o mundo o vê: ouro e item saíram da carga e entraram na
		// mochila, tudo no mesmo instante.
		if err := s.SalvarPersonagemComCarga(ctx, conta,
			personagemDoPar("par_saque_p", 1000, []domain.Item{{Slot: 0, Index: item}}),
			0, nil, nil, nil, 0, 0, false); err != nil {
			t.Fatalf("SalvarPersonagemComCarga: %v", err)
		}
		p, c := leOuro(ctx, t, s, conta)
		if p+c != 1000 {
			t.Errorf("soma depois do saque = %d (personagem %d + carga %d), queria 1000", p+c, p, c)
		}
		if p != 1000 || c != 0 {
			t.Errorf("personagem = %d, carga = %d; queria 1000 e 0", p, c)
		}
		if n := contaItens(ctx, t, s, conta, item); n != 1 {
			t.Errorf("o item existe %d vezes, queria 1", n)
		}
	})

	t.Run("deposito: a soma nunca cresce", func(t *testing.T) {
		conta := contaComPersonagem(ctx, t, s, "par_deposito", 1000, 0)
		if err := s.SalvarPersonagemComCarga(ctx, conta,
			personagemDoPar("par_deposito_p", 0, nil),
			1000, []domain.Item{{Slot: 0, Index: item}}, nil, nil, 0, 0, false); err != nil {
			t.Fatalf("SalvarPersonagemComCarga: %v", err)
		}
		p, c := leOuro(ctx, t, s, conta)
		if p+c != 1000 {
			t.Errorf("soma depois do depósito = %d (personagem %d + carga %d), queria 1000", p+c, p, c)
		}
		if n := contaItens(ctx, t, s, conta, item); n != 1 {
			t.Errorf("o item existe %d vezes, queria 1", n)
		}
	})
}

// TestParNaoDeixaMetadeGravada é o teste que prova o conserto, e ele olha as DUAS
// direções: se qualquer uma das metades falhar, NENHUMA fica no banco. Era
// exatamente a metade gravada sozinha que duplicava.
func TestParNaoDeixaMetadeGravada(t *testing.T) {
	s, ctx := freshStore(t)

	t.Run("personagem que nao existe nao grava a carga", func(t *testing.T) {
		conta := contaComPersonagem(ctx, t, s, "par_meia_a", 500, 500)
		// Slot 3 não existe: a metade do personagem falha primeiro.
		ch := personagemDoPar("fantasma", 0, nil)
		ch.Slot = 3
		err := s.SalvarPersonagemComCarga(ctx, conta, ch, 9999, nil, nil, nil, 0, 0, false)
		if !errors.Is(err, ErrNotFound) {
			t.Fatalf("erro = %v, queria ErrNotFound", err)
		}
		p, c := leOuro(ctx, t, s, conta)
		if p != 500 || c != 500 {
			t.Errorf("o banco mudou depois de um par que falhou: personagem %d, carga %d", p, c)
		}
	})

	t.Run("carga que falha nao grava o personagem", func(t *testing.T) {
		conta := contaComPersonagem(ctx, t, s, "par_meia_b", 500, 500)
		// A carga é a SEGUNDA metade, então só um erro dentro dela prova que a
		// primeira volta atrás. O gatilho recusa um item marcado, que é a maneira
		// de fazer a segunda metade falhar sem inventar uma conta inexistente.
		if _, err := s.pool.Exec(ctx, `
			CREATE FUNCTION recusa_item_marcado() RETURNS trigger AS $$
			BEGIN
				IF NEW.item_index = 31337 THEN
					RAISE EXCEPTION 'item recusado de proposito pelo teste';
				END IF;
				RETURN NEW;
			END $$ LANGUAGE plpgsql;
			CREATE TRIGGER recusa_item_marcado BEFORE INSERT ON item
				FOR EACH ROW EXECUTE FUNCTION recusa_item_marcado()`); err != nil {
			t.Fatalf("montando o gatilho: %v", err)
		}
		t.Cleanup(func() {
			_, _ = s.pool.Exec(ctx, `DROP TRIGGER IF EXISTS recusa_item_marcado ON item`)
			_, _ = s.pool.Exec(ctx, `DROP FUNCTION IF EXISTS recusa_item_marcado()`)
		})

		err := s.SalvarPersonagemComCarga(ctx, conta,
			personagemDoPar("par_meia_b_p", 0, nil),
			1000, []domain.Item{{Slot: 0, Index: 31337}}, nil, nil, 0, 0, false)
		if err == nil {
			t.Fatal("a gravação passou, e o gatilho tinha de ter derrubado a metade da carga")
		}
		p, c := leOuro(ctx, t, s, conta)
		if p != 500 || c != 500 {
			t.Errorf("a metade do personagem ficou gravada: personagem %d, carga %d; queria 500 e 500", p, c)
		}
	})
}
