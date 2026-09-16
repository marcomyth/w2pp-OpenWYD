// Package clientacessorio prepara o cliente 7662 para os efeitos novos da reforma
// dos acessórios (2026-09-16): "% de dano físico" (código 89) e "% de dano mágico"
// (código 90), que só este servidor conhece.
//
// O tooltip de item do WYD.exe não nomeia as linhas em código: ele percorre uma
// lista fixa de 49 pares (código do efeito → ponteiro de rótulo) em .data —
// códigos em VA 0x60F354, rótulos em VA 0x60F418 — e imprime a linha de cada
// efeito que o item tem (WYD.exe 7662, laços em 0x41ABA8 e 0x41BC68, limite
// `cmp ..., 0x31`). Código fora da lista nunca aparece, então dois lugares da
// lista mudam de dono:
//
//   - entrada 17: EF_PARRY (40, "Índice de Evasão") → 89, rótulo strdef 991;
//   - entrada 40: EF_DAMAGEADD (67, que repete o rótulo 79 da entrada 41) → 90,
//     rótulo strdef 992.
//
// Nenhum item do catálogo carrega 40 nem 67, e nenhuma conta de atributo do
// cliente lê 40, 67, 89 ou 90 (as chamadas às getters de efeito foram varridas).
// Os registros 991 e 992 do strdef estão vazios e nenhum ponteiro do exe aponta
// para eles. O "%" vai no rótulo: o tooltip só o acrescenta sozinho a códigos
// fixos (26, 45, 46, 60).
//
// Este pacote não toca as entradas 28-35, que são do clientmount (montarias).
package clientacessorio

import (
	"bytes"
	"encoding/binary"
	"fmt"
)

const (
	exeSize       = 2347008
	tooltipCodes  = 0x20F354 // arquivo = VA − 0x400000
	tooltipLabels = 0x20F418
	strdefVA      = 0x109A4B8

	strdefSize   = 255992
	strdefRecord = 128

	// EfDanoFisico e EfDanoMagico são os códigos que o tooltip passa a mostrar;
	// os mesmos de internal/itemeffect.
	EfDanoFisico = 89
	EfDanoMagico = 90
)

type entrada struct {
	pos         int
	code, label uint32
}

func labelPtr(idx uint32) uint32 { return strdefVA + idx*strdefRecord }

// As duas entradas como o cliente as trouxe e como este pacote as deixa.
var (
	original = [2]entrada{{17, 40, labelPtr(174)}, {40, 67, labelPtr(79)}}
	gerada   = [2]entrada{{17, EfDanoFisico, labelPtr(991)}, {40, EfDanoMagico, labelPtr(992)}}
)

// PatchExe devolve uma cópia do WYD.exe com as duas linhas do tooltip. Aceita o
// exe como veio ou como este pacote já o deixou, e recusa qualquer outra coisa:
// escrever na lista de outra build corrompe o executável.
func PatchExe(exe []byte) ([]byte, error) {
	if len(exe) != exeSize {
		return nil, fmt.Errorf("clientacessorio: o WYD.exe tem %d bytes, e o gerador só conhece a build 7662 de %d", len(exe), exeSize)
	}
	for k := range original {
		got := ler(exe, original[k].pos)
		if got != original[k] && got != gerada[k] {
			return nil, fmt.Errorf("clientacessorio: a entrada %d do tooltip é o código %d com o rótulo 0x%X, "+
				"nem a original nem a gerada; alguém mexeu nela, e o gerador não vai adivinhar", got.pos, got.code, got.label)
		}
	}
	out := bytes.Clone(exe)
	for _, e := range gerada {
		binary.LittleEndian.PutUint32(out[tooltipCodes+4*e.pos:], e.code)
		binary.LittleEndian.PutUint32(out[tooltipLabels+4*e.pos:], e.label)
	}
	return out, nil
}

func ler(exe []byte, pos int) entrada {
	return entrada{
		pos:   pos,
		code:  binary.LittleEndian.Uint32(exe[tooltipCodes+4*pos:]),
		label: binary.LittleEndian.Uint32(exe[tooltipLabels+4*pos:]),
	}
}

// rotulos são os textos novos, em Windows-1252.
var rotulos = []struct {
	idx  int
	novo string
}{
	{991, "Dano f\xedsico (%)"},
	{992, "Dano m\xe1gico (%)"},
}

// PatchStrdef devolve uma cópia do UI\strdef.bin com os dois rótulos. Cada
// registro tem de estar vazio ou já com o texto deste pacote.
func PatchStrdef(sd []byte) ([]byte, error) {
	if len(sd) != strdefSize {
		return nil, fmt.Errorf("clientacessorio: o strdef.bin tem %d bytes, esperava %d", len(sd), strdefSize)
	}
	out := bytes.Clone(sd)
	for _, r := range rotulos {
		rec := out[r.idx*strdefRecord : (r.idx+1)*strdefRecord]
		vazio := bytes.Count(rec, []byte{0}) == strdefRecord
		if !vazio && texto(rec) != r.novo {
			return nil, fmt.Errorf("clientacessorio: o registro %d do strdef.bin já tem %q, e o gerador só escreve em registro vazio", r.idx, texto(rec))
		}
		clear(rec)
		copy(rec, r.novo)
	}
	return out, nil
}

func texto(rec []byte) string {
	if i := bytes.IndexByte(rec, 0); i >= 0 {
		return string(rec[:i])
	}
	return string(rec)
}

// ItensDaReforma são os registros do ItemList.bin que mudaram: bracelete,
// pingente, brinco e colar de Hércules e de Hecate, e bracelete, pingente e
// brinco de Zeus (Defesa nova; Zeus não tem colar).
var ItensDaReforma = []int{507, 513, 514, 519, 520, 521, 593, 594, 595, 642, 643}

const itemRecord = 140

// CopiarRegistros copia de novo para base só os registros de itens. É como o
// ItemList.bin do cliente recebe a reforma sem perder o que outros geradores
// gravaram nele (as montarias do painel, por exemplo).
func CopiarRegistros(base, novo []byte, itens []int) ([]byte, error) {
	if len(base) != len(novo) {
		return nil, fmt.Errorf("clientacessorio: ItemList.bin de tamanhos diferentes (%d e %d)", len(base), len(novo))
	}
	out := bytes.Clone(base)
	for _, i := range itens {
		at := i * itemRecord
		if at+itemRecord > len(out) {
			return nil, fmt.Errorf("clientacessorio: item %d fora do ItemList.bin", i)
		}
		copy(out[at:at+itemRecord], novo[at:at+itemRecord])
	}
	return out, nil
}
