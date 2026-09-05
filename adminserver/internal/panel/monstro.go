package panel

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/jeanluca/w2pp-openwyd/adminserver/internal/audit"
	"github.com/jeanluca/w2pp-openwyd/adminserver/internal/gamedata"
	"github.com/jeanluca/w2pp-openwyd/adminserver/internal/personagem"
)

// monstros lists the template files a moderator can rebalance.
func (h *Handler) monstros(w http.ResponseWriter, r *http.Request) {
	sess, _ := staffFrom(r.Context())
	q := r.URL.Query().Get("q")

	achados, err := h.cfg.GameData.MobTemplates(r.Context(), sess.AccountID, q)
	if err != nil {
		h.recusaGameData(w, r, "listar os monstros", err)
		return
	}
	truncado := len(achados) > itensLimit
	if truncado {
		achados = achados[:itensLimit]
	}

	h.render(w, "monstros.html", struct {
		page
		Query     string
		Templates []gamedata.MobTemplate
		Truncado  bool
		Limite    int
		Aviso     string
	}{h.pageFor(r, "monstros"), q, achados, truncado, itensLimit, r.URL.Query().Get("aviso")})
}

// monstro shows one template's numbers, grouped.
func (h *Handler) monstro(w http.ResponseWriter, r *http.Request) {
	sess, _ := staffFrom(r.Context())
	nome := r.PathValue("nome")

	stat, err := h.cfg.GameData.MobStat(r.Context(), sess.AccountID, nome)
	if err != nil {
		h.recusaGameData(w, r, "carregar o monstro", err)
		return
	}

	// Group the fields for rendering: one section per group, in table order.
	campos := stat.Fields()
	porGrupo := map[string][]gamedata.MobField{}
	for _, c := range campos {
		porGrupo[c.Grupo] = append(porGrupo[c.Grupo], c)
	}
	grupos := make([]grupoCampos, 0, len(gamedata.GruposMob()))
	for _, g := range gamedata.GruposMob() {
		grupos = append(grupos, grupoCampos{Nome: g, Campos: porGrupo[g]})
	}

	// The gear grid reuses the character editor's cells, so a moderator reads a
	// mob's equipment the same way they read a player's.
	equip := make([]personagem.Item, 0, maxMobEquipSlots)
	for _, e := range stat.Equip() {
		equip = append(equip, personagem.Item{
			Slot: e.Slot, Index: int16(e.ItemIndex),
			Eff1: e.Eff1, EffV1: e.EffV1,
			Eff2: e.Eff2, EffV2: e.EffV2,
			Eff3: e.Eff3, EffV3: e.EffV3,
		})
	}

	// Which gear slot the form is editing, picked by a plain link — the panel
	// serves default-src 'none' with no script-src, so nothing here may depend on
	// JavaScript. -1 means none picked yet.
	sel := -1
	if bruto := r.URL.Query().Get("slot"); bruto != "" {
		if n, err := strconv.Atoi(bruto); err == nil && n >= 0 && n < maxMobEquipSlots {
			sel = n
		}
	}
	linhas := grade(equip, h.catalogo(r), 0, false)
	var escolhido itemView
	if sel >= 0 && sel < len(linhas) {
		escolhido = linhas[sel]
	}

	h.render(w, "monstro.html", struct {
		page
		Nome       string
		Exibido    string
		Overridden bool
		Grupos     []grupoCampos
		Equip      []itemView
		Sel        int
		Escolhido  itemView
		Aviso      string
	}{
		h.pageFor(r, "monstros"), stat.Name(), stat.DisplayName(), stat.Overridden(),
		grupos, linhas, sel, escolhido, r.URL.Query().Get("aviso"),
	})
}

// maxMobEquipSlots is MAX_EQUIP (STRUCT_MOB.Equip[16]).
const maxMobEquipSlots = 16

// setMonstroEquip replaces one Equip[] slot of a mob template.
//
// Unlike a character's inventory there is no presence to check: this is a
// template, read once at boot and never owned by the loop. The cost is the
// opposite — the change reaches the world only on the next restart, which is
// also how the legacy EDITAPPMOB behaved.
func (h *Handler) setMonstroEquip(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil || !h.checkCSRF(w, r) {
		if err != nil {
			http.Error(w, "Formulário ilegível.", http.StatusBadRequest)
		}
		return
	}
	sess, _ := staffFrom(r.Context())
	nome := r.PathValue("nome")

	stat, err := h.cfg.GameData.MobStat(r.Context(), sess.AccountID, nome)
	if err != nil {
		h.recusaGameData(w, r, "carregar o monstro", err)
		return
	}

	slot, err := strconv.Atoi(r.PostFormValue("slot"))
	if err != nil || slot < 0 || slot >= 16 {
		http.Error(w, "Slot inválido.", http.StatusBadRequest)
		return
	}
	novo, err := itemDoForm(r, slot)
	if err != nil {
		erroDeForma(w, err)
		return
	}
	if r.PostFormValue("remover") == "1" {
		novo = personagem.Item{Slot: slot}
	}

	atual := stat.Equip()
	antes := atual[slot]
	atual[slot] = gamedata.MobEquipItem{
		Slot: slot, ItemIndex: int32(novo.Index),
		Eff1: novo.Eff1, EffV1: novo.EffV1,
		Eff2: novo.Eff2, EffV2: novo.EffV2,
		Eff3: novo.Eff3, EffV3: novo.EffV3,
	}

	// The webServer refuses equipment for a template with no stat override yet,
	// so save the stats first. They are the file's own values at this point, so
	// this freezes the current defaults rather than changing anything.
	if !stat.Overridden() {
		if err := h.cfg.GameData.SaveMobStat(r.Context(), sess.AccountID, stat); err != nil {
			h.recusaGameData(w, r, "gravar o monstro", err)
			return
		}
	}
	if err := h.cfg.GameData.SaveMobEquip(r.Context(), sess.AccountID, nome, atual); err != nil {
		h.recusaGameData(w, r, "gravar o equipamento", err)
		return
	}
	if err := h.cfg.Audit.Write(r.Context(), audit.Record{
		ActorID: sess.AccountID, ActorRole: roleFrom(r.Context()),
		Action: acaoMobEquip,
		Old:    map[string]any{"template": nome, "slot": slot, "item": antes.ItemIndex},
		New:    map[string]any{"template": nome, "slot": slot, "item": novo.Index},
	}); err != nil {
		h.auditoriaFalhou(w, err)
		return
	}
	h.redirectMonstro(w, r, nome, "Equipamento gravado. Vale no próximo reinício do servidor.")
}

