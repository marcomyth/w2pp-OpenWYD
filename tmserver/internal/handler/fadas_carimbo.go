package handler

import (
	"slices"

	"github.com/jeanluca/w2pp-openwyd/internal/droprule"
	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/protocol"
	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/world"
)

// As faixas de adicional dos itens que NÃO saem do sorteio comum, para a dica do
// Painel de Drop das Fadas.
//
// Em alguns lugares o servidor troca os adicionais do drop por uma tabela própria
// (as funções ...Finish de dropTableRolls e os saques de chefe): Castelo Orc,
// Acampamento Troll, Amon, Caveiras, Ciclopes, as Armas D do Deserto, e as armas
// que o Ciclope Tirano, o Taron Tirano, o Boss Conjurador e a Gárgula Sábio
// entregam. A faixa desses itens sai da MESMA tabela que o carimbo usa — não há
// uma segunda cópia dos números —, e um teste roda o saque de verdade e confere
// que todo adicional que sai está dentro dela.

// faixasDaTabela é o menor e o maior valor de cada efeito de uma tabela de adds.
func faixasDaTabela(tabela []addArma) []protocol.FadasFaixa {
	var out []protocol.FadasFaixa
	for _, l := range tabela {
		out = uneFaixa(out, protocol.FadasFaixa{Efeito: l.efeito, Min: uint8(l.valor), Max: uint8(l.valor)})
	}
	return out
}

// faixasDoAcessorio é o mesmo para a tabela dos anéis e amuletos do Castelo Orc,
// que sorteia o valor em degraus dentro de cada linha.
func faixasDoAcessorio(tabela []addRoll) []protocol.FadasFaixa {
	var out []protocol.FadasFaixa
	for _, l := range tabela {
		maior := l.min + l.step*((l.max-l.min)/l.step)
		out = uneFaixa(out, protocol.FadasFaixa{Efeito: l.effect, Min: uint8(l.min), Max: uint8(maior)})
	}
	return out
}

// uneFaixa junta uma faixa a uma lista: o mesmo efeito alarga a que já existe.
func uneFaixa(lista []protocol.FadasFaixa, f protocol.FadasFaixa) []protocol.FadasFaixa {
	k := slices.IndexFunc(lista, func(o protocol.FadasFaixa) bool { return o.Efeito == f.Efeito })
	if k < 0 {
		lista = append(lista, f)
		slices.SortFunc(lista, func(a, b protocol.FadasFaixa) int { return int(a.Efeito) - int(b.Efeito) })
		return lista
	}
	lista[k].Min, lista[k].Max = min(lista[k].Min, f.Min), max(lista[k].Max, f.Max)
	return lista
}

func uneFaixas(lista, outras []protocol.FadasFaixa) []protocol.FadasFaixa {
	for _, f := range outras {
		lista = uneFaixa(lista, f)
	}
	return lista
}

// tabelaDaArmaC é a tabela que carimbaAddArmaC usa para uma Arma C, ou nil.
func tabelaDaArmaC(idx int16, fisica, magica []addArma) []addArma {
	switch {
	case slices.Contains(armasCFisicas, idx):
		return fisica
	case slices.Contains(armasCMagicas, idx):
		return magica
	}
	return nil
}

// tabelaDaArmaD é a tabela que carimbaAddArmaD usa para uma Arma D do Deserto, ou nil.
func tabelaDaArmaD(idx int16) []addArma {
	switch {
	case armasDDoDesertoFisicas[idx]:
		return addCiclopeFisica
	case armasDDoDesertoMagicas[idx]:
		return addCiclopeMagica
	}
	return nil
}

