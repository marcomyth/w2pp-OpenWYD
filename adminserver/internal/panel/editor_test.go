package panel

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/jeanluca/w2pp-openwyd/adminserver/internal/gamedata"
	"github.com/jeanluca/w2pp-openwyd/adminserver/internal/personagem"
)

// grade is what turns stored rows into the screen, and the reachable/locked
// distinction it computes is the difference between an item the player has and
// one they can only see in the database.
func TestGradeMarcaFaixaTravada(t *testing.T) {
	itens := make([]personagem.Item, personagem.MaxCarry)
	for i := range itens {
		itens[i].Slot = i
	}
	itens[0] = personagem.Item{Slot: 0, Index: 700}
	itens[personagem.SlotMarcadorBolsa1] = personagem.Item{
		Slot: personagem.SlotMarcadorBolsa1, Index: personagem.ItemBolsaAndarilho}

	// One bag: 45 slots reachable.
	linhas := grade(itens, nil, 45, true)

	if !linhas[0].Alcancavel {
		t.Error("slot 0 deveria estar liberado")
	}
	if !linhas[44].Alcancavel {
		t.Error("slot 44 deveria estar liberado com uma bolsa")
	}
	if linhas[45].Alcancavel {
		t.Error("slot 45 deveria estar travado sem a segunda bolsa")
	}
	// The markers sit past the reachable band but are not "locked": they are
	// what does the unlocking, and drawing them hatched would be a lie.
	if !linhas[personagem.SlotMarcadorBolsa1].Alcancavel {
		t.Error("marcador de bolsa não deveria aparecer travado")
	}
	if !linhas[personagem.SlotMarcadorBolsa1].Marcador {
		t.Error("slot 60 deveria estar marcado como marcador de bolsa")
	}
}

// The refine level is pulled out of whichever effect slot carries EF_SANC,
// because that is what a player calls the item's "+N".
func TestGradeExtraiRefino(t *testing.T) {
	itens := []personagem.Item{
		{Slot: 0, Index: 144, Eff1: 7, EffV1: 12, Eff2: efSanc, EffV2: 11},
		{Slot: 1, Index: 400},
	}
	linhas := grade(itens, nil, 0, false)

	if linhas[0].Refino != 11 {
		t.Errorf("refino = %d, want 11", linhas[0].Refino)
	}
	if linhas[1].Refino != 0 {
		t.Errorf("item sem EF_SANC não deveria ter refino, veio %d", linhas[1].Refino)
	}
	if linhas[0].Efeitos[0].Nome != "Força" {
		t.Errorf("efeito 7 = %q, want Força", linhas[0].Efeitos[0].Nome)
	}
}

// Without a webServer the catalog is nil, and the grid must still render with
// indices rather than blank cells.
func TestGradeSemCatalogo(t *testing.T) {
	linhas := grade([]personagem.Item{{Slot: 0, Index: 3467}}, nil, 0, false)
	if linhas[0].Nome != "item 3467" {
		t.Errorf("nome = %q, want \"item 3467\"", linhas[0].Nome)
	}
}

func TestGradeUsaCatalogo(t *testing.T) {
	catalogo := map[int32]gamedata.Item{
		3467: {Index: 3467, DisplayName: "Bolsa do Andarilho", IconURL: "/i/3467.png"},
	}
	linhas := grade([]personagem.Item{{Slot: 0, Index: 3467}}, catalogo, 0, false)
	if linhas[0].Nome != "Bolsa do Andarilho" || linhas[0].IconURL != "/i/3467.png" {
		t.Errorf("linha = %+v, want nome e ícone do catálogo", linhas[0])
	}
}

func TestNomeEfeito(t *testing.T) {
	cases := map[uint8]string{
		0:   "",
		43:  "Refino",
		7:   "Força",
		250: "efeito 250", // unknown effects still say something useful
	}
	for tipo, want := range cases {
		if got := nomeEfeito(tipo); got != want {
			t.Errorf("nomeEfeito(%d) = %q, want %q", tipo, got, want)
		}
	}
}

func TestItemDoForm(t *testing.T) {
	r := httptest.NewRequest("POST", "/", strings.NewReader(url.Values{
		"indice": {"144"},
		"eff1":   {"43"}, "effv1": {"11"},
		"eff2": {"7"}, "effv2": {"12"},
	}.Encode()))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	it, err := itemDoForm(r, 3)
	if err != nil {
		t.Fatalf("itemDoForm: %v", err)
	}
	if it.Index != 144 || it.Slot != 3 {
		t.Errorf("item = %+v, want índice 144 no slot 3", it)
	}
	if it.Eff1 != 43 || it.EffV1 != 11 || it.Eff2 != 7 || it.EffV2 != 12 {
		t.Errorf("efeitos = %+v", it)
	}
	// Blank effect fields are zero, not an error: most items use fewer than three.
	if it.Eff3 != 0 || it.EffV3 != 0 {
		t.Errorf("efeito 3 em branco deveria ser zero, veio %d/%d", it.Eff3, it.EffV3)
	}
}

