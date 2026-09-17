package grpcsrv

import (
	"context"
	"testing"

	"google.golang.org/protobuf/proto"

	webv1 "github.com/jeanluca/w2pp-openwyd/api/web/v1"
	"github.com/jeanluca/w2pp-openwyd/internal/domain"
)

// TestSetSemTetoDaRodadaGuardaOQueEstava: um portal que não conhece o teto não o
// desliga ao salvar os outros eventos.
func TestSetSemTetoDaRodadaGuardaOQueEstava(t *testing.T) {
	guardado := domain.DefaultWorldEventConfig()
	adm := &fakeWorldEventAdmin{guardado: guardado}
	_, err := NewWorldEventAdmin(adm).SetWorldEventConfig(context.Background(), &webv1.SetWorldEventConfigRequest{
		ModeratorId: 1,
		Config: &webv1.WorldEventConfig{
			TowerWarEnabled: proto.Bool(true), TowerWarHour: proto.Int32(20), BossRespawnHours: proto.Int32(24),
			RoundXpCap: []int64{1, 2}, // tamanho errado conta como ausente
		},
	})
	if err != nil || adm.gravado == nil {
		t.Fatalf("SetWorldEventConfig err = %v, gravado = %v", err, adm.gravado)
	}
	if adm.gravado.RoundXPCap != guardado.RoundXPCap || adm.gravado.RoundXPCapDouble != guardado.RoundXPCapDouble {
		t.Errorf("teto gravado %v / %v, want o guardado", adm.gravado.RoundXPCap, adm.gravado.RoundXPCapDouble)
	}
}

// TestSetComTetoDaRodadaGravaOPedido: cinco valores valem como vieram, e a
// leitura devolve sempre os cinco.
func TestSetComTetoDaRodadaGravaOPedido(t *testing.T) {
	adm := &fakeWorldEventAdmin{guardado: domain.DefaultWorldEventConfig()}
	_, err := NewWorldEventAdmin(adm).SetWorldEventConfig(context.Background(), &webv1.SetWorldEventConfigRequest{
		ModeratorId: 1,
		Config: &webv1.WorldEventConfig{
			TowerWarEnabled: proto.Bool(true), TowerWarHour: proto.Int32(20), BossRespawnHours: proto.Int32(24),
			RoundXpCap: []int64{5, 4, 3, 2, 1}, RoundXpCapDouble: []int64{10, 8, 6, 4, 2},
		},
	})
	if err != nil || adm.gravado == nil {
		t.Fatalf("SetWorldEventConfig err = %v, gravado = %v", err, adm.gravado)
	}
	if adm.gravado.RoundXPCap != [5]int64{5, 4, 3, 2, 1} || adm.gravado.RoundXPCapDouble != [5]int64{10, 8, 6, 4, 2} {
		t.Errorf("teto gravado %v / %v", adm.gravado.RoundXPCap, adm.gravado.RoundXPCapDouble)
	}
	got := worldEventConfigToWebProto(domain.DefaultWorldEventConfig())
	if len(got.RoundXpCap) != 5 || len(got.RoundXpCapDouble) != 5 || got.RoundXpCap[2] != 500454 {
		t.Errorf("leitura = %v / %v, want os cinco valores", got.RoundXpCap, got.RoundXpCapDouble)
	}
}
