//go:build integration

// Integration tests for item serials, against a real PostgreSQL.
//
//	W2PP_TEST_DSN=postgres://postgres@localhost:5432/postgres go test -tags=integration ./internal/store/
//
// Two things earn a database here. The reservation has to be atomic — two
// servers asking at once must get disjoint blocks, or two items end up with one
// identity and the whole feature turns into a false accusation. And the
// duplicate query has to ignore serial 0, which is not a number but the absence
// of one: every item that predates 0033 carries it.
package store

import (
	"context"
	"sync"
	"testing"

	"github.com/jeanluca/w2pp-openwyd/internal/domain"
)

func storeSerial(t *testing.T) *Store {
	t.Helper()
	pool := testPool(t)
	if err := Migrate(context.Background(), pool); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	s := New(pool)
	limpar := func() {
		ctx := context.Background()
		_, _ = s.pool.Exec(ctx, `DELETE FROM item`)
		_, _ = s.pool.Exec(ctx, `UPDATE item_serial_seq SET proximo = 1`)
	}
	limpar()
	t.Cleanup(limpar)
	return s
}

func TestReservarSerialNaoRepeteBloco(t *testing.T) {
	ctx := context.Background()
	s := storeSerial(t)

	a, err := s.ReserveSerials(ctx, 100)
	if err != nil {
		t.Fatalf("primeira reserva: %v", err)
	}
	b, err := s.ReserveSerials(ctx, 100)
	if err != nil {
		t.Fatalf("segunda reserva: %v", err)
	}
	if a != 1 {
		t.Errorf("primeiro bloco começa em %d, want 1", a)
	}
	if b != a+100 {
		t.Errorf("segundo bloco começa em %d, want %d; os blocos se encavalaram", b, a+100)
	}
}

// Dois servidores subindo ao mesmo tempo é o caso que a linha de contador
// existe para resolver. Nenhum número pode sair duas vezes.
func TestReservarSerialAguentaPedidosSimultaneos(t *testing.T) {
	ctx := context.Background()
	s := storeSerial(t)

	const pedidos, bloco = 12, 50
	inicios := make([]int64, pedidos)
	var wg sync.WaitGroup
	for i := range inicios {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			primeiro, err := s.ReserveSerials(ctx, bloco)
			if err != nil {
				t.Errorf("reserva %d: %v", i, err)
				return
			}
			inicios[i] = primeiro
		}(i)
	}
	wg.Wait()

	usado := map[int64]int{}
	for _, ini := range inicios {
		for n := ini; n < ini+bloco; n++ {
			usado[n]++
			if usado[n] > 1 {
				t.Fatalf("o número %d saiu em dois blocos diferentes", n)
			}
		}
	}
	if len(usado) != pedidos*bloco {
		t.Errorf("números distintos = %d, want %d", len(usado), pedidos*bloco)
	}
}

func TestReservarSerialRecusaBlocoAbsurdo(t *testing.T) {
	s := storeSerial(t)
	for _, n := range []int64{0, -1, 100_000_000} {
		if _, err := s.ReserveSerials(context.Background(), n); err == nil {
			t.Errorf("aceitou reservar %d números", n)
		}
	}
}

// O serial tem de sobreviver ao caminho inteiro: gravado no save, lido de volta
// no login. Sem isso o item ganha número novo a cada sessão e nada emparelha.
func TestSerialSobreviveAoSaveELoad(t *testing.T) {
	ctx := context.Background()
	s := storeSerial(t)

	acc := domain.Account{
		Name: "serial_ida_volta", PassHash: "x",
		Cargo: []domain.Item{{Slot: 0, Index: 1100, Serial: 91827}},
		Characters: []domain.Character{{
			Slot: 0, Name: "SerialHero",
			Carry: []domain.Item{{Slot: 1, Index: 1415, Serial: 5150}},
			Equip: []domain.Item{{Slot: 2, Index: 2200, Serial: 777}},
		}},
	}
	id, err := s.SaveAccount(ctx, acc)
	if err != nil {
		t.Fatalf("SaveAccount: %v", err)
	}
	t.Cleanup(func() { _, _ = s.pool.Exec(ctx, `DELETE FROM account WHERE id = $1`, id) })

	seriais := map[int16]int64{}
	rows, err := s.pool.Query(ctx, `SELECT item_index, serial FROM item WHERE account_id = $1 OR character_id IN (SELECT id FROM character WHERE account_id = $1)`, id)
	if err != nil {
		t.Fatalf("ler itens: %v", err)
	}
	defer rows.Close()
	for rows.Next() {
		var idx int16
		var serial int64
		if err := rows.Scan(&idx, &serial); err != nil {
			t.Fatalf("scan: %v", err)
		}
		seriais[idx] = serial
	}
	quer := map[int16]int64{1100: 91827, 1415: 5150, 2200: 777}
	for idx, s := range quer {
		if seriais[idx] != s {
			t.Errorf("item %d gravou serial %d, want %d", idx, seriais[idx], s)
		}
	}
}

