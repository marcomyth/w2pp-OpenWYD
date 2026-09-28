package panel

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/jeanluca/w2pp-openwyd/adminserver/internal/audit"
	"github.com/jeanluca/w2pp-openwyd/internal/store"
)

// A TELA DE QUEM ADMINISTRA, separada de quem joga (migração 0130).
//
// Antes disto, dar acesso ao painel era dar CARGO a uma conta de jogo — e isso obrigava
// quem administra a ter personagem, fazia tirar o cargo tirar as duas coisas de uma vez, e
// num servidor novo travava tudo, porque criar cargo exige o painel e abrir o painel exige
// cargo.
//
// SÓ ADMIN, e não staff. Criar usuário de painel é dar acesso ao painel: é a única ação
// desta ferramenta que fabrica mais gente com poder sobre ela.

// usuarioView é uma linha da tabela como a página mostra.
type usuarioView struct {
	store.UsuarioDoPainel
	// Eu diz se a linha é a da pessoa que está olhando. A tela usa isso para não oferecer
	// o botão de desativar a si mesma — quem se desativa sai do painel no clique seguinte
	// e, se for o único admin, não há por onde voltar.
	Eu bool
}

func (h *Handler) usuariosDoPainel(w http.ResponseWriter, r *http.Request) {
	if h.cfg.Painel == nil {
		http.NotFound(w, r)
		return
	}
	sess, _ := staffFrom(r.Context())
	lista, err := h.cfg.Painel.ListarUsuariosDoPainel(r.Context())
	if err != nil {
		h.cfg.Logger.Error("listando usuarios do painel", "err", err)
		http.Error(w, "Erro interno.", http.StatusInternalServerError)
		return
	}
	_, admins, err := h.cfg.Painel.ContarUsuariosDoPainel(r.Context())
	if err != nil {
		h.cfg.Logger.Error("contando usuarios do painel", "err", err)
		http.Error(w, "Erro interno.", http.StatusInternalServerError)
		return
	}

	linhas := make([]usuarioView, 0, len(lista))
	for _, u := range lista {
		linhas = append(linhas, usuarioView{
			UsuarioDoPainel: u,
			Eu:              sess.EhDoPainel() && u.ID == sess.PainelUsuarioID,
		})
	}
	h.render(w, "usuariosdopainel.html", struct {
		page
		Usuarios     []usuarioView
		AdminsAtivos int
		Aviso        string
	}{h.pageFor(r, "usuarios-do-painel"), linhas, admins, r.URL.Query().Get("aviso")})
}

