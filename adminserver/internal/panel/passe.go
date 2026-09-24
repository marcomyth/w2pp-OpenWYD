package panel

import (
	"context"
	"errors"
	"net/http"
	"strconv"

	"github.com/jeanluca/w2pp-openwyd/adminserver/internal/audit"
	"github.com/jeanluca/w2pp-openwyd/internal/store"
)

// PasseDaConta é a gravação do nível do passe (satisfeita por *store.Store).
//
// Interface própria e não um método a mais no Writer porque o passe não é
// administração de conta: é um cosmético comprado, e amanhã quem o dá é o site
// sozinho, quando a doação for confirmada. Separado, esse caminho novo não precisa
// passar pelo painel.
type PasseDaConta interface {
	DefinirPasseNivel(ctx context.Context, accountID int64, nivel int16) error
}

// setPasse grava o nível do passe e, se a pessoa estiver jogando, troca a moldura na
// hora.
//
// SÃO DOIS PASSOS COM PESOS DIFERENTES, e a ordem é a regra:
//
//  1. O BANCO É A VERDADE. Gravado, o nível vale — inclusive se tudo abaixo falhar.
//  2. O JOGO É CORTESIA. Redesenhar a moldura sem relogar é um agrado; falhar nisso
//     não desfaz a compra de ninguém, e a moldura aparece no próximo login.
//
// Inverter isso faria uma falha de rede com o servidor de jogo cancelar uma coisa que
// a pessoa pagou.
func (h *Handler) setPasse(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil || !h.checkCSRF(w, r) {
		if err != nil {
			http.Error(w, "Formulário ilegível.", http.StatusBadRequest)
		}
		return
	}
	nome, auth, ok := h.alvo(w, r)
	if !ok {
		return
	}
	sess, _ := staffFrom(r.Context())

	nivel, err := strconv.Atoi(r.PostFormValue("nivel"))
	if err != nil || nivel < 0 || nivel > store.PasseNivelMax {
		h.redirectConta(w, r, nome, "Nível inválido: escolha de 0 a 4.")
		return
	}

	if err := h.cfg.Passe.DefinirPasseNivel(r.Context(), auth.ID, int16(nivel)); err != nil {
		if errors.Is(err, store.ErrPasseNivelInvalido) {
			h.redirectConta(w, r, nome, "Nível inválido: escolha de 0 a 4.")
			return
		}
		h.cfg.Logger.Error("passe: gravar falhou", "conta", nome, "err", err)
		http.Error(w, "Erro ao gravar o nível do passe.", http.StatusInternalServerError)
		return
	}

	if err := h.cfg.Audit.Write(r.Context(), audit.Record{
		ActorID: sess.AccountID, ActorRole: roleFrom(r.Context()),
		Action: audit.ActionSetPasse, TargetID: auth.ID,
		New: map[string]any{"passe_nivel": nivel},
	}); err != nil {
		// Gravado e não registrado é a mudança que ninguém consegue explicar depois.
		// Erro visível, mesmo com a escrita já feita: melhor alguém conferir do que
		// um log com buraco.
		h.cfg.Logger.Error("passe gravado mas NAO auditado", "conta", nome, "err", err)
		http.Error(w, "O passe foi gravado, mas a auditoria falhou. Avise quem cuida do servidor.",
			http.StatusInternalServerError)
		return
	}

	aviso := "Passe gravado. A moldura aparece no próximo login."
	// A CORTESIA, e ela só existe se o painel tiver link com o jogo. Sem link, ou com
	// a pessoa desconectada, o texto diz a verdade em vez de prometer o que não
	// aconteceu.
	if h.cfg.Jogo != nil {
		if p, err := h.cfg.Jogo.TrocarPasse(r.Context(), nome, int32(nivel)); err != nil {
			// A gravação já valeu. Isto vira aviso e não erro, porque o que falhou foi
			// o agrado — e dizer "deu erro" faria a staff gravar de novo à toa.
			h.cfg.Logger.Warn("passe gravado, mas a troca em jogo falhou",
				"conta", nome, "err", err)
			aviso = "Passe gravado. Não consegui falar com o servidor de jogo, " +
				"então a moldura só aparece no próximo login."
		} else if p.Conectado {
			aviso = "Passe gravado, e a moldura já mudou em jogo."
			if p.Personagem != "" {
				aviso = "Passe gravado, e a moldura de " + p.Personagem + " já mudou."
			}
		}
	}

	h.cfg.Logger.Info("passe alterado", "ator", sess.AccountName, "conta", nome, "nivel", nivel)
	h.redirectConta(w, r, nome, aviso)
}
