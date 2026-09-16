package clientacessorio

import (
	"encoding/binary"
	"strings"
	"testing"
)

func exeOriginal() []byte {
	b := make([]byte, exeSize)
	for _, e := range original {
		binary.LittleEndian.PutUint32(b[tooltipCodes+4*e.pos:], e.code)
		binary.LittleEndian.PutUint32(b[tooltipLabels+4*e.pos:], e.label)
	}
	return b
}

func TestPatchExeTrocaAsDuasLinhas(t *testing.T) {
	out, err := PatchExe(exeOriginal())
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range gerada {
		if got := ler(out, e.pos); got != e {
			t.Errorf("entrada %d = %+v, esperado %+v", e.pos, got, e)
		}
	}
	// Bytes que o relatório de desmontagem conferiu: código 89 e rótulo do strdef 991.
	if got := out[0x20F398]; got != 0x59 {
		t.Errorf("código da entrada 17 = 0x%X, esperado 0x59", got)
	}
	if got := binary.LittleEndian.Uint32(out[0x20F45C:]); got != 0x010B9438 {
		t.Errorf("rótulo da entrada 17 = 0x%X, esperado 0x010B9438", got)
	}
	// Rodar de novo sobre o gerado não muda nada.
	again, err := PatchExe(out)
	if err != nil {
		t.Fatalf("segunda passada recusou o exe já gerado: %v", err)
	}
	if string(again) != string(out) {
		t.Error("segunda passada mudou o exe")
	}
}

func TestPatchExeRecusaListaDesconhecida(t *testing.T) {
	b := exeOriginal()
	binary.LittleEndian.PutUint32(b[tooltipCodes+4*17:], 42)
	if _, err := PatchExe(b); err == nil {
		t.Fatal("aceitou uma lista que não é nem a original nem a gerada")
	}
	if _, err := PatchExe(make([]byte, 10)); err == nil {
		t.Fatal("aceitou um exe de outro tamanho")
	}
}

func TestPatchStrdef(t *testing.T) {
	out, err := PatchStrdef(make([]byte, strdefSize))
	if err != nil {
		t.Fatal(err)
	}
	if got := texto(out[991*strdefRecord:]); got != "Dano f\xedsico (%)" {
		t.Errorf("991 = %q", got)
	}
	if got := texto(out[992*strdefRecord:]); got != "Dano m\xe1gico (%)" {
		t.Errorf("992 = %q", got)
	}
	if _, err := PatchStrdef(out); err != nil {
		t.Errorf("segunda passada recusou: %v", err)
	}
	ocupado := make([]byte, strdefSize)
	copy(ocupado[991*strdefRecord:], "outro texto")
	if _, err := PatchStrdef(ocupado); err == nil || !strings.Contains(err.Error(), "991") {
		t.Errorf("sobrescreveu um registro ocupado: %v", err)
	}
}

func TestCopiarRegistrosSoMexeNosItens(t *testing.T) {
	base := make([]byte, 6500*itemRecord+4)
	novo := make([]byte, len(base))
	for i := range novo {
		novo[i] = 7
	}
	out, err := CopiarRegistros(base, novo, []int{595})
	if err != nil {
		t.Fatal(err)
	}
	if out[595*itemRecord] != 7 || out[596*itemRecord] != 0 || out[595*itemRecord-1] != 0 {
		t.Error("copiou fora do registro 595")
	}
}