// fadasCarimboDoLugar são os adicionais de um item que o monstro dá pela Mesa de
// Drops num lugar que carimba os próprios (as ...Finish de dropTableRolls). ok
// falso: o lugar não mexe neste item, e vale o sorteio comum.
func fadasCarimboDoLugar(molde string, idx int16) (faixas []protocol.FadasFaixa, ok bool) {
	var tabela []addArma
	switch {
	case casteloOrcTemplates[molde]:
		switch {
		case idx >= itemAmuletFirst && idx <= itemAmuletLast:
			return faixasDoAcessorio(casteloOrcAmuletAdds), true
		case idx >= itemRingFirst && idx <= itemRingLast:
			return faixasDoAcessorio(casteloOrcRingAdds), true
		}
		return nil, false
	case acampamentoTrollTemplates[molde]:
		boss := molde == droprule.Canonical(acampamentoTrollBoss)
		switch {
		case armasTrollFisicas[idx] && boss:
			tabela = addTrollFisicaBoss
		case armasTrollFisicas[idx]:
			tabela = addTrollFisicaMob
		case armasTrollMagicas[idx] && boss:
			tabela = addTrollMagicaBoss
		case armasTrollMagicas[idx]:
			tabela = addTrollMagicaMob
		}
		if tabela == nil {
			return nil, false
		}
		faixas = faixasDaTabela(tabela)
		if boss {
			// O Troll Enigma às vezes põe a skill na segunda vaga.
			lo, hi := slices.Min(skillTroll), slices.Max(skillTroll)
			faixas = uneFaixa(faixas, protocol.FadasFaixa{Efeito: efSpecialAll, Min: uint8(lo), Max: uint8(hi)})
		}
		return faixas, true
	case geloAmon[molde]:
		switch {
		case armasAmonFisicas[idx]:
			tabela = addAmonFisica
		case armasAmonMagicas[idx]:
			tabela = addAmonMagica
		}
	case caveirasDoSpot[molde]:
		tabela = tabelaDaArmaC(idx, addCaveiraFisica, addCaveiraMagica)
	case ciclopesDoSpot[molde]:
		tabela = tabelaDaArmaC(idx, addCiclopeFisica, addCiclopeMagica)
	case monstrosDasArmasD[molde]:
		tabela = tabelaDaArmaD(idx)
	default:
		return nil, false
	}
	if tabela == nil {
		return nil, false
	}
	return faixasDaTabela(tabela), true
}

// fadasCarimboDoChefe são os adicionais de um item que o saque de chefe entrega
// (fadasSaqueEspecial). Nenhum para o que o chefe dá sem adicional (barras,
// âmagos, ovos, pedras).
func fadasCarimboDoChefe(m *fadaMonstro, idx int16) []protocol.FadasFaixa {
	mob := &world.Entity{TemplateName: m.molde}
	switch {
	case isCiclopeTirano(mob) && slices.Contains(armasDDoTirano, idx):
		if armasTrollMagicas[idx] {
			return faixasDaTabela(addCiclopeMagica)
		}
		return faixasDaTabela(addCiclopeFisica)
	case isTaronTirano(mob) && slices.Contains(armasDDoTaronTirano, idx):
		return faixasDaTabela(tabelaDaArmaD(idx))
	case isBossConjurador(mob) || isGargulaSabioChefe(mob):
		return faixasDaTabela(tabelaDaArmaC(idx, addConjuradorFisica, addConjuradorMagica))
	}
	return nil
}

// fadasFaixasDoPar são os adicionais que um item pode ter quando um monstro o dá,
// juntando as fontes por onde ele pode vir: o molde (sorteio comum), a Mesa
// (sorteio comum, ou a tabela do lugar) e o saque de chefe (a tabela do chefe).
func (d *Dispatcher) fadasFaixasDoPar(m *fadaMonstro, idx int16, molde, mesa, especial []int16) []protocol.FadasFaixa {
	var out []protocol.FadasFaixa
	comum := slices.Contains(molde, idx)
	if slices.Contains(mesa, idx) {
		if f, ok := fadasCarimboDoLugar(m.molde, idx); ok {
			out = uneFaixas(out, f)
		} else {
			comum = true
		}
	}
	if comum {
		for _, f := range d.fadasPossiveis().Do(d.fadasBaseDoItem(idx), int(m.nivel)) {
			out = uneFaixa(out, protocol.FadasFaixa{Efeito: f.Efeito, Min: f.Min, Max: f.Max})
		}
	}
	if slices.Contains(especial, idx) {
		out = uneFaixas(out, fadasCarimboDoChefe(m, idx))
	}
	return out
}
