package grpcsrv

import (
	"context"
	"net"
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/test/bufconn"

	dbv1 "github.com/jeanluca/w2pp-openwyd/api/db/v1"
	"github.com/jeanluca/w2pp-openwyd/internal/domain"
	"github.com/jeanluca/w2pp-openwyd/internal/secret"
	"github.com/jeanluca/w2pp-openwyd/internal/store"
)

// TestServiceOverWire runs the real AccountService through a gRPC connection
// (bufconn) end to end against the in-memory fakeStore, proving the generated
// codec + registration wire up correctly (not just the method logic).
func TestServiceOverWire(t *testing.T) {
	hash, _ := secret.HashSecret("pw")
	fs := &fakeStore{
		byName: map[string]store.AccountAuth{"alice": {ID: 1, PassHash: hash}},
		chars:  map[int64][]domain.Character{1: {{Slot: 0, Name: "hero", Level: 9}}},
	}

	lis := bufconn.Listen(1 << 20)
	srv := grpc.NewServer()
	dbv1.RegisterAccountServiceServer(srv, New(fs))
	go func() { _ = srv.Serve(lis) }()
	t.Cleanup(srv.Stop)

	conn, err := grpc.NewClient("passthrough:///bufnet",
		grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) {
			return lis.DialContext(ctx)
		}),
		grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	client := dbv1.NewAccountServiceClient(conn)

	resp, err := client.AccountLogin(context.Background(),
		&dbv1.AccountLoginRequest{AccountName: "alice", Password: "pw"})
	if err != nil {
		t.Fatalf("AccountLogin over wire: %v", err)
	}
	if resp.GetResult() != dbv1.LoginResult_LOGIN_RESULT_OK || resp.GetAccountId() != 1 {
		t.Fatalf("unexpected: result=%v id=%d", resp.GetResult(), resp.GetAccountId())
	}

	list, err := client.ListCharacters(context.Background(), &dbv1.ListCharactersRequest{AccountId: 1})
	if err != nil {
		t.Fatalf("ListCharacters over wire: %v", err)
	}
	if len(list.GetCharacters()) != 1 || list.GetCharacters()[0].GetName() != "hero" {
		t.Fatalf("characters not returned: %+v", list.GetCharacters())
	}
}

func (f *fakeStore) RecordTrade(context.Context, domain.TradeRecord) error { return nil }

// ReserveSerials hands out consecutive blocks, so a test can assert that two
// reservations never overlap.
func (f *fakeStore) ReserveSerials(_ context.Context, quantos int64) (int64, error) {
	if f.serialErr != nil {
		return 0, f.serialErr
	}
	primeiro := f.serialProximo + 1
	f.serialProximo += quantos
	return primeiro, nil
}

func (f *fakeStore) RecordChat(_ context.Context, linhas []domain.ChatLinha) error {
	f.chat = append(f.chat, linhas...)
	f.lotes = append(f.lotes, len(linhas))
	return nil
}

func (f *fakeStore) RecordGround(_ context.Context, g domain.GroundEvent) error {
	f.chao = append(f.chao, g)
	return nil
}

func (f *fakeStore) RecordReport(_ context.Context, r domain.PlayerReport) error {
	f.reports = append(f.reports, r)
	return nil
}

func (f *fakeStore) SetCharacterPresence(_ context.Context, name string, online bool) (bool, error) {
	if f.presence == nil {
		f.presence = map[string]bool{}
	}
	f.presence[name] = online
	return true, nil
}

func (f *fakeStore) ClearAllPresence(context.Context) (int64, error) {
	var n int64
	for name, online := range f.presence {
		if online {
			f.presence[name] = false
			n++
		}
	}
	return n, nil
}

// shopPoints is the in-memory personal-shop wallet, keyed by account id. Credits
// accumulate here exactly as the real store accumulates them in Postgres, so a
// test can assert on the running balance and not just on the last call.
func (f *fakeStore) AddShopPoints(_ context.Context, accountID int64, delta int32, _, _ string) (int32, error) {
	if f.shopPoints == nil {
		f.shopPoints = map[int64]int32{}
	}
	f.shopPoints[accountID] += delta
	return f.shopPoints[accountID], nil
}

func (f *fakeStore) ShopPoints(_ context.Context, accountID int64) (int32, error) {
	return f.shopPoints[accountID], nil
}

