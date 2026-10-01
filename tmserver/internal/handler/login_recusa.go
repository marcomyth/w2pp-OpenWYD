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

// As reservas, uma por recusa de login.
//
// COM ACENTO, como todo texto que o jogo manda: o painel viaja em cp1252, o
// protocol.ClientText converte, e o resto das frases do servidor é acentuado. Tirar o
// acento aqui seria a única frase torta do jogo, e ainda pela razão errada — o limite do
// painel é em BYTES, e é medido em teste, não estimado.
const (
	msgLoginVersao    = "Sua versão do jogo está velha. Baixe o launcher de novo."
	msgLoginAguarde   = "Entrando. Aguarde um momento."
	msgLoginTresErros = "Senha errada três vezes. Espere um pouco e tente de novo."
	msgLoginSenha     = "Senha incorreta."
	msgLoginSemConta  = "Não existe conta com esse nome."
	msgLoginBloqueada = "Esta conta está bloqueada."
	// O NoticeDBError NÃO TINHA FRASE NENHUMA, nem no Language.txt nem no noticeText:
	// quem caía nele via a tela muda mesmo com o 0x0101 no ar.
	msgLoginErroDeBanco = "Erro no servidor ao entrar. Tente de novo em instantes."
)

// limiteDeRecusasNoSocket é de quantas recusas com fechamento o socket aguenta antes de
// cair NA HORA, sem prazo.
//
// O prazo existe para a pessoa ler o motivo, e ela lê uma vez. Da terceira em diante não
// há mais nada a ler: ou é alguém insistindo numa conta que não vai entrar, ou é um
// cliente remendado repetindo a recusa — e sem o teto o socket seria mantido de pé por
// quem nunca vai passar do login.
const limiteDeRecusasNoSocket = 3

// momentoDaRecusa diz DE ONDE a recusa com fechamento vem, que é o que decide em que
// estado a sessão tem de estar para o fechamento poder ser atrasado.
type momentoDaRecusa uint8

const (
	// recusaNaChegada é a recusa dada pelo próprio 0x20D, antes de qualquer pedido ao
	// banco. Só é uma sessão de ninguém se estiver em UserAccept.
	recusaNaChegada momentoDaRecusa = iota
	// recusaNaVoltaDoBanco é a recusa dada pelo completeAccountLogin. Lá a sessão está em
	// UserLogin, e o login pendente que esse estado anuncia é o que acabou de voltar:
	// depois dele não sobra nada por chegar.
	recusaNaVoltaDoBanco
)

// ehDeNinguem diz se a sessão ainda está na tela de login, sem conta e sem nada pendente
// — o único caso em que o fechamento atrasado pode desfazer o login.
func (m momentoDaRecusa) ehDeNinguem(s *world.Session) bool {
	if s.AccountID != 0 {
		return false
	}
	if m == recusaNaVoltaDoBanco {
		return s.Mode == world.UserLogin
	}
	return s.Mode == world.UserAccept
}

// recusaEFecha é a recusa que TERMINA a conexão: diz o motivo, devolve a sessão ao
// estado de antes do login e agenda a queda do socket.
//
// A VOLTA AO ESTADO ANTERIOR NÃO É LIMPEZA, É O QUE FAZ O FECHAMENTO ACONTECER — e foi
// aqui que este arquivo errou antes. O fechaDepois só fecha se a sessão ainda não é de
// ninguém, e o caminho da conta bloqueada deixava o Mode em UserLogin, onde ele ficou
// desde que o accountLogin mandou o pedido ao banco. A guarda então recusava fechar, e o
// socket ficava aberto PARA SEMPRE: o defeito novo era pior que o mudo que isto conserta.
//
// Por isso o reset está no helper e não em cada caminho. Uma regra que cada chamador tem
// de lembrar de repetir é uma regra que um deles vai esquecer.
func (d *Dispatcher) recusaEFecha(w *world.World, s *world.Session, m momentoDaRecusa, n Notice, reserva string) {
	d.recusaLogin(w, s, n, reserva)
	d.fechaPorRecusa(w, s, m)
}

// fechaPorRecusa é a segunda metade do recusaEFecha, separada porque o acesso restrito
// manda o texto dele próprio (não é um Notice do cliente) e precisa do mesmo tratamento.
//
// O RESET SÓ VALE PARA UMA SESSÃO QUE NÃO É DE NINGUÉM, e a guarda mora AQUI pelo mesmo
// motivo de o reset morar: o segundo erro deste arquivo foi zerar a sessão de quem já
// estava jogando. A checagem de versão roda antes da de modo, então um 0x20D com a versão
// errada chega aqui em QUALQUER estado, e zerar a conta e o modo de uma sessão viva tem
// três estragos, os três medidos:
//
//   - o World só grava a saída de quem está em UserPlay com conta, e só solta a carga de
//     quem tem conta. A sessão zerada caía pelo prazo SEM GRAVAR personagem nem carga;
//   - com o socket ainda de pé, um login certo recarregava a conta do banco por cima do
//     que não foi gravado: rollback a pedido do cliente;
//   - com um login no banco, o Mode voltava a UserAccept com a resposta por chegar. Ela
//     punha a sessão na tela de personagens e o fechaDepois desistia de fechar.
//
// Fora da tela de login não há o que atrasar nem o que desfazer: o texto já saiu, e a
// conexão cai NA HORA pelo Close de sempre, com a sessão INTACTA — é ele que grava o
// personagem e a carga, solta a posse da conta e descarta a resposta de um login que
// ainda esteja no banco. É o que a recusa de versão fazia antes de existir o prazo.
func (d *Dispatcher) fechaPorRecusa(w *world.World, s *world.Session, m momentoDaRecusa) {
	if !m.ehDeNinguem(s) {
		d.log.Warn("recusa de login fora da tela de login: fechando na hora, com a sessao intacta",
			"conn", s.Conn, "account", s.AccountName, "mode", s.Mode)
		w.Close(s)
		return
	}
	s.AccountName = ""
	s.AccountID = 0
	s.Mode = world.UserAccept
	s.RecusasDeAcesso++
	if s.RecusasDeAcesso >= limiteDeRecusasNoSocket {
		d.log.Info("recusa de login: fechando na hora, chegou ao teto do socket",
			"conn", s.Conn, "recusas", s.RecusasDeAcesso)
		w.Close(s)
		return
	}
	d.fechaDepois(w, s)
}
