package mobspawns

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/jeanluca/w2pp-openwyd/internal/regions"
)

func TestPlacePreferecRegiaoDepoisCidade(t *testing.T) {
	tab := regions.Table{{X1: 1161, Y1: 3595, X2: 1267, Y2: 3695, Name: "Agua_M"}}

	// Inside a named rectangle the rectangle wins, translated.
	if got := Place(tab, 1250, 3608); got != "Água Místico" {
		t.Errorf("Place na Água = %q, want Água Místico", got)
	}
	// Outside every rectangle, mapzones names the town.
	if got := Place(tab, 2086, 2093); got != "Armia" {
		t.Errorf("Place no centro de Armia = %q, want Armia", got)
	}
	// Outside both, a landmark beats bare coordinates. Which landmark is
	// mapzones' business; that it says "perto de" something is this one's.
	got := Place(tab, 2453, 2300)
	if got == "" || got == "Campo" {
		t.Errorf("Place no campo = %q — devia citar um ponto de referência", got)
	}
}

// A follower rides on its leader's block, so it marks the place without adding
// monsters to the tally. Counting both would report twice the population that
// actually stands there.
func TestSeguidorNaoDobraAContagem(t *testing.T) {
	dir := t.TempDir()
	escreverConteudo(t, dir,
		"#\nLeader:\tChefe\nFollower:\tCapanga\nMaxNumMob:\t10\nMinuteGenerate:\t5\nStartX:\t1250\nStartY:\t3608\n",
		"1161, 3595, 1267, 3695 = Agua_M\n")

	idx, err := Build(dir, nil)
	if err != nil {
		t.Fatal(err)
	}
	chefe, capanga := idx["Chefe"], idx["Capanga"]
	if len(chefe) != 1 || len(capanga) != 1 {
		t.Fatalf("chefe=%d locais, capanga=%d locais, want 1 e 1", len(chefe), len(capanga))
	}
	if chefe[0].Amount != 10 {
		t.Errorf("chefe Amount = %d, want 10", chefe[0].Amount)
	}
	if capanga[0].Amount != 0 {
		t.Errorf("capanga Amount = %d, want 0 — os mobs são do bloco do líder", capanga[0].Amount)
	}
	if capanga[0].Place != "Água Místico" {
		t.Errorf("capanga Place = %q — o seguidor nasce onde o líder nasce", capanga[0].Place)
	}
}

// Dozens of blocks a few tiles apart are one place to a person. Without the
// merge the busiest templates render as forty near-identical rows.
func TestBlocosDoMesmoLugarViramUmaLinha(t *testing.T) {
	dir := t.TempDir()
	escreverConteudo(t, dir,
		"#\nLeader:\tGargula\nMaxNumMob:\t4\nMinuteGenerate:\t9\nStartX:\t1200\nStartY:\t3600\n"+
			"#\nLeader:\tGargula\nMaxNumMob:\t6\nMinuteGenerate:\t2\nStartX:\t1210\nStartY:\t3610\n",
		"1161, 3595, 1267, 3695 = Agua_M\n")

	idx, err := Build(dir, nil)
	if err != nil {
		t.Fatal(err)
	}
	o := idx["Gargula"]
	if len(o) != 1 {
		t.Fatalf("%d locais, want 1 — dois blocos na mesma região são uma linha", len(o))
	}
	if o[0].Points != 2 || o[0].Amount != 10 {
		t.Errorf("Points=%d Amount=%d, want 2 e 10", o[0].Points, o[0].Amount)
	}
	// The shortest period is the one a player feels standing there.
	if o[0].RespawnMin != 2 {
		t.Errorf("RespawnMin = %d, want 2", o[0].RespawnMin)
	}
}

