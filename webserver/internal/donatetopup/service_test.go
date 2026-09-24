package donatetopup

import (
	"context"
	"testing"

	"github.com/jeanluca/w2pp-openwyd/internal/domain"
	"github.com/jeanluca/w2pp-openwyd/internal/store"
)

// fakeStore is an in-memory Store for exercising validation and outcome mapping
// without a database.
type fakeStore struct {
	profiles  map[int64]profile     // account_id -> saved profile
	orders    map[string]*fakeOrder // external_reference -> order
	owners    map[string]int64      // external_reference -> owning account
	balances  map[int64]int32       // account_id -> donate balance
	accounts  map[int64]bool        // existing accounts (for FK checks)
	saveErr   error                 // forces SavePayerProfile to return this
	createErr error                 // forces CreateTopupOrder to return this

	// Pacotes: o que a tabela do servidor responderia, e quem é staff.
	pacotes      map[string]store.PacoteDoacao
	staff        map[int64]bool
	pacotePedido string
	staffVisto   bool
}

// ContaEhStaff finge a leitura do cargo. Falha fechado, como a de verdade: conta que
// não está no mapa não é staff.
func (f *fakeStore) ContaEhStaff(_ context.Context, accountID int64) (bool, error) {
	if !f.accounts[accountID] {
		return false, store.ErrNotFound
	}
	return f.staff[accountID], nil
}

// ConferirPacote repete as regras da de verdade sobre o mapa de pacotes. Repetir aqui é
// o preço de o teste do serviço não precisar de banco; quem prova as regras contra o
// banco é o teste de integração do store.
func (f *fakeStore) ConferirPacote(_ context.Context, id string, ehStaff bool,
	creditsPedidos int32, centavosPedidos int64,
) (store.PacoteDoacao, error) {
	f.pacotePedido, f.staffVisto = id, ehStaff
	p, ok := f.pacotes[id]
	if !ok {
		return store.PacoteDoacao{}, store.ErrPacoteDesconhecido
	}
	if !p.Ativo {
		return store.PacoteDoacao{}, store.ErrPacoteDesligado
	}
	if p.SoStaff && !ehStaff {
		return store.PacoteDoacao{}, store.ErrPacoteSoStaff
	}
	if creditsPedidos != p.Credits || centavosPedidos != p.AmountCents {
		return store.PacoteDoacao{}, store.ErrPacoteDivergente
	}
	return p, nil
}

type profile struct{ name, cpf string }

type fakeOrder struct {
	accountID int64
	credits   int32
	status    int16
}

func (f *fakeStore) SavePayerProfile(_ context.Context, accountID int64, name, cpf string) error {
	if f.saveErr != nil {
		return f.saveErr
	}
	if !f.accounts[accountID] {
		return store.ErrNotFound
	}
	f.profiles[accountID] = profile{name, cpf}
	return nil
}

func (f *fakeStore) GetPayerProfile(_ context.Context, accountID int64) (string, string, bool, error) {
	p, ok := f.profiles[accountID]
	if !ok {
		return "", "", false, nil
	}
	return p.name, p.cpf, true, nil
}

func (f *fakeStore) CreateTopupOrder(_ context.Context, o domain.TopupOrder) (int64, error) {
	if f.createErr != nil {
		return 0, f.createErr
	}
	if _, dup := f.orders[o.ExternalReference]; dup {
		return 0, store.ErrDuplicateExternalRef
	}
	if !f.accounts[o.AccountID] {
		return 0, store.ErrNotFound
	}
	f.orders[o.ExternalReference] = &fakeOrder{accountID: o.AccountID, credits: o.Credits, status: store.TopupStatusPending}
	f.owners[o.ExternalReference] = o.AccountID
	return int64(len(f.orders)), nil
}

func (f *fakeStore) ConfirmTopupOrder(_ context.Context, ref string) (store.TopupConfirmOutcome, int32, error) {
	o, ok := f.orders[ref]
	if !ok {
		return store.TopupNotFound, 0, nil
	}
	if o.status == store.TopupStatusPaid {
		return store.TopupAlreadyConfirmed, f.balances[o.accountID], nil
	}
	f.balances[o.accountID] += o.credits
	o.status = store.TopupStatusPaid
	return store.TopupConfirmed, f.balances[o.accountID], nil
}

