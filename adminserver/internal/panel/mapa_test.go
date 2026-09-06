package panel

import (
	"strings"
	"testing"

	"github.com/jeanluca/w2pp-openwyd/adminserver/internal/jogo"
	"github.com/jeanluca/w2pp-openwyd/internal/mapzones"
)

// Coordinates used across these tests. Armia's rectangle is 2052,2052..2171,2163
// (internal/mapzones); campoX/campoY sit well outside every city so the
// clustering rule can actually fire there.
const (
	armiaX, armiaY = 2100, 2100
	campoX, campoY = 2800, 2600
)

func TestPontosIgnoraQuemNaoEstaNoMundo(t *testing.T) {
	// A session still on the character screen reports no position, and its zero
	// coordinates are a REAL spot on the grid. Drawing it would put a permanent
	// phantom crowd in the top-left corner — and that corner is inside no city,
	// so the crowd would also register as an aglomeração.
	ps := pontos([]jogo.Player{
		{Conta: "ana", Personagem: "Heroina", X: armiaX, Y: armiaY, Jogando: true},
		{Conta: "bruno", Jogando: false},
	})
	if len(ps) != 1 {
		t.Fatalf("pontos = %d, want 1 — só quem está no mundo tem posição", len(ps))
	}
	if ps[0].Conta != "ana" {
		t.Errorf("ponto = %q, want ana", ps[0].Conta)
	}
}

func TestRegiaoNomeiaACidade(t *testing.T) {
	if got := regiao(armiaX, armiaY); got != "Armia" {
		t.Errorf("regiao = %q, want Armia", got)
	}
}

func TestRegiaoNoCampoDaUmaReferencia(t *testing.T) {
	// "Campo" alone answers nothing: a moderator deciding whether to go look
	// needs to know WHERE the field is. mapzones.Nearest gives the landmark.
	got := regiao(campoX, campoY)
	if !strings.HasPrefix(got, "Campo") {
		t.Fatalf("regiao = %q, want começando em Campo", got)
	}
	if !strings.Contains(got, "perto de") {
		t.Errorf("regiao = %q — campo sem referência não diz onde é", got)
	}
}

func TestAglomeracaoIgnoraCidade(t *testing.T) {
	// Everybody stands in Armia: the shops, the bank and the respawn are there.
	// A detector that counted town crowds would fire every minute of every day
	// and be ignored within a week.
	if !mapzones.InTown(armiaX, armiaY) {
		t.Fatal("premissa errada: a coordenada de teste não está dentro de Armia")
	}
	ps := []PontoMapa{
		{Conta: "a", X: armiaX, Y: armiaY},
		{Conta: "b", X: armiaX + 1, Y: armiaY},
		{Conta: "c", X: armiaX, Y: armiaY + 1},
		{Conta: "d", X: armiaX + 2, Y: armiaY + 2},
	}
	if g := aglomerar(ps); len(g) != 0 {
		t.Errorf("grupos = %d, want 0 — cidade cheia é cidade, não fazenda", len(g))
	}
	for _, p := range ps {
		if p.Junto {
			t.Errorf("%s foi marcado como aglomerado dentro da cidade", p.Conta)
		}
	}
}

func TestAglomeracaoNoCampoMarcaOsPontos(t *testing.T) {
	if mapzones.InTown(campoX, campoY) {
		t.Fatal("premissa errada: a coordenada de teste está dentro de uma cidade")
	}
	ps := []PontoMapa{
		{Conta: "c", X: campoX, Y: campoY},
		{Conta: "a", X: campoX + 3, Y: campoY + 2},
		{Conta: "b", X: campoX - 4, Y: campoY + 5},
	}
	g := aglomerar(ps)
	if len(g) != 1 {
		t.Fatalf("grupos = %d, want 1", len(g))
	}
	// Sorted, so the page reads the same way twice in a row.
	if strings.Join(g[0].Contas, ",") != "a,b,c" {
		t.Errorf("contas = %v, want a,b,c em ordem", g[0].Contas)
	}
	for _, p := range ps {
		if !p.Junto {
			t.Errorf("%s ficou sem marca — o desenho não vai destacar o grupo", p.Conta)
		}
	}
}

func TestDoisJuntosSaoUmaDupla(t *testing.T) {
	// Two players together is a party. The number that makes a moderator walk
	// over is three.
	ps := []PontoMapa{
		{Conta: "a", X: campoX, Y: campoY},
		{Conta: "b", X: campoX + 2, Y: campoY},
	}
	if g := aglomerar(ps); len(g) != 0 {
		t.Errorf("grupos = %d, want 0", len(g))
	}
}