// SpendShopPoints espelha o contrato do store real: saldo curto é uma RECUSA
// (paid=false, err=nil), não um erro, e nesse caso a carteira não se mexe.
func (f *fakeStore) SpendShopPoints(_ context.Context, accountID int64, cost int32, _, _ string) (int32, bool, error) {
	if f.shopPoints == nil || f.shopPoints[accountID] < cost {
		return 0, false, nil
	}
	f.shopPoints[accountID] -= cost
	return f.shopPoints[accountID], true, nil
}

// ClaimNewbieKit: a primeira chamada de cada conta concede, as seguintes não —
// o mesmo contrato do INSERT ... ON CONFLICT DO NOTHING do store real.
func (f *fakeStore) ClaimNewbieKit(_ context.Context, accountID int64, _ string) (bool, error) {
	if f.newbieKit == nil {
		f.newbieKit = map[int64]bool{}
	}
	if f.newbieKit[accountID] {
		return false, nil
	}
	f.newbieKit[accountID] = true
	return true, nil
}

// A carteira de DONATE é outra: os pontos de lojinha acima são tempo, esta é
// dinheiro, e a RCoin é o que a enche pelo jogo.
func (f *fakeStore) CreditDonateInGame(_ context.Context, accountID int64, amount int32, _, _ string) (int32, error) {
	if f.donate == nil {
		f.donate = map[int64]int32{}
	}
	f.donate[accountID] += amount
	return f.donate[accountID], nil
}

func (f *fakeStore) DonateBalance(_ context.Context, accountID int64) (int32, error) {
	return f.donate[accountID], nil
}

// TestCreditDonateOverWire cobre o caminho que a RCoin percorre: o tmServer pede
// o crédito, o dbServer soma na carteira de donate e devolve o saldo.
//
// O valor não positivo é recusado no servidor, e não no chamador: nada em jogo
// tira donate, então um amount errado é defeito de quem chamou e não pode virar
// um débito silencioso na carteira de um jogador.
func TestCreditDonateOverWire(t *testing.T) {
	fs := &fakeStore{}
	lis := bufconn.Listen(1 << 20)
	srv := grpc.NewServer()
	dbv1.RegisterAccountServiceServer(srv, New(fs))
	go func() { _ = srv.Serve(lis) }()
	t.Cleanup(srv.Stop)

	conn, err := grpc.NewClient("passthrough:///bufnet",
		grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) {
			return lis.DialContext(ctx)
		}),
		grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	client := dbv1.NewAccountServiceClient(conn)
	ctx := context.Background()

	// Duas moedas seguidas somam: a carteira acumula, não é sobrescrita.
	for i, quer := range []int32{100, 1100} {
		resp, err := client.CreditDonate(ctx, &dbv1.CreditDonateRequest{
			AccountId: 7, Amount: []int32{100, 1000}[i], CharacterName: "Hero", Reason: "teste",
		})
		if err != nil {
			t.Fatalf("CreditDonate #%d: %v", i+1, err)
		}
		if resp.GetBalance() != quer {
			t.Fatalf("saldo depois do crédito #%d = %d, quer %d", i+1, resp.GetBalance(), quer)
		}
	}

	saldo, err := client.DonateBalance(ctx, &dbv1.DonateBalanceRequest{AccountId: 7})
	if err != nil {
		t.Fatalf("DonateBalance: %v", err)
	}
	if saldo.GetBalance() != 1100 {
		t.Errorf("DonateBalance = %d, quer 1100", saldo.GetBalance())
	}

	for _, amount := range []int32{0, -100} {
		if _, err := client.CreditDonate(ctx, &dbv1.CreditDonateRequest{AccountId: 7, Amount: amount}); err == nil {
			t.Errorf("CreditDonate aceitou amount %d, queria recusa", amount)
		}
	}
	// A recusa não pode ter mexido na carteira.
	saldo, err = client.DonateBalance(ctx, &dbv1.DonateBalanceRequest{AccountId: 7})
	if err != nil {
		t.Fatalf("DonateBalance: %v", err)
	}
	if saldo.GetBalance() != 1100 {
		t.Errorf("a recusa mexeu na carteira: saldo = %d, quer 1100", saldo.GetBalance())
	}
}

