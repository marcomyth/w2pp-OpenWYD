package itemicons

import (
	"encoding/binary"
	"image"
	"image/color"
	"os"
	"path/filepath"
	"testing"
)

// TestSetIconLimpaArteOrfa: a célula livre do cliente real tem arte órfã pintada
// até a borda. Um desenho menor que a célula não pode deixar essa borda à mostra:
// ela vira o fundo preto opaco dos ícones do cliente.
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
		if c := pixelGravado(t, dados, p.X, p.Y); c != (color.NRGBA{A: 0xff}) {
			t.Errorf("borda %v = %+v, want preto opaco", p, c)
		}
	}
	// A célula vizinha, que não foi tomada, continua como estava.
	if c := atlas.NRGBAAt(CellSize, 0); c.G != 200 {
		t.Errorf("célula vizinha = %+v, want intacta", c)
	}
}

// TestSetIconPretoDoDesenhoFicaOpaco: o preto do desenho é fundo, não buraco.
// Transparente, ele deixava a moldura de raridade do slot aparecer atrás.
func TestSetIconPretoDoDesenhoFicaOpaco(t *testing.T) {
	dir := clienteFalso(t)
	bmp := bmp24(t, dir, 32, 32, color.NRGBA{A: 0xff})
	if _, err := SetIcon(dir, 762, bmp); err != nil {
		t.Fatalf("SetIcon: %v", err)
	}
	dados, err := os.ReadFile(filepath.Join(dir, "UI", "itemicon01.wyt"))
	if err != nil {
		t.Fatal(err)
	}
	if c := pixelGravado(t, dados, CellSize/2, CellSize/2); c.A != 0xff {
		t.Errorf("preto do desenho = %+v, want opaco", c)
	}
}

// pixelGravado lê o pixel como o cliente o recebe, direto dos bytes do TGA. O
// decodeWYT não serve aqui: ele é o da exportação web e apaga o preto.
func pixelGravado(t *testing.T, wyt []byte, x, y int) color.NRGBA {
	t.Helper()
	tga := wyt[len(standardWYTWrapper):]
	w := int(binary.LittleEndian.Uint16(tga[12:]))
	h := int(binary.LittleEndian.Uint16(tga[14:]))
	i := 18 + int(tga[0]) + ((h-1-y)*w+x)*4
	return color.NRGBA{B: tga[i], G: tga[i+1], R: tga[i+2], A: tga[i+3]}
}

// TestSetIconNaoApagaOPretoDoAtlas: o atlas é lido pelo decodificador web, que
// apaga o preto. Gravar de volta não pode levar isso para os ícones vizinhos.
func TestSetIconNaoApagaOPretoDoAtlas(t *testing.T) {
	dir := clienteFalso(t) // atlas inteiro preto opaco, como o do cliente
	bmp := bmp24(t, dir, 32, 32, color.NRGBA{R: 200, A: 0xff})
	if _, err := SetIcon(dir, 762, bmp); err != nil {
		t.Fatalf("SetIcon: %v", err)
	}
	dados, err := os.ReadFile(filepath.Join(dir, "UI", "itemicon01.wyt"))
	if err != nil {
		t.Fatal(err)
	}
	if c := pixelGravado(t, dados, 5*CellSize+3, 4*CellSize+3); c != (color.NRGBA{A: 0xff}) {
		t.Errorf("pixel de outro ícone = %+v, want preto opaco", c)
	}
}