func TestItemDoFormRecusaValoresInvalidos(t *testing.T) {
	cases := []struct {
		nome   string
		campos url.Values
	}{
		{"índice não numérico", url.Values{"indice": {"abc"}}},
		{"índice negativo", url.Values{"indice": {"-1"}}},
		{"índice acima de int16", url.Values{"indice": {"40000"}}},
		// An effect byte over 255 would wrap on the way into the column, which
		// turns a typo into a different effect entirely.
		{"efeito acima de 255", url.Values{"indice": {"144"}, "eff1": {"300"}}},
		{"valor negativo", url.Values{"indice": {"144"}, "effv1": {"-5"}}},
	}
	for _, c := range cases {
		t.Run(c.nome, func(t *testing.T) {
			r := httptest.NewRequest("POST", "/", strings.NewReader(c.campos.Encode()))
			r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			if _, err := itemDoForm(r, 0); err == nil {
				t.Error("esperava recusa")
			}
		})
	}
}

func TestAtributosDoForm(t *testing.T) {
	r := httptest.NewRequest("POST", "/", strings.NewReader(url.Values{
		"level": {"400"}, "exp": {"1284902117"}, "coin": {"1902774310"},
		"str": {"1372"}, "int": {"180"}, "dex": {"640"}, "con": {"908"},
	}.Encode()))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	a, err := atributosDoForm(r)
	if err != nil {
		t.Fatalf("atributosDoForm: %v", err)
	}
	if a.Level != 400 || a.Exp != 1284902117 || a.Coin != 1902774310 {
		t.Errorf("atributos = %+v", a)
	}
	if a.Str != 1372 || a.Int != 180 || a.Dex != 640 || a.Con != 908 {
		t.Errorf("atributos = %+v", a)
	}
}

// The per-field ceilings exist because the columns are narrow: a value that
// wraps an int16 becomes a negative attribute, which the game reads as an
// enormous one. Refusing beats storing the wrap.
func TestAtributosDoFormRecusaEstouro(t *testing.T) {
	cases := []url.Values{
		{"str": {"40000"}},
		{"con": {"70000"}},
		{"level": {"5000"}},
		{"coin": {"9999999999"}},
		{"dex": {"-3"}},
		{"int": {"abc"}},
	}
	for _, campos := range cases {
		r := httptest.NewRequest("POST", "/", strings.NewReader(campos.Encode()))
		r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		if _, err := atributosDoForm(r); err == nil {
			t.Errorf("esperava recusa para %v", campos)
		}
	}
}

// A blank field means "leave it alone", so it must not silently write a zero
// over a real attribute.
func TestAtributosDoFormCamposEmBranco(t *testing.T) {
	r := httptest.NewRequest("POST", "/", strings.NewReader(url.Values{"level": {"400"}}.Encode()))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	a, err := atributosDoForm(r)
	if err != nil {
		t.Fatalf("atributosDoForm: %v", err)
	}
	if a.Level != 400 || a.Str != 0 {
		t.Errorf("atributos = %+v", a)
	}
}

// The panel serves default-src 'none' with no script-src, so inline JavaScript
// never runs in production. A grid that filled the form from an onclick looked
// alive locally and was dead in the browser — clicking a slot did nothing, which
// is exactly how this was found. No template may reintroduce a script.
func TestTelasNaoDependemDeJavaScript(t *testing.T) {
	entradas, err := uiFS.ReadDir("ui")
	if err != nil {
		t.Fatalf("ler ui: %v", err)
	}
	for _, e := range entradas {
		if e.IsDir() {
			continue
		}
		raw, err := uiFS.ReadFile("ui/" + e.Name())
		if err != nil {
			t.Fatalf("ler %s: %v", e.Name(), err)
		}
		corpo := strings.ToLower(string(raw))
		if strings.Contains(corpo, "<script") {
			t.Errorf("%s tem <script>: a CSP do painel bloqueia, então ele nunca roda", e.Name())
		}
		// onclick= and friends are blocked by the same policy.
		for _, atributo := range []string{"onclick=", "onchange=", "onsubmit=", "onload="} {
			if strings.Contains(corpo, atributo) {
				t.Errorf("%s usa %s: bloqueado pela CSP", e.Name(), atributo)
			}
		}
	}
}

// The same policy has no img-src, so an <img> renders as a broken box. The item
// grids show names, not icons, for that reason.
func TestTelasNaoCarregamImagens(t *testing.T) {
	entradas, _ := uiFS.ReadDir("ui")
	for _, e := range entradas {
		raw, err := uiFS.ReadFile("ui/" + e.Name())
		if err != nil {
			continue
		}
		if strings.Contains(strings.ToLower(string(raw)), "<img") {
			t.Errorf("%s tem <img>: a CSP não permite img-src", e.Name())
		}
	}
}