// A Loja de Rcoin no fake: uma fada de 60 na aba 5, e a compra que confere o
// preço visto e o saldo como o banco confere.
func (f *fakeStore) ListRcoinOffers(_ context.Context, category int32) ([]domain.RcoinOffer, error) {
	fada := domain.RcoinOffer{
		DonateShopItem: domain.DonateShopItem{ID: 55, ItemIndex: 3901, Eff1: 106, EffV1: 3,
			Price: 60, Title: "Fada Azul 3 dias", ExpiresDays: 3, Enabled: true},
		Category: 5,
	}
	if category == 0 || category == 5 {
		return []domain.RcoinOffer{fada}, nil
	}
	return nil, nil
}

func (f *fakeStore) BuyRcoinOffer(_ context.Context, accountID, offerID int64, seenPrice int32) (store.RcoinBuyResult, int32, int64, error) {
	saldo := f.donate[accountID]
	switch {
	case offerID != 55:
		return store.RcoinBuyUnavailable, saldo, 0, nil
	case seenPrice != 60:
		return store.RcoinBuyPriceChanged, saldo, 0, nil
	case saldo < 60:
		return store.RcoinBuyNoFunds, saldo, 0, nil
	}
	f.donate[accountID] = saldo - 60
	return store.RcoinBuyOK, saldo - 60, 900, nil
}

// TestLojaDeRcoinOverWire cobre o que o tmServer pede ao dbServer pela janela de
// Rcoin: a aba com o saldo junto, e a compra com cada resultado do contrato.
func TestLojaDeRcoinOverWire(t *testing.T) {
	fs := &fakeStore{donate: map[int64]int32{7: 100}}
	lis := bufconn.Listen(1 << 20)
	srv := grpc.NewServer()
	dbv1.RegisterAccountServiceServer(srv, New(fs))
	go func() { _ = srv.Serve(lis) }()
	t.Cleanup(srv.Stop)
	conn, err := grpc.NewClient("passthrough:///bufnet",
		grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) {
			return lis.DialContext(ctx)
		}),
		grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	client := dbv1.NewAccountServiceClient(conn)
	ctx := context.Background()

	lista, err := client.ListRcoinOffers(ctx, &dbv1.ListRcoinOffersRequest{Category: 5, AccountId: 7})
	if err != nil {
		t.Fatalf("ListRcoinOffers: %v", err)
	}
	if lista.GetBalance() != 100 || len(lista.GetOffers()) != 1 {
		t.Fatalf("lista = %+v", lista)
	}
	o := lista.GetOffers()[0]
	if o.GetId() != 55 || o.GetCategory() != 5 || o.GetEff1() != 106 || o.GetEffv1() != 3 || o.GetTitle() != "Fada Azul 3 dias" {
		t.Errorf("oferta = %+v", o)
	}
	if _, err := client.ListRcoinOffers(ctx, &dbv1.ListRcoinOffersRequest{Category: 5}); err == nil {
		t.Error("lista sem conta foi aceita")
	}

	casos := []struct {
		nome   string
		oferta int64
		preco  int32
		quer   dbv1.RcoinBuyResult
		saldo  int32
	}{
		{"preço mudou", 55, 59, dbv1.RcoinBuyResult_RCOIN_BUY_PRICE_CHANGED, 100},
		{"indisponível", 56, 60, dbv1.RcoinBuyResult_RCOIN_BUY_UNAVAILABLE, 100},
		{"compra", 55, 60, dbv1.RcoinBuyResult_RCOIN_BUY_OK, 40},
		{"sem saldo", 55, 60, dbv1.RcoinBuyResult_RCOIN_BUY_NO_FUNDS, 40},
	}
	for _, c := range casos {
		resp, err := client.BuyRcoinOffer(ctx, &dbv1.BuyRcoinOfferRequest{AccountId: 7, OfferId: c.oferta, SeenPrice: c.preco})
		if err != nil {
			t.Fatalf("%s: %v", c.nome, err)
		}
		if resp.GetResult() != c.quer || resp.GetBalance() != c.saldo {
			t.Errorf("%s = (%v, %d), quer (%v, %d)", c.nome, resp.GetResult(), resp.GetBalance(), c.quer, c.saldo)
		}
		if (c.quer == dbv1.RcoinBuyResult_RCOIN_BUY_OK) != (resp.GetDeliveryId() != 0) {
			t.Errorf("%s: delivery_id = %d", c.nome, resp.GetDeliveryId())
		}
	}
}
