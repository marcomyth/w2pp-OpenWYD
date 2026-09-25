package handler

import (
	"github.com/jeanluca/w2pp-openwyd/internal/acesso"
	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/world"
)

// podeUsarRMT responde se ESTA sessão pode anunciar ou comprar por dinheiro real.
//
// UM LUGAR SÓ para as duas pontas, a de anunciar e a de comprar. Duas cópias da mesma
// pergunta é como uma delas fica para trás no dia em que o estado ganhar um valor novo — e
// a que ficar para trás abre metade do mercado sem ninguém notar.
//
// O PADRÃO DO TIPO É FECHADO: acesso.RMTFechado é o zero de EstadoRMT, então um Config sem
// este campo trava em vez de liberar. Este switch escreve os três casos de propósito, sem
// `default: true`, para um valor novo amanhã cair no fechado e não no aberto.
func (d *Dispatcher) podeUsarRMT(s *world.Session) bool {
	switch d.cfg.RMT {
	case acesso.RMTAberto:
		return true
	case acesso.RMTStaff:
		// EhStaff() é moderador ou acima, que é o que foi combinado. O nível vem do
		// account.role gravado no login (login.go), e não de nada que o cliente mande.
		return s != nil && s.AccessLevel.EhStaff()
	default:
		return false
	}
}
