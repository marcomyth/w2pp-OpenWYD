package handler

import (
	"context"
	"io"
	"log/slog"
	"net"
	"path/filepath"
	"regexp"
	"strconv"
	"testing"
	"time"

	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/combine"
	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/content"
	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/protocol"
	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/world"
)

// A receita da Tiny que o catálogo real aceita: a arma Anct que recebe o ADD
// (EF_MOBTYPE 1, grau 5), outra Anct da mesma posição que o entrega (EF_ITEMLEVEL
// 6, que soma 30 à chance) e um terceiro item +9 qualquer.
const (
	tinyAlvo     = 2731 // Caliburn(Anct)
	tinyDoadora  = 2732 // Caliburn(Anct), o outro grau
	tinyTerceiro = 1331
	tinyNivel    = 6 // EF_ITEMLEVEL da doadora: MatchTiny soma 5 por ponto
)

func tinyReceita() [3]world.Item {
	return [3]world.Item{
		{Index: tinyAlvo, Effects: [3]world.Effect{{Effect: efSanc, Value: 9}, {Effect: efDamage, Value: 11}}},
		{Index: tinyDoadora, Effects: [3]world.Effect{{Effect: efSanc, Value: 9}, {Effect: efDamage, Value: 22}}},
		{Index: tinyTerceiro, Effects: [3]world.Effect{{Effect: efSanc, Value: 9}}},
	}
}

// tinyDaMesa fixa a base da Tiny na Mesa das Máquinas. A chance que vai ao
// sorteio é essa base mais 5 × o EF_ITEMLEVEL da doadora (MatchTiny).
func tinyDaMesa(base int32) combine.RateConfig {
	return combine.NewRateConfig(1, []combine.RateRow{{Family: "Tiny", Key: "ChanceBase", Rate: base}}, nil)
}

func startServerTiny(t *testing.T, rates combine.RateConfig) (string, func()) {
	t.Helper()
	root := filepath.Join("..", "..", "..", "Release", "Common")
	items, err := content.LoadItemList(filepath.Join(root, "ItemList.csv"))
	if err != nil {
		t.Skipf("ItemList.csv unavailable: %v", err)
	}
	comp, err := content.LoadCompRate(filepath.Join(root, "Settings", "CompRate.txt"))
	if err != nil {
		t.Skipf("CompRate.txt unavailable: %v", err)
	}
	st := world.CharacterState{Slot: 0, Name: "Hero", X: 5, Y: 5, HP: 1000, MaxHP: 1000, Coin: tinyCost}
	r := tinyReceita()
	st.Carry[0], st.Carry[1], st.Carry[2] = r[0], r[1], r[2]
	db := newDB()
	db.loadResult = st

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	d := New(Config{Log: log, CombineCatalog: NewCombineCatalog(items, comp), CompRate: comp, CombineRates: rates})
	w := world.New(world.Config{GridDim: 16}, log, db, d.Handle)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { _ = w.Serve(ctx, ln); close(done) }()
	return ln.Addr().String(), func() {
		cancel()
		select {
		case <-done:
		case <-time.After(2 * time.Second):
			t.Error("server did not stop")
		}
	}
}

func sendTiny(t *testing.T, c net.Conn) {
	t.Helper()
	var body protocol.MsgCombineItemBody
	for i := 0; i < 3; i++ {
		body.InvenPos[i] = uint8(i)
	}
	for i, it := range tinyReceita() {
		body.Item[i] = protocol.WireItem{Index: it.Index}
		for k, ef := range it.Effects {
			body.Item[i].Effects[k] = protocol.WireEffect{Effect: ef.Effect, Value: ef.Value}
		}
	}
	send(t, c, protocol.MsgCombineItemTiny, body.Encode())
}

// TestTinyAnunciaOResultado: a Tiny era a única máquina de sorteio que não
// anunciava nada, nem o sucesso nem a falha (pedido da equipe, 24/09/2026). Agora
// fala como a Agatha, para o servidor inteiro, com o número sorteado contra a
// chance que ele tinha de bater, e essa linha substitui a do próprio jogador.
func TestTinyAnunciaOResultado(t *testing.T) {
	casos := []struct {
		nome       string
		base       int32
		wantParm   int32
		wantLinha  *regexp.Regexp
		wantChance int
	}{
		{"sucesso", 100, combineSuccess, regexp.MustCompile(`^Hero conseguiu em (\d+)/(\d+) passar o ADD para \S+!$`), 100 + tinyNivel*5},
		// A Mesa não guarda base abaixo de 1, então a menor chance é 1 + 30. O
		// gerador do mundo é o LCG do MSVC com semente fixa: o primeiro sorteio
		// passa de 31, e este caso é sempre uma falha.
		{"falha", 1, combineFailed, regexp.MustCompile(`^Hero falhou em (\d+)/(\d+) ao passar o ADD para \S+\.$`), 1 + tinyNivel*5},
	}
	for _, tc := range casos {
		t.Run(tc.nome, func(t *testing.T) {
			addr, stop := startServerTiny(t, tinyDaMesa(tc.base))
			defer stop()
			c := enterWorld(t, addr)
			defer c.Close()

			sendTiny(t, c)
			r := lerResultadoDaAilyn(t, c)

			if r.parm != tc.wantParm {
				t.Fatalf("CombineComplete parm = %d, esperado %d — a receita não passou ou o sorteio não obedeceu à Mesa (textos %q)", r.parm, tc.wantParm, r.textos)
			}
			if len(r.textos) != 1 {
				t.Fatalf("linhas no chat = %q, esperado exatamente um anúncio", r.textos)
			}
			m := tc.wantLinha.FindStringSubmatch(r.textos[0])
			if m == nil {
				t.Fatalf("anúncio = %q, não bate com %s", r.textos[0], tc.wantLinha)
			}
			sorteio, _ := strconv.Atoi(m[1])
			chance, _ := strconv.Atoi(m[2])
			if chance != tc.wantChance {
				t.Errorf("chance anunciada = %d, esperado %d (a que o sorteio usou)", chance, tc.wantChance)
			}
			if (sorteio <= chance) != (tc.wantParm == combineSuccess) {
				t.Errorf("anúncio %d/%d contradiz o desfecho parm=%d", sorteio, chance, tc.wantParm)
			}
		})
	}
}