func TestListaRepetidosAchaACopiaEIgnoraOsSemMarca(t *testing.T) {
	ctx := context.Background()
	s := storeSerial(t)
	id := conta(t, s, "serial_copia")
	pA := personagem(t, s, id, "SerialDono")
	pB := personagem(t, s, id, "SerialOutro")

	// A cópia: o mesmo número em dois personagens.
	itemSerial(t, s, "char_carry", nil, &pA, 0, 1415, 4242)
	itemSerial(t, s, "char_equip", nil, &pB, 1, 1415, 4242)
	// Um item marcado sozinho, que não é cópia de nada.
	itemSerial(t, s, "char_carry", nil, &pA, 2, 1100, 99)
	// E dois sem marca. Serial 0 não é o número zero, é a ausência dele: item
	// que já existia antes de 0033. Tratá-los como iguais acusaria o mundo
	// inteiro de uma vez.
	itemSerial(t, s, "account_cargo", &id, nil, 0, 30, 0)
	itemSerial(t, s, "account_cargo", &id, nil, 1, 30, 0)

	dups, err := s.ListDupes(ctx, 50)
	if err != nil {
		t.Fatalf("ListDupes: %v", err)
	}
	if len(dups) != 2 {
		t.Fatalf("linhas = %d, want 2 (as duas cópias e nada mais): %+v", len(dups), dups)
	}
	for _, d := range dups {
		if d.Serial != 4242 || d.Index != 1415 {
			t.Errorf("linha inesperada: %+v", d)
		}
		if d.Account != "serial_copia" {
			t.Errorf("conta = %q, want serial_copia", d.Account)
		}
	}
	nomes := map[string]bool{dups[0].Character: true, dups[1].Character: true}
	if !nomes["SerialDono"] || !nomes["SerialOutro"] {
		t.Errorf("personagens = %v, want os dois donos", nomes)
	}
}

// Item no baú é da conta e não tem personagem. A tela precisa mostrar a conta
// mesmo assim, senão a cópia aparece sem dono nenhum.
func TestListaRepetidosMostraAContaDoBau(t *testing.T) {
	ctx := context.Background()
	s := storeSerial(t)
	id := conta(t, s, "serial_bau")
	p := personagem(t, s, id, "SerialBau")

	itemSerial(t, s, "char_carry", nil, &p, 0, 1415, 313)
	itemSerial(t, s, "account_cargo", &id, nil, 0, 1415, 313)

	dups, err := s.ListDupes(ctx, 50)
	if err != nil {
		t.Fatalf("ListDupes: %v", err)
	}
	if len(dups) != 2 {
		t.Fatalf("linhas = %d, want 2", len(dups))
	}
	var achouBau bool
	for _, d := range dups {
		if d.Onde == "account_cargo" {
			achouBau = true
			if d.Character != "" {
				t.Errorf("item de baú veio com personagem %q", d.Character)
			}
			if d.Account != "serial_bau" {
				t.Errorf("item de baú veio sem conta: %+v", d)
			}
		}
	}
	if !achouBau {
		t.Error("a cópia do baú não apareceu")
	}
}

// Lista vazia não quer dizer nada até se saber se existe algo marcado. Logo
// depois da migração tudo é zero, e isso não é o mesmo que "sem cópias".
func TestContarMarcadosSeparaOsDoisEstados(t *testing.T) {
	ctx := context.Background()
	s := storeSerial(t)
	id := conta(t, s, "serial_conta_marcados")
	p := personagem(t, s, id, "SerialConta")

	itemSerial(t, s, "char_carry", nil, &p, 0, 1415, 10)
	itemSerial(t, s, "char_carry", nil, &p, 1, 1415, 11)
	itemSerial(t, s, "char_carry", nil, &p, 2, 30, 0)

	marcados, semMarca, err := s.CountMarked(ctx)
	if err != nil {
		t.Fatalf("CountMarked: %v", err)
	}
	if marcados != 2 || semMarca != 1 {
		t.Errorf("marcados/sem marca = %d/%d, want 2/1", marcados, semMarca)
	}
}

// itemSerial seeds one item row carrying a serial.
func itemSerial(t *testing.T, s *Store, kind string, contaID, charID *int64, slot, index int16, serial int64) {
	t.Helper()
	_, err := s.pool.Exec(context.Background(),
		`INSERT INTO item (owner_kind, account_id, character_id, slot, item_index, serial)
		 VALUES ($1, $2, $3, $4, $5, $6)`,
		kind, contaID, charID, slot, index, serial)
	if err != nil {
		t.Fatalf("seed item %d serial %d: %v", index, serial, err)
	}
}
