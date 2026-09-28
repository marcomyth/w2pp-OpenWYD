//go:build integration

package store

import (
	"context"
	"testing"
)

// forcaEstadoDoRepasse põe a linha num estado à força, por SQL.
//
// POR QUE ELE EXISTE. ENVIADO e INCERTO não têm mais quem os escreva: eles nasciam do
// saque automático, que saiu em 25/09/2026. Mas as linhas existem em produção, e a tela
// da staff que as resolve continua de pé — então os testes dela precisam de um jeito de
// MONTAR esses estados sem uma função de produção que ninguém mais chama.
//
// É de propósito que ele mora num arquivo de teste. Uma função de produção capaz de
// empurrar um repasse para ENVIADO seria exatamente o código morto que move dinheiro que
// este PR foi apagar — e a primeira pessoa a encontrá-la ia supor que serve para usar.
func forcaEstadoDoRepasse(ctx context.Context, t *testing.T, s *Store, id int64, estado EstadoRepasse) {
	t.Helper()
	tag, err := s.pool.Exec(ctx,
		`UPDATE rmt_repasse SET status = $2 WHERE id = $1`, id, int16(estado))
	if err != nil {
		t.Fatalf("forcando o repasse %d para o estado %d: %v", id, estado, err)
	}
	if tag.RowsAffected() != 1 {
		t.Fatalf("forcando o repasse %d: %d linhas mexidas", id, tag.RowsAffected())
	}
}
