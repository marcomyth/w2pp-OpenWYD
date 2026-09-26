package gamedata

import (
	"context"
	"testing"

	"google.golang.org/grpc"

	webv1 "github.com/jeanluca/w2pp-openwyd/api/web/v1"
)

// npcAdminFalso responde GetNpc com uma loja fixa e guarda o que SetNpcShop
// recebeu. Os outros métodos da interface ficam no valor embutido, que é nil: um
// chamado inesperado derruba o teste em vez de passar calado.
type npcAdminFalso struct {
	webv1.NpcAdminServiceClient
	loja     []*webv1.AdminNpcShopItem
	gravados []*webv1.AdminNpcShopItem
}

func (f *npcAdminFalso) GetNpc(context.Context, *webv1.GetNpcRequest, ...grpc.CallOption) (*webv1.GetNpcResponse, error) {
	return &webv1.GetNpcResponse{Npc: &webv1.AdminNpc{Id: 6, TemplateName: "God_of_War", Shop: f.loja}}, nil
}

func (f *npcAdminFalso) SetNpcShop(_ context.Context, req *webv1.SetNpcShopRequest, _ ...grpc.CallOption) (*webv1.AdminAck, error) {
	f.gravados = req.GetItems()
	return &webv1.AdminAck{}, nil
}

// O preço em pontos faz a viagem inteira: chega na leitura do NPC e volta na
// gravação da loja. O painel grava a loja TODA a cada vaga editada, então um
// preço perdido em qualquer das duas pontas era apagado das outras vagas — foi
// assim até 26/09/2026, quando a Loja de Honra passou a morar nessas vagas.
func TestPrecoEmPontosIdaEVolta(t *testing.T) {
	cem := int32(100)
	falso := &npcAdminFalso{loja: []*webv1.AdminNpcShopItem{
		{Slot: 0, ItemIndex: 413, Quantity: 1, PricePoints: &cem},
		{Slot: 1, ItemIndex: 1100, Quantity: 1}, // em ouro
	}}
	c := &Client{npc: falso}

	n, err := c.NPC(context.Background(), 1, 6)
	if err != nil {
		t.Fatalf("NPC: %v", err)
	}
	if len(n.Shop) != 2 {
		t.Fatalf("loja com %d vagas, quer 2", len(n.Shop))
	}
	if p := n.Shop[0].PricePoints; p == nil || *p != 100 {
		t.Fatalf("vaga 0 lida com preço em pontos %v, quer 100", p)
	}
	if p := n.Shop[1].PricePoints; p != nil {
		t.Fatalf("vaga 1, em ouro, lida com preço em pontos %d", *p)
	}

	if err := c.SetShop(context.Background(), 1, 6, n.Shop); err != nil {
		t.Fatalf("SetShop: %v", err)
	}
	if len(falso.gravados) != 2 {
		t.Fatalf("gravou %d vagas, quer 2", len(falso.gravados))
	}
	if p := falso.gravados[0].PricePoints; p == nil || *p != 100 {
		t.Errorf("vaga 0 gravada com preço em pontos %v, quer 100", p)
	}
	if falso.gravados[1].PricePoints != nil {
		t.Errorf("vaga 1 gravada em pontos (%d); era ouro", falso.gravados[1].GetPricePoints())
	}
}