func (f *fakeStore) GetTopupOrder(_ context.Context, ref string, accountID int64) (int16, int32, int32, error) {
	o, ok := f.orders[ref]
	if !ok || o.accountID != accountID {
		return 0, 0, 0, store.ErrNotFound
	}
	var bal int32
	if o.status == store.TopupStatusPaid {
		bal = f.balances[o.accountID]
	}
	return o.status, o.credits, bal, nil
}

func newFake() *fakeStore {
	return &fakeStore{
		profiles: map[int64]profile{},
		orders:   map[string]*fakeOrder{},
		owners:   map[string]int64{},
		balances: map[int64]int32{7: 100},
		accounts: map[int64]bool{7: true, 8: true},
		pacotes:  map[string]store.PacoteDoacao{},
		staff:    map[int64]bool{},
	}
}

// TestSavePayerProfileValidation covers CPF normalization and field validation.
func TestSavePayerProfileValidation(t *testing.T) {
	tests := []struct {
		name       string
		account    int64
		payer      string
		cpf        string
		want       Result
		wantStored string // normalized CPF expected in the store (empty = not stored)
	}{
		{"ok formatted cpf", 7, "Jean", "123.456.789-09", OK, "12345678909"},
		{"ok bare digits", 7, "Jean", "12345678909", OK, "12345678909"},
		{"empty name", 7, "", "12345678909", Invalid, ""},
		{"blank name", 7, "   ", "12345678909", Invalid, ""},
		{"short cpf", 7, "Jean", "1234567890", Invalid, ""},
		{"long cpf", 7, "Jean", "123456789012", Invalid, ""},
		{"non-positive account", 0, "Jean", "12345678909", Invalid, ""},
		{"missing account", 555, "Jean", "12345678909", NotFound, ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			f := newFake()
			s := New(f)
			r, err := s.SavePayerProfile(context.Background(), tc.account, tc.payer, tc.cpf)
			if err != nil {
				t.Fatalf("SavePayerProfile error: %v", err)
			}
			if r != tc.want {
				t.Errorf("result = %v, want %v", r, tc.want)
			}
			if tc.wantStored != "" && f.profiles[tc.account].cpf != tc.wantStored {
				t.Errorf("stored cpf = %q, want %q", f.profiles[tc.account].cpf, tc.wantStored)
			}
		})
	}
}

// TestGetPayerProfile reports found=false for an unknown account, and the stored
// values after a save.
func TestGetPayerProfile(t *testing.T) {
	f := newFake()
	s := New(f)
	ctx := context.Background()

	if found, _, _, err := s.GetPayerProfile(ctx, 7); err != nil || found {
		t.Errorf("GetPayerProfile(unset) = (found=%v, %v), want (false, nil)", found, err)
	}
	if _, err := s.SavePayerProfile(ctx, 7, "Jean", "12345678909"); err != nil {
		t.Fatalf("save: %v", err)
	}
	found, name, cpf, err := s.GetPayerProfile(ctx, 7)
	if err != nil || !found || name != "Jean" || cpf != "12345678909" {
		t.Errorf("GetPayerProfile = (%v, %q, %q, %v)", found, name, cpf, err)
	}
}

