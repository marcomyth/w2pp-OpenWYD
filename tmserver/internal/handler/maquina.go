package handler

import (
	"fmt"

	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/world"
)

// Uma lojinha rendendo por computador.
//
// Duas contas no mesmo computador abriam duas barracas e somavam os pontos das
// duas (visto em jogo em 25/09/2026). A regra é a mesma que o legado quis ter no
// "Lojinha Offline" (ProcessSecMinTimer.cpp:912-965, que compara pUser.Mac e fica
// com a barraca de ISTradTimer menor): dentre as barracas ABASTECIDAS de uma mesma
// máquina, só a mais antiga rende. As outras continuam de pé e vendendo — a trava
// é no prêmio, não na venda, porque vender com duas contas não tira nada de
// ninguém; o que tirava era ganhar tempo em dobro.
//
// O legado tem um erro que não foi copiado: ali cada barraca da máquina paga a
// escolhida, então a mais antiga recebia uma vez por barraca aberta e o prêmio
// continuava dobrando, só que numa conta só.

// maquinaConhecida diz se o cliente mandou uma máquina com que dê para comparar.
// Zero e -1 em tudo são os dois vazios possíveis — o legado preenche 0xFF quando
// o pacote vem curto (_MSG_AccountLogin.cpp:68), e o WYD.exe deixa zero quando
// GetAdaptersInfo não acha placa. Tratá-los como uma máquina só faria todo
// cliente sem placa de rede disputar a mesma barraca, então a trava simplesmente
// não se aplica a eles.
func maquinaConhecida(m [4]int32) bool {
	return m != [4]int32{} && m != [4]int32{-1, -1, -1, -1}
}

// maquinaTexto é a máquina no formato em que ela aparece no log de login, para
// dar para cruzar duas contas lendo o log.
func maquinaTexto(m [4]int32) string {
	return fmt.Sprintf("%08x-%08x-%08x-%08x", uint32(m[0]), uint32(m[1]), uint32(m[2]), uint32(m[3]))
}

// lojaPrecedidaNaMaquina diz se outra barraca abastecida do mesmo computador
// chegou antes da de s — e, portanto, se é ela, e não a de s, que rende.
//
// "Antes" é pela idade no relógio do laço (now − OpenedAt), que é a conta que
// sobrevive à volta do uint32; empate cai para a conexão menor, só para que as
// duas nunca se considerem precedidas ao mesmo tempo e nenhuma renda.
//
// Barraca vazia não disputa: ela não rende mesmo (shopStocked), e deixá-la na
// disputa faria a segunda barraca ficar sem prêmio por causa de uma que não está
// ganhando nada.
//
// forEach percorre as sessões; no jogo é World.ForEachSession, nos testes uma
// lista. Loop-only.
func lojaPrecedidaNaMaquina(now uint32, s *world.Session, forEach func(func(*world.Session))) bool {
	if s == nil || s.AutoTrade == nil || !maquinaConhecida(s.Maquina) {
		return false
	}
	minhaIdade := now - s.AutoTrade.OpenedAt
	precedida := false
	forEach(func(o *world.Session) {
		if precedida || o == nil || o == s || o.Conn == s.Conn || o.AutoTrade == nil {
			return
		}
		if o.Maquina != s.Maquina || !shopStocked(o.AutoTrade) {
			return
		}
		idade := now - o.AutoTrade.OpenedAt
		if idade > minhaIdade || (idade == minhaIdade && o.Conn < s.Conn) {
			precedida = true
		}
	})
	return precedida
}

// precedidaNoMundo é lojaPrecedidaNaMaquina sobre as sessões do mundo.
func precedidaNoMundo(w *world.World, s *world.Session) bool {
	return lojaPrecedidaNaMaquina(w.Now(), s, func(fn func(*world.Session)) {
		w.ForEachSession(func(o *world.Session, _ *world.Entity) { fn(o) })
	})
}

// msgOutraLojaRende é o que o dono da segunda barraca lê. Diz que a loja vende —
// senão ele fecha achando que ela não serve — e por que não rende.
const msgOutraLojaRende = "Outra lojinha deste computador já está rendendo pontos. Esta vende normalmente, mas não rende."