// grupoCampos is one titled section of the stat form.
type grupoCampos struct {
	Nome   string
	Campos []gamedata.MobField
}

// setMonstro saves the override.
//
// It reads the current values first and writes onto them. Upsert replaces the
// whole row, so a field the form does not carry — the equipment list above all,
// which has its own RPC and no place in this form — would be zeroed by building
// a fresh message from the request.
func (h *Handler) setMonstro(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil || !h.checkCSRF(w, r) {
		if err != nil {
			http.Error(w, "Formulário ilegível.", http.StatusBadRequest)
		}
		return
	}
	sess, _ := staffFrom(r.Context())
	nome := r.PathValue("nome")

	stat, err := h.cfg.GameData.MobStat(r.Context(), sess.AccountID, nome)
	if err != nil {
		h.recusaGameData(w, r, "carregar o monstro", err)
		return
	}

	mudados := map[string]any{}
	for _, c := range stat.Fields() {
		bruto := strings.TrimSpace(r.PostFormValue(c.Nome))
		if bruto == "" {
			continue // field absent from the form: leave it as it was
		}
		v, err := strconv.ParseInt(bruto, 10, 64)
		if err != nil {
			http.Error(w, "Valor inválido em "+c.Rotulo+".", http.StatusBadRequest)
			return
		}
		if v != c.Valor {
			mudados[c.Nome] = v
		}
		stat.Set(c.Nome, v)
	}
	// Present-but-empty clears the name; absent leaves it alone. Without the
	// Has check any partial post would silently wipe it, which is the opposite
	// of how every number above behaves.
	if r.PostForm.Has("display_name") {
		exibido := strings.TrimSpace(r.PostFormValue("display_name"))
		if exibido != stat.DisplayName() {
			mudados["display_name"] = exibido
		}
		stat.SetDisplayName(exibido)
	}

	if err := h.cfg.GameData.SaveMobStat(r.Context(), sess.AccountID, stat); err != nil {
		h.recusaGameData(w, r, "gravar o monstro", err)
		return
	}
	if err := h.cfg.Audit.Write(r.Context(), audit.Record{
		ActorID: sess.AccountID, ActorRole: roleFrom(r.Context()),
		Action: audit.ActionSetMobStat,
		New:    map[string]any{"template": nome, "campos": mudados},
	}); err != nil {
		h.cfg.Logger.Error("mob stat changed but NOT audited", "template", nome, "err", err)
		http.Error(w, "O monstro foi alterado, mas a auditoria falhou. Avise quem cuida do servidor.",
			http.StatusInternalServerError)
		return
	}

	h.cfg.Logger.Info("mob stat changed", "actor", sess.AccountName, "template", nome, "campos", len(mudados))
	h.redirectMonstro(w, r, nome,
		"Gravado. Só entra em jogo depois de reiniciar o servidor — o aviso está na página inicial.")
}

// limparMonstro drops the override, restoring the template file's values.
func (h *Handler) limparMonstro(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil || !h.checkCSRF(w, r) {
		if err != nil {
			http.Error(w, "Formulário ilegível.", http.StatusBadRequest)
		}
		return
	}
	sess, _ := staffFrom(r.Context())
	nome := r.PathValue("nome")

	if err := h.cfg.GameData.ClearMobStat(r.Context(), sess.AccountID, nome); err != nil {
		h.recusaGameData(w, r, "limpar o monstro", err)
		return
	}
	if err := h.cfg.Audit.Write(r.Context(), audit.Record{
		ActorID: sess.AccountID, ActorRole: roleFrom(r.Context()),
		Action: audit.ActionClearMobStat,
		Old:    map[string]any{"template": nome},
	}); err != nil {
		h.cfg.Logger.Error("mob stat cleared but NOT audited", "template", nome, "err", err)
		http.Error(w, "O monstro foi restaurado, mas a auditoria falhou. Avise quem cuida do servidor.",
			http.StatusInternalServerError)
		return
	}
	h.redirectMonstro(w, r, nome,
		"Valores do arquivo restaurados. Só entra em jogo depois de reiniciar o servidor.")
}

func (h *Handler) redirectMonstro(w http.ResponseWriter, r *http.Request, nome, msg string) {
	http.Redirect(w, r, "/monstros/"+urlPath(nome)+"?aviso="+urlQuery(msg), http.StatusSeeOther)
}
