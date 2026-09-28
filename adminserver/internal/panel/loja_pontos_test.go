package panel

import (
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/jeanluca/w2pp-openwyd/adminserver/internal/gamedata"
)

// A Loja de Honra mora nas vagas do God of War desde 26/09/2026, cobradas em
// pontos. Estes testes guardam o preço em pontos no formulário de uma vaga.

func pts(n int32) *int32 { return &n }

// comLojaDeHonra põe no painel falso um God of War com duas vagas em pontos.
func comLojaDeHonra(game *fakeGameData) *fakeGameData {
	game.npcs = append(game.npcs, gamedata.NPC{
		ID: 6, Slug: "God_of_War-590", DisplayName: "Honor Store", TemplateName: "God_of_War",
		Enabled: true, X: 2130, Y: 2088, Merchant: 104, Origin: "content",
		Shop: []gamedata.ShopItem{
			{Slot: 0, ItemIndex: 1415, Quantity: 1, PricePoints: pts(100)},
			{Slot: 1, ItemIndex: 2000, Quantity: 1, PricePoints: pts(360)},
		},
	})
	return game
}

func vagaGravada(t *testing.T, game *fakeGameData, slot int32) gamedata.ShopItem {
	t.Helper()
	if len(game.shopSaves) != 1 {
		t.Fatalf("gravações = %d, quer 1", len(game.shopSaves))
	}
	for _, it := range game.shopSaves[0] {
		if it.Slot == slot {
			return it
		}
	}
	t.Fatalf("a vaga %d não foi gravada: %+v", slot, game.shopSaves[0])
	return gamedata.ShopItem{}
}

// Gravar uma vaga não pode tirar o preço em pontos das outras: o serviço grava a
// loja inteira de uma vez.
func TestSetLojaMantemOsPontosDasOutrasVagas(t *testing.T) {
	game := comLojaDeHonra(newFakeGameData())
	post, token := signedInPost(t, newTestPanelGame(t, newFakeAudit(), game))

	rec := post("/npcs/6/loja", url.Values{"csrf": {token}, "slot": {"1"}, "indice": {"2000"},
		"qtd": {"1"}, "pontos": {"400"}})
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, quer 303 (corpo: %s)", rec.Code, rec.Body.String())
	}
	if p := vagaGravada(t, game, 0).PricePoints; p == nil || *p != 100 {
		t.Errorf("a vaga 0, que ninguém editou, ficou com preço em pontos %v; quer 100", p)
	}
	if p := vagaGravada(t, game, 1).PricePoints; p == nil || *p != 400 {
		t.Errorf("a vaga 1 foi gravada com %v pontos, quer 400", p)
	}
}

// Deixar o campo vazio devolve a vaga ao ouro.
func TestSetLojaPontosVazioVoltaParaOuro(t *testing.T) {
	game := comLojaDeHonra(newFakeGameData())
	post, token := signedInPost(t, newTestPanelGame(t, newFakeAudit(), game))

	rec := post("/npcs/6/loja", url.Values{"csrf": {token}, "slot": {"1"}, "indice": {"2000"},
		"qtd": {"1"}, "pontos": {""}})
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, quer 303", rec.Code)
	}
	if p := vagaGravada(t, game, 1).PricePoints; p != nil {
		t.Errorf("com o campo vazio a vaga 1 ficou em %d pontos; quer ouro", *p)
	}
}

func TestSetLojaRecusaPontosInvalidos(t *testing.T) {
	for nome, valor := range map[string]string{
		"negativo":      "-1",
		"não é número":  "cem",
		"acima do teto": "1000001",
	} {
		t.Run(nome, func(t *testing.T) {
			game := comLojaDeHonra(newFakeGameData())
			post, token := signedInPost(t, newTestPanelGame(t, newFakeAudit(), game))
			rec := post("/npcs/6/loja", url.Values{"csrf": {token}, "slot": {"1"}, "indice": {"2000"},
				"pontos": {valor}})
			if rec.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, quer 400", rec.Code)
			}
			if len(game.shopSaves) != 0 {
				t.Fatal("um preço recusado mudou a loja")
			}
		})
	}
}

// A grade mostra o preço em pontos, e o formulário da vaga vem preenchido com ele
// — inclusive depois de escolher outro item na busca, porque o preço é da vaga.
func TestNpcPageMostraOsPontos(t *testing.T) {
	get := signedIn(t, newTestPanelGame(t, newFakeAudit(), comLojaDeHonra(newFakeGameData())))

	if body := get("/npcs/6").Body.String(); !strings.Contains(body, `<span class="pts">100 pts</span>`) {
		t.Error("a grade não mostra o preço em pontos da vaga 0")
	}
	body := get("/npcs/6?slot=1").Body.String()
	if !strings.Contains(body, `name="pontos" inputmode="numeric" placeholder="vazio = ouro"`) ||
		!strings.Contains(body, `value="360"`) {
		t.Error("o formulário da vaga 1 não veio com o preço em pontos")
	}
	if !strings.Contains(body, "Esta é a Loja de Honra") {
		t.Error("o formulário do God of War não explica a regra da Loja de Honra")
	}
	body = get("/npcs/6?slot=1&indice=1415").Body.String()
	if !strings.Contains(body, `value="360"`) {
		t.Error("escolher outro item na busca apagou o preço em pontos da vaga")
	}
}
