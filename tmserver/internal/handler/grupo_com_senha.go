package handler

import (
	"crypto/subtle"
	"strings"

	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/world"
)

// GRUPO COM SENHA: entrar num grupo sem depender de convite.
//
// O CONVITE EXIGE OS DOIS AO MESMO TEMPO. O caminho normal é o líder mandar um
// convite e a pessoa aceitar na janela, e isso só funciona se as duas estiverem na
// frente do teclado no mesmo minuto. Quem chega depois, ou quem cai e volta, precisa
// achar o líder e pedir de novo. Com senha, o grupo fica aberto para quem sabe a
// palavra, e a pessoa entra sozinha, de onde estiver.
//
// TRÊS COMANDOS, e os três passam pelo caminho de grupo que já existe (party.go):
//
//	/criargrupo <senha>        — cria com você de líder, ou troca a senha do seu grupo
//	/entrar <personagem> <senha> — entra no grupo daquele personagem
//	/translider <personagem>   — passa a liderança, e a senha vai junto
//
// O ESTADO MORA NO Dispatcher E NÃO NO MUNDO, como os contadores de senha errada do
// login: só o laço do jogo toca nele, então não precisa de trava. E mora fora da
// Entity de propósito — a Entity é reaproveitada quando um conn é reciclado para
// outro jogador, e uma senha grudada ali viraria a senha do grupo de um estranho.
// A limpeza está em SessionEnd e em todo lugar onde o grupo se desfaz.
//
// A SENHA NUNCA VAI PARA LOG NENHUM. Conferido, e não suposto: o comando chega como
// sussurro, o runCommand devolve verdadeiro e o chat.go retorna ANTES do
// registraFala, que é quem grava fala no banco. O log de comando grava só a palavra
// do comando ("chat command", cmd=criargrupo), nunca o argumento. E nenhuma das
// funções aqui recebe o logger com a senha.

// Limites da senha de grupo.
//
// QUATRO A DOZE, e só letras e números ASCII. O teto de doze é o mesmo da senha de
// jogo, porque quem digita é a mesma pessoa no mesmo cliente de 2003, e um limite
// diferente só criaria uma segunda régua para decorar. Letras e números apenas
// porque a senha é digitada no chat: espaço quebraria a separação dos argumentos, e
// acento sai do cliente em cp1252, então "senhá" digitada em duas máquinas com
// configuração diferente não seria a mesma senha.
const (
	minSenhaDeGrupo = 4
	maxSenhaDeGrupo = 12
)

// Limite de tentativas de /entrar.
//
// CINCO POR MINUTO, POR PERSONAGEM. Sem isto a senha de quatro caracteres cai por
// tentativa e erro: o chat aceita comando quase sem pausa, e um laço acharia uma
// senha curta em minutos. A sexta tentativa é recusada SEM CONFERIR a senha — parar
// antes da comparação é o que tira a informação do atacante, porque uma recusa que
// ainda compara devolve "errou a senha" e confirma que o grupo existe.
const (
	maxTentativasDeEntrar = 5
	janelaDeTentativas    = 60 // segundos
)

// senhaDeGrupoValida diz se a senha serve, e é a MESMA régua na criação e na entrada.
func senhaDeGrupoValida(senha string) bool {
	if len(senha) < minSenhaDeGrupo || len(senha) > maxSenhaDeGrupo {
		return false
	}
	for i := 0; i < len(senha); i++ {
		c := senha[i]
		switch {
		case c >= '0' && c <= '9':
		case c >= 'a' && c <= 'z':
		case c >= 'A' && c <= 'Z':
		default:
			return false
		}
	}
	return true
}

// senhaBate compara sem vazar o tempo.
//
// SEM == DE STRING, e a razão não é teoria: o == do Go para em cima do primeiro byte
// diferente, então uma senha que erra no último caractere leva mais tempo que uma que
// erra no primeiro. Com cinco tentativas por minuto isso é difícil de explorar, mas o
// custo de fazer certo aqui é uma linha, e o custo de fazer errado é descobrir depois
// que o limite de tentativas era a única defesa.
func senhaBate(guardada, tentada string) bool {
	return subtle.ConstantTimeCompare([]byte(guardada), []byte(tentada)) == 1
}

// argumentosDoComando parte o argumento do comando em palavras.
//
// O ARGUMENTO VEM COMO BYTES CRUS do corpo do sussurro, com o resto do campo cheio
// de zeros; cstr corta no primeiro zero. Sem isso a senha viria com uma cauda de
// bytes nulos grudada e nunca bateria com a guardada.
func argumentosDoComando(args []byte) []string {
	return strings.Fields(cstr(args))
}