// Busiest first: the moderator wants the main population, not whichever place
// the map iteration happened to yield first.
func TestOrdenadoPeloMaiorAmount(t *testing.T) {
	dir := t.TempDir()
	escreverConteudo(t, dir,
		"#\nLeader:\tX\nMaxNumMob:\t2\nStartX:\t1200\nStartY:\t3600\n"+
			"#\nLeader:\tX\nMaxNumMob:\t30\nStartX:\t2200\nStartY:\t2100\n",
		"1161, 3595, 1267, 3695 = Agua_M\n2164, 2054, 2684, 2170 = Armia\n")

	idx, err := Build(dir, nil)
	if err != nil {
		t.Fatal(err)
	}
	o := idx["X"]
	if len(o) != 2 {
		t.Fatalf("%d locais, want 2", len(o))
	}
	if o[0].Place != "Armia" || o[0].Amount != 30 {
		t.Errorf("primeiro = %q com %d, want Armia com 30", o[0].Place, o[0].Amount)
	}
}

// The whole point of the feature: a template nobody generates has no origins,
// and the caller must be able to see that rather than get a made-up place.
func TestTemplateSemGeradorFicaDeFora(t *testing.T) {
	dir := t.TempDir()
	escreverConteudo(t, dir, "#\nLeader:\tGargula\nMaxNumMob:\t4\nStartX:\t1200\nStartY:\t3600\n",
		"1161, 3595, 1267, 3695 = Agua_M\n")

	idx, err := Build(dir, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(idx["@@Gargula"]) != 0 {
		t.Errorf("@@Gargula não está no NPCGener e mesmo assim ganhou origem")
	}
}

// Regions.txt is optional. Losing it costs the dungeon names, not the feature.
func TestSemRegionsAindaFunciona(t *testing.T) {
	dir := t.TempDir()
	escreverConteudo(t, dir, "#\nLeader:\tGargula\nMaxNumMob:\t4\nStartX:\t2086\nStartY:\t2093\n", "")
	if err := os.Remove(filepath.Join(dir, "TMsrv", "run", "Regions.txt")); err != nil {
		t.Fatal(err)
	}

	idx, err := Build(dir, nil)
	if err != nil {
		t.Fatalf("Build devia sobreviver sem Regions.txt: %v", err)
	}
	if o := idx["Gargula"]; len(o) != 1 || o[0].Place != "Armia" {
		t.Errorf("origem = %+v, want Armia via mapzones", o)
	}
}

func TestSemNPCGenerEhErro(t *testing.T) {
	if _, err := Build(t.TempDir(), nil); err == nil {
		t.Error("sem NPCGener.txt não há índice nenhum — Build tinha que falhar")
	}
}

// The index is checked against the shipped content tree too: the format
// assumptions above hold for fixtures by construction, and the file they
// actually have to hold for is Release/.
func TestContraOConteudoReal(t *testing.T) {
	if _, err := os.Stat(filepath.Join("..", "..", "..", "Release", "TMsrv", "run", "NPCGener.txt")); err != nil {
		t.Skip("Release/ não está montado nesta máquina")
	}
	idx, err := Build(filepath.Join("..", "..", "..", "Release"), nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(idx) < 500 {
		t.Fatalf("%d templates com origem — o NPCGener nomeia 620, o parser está perdendo blocos", len(idx))
	}
	if o := idx["Gargula_"]; len(o) == 0 || o[0].Place != "Água Místico" {
		t.Errorf("Gargula_ = %+v, want Água Místico primeiro", o)
	}
	// The case that motivated the feature: the template the editor opens by
	// default sits beside the real one and spawns nowhere.
	if o := idx["@@Gargula"]; len(o) != 0 {
		t.Errorf("@@Gargula ganhou %d origens — o NPCGener não o cita", len(o))
	}
}

// escreverConteudo lays out the two content files Build reads.
func escreverConteudo(t *testing.T, dir, npcgener, regioes string) {
	t.Helper()
	run := filepath.Join(dir, "TMsrv", "run")
	if err := os.MkdirAll(run, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(run, "NPCGener.txt"), []byte(npcgener), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(run, "Regions.txt"), []byte(regioes), 0o644); err != nil {
		t.Fatal(err)
	}
}
