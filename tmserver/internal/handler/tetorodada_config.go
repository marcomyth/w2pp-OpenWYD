package handler

import "github.com/jeanluca/w2pp-openwyd/internal/domain"

// A configuração do teto de XP por rodada (tetorodada.go): os valores vêm da
// linha de eventos do painel (world_event_config) e valem a partir do próximo
// ganho. Antes da primeira leitura valem os padrões decididos.

type tetoDaRodadaConfig struct {
	lido        bool
	base, dobro [5]int64
}

// setTetoDaRodada aplica os valores do painel. Negativo não existe na coluna
// (CHECK >= 0); se chegar, vira zero, que é "sem teto" naquela faixa.
func (d *Dispatcher) setTetoDaRodada(base, dobro [5]int64) {
	for i := range base {
		base[i] = max(base[i], 0)
		dobro[i] = max(dobro[i], 0)
	}
	d.tetoRodadaCfg = tetoDaRodadaConfig{lido: true, base: base, dobro: dobro}
}

// valoresDoTeto devolve os tetos em vigor, sem e com o dobro.
func (d *Dispatcher) valoresDoTeto() (base, dobro [5]int64) {
	if !d.tetoRodadaCfg.lido {
		return domain.DefaultRoundXPCap, domain.DefaultRoundXPCapDouble
	}
	return d.tetoRodadaCfg.base, d.tetoRodadaCfg.dobro
}