// criarUsuarioDoPainel grava um usuário novo.
//
// A SENHA VEM DO FORMULÁRIO e é gravada cifrada pelo store, com o mesmo argon2id do jogo e
// do site. Ela NÃO volta para a tela e não vai para log nenhum: quem criou combina a senha
// com a pessoa por fora, e a pessoa troca depois.
func (h *Handler) criarUsuarioDoPainel(w http.ResponseWriter, r *http.Request) {
	if h.cfg.Painel == nil {
		http.NotFound(w, r)
		return
	}
	if err := r.ParseForm(); err != nil || !h.checkCSRF(w, r) {
		if err != nil {
			http.Error(w, "Formulário ilegível.", http.StatusBadRequest)
		}
		return
	}
	sess, _ := staffFrom(r.Context())
	login := r.PostFormValue("login")
	senha := r.PostFormValue("senha")
	papel := r.PostFormValue("papel")

	if len(senha) < senhaMinimaDoPainel {
		h.voltaParaUsuarios(w, r, "A senha precisa de pelo menos "+
			strconv.Itoa(senhaMinimaDoPainel)+" caracteres.")
		return
	}

	// QUEM CRIOU fica gravado na linha, e só é nulo para o primeiro usuário, que nasce
	// pelo subcomando de linha. Aqui sempre há criador — mas ele pode ser uma CONTA DE
	// JOGO, no período em que os dois caminhos convivem, e aí não há id de painel para
	// apontar: a coluna referencia painel_usuario, então fica nula e quem registra o autor
	// é a auditoria.
	var criador *int64
	if sess.EhDoPainel() {
		id := sess.PainelUsuarioID
		criador = &id
	}

	u, err := h.cfg.Painel.CriarUsuarioDoPainel(r.Context(), login, senha, papel, criador)
	switch {
	case errors.Is(err, store.ErrLoginEmUso):
		h.voltaParaUsuarios(w, r, "Esse login já existe. Se a pessoa foi desativada, reative em vez de criar de novo.")
		return
	case errors.Is(err, store.ErrLoginInvalido), errors.Is(err, store.ErrPapelInvalido):
		h.voltaParaUsuarios(w, r, "Não dá para criar: "+err.Error())
		return
	case err != nil:
		h.cfg.Logger.Error("criando usuario do painel", "login", login, "err", err)
		http.Error(w, "Erro interno.", http.StatusInternalServerError)
		return
	}

	// A SENHA NÃO ENTRA NA AUDITORIA, obviamente, e nem o hash dela: o registro diz QUEM
	// ganhou acesso e com que papel, que é o que uma pessoa precisa saber depois.
	if err := h.cfg.Audit.Write(r.Context(), audit.Record{
		ActorID: sess.AccountID, AtorPainelID: sess.PainelUsuarioID,
		ActorRole: roleFrom(r.Context()), Action: audit.ActionPainelUsuarioCriado,
		New: map[string]any{"painel_usuario": u.ID, "login": u.Login, "papel": u.Papel},
	}); err != nil {
		h.cfg.Logger.Error("usuario do painel criado mas NAO auditado", "id", u.ID, "err", err)
		http.Error(w, "O usuário foi criado, mas a auditoria falhou. Avise quem cuida do servidor.",
			http.StatusInternalServerError)
		return
	}
	h.cfg.Logger.Info("usuario do painel criado", "id", u.ID, "login", u.Login, "papel", u.Papel,
		"por", sess.AccountName)
	h.voltaParaUsuarios(w, r, "Usuário "+u.Login+" criado. Combine a senha com a pessoa e peça para ela trocar.")
}

// mudarAtivoDoUsuarioDoPainel liga ou desliga alguém.
//
// DESATIVAR E NÃO APAGAR, sempre: a auditoria aponta para esta linha, e apagar o usuário
// arrancaria o nome de todas as ações que ele fez.
func (h *Handler) mudarAtivoDoUsuarioDoPainel(w http.ResponseWriter, r *http.Request) {
	if h.cfg.Painel == nil {
		http.NotFound(w, r)
		return
	}
	if err := r.ParseForm(); err != nil || !h.checkCSRF(w, r) {
		if err != nil {
			http.Error(w, "Formulário ilegível.", http.StatusBadRequest)
		}
		return
	}
	sess, _ := staffFrom(r.Context())
	id, err := strconv.ParseInt(r.PathValue("usuario"), 10, 64)
	if err != nil || id <= 0 {
		http.NotFound(w, r)
		return
	}
	ativar := r.PostFormValue("ativo") == "1"

	// NÃO SE DESATIVA A SI MESMA. O botão não aparece na tela, e a trava está aqui também
	// porque um formulário forjado não passa pela tela: quem se desativa sai no clique
	// seguinte, e se for o único admin não há por onde voltar.
	if !ativar && sess.EhDoPainel() && id == sess.PainelUsuarioID {
		h.voltaParaUsuarios(w, r, "Você não pode desativar a si mesma. Deixe outro admin fazer isso.")
		return
	}

	switch err := h.cfg.Painel.DefinirAtivoDoPainel(r.Context(), id, ativar); {
	case errors.Is(err, store.ErrUltimoAdmin):
		h.voltaParaUsuarios(w, r,
			"Esse é o último admin ativo. Crie ou reative outro admin antes de desativar este, "+
				"senão ninguém consegue mais criar usuários.")
		return
	case errors.Is(err, store.ErrNotFound):
		h.voltaParaUsuarios(w, r, "Esse usuário não existe mais. Recarregue a página.")
		return
	case err != nil:
		h.cfg.Logger.Error("mudando o ativo do usuario do painel", "id", id, "err", err)
		http.Error(w, "Erro interno.", http.StatusInternalServerError)
		return
	}

	acao := audit.ActionPainelUsuarioDesativado
	aviso := "Usuário desativado."
	if ativar {
		acao, aviso = audit.ActionPainelUsuarioReativado, "Usuário reativado."
	}
	if err := h.cfg.Audit.Write(r.Context(), audit.Record{
		ActorID: sess.AccountID, AtorPainelID: sess.PainelUsuarioID,
		ActorRole: roleFrom(r.Context()), Action: acao,
		New: map[string]any{"painel_usuario": id, "ativo": ativar},
	}); err != nil {
		h.cfg.Logger.Error("ativo do usuario mudou mas NAO foi auditado", "id", id, "err", err)
		http.Error(w, "A mudança aconteceu, mas a auditoria falhou. Avise quem cuida do servidor.",
			http.StatusInternalServerError)
		return
	}
	h.voltaParaUsuarios(w, r, aviso)
}

