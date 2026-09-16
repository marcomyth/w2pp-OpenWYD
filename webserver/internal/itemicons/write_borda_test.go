package itemicons

import (
	"image"
	"image/color"
	"os"
	"path/filepath"
	"testing"
)

// TestSetIconLimpaArteOrfa: a célula livre do cliente real tem arte órfã pintada
// até a borda. Um desenho menor que a célula não pode deixar essa borda à mostra.
func TestSetIconLimpaArteOrfa(t *testing.T) {
	dir := clienteFalso(t)
	orfa := image.NewNRGBA(image.Rect(0, 0, Columns*CellSize, (IconsPerAtlas/Columns)*CellSize))
	for y := range orfa.Bounds().Dy() {
		for x := range orfa.Bounds().Dx() {
			orfa.SetNRGBA(x, y, color.NRGBA{G: 200, A: 0xff})
		}
	}
	if err := os.WriteFile(filepath.Join(dir, "UI", "itemicon01.wyt"), encodeWYT(orfa), 0o644); err != nil {
		t.Fatal(err)
	}
	bmp := bmp24(t, dir, 32, 32, color.NRGBA{R: 200, A: 0xff})
	if _, err := SetIcon(dir, 762, bmp); err != nil {
		t.Fatalf("SetIcon: %v", err)
	}
	dados, err := os.ReadFile(filepath.Join(dir, "UI", "itemicon01.wyt"))
	if err != nil {
		t.Fatal(err)
	}
	atlas, err := decodeWYT(dados)
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range []image.Point{{0, 0}, {CellSize - 1, CellSize - 1}, {CellSize / 2, CellSize - 1}} {
		if c := atlas.NRGBAAt(p.X, p.Y); c.A != 0 {
			t.Errorf("borda %v = %+v, a arte órfã ficou à mostra", p, c)
		}
	}
	// A célula vizinha, que não foi tomada, continua como estava.
	if c := atlas.NRGBAAt(CellSize, 0); c.G != 200 {
		t.Errorf("célula vizinha = %+v, want intacta", c)
	}
}