// criarGrupoComSenha atende /criargrupo <senha>.
func (d *Dispatcher) criarGrupoComSenha(w *world.World, s *world.Session, args []byte) {
	e := w.Entity(s.Conn)
	if e == nil || s.Mode != world.UserPlay {
		return
	}
	campos := argumentosDoComando(args)
	if len(campos) != 1 {
		sendClientMessage(w, s, msgGrupoComoUsarCriar)
		return
	}
	senha := campos[0]
	if !senhaDeGrupoValida(senha) {
		sendClientMessage(w, s, msgGrupoSenhaInvalida)
		return
	}
	// QUEM É MEMBRO NÃO PODE, e a frase diz o motivo em vez de só negar: a pessoa
	// está num grupo que não é dela, e o que ela precisa saber é que tem de sair
	// primeiro, não que o comando não existe.
	if e.Leader != 0 {
		sendClientMessage(w, s, msgGrupoVoceNaoEOLider)
		return
	}
	// Líder de um grupo que já existe troca a senha; quem está sozinho cria.
	//
	// O MESMO COMANDO PARA OS DOIS CASOS de propósito: um /trocarsenhagrupo à parte
	// seria uma quarta palavra para decorar, e a pergunta que a pessoa faz é a mesma
	// ("qual é a senha do meu grupo").
	d.senhasDeGrupo[s.Conn] = senha
	if partyMemberCount(e) > 0 {
		sendClientMessage(w, s, msgGrupoSenhaTrocada)
		return
	}
	sendClientMessage(w, s, msgGrupoCriado)
}

// entrarNoGrupoComSenha atende /entrar <personagem> <senha>.
func (d *Dispatcher) entrarNoGrupoComSenha(w *world.World, s *world.Session, args []byte) {
	e := w.Entity(s.Conn)
	if e == nil || s.Mode != world.UserPlay {
		return
	}
	campos := argumentosDoComando(args)
	if len(campos) != 2 {
		sendClientMessage(w, s, msgGrupoComoUsarEntrar)
		return
	}
	// O LIMITE VEM ANTES DE TUDO, inclusive antes de procurar o personagem. Contar
	// só as tentativas que chegaram a comparar a senha deixaria o atacante varrer
	// nomes à vontade, e saber QUE um nome existe já é meia senha.
	if !d.podeTentarEntrarNoGrupo(w.Now(), s.Conn) {
		sendClientMessage(w, s, msgGrupoMuitasTentativas)
		return
	}
	if e.Leader != 0 || partyMemberCount(e) > 0 {
		sendClientMessage(w, s, msgGrupoVoceJaEstaEmGrupo)
		return
	}
	alvoSess, alvo := w.SessionByName(campos[0])
	if alvoSess == nil || alvo == nil || alvoSess.Mode != world.UserPlay {
		sendClientMessage(w, s, msgGrupoNomeNaoEncontrado)
		return
	}
	if alvoSess.Conn == s.Conn {
		sendClientMessage(w, s, msgGrupoVoceMesmo)
		return
	}
	// QUALQUER MEMBRO SERVE DE ENDEREÇO, não só o líder: quem está entrando conhece
	// o amigo que a chamou, e não necessariamente quem lidera. Aqui a gente sobe do
	// membro para o líder.
	liderConn := alvoSess.Conn
	if alvo.Leader != 0 {
		liderConn = alvo.Leader
	}
	liderSess, lider := w.Session(liderConn), w.Entity(liderConn)
	if liderSess == nil || lider == nil || liderSess.Mode != world.UserPlay || lider.Leader != 0 {
		sendClientMessage(w, s, msgGrupoNaoTemSenha)
		return
	}
	guardada, temSenha := d.senhasDeGrupo[liderConn]
	if !temSenha {
		sendClientMessage(w, s, msgGrupoNaoTemSenha)
		return
	}
	if !senhaBate(guardada, campos[1]) {
		sendClientMessage(w, s, msgGrupoSenhaErrada)
		return
	}
	// AS MESMAS RECUSAS DO CONVITE, e pelo mesmo motivo que o acceptParty as repete:
	// a senha abre a porta, não as regras. Um grupo com senha que aceitasse alguém
	// fora do limite de nível seria um jeito de furar o limite sem convite.
	if !partyLevelOK(lider, e) {
		sendClientMessage(w, s, msgGrupoLimiteDeNivel)
		return
	}
	if d.batalhaTiraGrupo(e) {
		return
	}
	if partyMemberCount(lider) >= world.MaxParty {
		sendClientMessage(w, s, msgGrupoCheio)
		return
	}
	slot, ok := addMember(lider, s.Conn)
	if !ok {
		sendClientMessage(w, s, msgGrupoCheio)
		return
	}
	e.Leader = liderConn
	e.LastReqParty = 0
	// A ENTRADA CERTA ZERA O CONTADOR. Quem acertou não é quem o limite procura, e
	// deixar o contador cheio puniria a pessoa por ter esquecido a senha uma vez.
	delete(d.tentativasDeGrupo, s.Conn)
	d.syncAcceptedParty(w, liderConn, s.Conn, slot+1)
	sendClientMessage(w, s, msgGrupoEntrou)
	if liderSess != nil {
		sendClientMessage(w, liderSess, msgGrupoAlguemEntrou)
	}
}