// trocarSenhaDoUsuarioDoPainel grava uma senha nova para alguém.
func (h *Handler) trocarSenhaDoUsuarioDoPainel(w http.ResponseWriter, r *http.Request) {
	if h.cfg.Painel == nil {
		http.NotFound(w, r)
		return
	}
	if err := r.ParseForm(); err != nil || !h.checkCSRF(w, r) {
		if err != nil {
			http.Error(w, "Formulário ilegível.", http.StatusBadRequest)
		}
		return
	}
	sess, _ := staffFrom(r.Context())
	id, err := strconv.ParseInt(r.PathValue("usuario"), 10, 64)
	if err != nil || id <= 0 {
		http.NotFound(w, r)
		return
	}
	senha := r.PostFormValue("senha")
	if len(senha) < senhaMinimaDoPainel {
		h.voltaParaUsuarios(w, r, "A senha precisa de pelo menos "+
			strconv.Itoa(senhaMinimaDoPainel)+" caracteres.")
		return
	}

	switch err := h.cfg.Painel.TrocarSenhaDoPainel(r.Context(), id, senha); {
	case errors.Is(err, store.ErrNotFound):
		h.voltaParaUsuarios(w, r, "Esse usuário não existe mais. Recarregue a página.")
		return
	case err != nil:
		h.cfg.Logger.Error("trocando a senha do usuario do painel", "id", id, "err", err)
		http.Error(w, "Erro interno.", http.StatusInternalServerError)
		return
	}

	// A AUDITORIA REGISTRA QUE A SENHA MUDOU, e nada sobre ela. Nem tamanho, nem hash: o
	// que uma pessoa precisa saber depois é que alguém trocou a senha de outra pessoa, e
	// quem foi.
	if err := h.cfg.Audit.Write(r.Context(), audit.Record{
		ActorID: sess.AccountID, AtorPainelID: sess.PainelUsuarioID,
		ActorRole: roleFrom(r.Context()), Action: audit.ActionPainelSenhaTrocada,
		New: map[string]any{"painel_usuario": id},
	}); err != nil {
		h.cfg.Logger.Error("senha trocada mas NAO auditada", "id", id, "err", err)
		http.Error(w, "A senha foi trocada, mas a auditoria falhou. Avise quem cuida do servidor.",
			http.StatusInternalServerError)
		return
	}
	h.voltaParaUsuarios(w, r, "Senha trocada. Combine a nova com a pessoa.")
}

// senhaMinimaDoPainel é o piso de tamanho.
//
// Doze e não oito: esta senha abre o painel inteiro, e quem a digita é um punhado de
// pessoas que a guarda num gerenciador — o custo de ser mais longa é baixo, e o que ela
// protege é tudo.
const senhaMinimaDoPainel = 12

func (h *Handler) voltaParaUsuarios(w http.ResponseWriter, r *http.Request, aviso string) {
	http.Redirect(w, r, "/usuarios-do-painel?aviso="+urlQuery(aviso), http.StatusSeeOther)
}