// TestCreateTopupOrderValidation rejects malformed orders and maps store errors.
func TestCreateTopupOrderValidation(t *testing.T) {
	valid := domain.TopupOrder{AccountID: 7, ExternalReference: "ref-1", Credits: 10, AmountCents: 500, PaymentMethod: 1}
	tests := []struct {
		name  string
		order domain.TopupOrder
		want  Result
	}{
		{"ok pix", valid, OK},
		{"ok credit card smoke", domain.TopupOrder{AccountID: 7, ExternalReference: "ref-cc", Credits: 10, AmountCents: 500, PaymentMethod: 2}, OK},
		{"no account", domain.TopupOrder{ExternalReference: "r", Credits: 10, AmountCents: 500, PaymentMethod: 1}, Invalid},
		{"no credits", domain.TopupOrder{AccountID: 7, ExternalReference: "r", Credits: 0, AmountCents: 500, PaymentMethod: 1}, Invalid},
		{"no amount", domain.TopupOrder{AccountID: 7, ExternalReference: "r", Credits: 10, AmountCents: 0, PaymentMethod: 1}, Invalid},
		{"blank ref", domain.TopupOrder{AccountID: 7, ExternalReference: "  ", Credits: 10, AmountCents: 500, PaymentMethod: 1}, Invalid},
		{"unspecified method", domain.TopupOrder{AccountID: 7, ExternalReference: "r", Credits: 10, AmountCents: 500, PaymentMethod: 0}, Invalid},
		{"missing account fk", domain.TopupOrder{AccountID: 999, ExternalReference: "r", Credits: 10, AmountCents: 500, PaymentMethod: 1}, NotFound},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			s := New(newFake())
			r, id, err := s.CreateTopupOrder(context.Background(), tc.order)
			if err != nil {
				t.Fatalf("CreateTopupOrder error: %v", err)
			}
			if r != tc.want {
				t.Errorf("result = %v, want %v", r, tc.want)
			}
			if tc.want == OK && id == 0 {
				t.Errorf("expected non-zero order id on OK")
			}
		})
	}
}

// TestCreateTopupOrderDuplicate maps a reused external_reference to Invalid.
func TestCreateTopupOrderDuplicate(t *testing.T) {
	s := New(newFake())
	ctx := context.Background()
	o := domain.TopupOrder{AccountID: 7, ExternalReference: "dup", Credits: 10, AmountCents: 500, PaymentMethod: 1}
	if r, _, err := s.CreateTopupOrder(ctx, o); err != nil || r != OK {
		t.Fatalf("first create = (%v, %v), want OK", r, err)
	}
	if r, _, err := s.CreateTopupOrder(ctx, o); err != nil || r != Invalid {
		t.Errorf("duplicate create = (%v, %v), want Invalid", r, err)
	}
}

// TestConfirmTopupIdempotent is the critical test: a second confirm never
// credits again.
func TestConfirmTopupIdempotent(t *testing.T) {
	f := newFake()
	s := New(f)
	ctx := context.Background()
	if _, _, err := s.CreateTopupOrder(ctx, domain.TopupOrder{
		AccountID: 7, ExternalReference: "ref-c", Credits: 40, AmountCents: 500, PaymentMethod: 1,
	}); err != nil {
		t.Fatalf("create: %v", err)
	}

	// First confirm credits: 100 -> 140.
	out, bal, err := s.ConfirmTopupOrder(ctx, "ref-c")
	if err != nil || out != Confirmed || bal != 140 {
		t.Fatalf("first confirm = (%v, %d, %v), want (Confirmed, 140, nil)", out, bal, err)
	}
	// Second confirm is a no-op returning the current balance.
	out, bal, err = s.ConfirmTopupOrder(ctx, "ref-c")
	if err != nil || out != AlreadyConfirmed || bal != 140 {
		t.Errorf("second confirm = (%v, %d, %v), want (AlreadyConfirmed, 140, nil)", out, bal, err)
	}
	if f.balances[7] != 140 {
		t.Errorf("balance = %d after double-confirm, want 140 (credited once)", f.balances[7])
	}
}

// TestConfirmTopupNotFound reports ConfirmNotFound for an unknown reference.
func TestConfirmTopupNotFound(t *testing.T) {
	s := New(newFake())
	out, bal, err := s.ConfirmTopupOrder(context.Background(), "nope")
	if err != nil || out != ConfirmNotFound || bal != 0 {
		t.Errorf("confirm unknown = (%v, %d, %v), want (ConfirmNotFound, 0, nil)", out, bal, err)
	}
}