// transferirLiderancaDoGrupo atende /translider <personagem>.
func (d *Dispatcher) transferirLiderancaDoGrupo(w *world.World, s *world.Session, args []byte) {
	e := w.Entity(s.Conn)
	if e == nil || s.Mode != world.UserPlay {
		return
	}
	campos := argumentosDoComando(args)
	if len(campos) != 1 {
		sendClientMessage(w, s, msgGrupoComoUsarTransLider)
		return
	}
	if e.Leader != 0 {
		sendClientMessage(w, s, msgGrupoVoceNaoEOLider)
		return
	}
	if partyMemberCount(e) == 0 {
		sendClientMessage(w, s, msgGrupoSemMembros)
		return
	}
	novoSess, novo := w.SessionByName(campos[0])
	if novoSess == nil || novo == nil || novoSess.Mode != world.UserPlay {
		sendClientMessage(w, s, msgGrupoNomeNaoEncontrado)
		return
	}
	if novoSess.Conn == s.Conn {
		sendClientMessage(w, s, msgGrupoVoceMesmo)
		return
	}
	if !hasMember(e, novoSess.Conn) || novo.Leader != s.Conn {
		sendClientMessage(w, s, msgGrupoNaoEMembro)
		return
	}

	// DE QUALQUER LUGAR DO MAPA, e é por isso que este comando existe em vez de
	// reaproveitar o caminho de "o líder saiu": aquele promove quem estiver na
	// primeira vaga e desfaz o resto, e aqui a pessoa escolhe quem fica com o grupo
	// sem ter de se encontrar com ela no mundo.
	antigos := partyMembers(e)
	senha, tinhaSenha := d.senhasDeGrupo[s.Conn]

	// Desmonta a lista atual e avisa todo mundo, como o leaderLeaveParty faz.
	e.PartyList = [world.MaxParty]int{}
	for _, membro := range antigos {
		if me := w.Entity(membro); me != nil && me.Leader == s.Conn {
			me.Leader = 0
			me.PartyList = [world.MaxParty]int{}
		}
		d.sendRemoveParty(w, membro, 0)
	}
	d.sendRemoveParty(w, s.Conn, 0)
	delete(d.senhasDeGrupo, s.Conn)

	// Remonta com o novo líder. O antigo líder entra como membro: ele continua no
	// grupo que acabou de entregar, que é o que a pessoa espera de "passar a
	// liderança" e não de "sair do grupo".
	novo.Leader = 0
	novo.PartyList = [world.MaxParty]int{}
	if tinhaSenha {
		// A SENHA VAI JUNTO. Sem isto o grupo continuaria existindo e ficaria mudo:
		// ninguém mais conseguiria entrar, e quem soubesse a senha antiga receberia
		// "este grupo não tem senha" sem entender por quê.
		d.senhasDeGrupo[novoSess.Conn] = senha
	}
	for _, membro := range append(antigos, s.Conn) {
		if membro == novoSess.Conn {
			continue
		}
		me := w.Entity(membro)
		ms := w.Session(membro)
		if me == nil || ms == nil || ms.Mode != world.UserPlay {
			continue
		}
		slot, ok := addMember(novo, membro)
		if !ok {
			break
		}
		me.Leader = novoSess.Conn
		d.syncAcceptedParty(w, novoSess.Conn, membro, slot+1)
	}
	sendClientMessage(w, s, msgGrupoLiderancaPassada)
	sendClientMessage(w, novoSess, msgGrupoVoceEOLiderAgora)
}

// podeTentarEntrarNoGrupo conta as tentativas de /entrar numa janela de um minuto.
//
// A JANELA ANDA COM O RELÓGIO DO MUNDO e não com o da máquina: o laço já tem
// World.Now, e usar time.Now aqui traria uma segunda noção de tempo para dentro do
// laço, que é exatamente o que este servidor evita.
//
// O RELÓGIO ENTRA COMO ARGUMENTO e não é lido aqui dentro: assim o teste do limite
// anda um minuto sem subir servidor nenhum, e sem inventar um segundo relógio só
// para o teste — que é o jeito clássico de um teste de janela de tempo passar a
// medir a si mesmo.
func (d *Dispatcher) podeTentarEntrarNoGrupo(agora uint32, conn int) bool {
	t := d.tentativasDeGrupo[conn]
	if agora-t.desde >= janelaDeTentativas {
		t = tentativasDeGrupo{desde: agora}
	}
	if t.quantas >= maxTentativasDeEntrar {
		d.tentativasDeGrupo[conn] = t
		return false
	}
	t.quantas++
	d.tentativasDeGrupo[conn] = t
	return true
}

// esqueceGrupoComSenha limpa o que este arquivo guarda para um conn.
//
// CHAMADA DE SessionEnd, e é o que impede a pior falha possível aqui: conn é
// reciclado para o próximo jogador que entra, e uma senha deixada para trás viraria a
// senha do grupo de um estranho — que nunca a escolheu e não sabe que ela existe.
func (d *Dispatcher) esqueceGrupoComSenha(conn int) {
	delete(d.senhasDeGrupo, conn)
	delete(d.tentativasDeGrupo, conn)
}

// tentativasDeGrupo é a contagem de /entrar de um personagem na janela corrente.
type tentativasDeGrupo struct {
	desde   uint32 // World.Now em que a janela começou
	quantas int
}