// Picking a slot has to survive without scripts: the grid links carry the choice
// in the query string and the server sends the form back filled.
func TestSelecaoDeSlotVemDaQueryString(t *testing.T) {
	ficha := fichaTeste(false)
	ficha.Carry[3] = personagem.Item{Slot: 3, Index: 700, Eff1: efSanc, EffV1: 9}

	sel := selecaoDe(reqCom("?onde=char_carry&slot=3"), ficha, nil)
	if !sel.Ativa {
		t.Fatal("seleção deveria estar ativa")
	}
	if sel.Destino != "char_carry" || sel.Slot != 3 {
		t.Errorf("seleção = %+v", sel)
	}
	if sel.Item.Index != 700 || sel.Item.Refino != 9 {
		t.Errorf("item da seleção = %+v, want índice 700 refino 9", sel.Item)
	}
}

// An empty slot is still a valid pick — it is how an item gets put somewhere.
func TestSelecaoDeSlotVazio(t *testing.T) {
	sel := selecaoDe(reqCom("?onde=char_equip&slot=5"), fichaTeste(false), nil)
	if !sel.Ativa || sel.Slot != 5 {
		t.Fatalf("seleção = %+v", sel)
	}
	if !sel.Item.Vazio {
		t.Errorf("slot 5 deveria estar vazio, veio %+v", sel.Item)
	}
}

func TestSelecaoInvalidaEIgnorada(t *testing.T) {
	for _, q := range []string{"", "?onde=char_carry", "?slot=3", "?onde=inventado&slot=3", "?onde=char_carry&slot=abc"} {
		if sel := selecaoDe(reqCom(q), fichaTeste(false), nil); sel.Ativa {
			t.Errorf("query %q não deveria selecionar nada, veio %+v", q, sel)
		}
	}
}

func reqCom(query string) *http.Request {
	return httptest.NewRequest("GET", "/contas/ana/personagens/Vandalyzz"+query, nil)
}

// fichaTeste is a character with the three containers allocated, so a slot pick
// always resolves to something.
func fichaTeste(emJogo bool) personagem.Ficha {
	f := personagem.Ficha{
		ID: 42, AccountID: 7, Nome: "Vandalyzz", Level: 400,
		Equip: make([]personagem.Item, personagem.MaxEquip),
		Carry: make([]personagem.Item, personagem.MaxCarry),
		Cargo: make([]personagem.Item, personagem.MaxCargo),
	}
	for i := range f.Equip {
		f.Equip[i].Slot = i
	}
	for i := range f.Carry {
		f.Carry[i].Slot = i
	}
	for i := range f.Cargo {
		f.Cargo[i].Slot = i
	}
	if emJogo {
		agora := time.Now()
		f.OnlineDesde = &agora
	}
	return f
}

// Emptying a slot is the most common edit there is. Refusing a blank index with
// "índice inválido" — as this did — sends the operator hunting for a number to
// type when what they wanted was nothing at all.
func TestIndiceVazioEsvaziaOSlot(t *testing.T) {
	for _, bruto := range []string{"", "   ", "0"} {
		r := httptest.NewRequest("POST", "/", strings.NewReader(url.Values{"indice": {bruto}}.Encode()))
		r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		it, err := itemDoForm(r, 4)
		if err != nil {
			t.Errorf("índice %q: %v — deveria esvaziar, não recusar", bruto, err)
			continue
		}
		if !it.Vazio() || it.Slot != 4 {
			t.Errorf("índice %q = %+v, want item vazio no slot 4", bruto, it)
		}
	}
}

// A real typo still has to be refused: "abc" is not an attempt to empty a slot.
func TestIndiceInvalidoAindaERecusado(t *testing.T) {
	for _, bruto := range []string{"abc", "-1", "40000"} {
		r := httptest.NewRequest("POST", "/", strings.NewReader(url.Values{"indice": {bruto}}.Encode()))
		r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		if _, err := itemDoForm(r, 0); err == nil {
			t.Errorf("índice %q deveria ser recusado", bruto)
		}
	}
}

// Picking an item from the search puts THAT item in the form, not whatever the
// slot still holds — otherwise choosing a replacement would show the old one.
func TestIndiceNaQueryVenceOItemDoSlot(t *testing.T) {
	ficha := fichaTeste(false)
	ficha.Carry[2] = personagem.Item{Slot: 2, Index: 400}

	sel := selecaoDe(reqCom("?onde=char_carry&slot=2&indice=700"), ficha, nil)
	if sel.Item.Index != 700 {
		t.Errorf("índice = %d, want 700 (o escolhido na busca)", sel.Item.Index)
	}
	// And without it, the slot's own item is what shows.
	sel = selecaoDe(reqCom("?onde=char_carry&slot=2"), ficha, nil)
	if sel.Item.Index != 400 {
		t.Errorf("índice = %d, want 400 (o que está no slot)", sel.Item.Index)
	}
}