// TestGetTopupOrderOwnership ensures another account's order reads as absent.
func TestGetTopupOrderOwnership(t *testing.T) {
	f := newFake()
	s := New(f)
	ctx := context.Background()
	if _, _, err := s.CreateTopupOrder(ctx, domain.TopupOrder{
		AccountID: 7, ExternalReference: "ref-o", Credits: 10, AmountCents: 500, PaymentMethod: 1,
	}); err != nil {
		t.Fatalf("create: %v", err)
	}

	// Owner sees PENDING (status 1) with credits.
	st, cr, bal, err := s.GetTopupOrder(ctx, "ref-o", 7)
	if err != nil || st != store.TopupStatusPending || cr != 10 || bal != 0 {
		t.Errorf("owner get = (%d, %d, %d, %v), want (1, 10, 0, nil)", st, cr, bal, err)
	}
	// A different account gets zero-value status (treated as not found, no error).
	st, cr, bal, err = s.GetTopupOrder(ctx, "ref-o", 8)
	if err != nil || st != 0 || cr != 0 || bal != 0 {
		t.Errorf("other-account get = (%d, %d, %d, %v), want (0, 0, 0, nil)", st, cr, bal, err)
	}

	// After confirming, the owner sees PAID (status 2) with the new balance.
	if _, _, err := s.ConfirmTopupOrder(ctx, "ref-o"); err != nil {
		t.Fatalf("confirm: %v", err)
	}
	st, _, bal, err = s.GetTopupOrder(ctx, "ref-o", 7)
	if err != nil || st != store.TopupStatusPaid || bal != 110 {
		t.Errorf("owner get after paid = (%d, bal=%d, %v), want (2, 110, nil)", st, bal, err)
	}
}

// AS QUATRO RECUSAS DE PACOTE NÃO CRIAM ORDEM, e nenhuma delas pode passar.
//
// Cada uma protege uma coisa diferente:
//   - id desconhecido: o site e o servidor discordam sobre o que está à venda, e
//     creditar assim mesmo entregaria créditos por um preço que ninguém conferiu;
//   - desligado: o pacote saiu de venda entre a tela e o clique;
//   - só-staff: o pacote de R$ 1,00 existe para a dona do servidor testar pagamento de
//     verdade, e esconder na tela não é trava;
//   - divergência: o preço e os créditos que valem são os da TABELA, e os do pedido são
//     conferidos contra ela.
func TestCreateTopupOrderRecusaOsQuatroCasosDePacote(t *testing.T) {
	const conta = int64(7)
	base := store.PacoteDoacao{ID: "apoiador-bronze", Credits: 575, AmountCents: 4990, Ativo: true}
	soStaff := store.PacoteDoacao{ID: "teste-real", Credits: 10, AmountCents: 100, Ativo: true, SoStaff: true}
	desligado := store.PacoteDoacao{ID: "apoiador-velho", Credits: 100, AmountCents: 1000}

	casos := []struct {
		nome     string
		pacote   string
		credits  int32
		centavos int64
	}{
		{"id que nao existe na tabela", "apoiador-inventado", 575, 4990},
		{"pacote desligado", "apoiador-velho", 100, 1000},
		{"creditos divergentes", "apoiador-bronze", 5750, 4990},
		{"preco divergente", "apoiador-bronze", 575, 490},
	}
	for _, c := range casos {
		f := newFake()
		f.accounts[conta] = true
		f.pacotes = map[string]store.PacoteDoacao{
			"apoiador-bronze": base, "teste-real": soStaff, "apoiador-velho": desligado,
		}
		svc := New(f)

		res, id, err := svc.CreateTopupOrder(context.Background(), domain.TopupOrder{
			AccountID: conta, ExternalReference: "ref-" + c.nome, Credits: c.credits,
			AmountCents: c.centavos, PaymentMethod: 1, PacoteID: c.pacote,
		})

		if err != nil {
			t.Errorf("%s: virou erro de infra: %v", c.nome, err)
			continue
		}
		if res != Invalid || id != 0 {
			t.Errorf("%s: resultado = %v id = %d, queria Invalid e nenhuma ordem", c.nome, res, id)
		}
		// E A ORDEM NÃO EXISTE. Recusar depois de gravar deixaria uma ordem pendente
		// que o site poderia confirmar, e aí a recusa não teria servido para nada.
		if len(f.orders) != 0 {
			t.Errorf("%s: gravou %d ordem(ns) apesar de recusar", c.nome, len(f.orders))
		}
	}
}