func TestAglomeracaoEncadeiaAoLongoDaLinha(t *testing.T) {
	// A farm spread along a spawn line is ONE operation, not three pairs: the
	// first and the last are 40 apart, past raioAglomeracao, but each is within
	// reach of the next. Single-link is what keeps them together.
	ps := []PontoMapa{
		{Conta: "a", X: campoX, Y: campoY},
		{Conta: "b", X: campoX + 20, Y: campoY},
		{Conta: "c", X: campoX + 40, Y: campoY},
	}
	g := aglomerar(ps)
	if len(g) != 1 {
		t.Fatalf("grupos = %d, want 1 — a fila virou grupos separados", len(g))
	}
	if len(g[0].Contas) != 3 {
		t.Errorf("contas = %v, want as três", g[0].Contas)
	}
}

func TestLongeDemaisNaoEAglomeracao(t *testing.T) {
	ps := []PontoMapa{
		{Conta: "a", X: campoX, Y: campoY},
		{Conta: "b", X: campoX + 200, Y: campoY},
		{Conta: "c", X: campoX + 400, Y: campoY},
	}
	if g := aglomerar(ps); len(g) != 0 {
		t.Errorf("grupos = %d, want 0 — gente espalhada não é fazenda", len(g))
	}
}

func TestPorRegiaoOrdenaPelaMaisCheia(t *testing.T) {
	ps := []PontoMapa{
		{Regiao: "Armia"}, {Regiao: "Armia"}, {Regiao: "Armia"},
		{Regiao: "Azran"},
	}
	r := porRegiao(ps)
	if len(r) != 2 || r[0].Nome != "Armia" || r[0].Total != 3 {
		t.Fatalf("regiões = %+v, want Armia com 3 na frente", r)
	}
}

func TestMapaDesenhaOsJogadoresEDizOndeEstao(t *testing.T) {
	j := &fakeJogo{estado: jogo.Estado{
		Jogando: 3, Conectados: 4,
		Players: []jogo.Player{
			{Conta: "ana", Personagem: "Heroina", Nivel: 200, X: campoX, Y: campoY, Jogando: true},
			{Conta: "bruno", Personagem: "Bruno", Nivel: 199, X: campoX + 5, Y: campoY, Jogando: true},
			{Conta: "caio", Personagem: "Caio", Nivel: 198, X: campoX, Y: campoY + 5, Jogando: true},
			{Conta: "dora", Jogando: false},
		},
	}}
	body := getSignedIn(t, newTestPanelJogo(t, newFakeAudit(), j), "/mapa").Body.String()

	if !strings.Contains(body, "<svg") {
		t.Fatal("a página não desenhou o mapa")
	}
	// The picture is served as part of the page: the panel serves no JavaScript
	// and its CSP forbids script, so a map that needed a library would render
	// as an empty box.
	if strings.Contains(strings.ToLower(body), "<script") {
		t.Error("o mapa passou a depender de JavaScript, que a política de segurança bloqueia")
	}
	if !strings.Contains(body, "Heroina") {
		t.Error("o mapa não nomeia quem está no ponto")
	}
	if strings.Contains(body, "dora") {
		t.Error("uma sessão ainda na tela de personagem foi desenhada no mundo")
	}
	if !strings.Contains(body, "personagens juntos") {
		t.Error("três no mesmo lugar fora de cidade não viraram uma aglomeração na tela")
	}
}

