package domain

import "errors"

// Ator é quem fez uma ação administrativa: uma conta de JOGO ou um usuário do PAINEL.
//
// POR QUE UM TIPO E NÃO DOIS PARÂMETROS int64. As escritas do painel recebiam
// `moderatorID int64`, e um usuário de painel não tem conta de jogo — o número que
// sobrava era ZERO. Zero é um id válido de se escrever e inválido de se ler: a linha
// de auditoria ficava gravada, a ação aparecia como feita, e o autor era uma conta
// que não existe. Era exatamente isso que o interceptador do webServer estava
// evitando ao recusar toda escrita de usuário do painel.
//
// Com um tipo, "sem ator" deixa de ser representável por acidente: quem chama tem de
// dizer QUAL dos dois é, e Conferir() recusa antes de qualquer INSERT.
//
// OS NOMES SEGUEM A AUDITORIA DO PAINEL (audit.Record.ActorID e AtorPainelID) para
// quem procurar um achar o outro.
type Ator struct {
	// ContaID é a conta de jogo. Zero quando o ator não é uma conta.
	ContaID int64
	// PainelUsuarioID é a linha em painel_usuario. Zero quando o ator não é um
	// usuário do painel.
	PainelUsuarioID int64
}

// AtorDaConta é o ator que já existia: uma conta de jogo com cargo.
func AtorDaConta(id int64) Ator { return Ator{ContaID: id} }

// AtorDoPainel é o ator novo: um usuário do painel, sem personagem no jogo.
func AtorDoPainel(id int64) Ator { return Ator{PainelUsuarioID: id} }

// ErrSemAtor é a recusa de uma escrita que não sabe quem a pediu.
var ErrSemAtor = errors.New("domain: escrita administrativa sem ator")

// ErrAtorDuplo é a recusa de uma escrita com os dois atores preenchidos.
var ErrAtorDuplo = errors.New("domain: escrita administrativa com dois atores")

// Conferir recusa o que a trava do banco também recusaria, mas ANTES do INSERT.
//
// A MESMA REGRA EM DOIS LUGARES, e não é duplicação por descuido: a trava no banco
// (a CHECK "um ator" da migração 0181) é a que garante que nenhum caminho, nem um
// escrito amanhã, grave linha sem autor. Esta aqui é a que dá uma mensagem que diz o
// que fazer, em vez de um erro de constraint com nome de tabela que quem lê o painel
// não entende.
func (a Ator) Conferir() error {
	switch {
	case a.ContaID == 0 && a.PainelUsuarioID == 0:
		return ErrSemAtor
	case a.ContaID != 0 && a.PainelUsuarioID != 0:
		return ErrAtorDuplo
	}
	return nil
}

// EhDoPainel diz se quem está agindo é um usuário do painel.
func (a Ator) EhDoPainel() bool { return a.PainelUsuarioID != 0 }

// ParaSQL devolve os dois valores como o banco os quer: exatamente um deles NULO.
//
// DEVOLVE any E NÃO int64 porque NULO e zero são coisas diferentes aqui. Passar 0
// gravaria a conta 0 — o defeito que este tipo existe para tornar impossível — e a
// CHECK da 0181 recusaria a linha, o que é melhor que gravar, mas chega tarde: o
// erro sairia como violação de constraint em vez de dizer que falta ator.
//
// COM OS DOIS PREENCHIDOS, ELE DEVOLVE OS DOIS, e não escolhe um. Escolher seria
// descartar um ator em silêncio e gravar uma linha que diz que UMA pessoa fez o que
// duas informações reivindicam — e ninguém depois saberia que houve descarte.
// Devolvendo os dois, a trava do banco recusa a linha, alto e claro. Quem chama
// deveria ter parado antes, no Conferir().
func (a Ator) ParaSQL() (conta any, painel any) {
	if a.ContaID != 0 {
		conta = a.ContaID
	}
	if a.PainelUsuarioID != 0 {
		painel = a.PainelUsuarioID
	}
	return conta, painel
}
