package handler

// A TRAVA DAS TRÊS SENHAS ERRADAS NÃO SOLTAVA NUNCA, e a causa é a ordem das checagens.
//
// O contador (d.fails) só era apagado num login que desse CERTO (login.go), e a trava
// recusa o 0x20D ANTES de o pedido ir ao banco. Ou seja: depois da terceira senha
// errada, o único caminho que apagava o contador ficava inalcançável. Nem a senha certa
// nem a senha trocada pelo site entravam, e só reiniciar o tmserver soltava a conta.
// Medido em produção: uma conta travada à 01:27Z continuava travada mais de 13 horas
// depois.
//
// O LEGADO SOLTA SOZINHO, e é isso que está portado aqui. O ProcessMinTimer zera a lista
// INTEIRA de dez em dez passadas (ProcessSecMinTimer.cpp:2645-2646,
// `if ((MinCounter % 10) == 0) memset(FailAccount, ...)`), o MinCounter anda um por
// passada (ProcessSecMinTimer.cpp:2791) e a passada é de 12000 ms (o TIMER_MIN de
// Server.cpp:4087 — "minuto" só no nome, ver mintimer.go). Dez passadas são 120 s.
//
// É A LISTA TODA, E NÃO UM PRAZO POR CONTA: quem erra a terceira senha faltando cinco
// segundos para a limpeza fica travado cinco segundos, e quem erra logo depois dela
// fica quase os 120. É assim no legado, e é o que a frase da recusa promete ("espere um
// pouco"). O que a trava limita é o ritmo — três palpites por conta a cada dois
// minutos —, e não é ela que guarda a conta de quem esqueceu a senha.
const travaDeSenhaPassadas = 10

// travaDeSenhaTicks é o período da limpeza em tiques do mundo (1 s cada): 120.
const travaDeSenhaTicks = travaDeSenhaPassadas * minTimerTicks

// tickTravaDeSenha zera os contadores de senha errada quando o período fecha.
//
// PENDURADO NO Tick E NÃO NUM TIMER PRÓPRIO: o d.fails só é tocado pela goroutine do
// laço, e é por isso que ele não tem lock. O Tick roda nela, e roda com o servidor
// vazio também — que é justamente quando alguém travado está tentando entrar.
func (d *Dispatcher) tickTravaDeSenha() {
	if d.tickCount%travaDeSenhaTicks != 0 || len(d.fails) == 0 {
		return
	}
	d.log.Info("account login: wrong-password counters cleared", "accounts", len(d.fails))
	clear(d.fails)
}
