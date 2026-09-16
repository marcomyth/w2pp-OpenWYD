package panel

import (
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/jeanluca/w2pp-openwyd/adminserver/internal/audit"
	"github.com/jeanluca/w2pp-openwyd/adminserver/internal/gamedata"
	"github.com/jeanluca/w2pp-openwyd/adminserver/internal/personagem"
)

// The merchant page: one NPC and the 27 slots of its shop, edited one slot at a
// time the way the monster equipment is.
//
// The grid is laid out in the game's order — five columns, filled row by row —
// so a moderator holding a screenshot of the shop window finds the same item in
// the same cell. 0070_reforma_acessorios puts the Aki's five rings in slots
// 18-22, which is exactly the end of row four and the start of row five on the
// client.

// maxQtdLoja is the largest stack a shop slot sells (npcadmin maxQuantity).
const maxQtdLoja = 255

// npc shows one merchant, its shop as a grid, and the form for the chosen slot.
func (h *Handler) npc(w http.ResponseWriter, r *http.Request) {
	sess, _ := staffFrom(r.Context())
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		http.Error(w, "NPC inválido.", http.StatusBadRequest)
		return
	}

	n, err := h.cfg.GameData.NPC(r.Context(), sess.AccountID, id)
	if err != nil {
		h.recusaGameData(w, r, "carregar o NPC", err)
		return
	}

	catalogo := h.catalogo(r)
	linhas := gradeLoja(n.Shop, catalogo)
	sel := selecaoLoja(r, linhas, catalogo)
	if sel.Ativa {
		sel = h.buscaItens(r, sel)
	}

	h.render(w, "npc.html", struct {
		page
		NPC     gamedata.NPC
		Loja    []itemView
		Sel     selecao
		Overlay avisoOverlay
		Aviso   string
	}{h.pageFor(r, "npcs"), n, linhas, sel, h.overlayNPCs(r), r.URL.Query().Get("aviso")})
}

// gradeLoja renders every slot, not only the occupied ones: an empty cell is
// where stock gets added, and a grid that hid them would give no way in.
func gradeLoja(shop []gamedata.ShopItem, catalogo map[int32]gamedata.Item) []itemView {
	itens := make([]personagem.Item, gamedata.MaxShopSlot()+1)
	qtd := make([]int, len(itens))
	for i := range itens {
		itens[i].Slot = i
	}
	for _, s := range shop {
		if s.Slot < 0 || int(s.Slot) >= len(itens) {
			continue
		}
		itens[s.Slot] = itemDaLoja(s)
		qtd[s.Slot] = int(s.Quantity)
	}
	linhas := grade(itens, catalogo, 0, false)
	for i := range linhas {
		linhas[i].Qtd = qtd[i]
	}
	return linhas
}

// itemDaLoja converts one stock row to the cell shape the shared grid draws. The
// service stores effects as int32 but writes them from bytes, so the narrowing
// loses nothing.
func itemDaLoja(s gamedata.ShopItem) personagem.Item {
	return personagem.Item{
		Slot: int(s.Slot), Index: int16(s.ItemIndex),
		Eff1: uint8(s.Eff[0][0]), EffV1: uint8(s.Eff[0][1]),
		Eff2: uint8(s.Eff[1][0]), EffV2: uint8(s.Eff[1][1]),
		Eff3: uint8(s.Eff[2][0]), EffV3: uint8(s.Eff[2][1]),
	}
}

// selecaoLoja reads ?slot= (and ?indice= from a search pick) against the loaded
// shop, so the form comes back filled with what the slot actually sells.
func selecaoLoja(r *http.Request, linhas []itemView, catalogo map[int32]gamedata.Item) selecao {
	slot, err := strconv.Atoi(r.URL.Query().Get("slot"))
	if err != nil || slot < 0 || slot >= len(linhas) {
		return selecao{}
	}
	it := linhas[slot]
	// An index in the query means the operator just picked an item from the
	// search and expects to see THAT. Its effects and stack are the old item's,
	// so they do not carry over.
	if q := r.URL.Query().Get("indice"); q != "" {
		if n, err := strconv.Atoi(q); err == nil && n > 0 && n <= 32767 && int16(n) != it.Index {
			it = grade([]personagem.Item{{Slot: slot, Index: int16(n)}}, catalogo, 0, false)[0]
		}
	}
	if !it.Vazio && it.Qtd < 1 {
		it.Qtd = 1
	}
	return selecao{Ativa: true, Slot: slot, Item: it, Prefixo: fmt.Sprintf("slot=%d", slot)}
}

