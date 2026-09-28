package store

import (
	"testing"

	"github.com/jeanluca/w2pp-openwyd/internal/domain"
)

func TestDonateShopPayloadEntregaPrazoNaoIniciado(t *testing.T) {
	casos := []struct {
		nome    string
		oferta  domain.DonateShopItem
		want    itemPayload
		comVenc bool // expires_at absoluto, o caminho antigo
	}{
		{
			nome:   "permanente fica como veio",
			oferta: domain.DonateShopItem{ItemIndex: 3393, Eff1: 61, EffV1: 5},
			want:   itemPayload{ItemIndex: 3393, Eff1: 61, EffV1: 5},
		},
		{
			nome:   "fada de 5 dias vira EF_WDAY no primeiro slot livre",
			oferta: domain.DonateShopItem{ItemIndex: 3901, ExpiresDays: 5},
			want:   itemPayload{ItemIndex: 3901, Eff1: efWDay, EffV1: 5},
		},
		{
			nome:   "prazo pula o slot ocupado",
			oferta: domain.DonateShopItem{ItemIndex: 1234, Eff1: 43, EffV1: 9, ExpiresDays: 7},
			want:   itemPayload{ItemIndex: 1234, Eff1: 43, EffV1: 9, Eff2: efWDay, EffV2: 7},
		},
		{
			nome:   "oferta que já traz EF_WDAY não ganha um segundo prazo",
			oferta: domain.DonateShopItem{ItemIndex: 3980, Eff1: efWDay, EffV1: 3, ExpiresDays: 30},
			want:   itemPayload{ItemIndex: 3980, Eff1: efWDay, EffV1: 3},
		},
		{
			nome: "sem slot livre cai no vencimento absoluto",
			oferta: domain.DonateShopItem{ItemIndex: 1234, Eff1: 1, EffV1: 1, Eff2: 2, EffV2: 2, Eff3: 3, EffV3: 3,
				ExpiresDays: 7},
			want:    itemPayload{ItemIndex: 1234, Eff1: 1, EffV1: 1, Eff2: 2, EffV2: 2, Eff3: 3, EffV3: 3},
			comVenc: true,
		},
		{
			nome:   "empilhável sem quantidade ganha EF_AMOUNT 1",
			oferta: domain.DonateShopItem{ItemIndex: 4140},
			want:   itemPayload{ItemIndex: 4140, Eff1: 61, EffV1: 1},
		},
		{
			nome:   "empilhável com quantidade fica como veio",
			oferta: domain.DonateShopItem{ItemIndex: 4140, Eff1: 61, EffV1: 10},
			want:   itemPayload{ItemIndex: 4140, Eff1: 61, EffV1: 10},
		},
		{
			nome:   "não empilhável sem quantidade não ganha nada",
			oferta: domain.DonateShopItem{ItemIndex: 3343},
			want:   itemPayload{ItemIndex: 3343},
		},
		{
			nome:    "mais de 255 dias não cabe no byte",
			oferta:  domain.DonateShopItem{ItemIndex: 1234, ExpiresDays: 365},
			want:    itemPayload{ItemIndex: 1234},
			comVenc: true,
		},
	}
	for _, c := range casos {
		t.Run(c.nome, func(t *testing.T) {
			got := donateShopPayload(c.oferta)
			if c.comVenc {
				if got.ExpiresAt == 0 {
					t.Errorf("expires_at = 0, want um vencimento absoluto")
				}
				got.ExpiresAt = 0
			}
			if got != c.want {
				t.Errorf("payload = %+v, want %+v", got, c.want)
			}
		})
	}
}