// O MESMO PACOTE SÓ-STAFF PASSA PARA QUEM É STAFF. É o outro lado da trava: ela não
// pode ser tão apertada que impeça a dona do servidor de testar pagamento de verdade.
func TestCreateTopupOrderAceitaOPacoteDeStaffParaStaff(t *testing.T) {
	const conta = int64(9)
	f := newFake()
	f.accounts[conta] = true
	f.staff[conta] = true
	f.pacotes = map[string]store.PacoteDoacao{
		"teste-real": {ID: "teste-real", Credits: 10, AmountCents: 100, Ativo: true, SoStaff: true},
	}

	res, id, err := New(f).CreateTopupOrder(context.Background(), domain.TopupOrder{
		AccountID: conta, ExternalReference: "ref-staff", Credits: 10,
		AmountCents: 100, PaymentMethod: 1, PacoteID: "teste-real",
	})

	if err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	if res != OK || id == 0 {
		t.Errorf("resultado = %v id = %d, queria OK com ordem criada", res, id)
	}
	if !f.staffVisto {
		t.Error("o servico nao passou ehStaff=true para a conferencia")
	}
}

// DOAÇÃO SEM PACOTE CONTINUA VALENDO, e não passa pela conferência nenhuma. É toda
// ordem anterior aos pacotes existirem, e qualquer caminho futuro que credite sem
// vender pacote. Vazio não é desconhecido.
func TestCreateTopupOrderSemPacoteNaoConfereNada(t *testing.T) {
	const conta = int64(11)
	f := newFake()
	f.accounts[conta] = true

	res, _, err := New(f).CreateTopupOrder(context.Background(), domain.TopupOrder{
		AccountID: conta, ExternalReference: "ref-sem-pacote", Credits: 100,
		AmountCents: 1000, PaymentMethod: 1,
	})

	if err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	if res != OK {
		t.Errorf("resultado = %v, queria OK", res)
	}
	if f.pacotePedido != "" {
		t.Errorf("conferiu o pacote %q numa doacao sem pacote", f.pacotePedido)
	}
}

// Conta que não existe é NotFound, e não Invalid: o site precisa distinguir "essa conta
// não é sua" de "esse pedido está malformado".
func TestCreateTopupOrderComPacoteEContaInexistenteENotFound(t *testing.T) {
	f := newFake()
	f.pacotes = map[string]store.PacoteDoacao{
		"apoiador-bronze": {ID: "apoiador-bronze", Credits: 575, AmountCents: 4990, Ativo: true},
	}

	res, _, err := New(f).CreateTopupOrder(context.Background(), domain.TopupOrder{
		AccountID: 404, ExternalReference: "ref-sem-conta", Credits: 575,
		AmountCents: 4990, PaymentMethod: 1, PacoteID: "apoiador-bronze",
	})

	if err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	if res != NotFound {
		t.Errorf("resultado = %v, queria NotFound", res)
	}
}

// O PACOTE DE STAFF PEDIDO POR JOGADOR TEM CÓDIGO PRÓPRIO, e não o Invalid genérico.
//
// O site precisa dizer coisas diferentes: o Invalid deste caminho cobre referência
// repetida, pedido malformado e pacote fora de sincronia — três causas que pedem três
// mensagens —, e a única que é sobre QUEM está comprando é esta. Sem separá-la, a tela
// diria "esse pacote não está disponível" para um UUID repetido, que é bug do site.
func TestPacoteDeStaffParaJogadorEForbidden(t *testing.T) {
	const conta = int64(7)
	f := newFake()
	f.accounts[conta] = true
	f.pacotes = map[string]store.PacoteDoacao{
		"teste-real": {ID: "teste-real", Credits: 10, AmountCents: 100, Ativo: true, SoStaff: true},
	}

	res, id, err := New(f).CreateTopupOrder(context.Background(), domain.TopupOrder{
		AccountID: conta, ExternalReference: "ref-staff-negada", Credits: 10,
		AmountCents: 100, PaymentMethod: 1, PacoteID: "teste-real",
	})

	if err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	if res != Forbidden {
		t.Errorf("resultado = %v, quero Forbidden", res)
	}
	if id != 0 || len(f.orders) != 0 {
		t.Errorf("criou ordem: id=%d ordens=%d", id, len(f.orders))
	}
}