// setLoja writes or empties ONE shop slot.
//
// The service replaces the whole stock at once, so this reads the current shop
// and changes only the chosen slot. Posting the entire grid instead would let a
// page opened before someone else's save quietly undo it.
func (h *Handler) setLoja(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil || !h.checkCSRF(w, r) {
		if err != nil {
			http.Error(w, "Formulário ilegível.", http.StatusBadRequest)
		}
		return
	}
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		http.Error(w, "NPC inválido.", http.StatusBadRequest)
		return
	}
	slot, err := strconv.Atoi(strings.TrimSpace(r.PostFormValue("slot")))
	if err != nil || slot < 0 || slot > gamedata.MaxShopSlot() {
		http.Error(w, "Espaço da loja inválido.", http.StatusBadRequest)
		return
	}

	remover := r.PostFormValue("remover") == "1"
	var novo personagem.Item
	qtd := 1
	if !remover {
		if novo, err = itemDoForm(r, slot); err != nil {
			erroDeForma(w, err)
			return
		}
		if bruto := strings.TrimSpace(r.PostFormValue("qtd")); bruto != "" {
			qtd, err = strconv.Atoi(bruto)
			if err != nil || qtd < 1 || qtd > maxQtdLoja {
				http.Error(w, fmt.Sprintf("Quantidade inválida: vai de 1 a %d.", maxQtdLoja), http.StatusBadRequest)
				return
			}
		}
		// An empty index is how the character editor clears a slot; here it
		// would be a silent removal behind the "Gravar" button.
		if novo.Index == 0 {
			http.Error(w, "Escolha um item, ou use Esvaziar espaço.", http.StatusBadRequest)
			return
		}
	}

	sess, _ := staffFrom(r.Context())
	n, err := h.cfg.GameData.NPC(r.Context(), sess.AccountID, id)
	if err != nil {
		h.recusaGameData(w, r, "carregar o NPC", err)
		return
	}

	var antes int32
	itens := make([]gamedata.ShopItem, 0, len(n.Shop)+1)
	for _, s := range n.Shop {
		if int(s.Slot) == slot {
			antes = s.ItemIndex
			continue
		}
		itens = append(itens, s)
	}
	if !remover {
		itens = append(itens, gamedata.ShopItem{
			Slot: int32(slot), ItemIndex: int32(novo.Index), Quantity: int32(qtd),
			Eff: [3][2]int32{
				{int32(novo.Eff1), int32(novo.EffV1)},
				{int32(novo.Eff2), int32(novo.EffV2)},
				{int32(novo.Eff3), int32(novo.EffV3)},
			},
		})
	}

	if err := h.cfg.GameData.SetShop(r.Context(), sess.AccountID, id, itens); err != nil {
		h.recusaGameData(w, r, "gravar a loja", err)
		return
	}

	if err := h.cfg.Audit.Write(r.Context(), audit.Record{
		ActorID: sess.AccountID, ActorRole: roleFrom(r.Context()),
		Action: audit.ActionSetNpcShop,
		Old:    map[string]any{"npc_id": id, "slot": slot, "item": antes},
		New:    map[string]any{"npc_id": id, "slot": slot, "item": novo.Index, "qtd": qtd},
	}); err != nil {
		h.cfg.Logger.Error("shop changed but NOT audited", "npc", id, "err", err)
		http.Error(w, "A loja foi alterada, mas a auditoria falhou. Avise quem cuida do servidor.",
			http.StatusInternalServerError)
		return
	}

	h.cfg.Logger.Info("npc shop changed", "actor", sess.AccountName, "npc", id,
		"slot", slot, "before", antes, "after", novo.Index)
	msg := fmt.Sprintf("Espaço %d gravado. Entra em jogo em até 15 segundos.", slot)
	if remover {
		msg = fmt.Sprintf("Espaço %d esvaziado. Entra em jogo em até 15 segundos e continua vazio depois do reinício.", slot)
	}
	http.Redirect(w, r, fmt.Sprintf("/npcs/%d?slot=%d&aviso=%s", id, slot, url.QueryEscape(msg)),
		http.StatusSeeOther)
}