func TestMapaSemNinguemNaoQuebra(t *testing.T) {
	j := &fakeJogo{estado: jogo.Estado{}}
	rec := getSignedIn(t, newTestPanelJogo(t, newFakeAudit(), j), "/mapa")
	if rec.Code != 200 {
		t.Fatalf("status = %d, want 200 (body: %s)", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "Ninguém em jogo") {
		t.Error("servidor vazio deveria dizer que está vazio")
	}
}

func TestMapaComServidorForaDoArAindaAbre(t *testing.T) {
	// The page exists to answer "what is going on"; failing to reach the game is
	// itself part of that answer, so it is shown rather than turned into a 502.
	j := &fakeJogo{estadoErr: jogo.ErrForaDoAr}
	rec := getSignedIn(t, newTestPanelJogo(t, newFakeAudit(), j), "/mapa")
	if rec.Code != 200 {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "nota perigo") {
		t.Error("a falha de leitura não apareceu na página")
	}
}

func TestCidadesSempreNomeiaAsCincoConhecidas(t *testing.T) {
	// The five canonical cities carry the legacy CityLimit rectangle and are how
	// a person finds themselves on this picture. They are named even on an empty
	// server.
	cs := cidades(nil)
	rotuladas := map[string]bool{}
	for _, c := range cs {
		if c.Rotular {
			rotuladas[c.Nome] = true
		}
	}
	for _, nome := range []string{"Armia", "Azran", "Erion", "Nippleheim", "Noatum"} {
		if !rotuladas[nome] {
			t.Errorf("%s não foi nomeada — sem as cidades conhecidas ninguém se localiza", nome)
		}
	}
	// And the three dungeon instances stay unnamed while empty: they sit within
	// 200 units of each other and three names there overlap into a smear.
	if rotuladas["Pesadelo Místico"] {
		t.Error("masmorra vazia foi rotulada; os três nomes se atropelam no canto")
	}
}

func TestCidadeGanhaNomeQuandoTemGente(t *testing.T) {
	cs := cidades([]PontoMapa{{Regiao: "Pesadelo Místico (masmorra)"}})
	for _, c := range cs {
		if c.Nome == "Pesadelo Místico" {
			if !c.Rotular {
				t.Error("a masmorra com gente dentro continuou sem nome")
			}
			return
		}
	}
	t.Fatal("a masmorra sumiu do desenho")
}

func TestNomeDoDesenhoPerdeOParentese(t *testing.T) {
	// "Pesadelo Místico (masmorra)" is right in a table and far too long to sit
	// on a circle 60 units wide.
	if got := curto("Pesadelo Místico (masmorra)"); got != "Pesadelo Místico" {
		t.Errorf("curto = %q", got)
	}
	if got := curto("Armia"); got != "Armia" {
		t.Errorf("curto = %q, want Armia intacto", got)
	}
}

func TestCampoAgrupaMesmoComDistanciasDiferentes(t *testing.T) {
	// The region string is a grouping key: porRegiao counts equal strings. Four
	// players standing on top of each other are at slightly different distances
	// from the same landmark, and carrying that number in the name filed them
	// under four different regions — one row per player, which is no tally at
	// all.
	ps := pontos([]jogo.Player{
		{Conta: "a", X: campoX, Y: campoY, Jogando: true},
		{Conta: "b", X: campoX + 6, Y: campoY + 4, Jogando: true},
		{Conta: "c", X: campoX - 5, Y: campoY + 10, Jogando: true},
		{Conta: "d", X: campoX + 12, Y: campoY - 4, Jogando: true},
	})
	r := porRegiao(ps)
	if len(r) != 1 {
		t.Fatalf("regiões = %+v, want uma só — quatro no mesmo lugar não são quatro regiões", r)
	}
	if r[0].Total != 4 {
		t.Errorf("total = %d, want 4", r[0].Total)
	}
}

func TestOAnelDoGrupoEnvolveOsPontos(t *testing.T) {
	// A tight farm has everyone within a few units of the center, so the honest
	// radius is smaller than the dots and the ring draws underneath them — the
	// one case the ring exists for is the one where it disappeared.
	const raioDoPonto = 16
	ps := []PontoMapa{
		{Conta: "a", X: campoX, Y: campoY},
		{Conta: "b", X: campoX + 1, Y: campoY},
		{Conta: "c", X: campoX, Y: campoY + 1},
	}
	g := aglomerar(ps)
	if len(g) != 1 {
		t.Fatalf("grupos = %d, want 1", len(g))
	}
	if g[0].RaioDesenho <= g[0].Raio+raioDoPonto {
		t.Errorf("raio de desenho = %d com raio real %d — o anel fica embaixo das bolinhas",
			g[0].RaioDesenho, g[0].Raio)
	}
	// The number the page prints stays the true one.
	if g[0].Raio > 2 {
		t.Errorf("raio real = %d, want a distância de verdade", g[0].Raio)
	}
}

// --- contas na mesma conexão ---

func TestConexaoJuntaContasDiferentesDoMesmoEndereco(t *testing.T) {
	c := conexoes([]jogo.Player{
		{Conta: "bot1", IP: "10.0.0.9", Jogando: true},
		{Conta: "bot2", IP: "10.0.0.9", Jogando: true},
		{Conta: "bot3", IP: "10.0.0.9", Jogando: false},
		{Conta: "outra", IP: "10.0.0.8", Jogando: true},
	})
	if len(c) != 1 {
		t.Fatalf("conexões = %d, want 1", len(c))
	}
	if strings.Join(c[0].Contas, ",") != "bot1,bot2,bot3" {
		t.Errorf("contas = %v", c[0].Contas)
	}
	// Two of the three are in the world; the third is still choosing. A farm
	// signing in is visible here before it has any position at all.
	if c[0].EmJogo != 2 {
		t.Errorf("em jogo = %d, want 2", c[0].EmJogo)
	}
}

func TestUmaContaComDuasSessoesNaoEDuasContas(t *testing.T) {
	// One person reconnecting is not two people, and listing them would send a
	// moderator after somebody whose internet dropped.
	if c := conexoes([]jogo.Player{
		{Conta: "ana", IP: "10.0.0.9", Jogando: true},
		{Conta: "ana", IP: "10.0.0.9", Jogando: false},
	}); len(c) != 0 {
		t.Errorf("conexões = %d, want 0 — a mesma conta duas vezes não é duas contas", len(c))
	}
}

func TestConexaoSemEnderecoNaoAgrupa(t *testing.T) {
	// The game does not always have an address, and several blanks are not a
	// match — grouping them would invent a farm out of missing data.
	if c := conexoes([]jogo.Player{
		{Conta: "a", IP: "", Jogando: true},
		{Conta: "b", IP: "", Jogando: true},
	}); len(c) != 0 {
		t.Errorf("conexões = %d, want 0", len(c))
	}
}

// The address is the whole reason this is delicate: the panel needs the FACT
// that accounts share a connection and must never put the number on a page.
// PontoMapa keeps it in an unexported field, which a template cannot reach, so
// the rule is enforced by the language rather than by remembering it.
func TestOEnderecoNuncaChegaNaTela(t *testing.T) {
	const endereco = "203.0.113.77"
	j := &fakeJogo{estado: jogo.Estado{
		Jogando: 3, Conectados: 3,
		Players: []jogo.Player{
			{Conta: "bot1", Personagem: "Aaa", X: campoX, Y: campoY, Jogando: true, IP: endereco},
			{Conta: "bot2", Personagem: "Bbb", X: campoX + 4, Y: campoY, Jogando: true, IP: endereco},
			{Conta: "bot3", Personagem: "Ccc", X: campoX, Y: campoY + 4, Jogando: true, IP: endereco},
		},
	}}
	body := getSignedIn(t, newTestPanelJogo(t, newFakeAudit(), j), "/mapa").Body.String()

	if strings.Contains(body, endereco) {
		t.Error("o endereço foi parar na página")
	}
	if !strings.Contains(body, "3 contas") {
		t.Error("a página não avisou que três contas dividem a conexão")
	}
	// Position and connection together: circumstantial plus explainable is the
	// closest this panel gets to proof, so the group carries the mark.
	if !strings.Contains(body, "mesma conexão") {
		t.Error("o grupo amontoado e na mesma conexão não foi marcado")
	}
}

func TestAglomeracaoDeConexoesDiferentesNaoGanhaAMarca(t *testing.T) {
	ps := []PontoMapa{
		{Conta: "a", X: campoX, Y: campoY, ip: "10.0.0.1"},
		{Conta: "b", X: campoX + 3, Y: campoY, ip: "10.0.0.2"},
		{Conta: "c", X: campoX, Y: campoY + 3, ip: "10.0.0.3"},
	}
	g := aglomerar(ps)
	if len(g) != 1 {
		t.Fatalf("grupos = %d, want 1", len(g))
	}
	if g[0].MesmaConexao {
		t.Error("três pessoas de conexões diferentes foram marcadas como uma só")
	}
}

func TestGrupoSemEnderecoNaoEMesmaConexao(t *testing.T) {
	ps := []PontoMapa{
		{Conta: "a", X: campoX, Y: campoY},
		{Conta: "b", X: campoX + 3, Y: campoY},
		{Conta: "c", X: campoX, Y: campoY + 3},
	}
	g := aglomerar(ps)
	if len(g) != 1 {
		t.Fatalf("grupos = %d, want 1", len(g))
	}
	if g[0].MesmaConexao {
		t.Error("endereço faltando virou coincidência de endereço")
	}
}
