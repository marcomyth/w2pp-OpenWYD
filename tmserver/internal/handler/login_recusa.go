package handler

import "github.com/jeanluca/w2pp-openwyd/tmserver/internal/world"

// A RECUSA DE LOGIN NÃO APARECIA NA TELA, e a causa é o pacote errado.
//
// O notify (notice.go) manda DOIS pacotes: primeiro um 0x0102 MsgMessageBoxOk com
// quatro bytes, e depois o 0x0101 com o texto. Os quatro bytes são um código do NOSSO
// iota — o comentário do próprio notice.go diz que os números são "local identifiers,
// NOT the original notification numbers" —, e no legado a MSG_MessageBoxOk tem 116
// bytes de corpo. Ou seja, o cliente recebe uma caixa de tamanho errado com um número
// que não significa nada para ele.
//
// E O LEGADO NUNCA MANDA 0x0102 NO LOGIN: só o 0x0101 e o CloseUser
// (ProcessDBMessage.cpp:525, 603). O único caminho de recusa que FUNCIONA hoje é o do
// acesso restrito (login.go), que manda só o 0x0101 — foi ele que mostrou onde estava o
// erro.
//
// Então aqui a recusa de login manda SÓ o texto. O notify geral fica como está: ele é
// usado em dezenas de lugares dentro do jogo, onde o cliente já está em cena e a caixa
// tem outro comportamento, e mexer nele seria trocar um defeito visível por um invisível.

// recusaLogin manda ao cliente o motivo da recusa, e só isso.
//
// A RESERVA NÃO É ZELO: o texto vem do Language.txt do cliente, e se aquela chave não
// existir naquele arquivo o noticeLine devolve vazio — e um sendClientMessage com texto
// vazio não manda nada. A pessoa voltaria à tela muda, que é exatamente o defeito que
// isto conserta. Com a reserva, o pior caso é uma frase genérica em vez de nenhuma.
func (d *Dispatcher) recusaLogin(w *world.World, s *world.Session, n Notice, reserva string) {
	texto, ok := d.noticeLine(n)
	if !ok || texto == "" {
		texto = reserva
	}
	sendClientMessage(w, s, texto)
}

// As reservas, uma por recusa de login. Todas cabem em 94 bytes em cp1252, e há teste.
const (
	msgLoginVersao    = "Sua versao do jogo esta velha. Baixe o launcher de novo."
	msgLoginAguarde   = "Entrando. Aguarde um momento."
	msgLoginTresErros = "Senha errada tres vezes. Espere um pouco e tente de novo."
	msgLoginSenha     = "Senha incorreta."
	msgLoginSemConta  = "Nao existe conta com esse nome."
	msgLoginBloqueada = "Esta conta esta bloqueada."
	// O NoticeDBError NÃO TINHA FRASE NENHUMA, nem no Language.txt nem no noticeText:
	// quem caía nele via a tela muda mesmo com o 0x0101 no ar.
	msgLoginErroDeBanco = "Erro no servidor ao entrar. Tente de novo em instantes."
)
