package handler

import (
	"sort"
	"testing"

	"github.com/jeanluca/w2pp-openwyd/internal/droprule"
	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/loot"
	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/rng"
)

// Uma entrada de 10 minutos na arena das Hidras, com um grupo que limpa tudo o
// que nasce: as 11 Douradas e 40 Imortais do começo, o bloco 3498 (MinuteGenerate
// -1, renasce 15 s depois de morrer) umas 25 vezes, as Imortais soltas (3500-3508,
// de 2 em 2 minutos) e o gerador de 12 minutos, que costuma voltar uma vez dentro
// da janela. É a conta da migração 0176; com ela, a regra antiga dá os ~3,3
// chaves em duas entradas que bateram com o relato de 27/09/2026 (4 em 2).
const (
	hidrasDouradasPorEntrada = 37
	hidrasImortaisPorEntrada = 180
	hidrasPersonagens        = 4 // o grupo do relato
	hidrasEntradas           = 20000
	itemOvoDenteDeSabre      = 2305
)

// hidrasTabela monta a Mesa das Hidras a partir das migrações nomeadas, uma
// sobrescrevendo a outra na ordem dada, como o banco faz.
func hidrasTabela(t *testing.T, nomes ...string) droprule.Table {
	t.Helper()
	regras := map[string]map[int16]int32{}
	for _, nome := range nomes {
		for mob, itens := range linhasDaMigracao(t, nome) {
			if regras[mob] == nil {
				regras[mob] = map[int16]int32{}
			}
			for item, chance := range itens {
				regras[mob][item] = chance
			}
		}
	}
	var out []droprule.Rule
	for mob, itens := range regras {
		for item, chance := range itens {
			out = append(out, droprule.Rule{Mob: mob, Item: item, Chance: chance})
		}
	}
	return droprule.NewTable(out)
}

// hidrasContagem é o que uma entrada rende dos três itens da 0176.
type hidrasContagem struct{ chaves, ovos, moedas int }

// hidrasEntrada simula uma entrada: o sorteio da porta (umEm > 0 é o de antes da
// 0176, um por personagem), as mortes pela Mesa como dropTableRolls, e — quando
// moedaDoTemplate — a moeda da Imortal pelos slots 48, 49 e 62 do template, que
// a Mesa não governava antes da 0176.
func hidrasEntrada(tab droprule.Table, r *rng.MSVC, umEm int, moedaDoTemplate bool) hidrasContagem {
	var c hidrasContagem
	for range hidrasPersonagens {
		if umEm > 0 && r.Intn(umEm) == 0 {
			c.chaves++
		}
	}
	conta := func(mob string) {
		for _, regra := range tab.Rolls(mob) {
			if !droprule.Roll(regra.Chance, r.Intn) {
				continue
			}
			switch regra.Item {
			case itemChaveCasteloOrc:
				c.chaves++
			case itemOvoDenteDeSabre:
				c.ovos++
			case itemMoeda5Mi:
				c.moedas++
			}
		}
	}
	for range hidrasDouradasPorEntrada {
		conta("Hidra_Dourada")
	}
	for range hidrasImortaisPorEntrada {
		conta("Hidra_Imortal")
		if moedaDoTemplate {
			for _, slot := range []int{48, 49, 62} {
				if loot.Drops(r, loot.EffectiveDropRate(slot, 0, 311)) {
					c.moedas++
				}
			}
		}
	}
	return c
}

func hidrasResumo(t *testing.T, nome string, v []hidrasContagem) (chaves, ovos, moedas float64) {
	t.Helper()
	porChave := make([]int, len(v))
	for i, c := range v {
		chaves += float64(c.chaves)
		ovos += float64(c.ovos)
		moedas += float64(c.moedas)
		porChave[i] = c.chaves
	}
	n := float64(len(v))
	chaves, ovos, moedas = chaves/n, ovos/n, moedas/n
	sort.Ints(porChave)
	semChave := sort.SearchInts(porChave, 1)
	t.Logf("%-6s por entrada: %.2f chaves (%.0f%% das entradas sem nenhuma), %.2f ovos, %.2f moedas de 5Mi (%.1f milhões)",
		nome, chaves, 100*float64(semChave)/n, ovos, moedas, 5*moedas)
	return chaves, ovos, moedas
}

// A arena das Hidras antes e depois da 0176, com o grupo de 4 do relato. Rode
// com -v para ver os números.
func TestHidrasEntradaAntesEDepois(t *testing.T) {
	antes := hidrasTabela(t, "0065_arenas_kaizen_hidra.up.sql", "0075_arena_elfos_e_chave_orc.up.sql")
	depois := hidrasTabela(t, "0065_arenas_kaizen_hidra.up.sql", "0075_arena_elfos_e_chave_orc.up.sql",
		"0176_hidras_chave_so_no_abate.up.sql")

	r := rng.NewSeeded(27092026)
	va := make([]hidrasContagem, hidrasEntradas)
	vd := make([]hidrasContagem, hidrasEntradas)
	for i := range hidrasEntradas {
		va[i] = hidrasEntrada(antes, r, 4, true)
		vd[i] = hidrasEntrada(depois, r, 0, false)
	}
	chA, _, moA := hidrasResumo(t, "antes", va)
	chD, ovD, moD := hidrasResumo(t, "depois", vd)

	if chA < 1.5 || chA > 1.9 {
		t.Errorf("antes: %.2f chaves por entrada, a conta da 0176 dá ~1,7", chA)
	}
	// O pedido: 1 chave a cada 2 entradas.
	if chD < 0.4 || chD > 0.6 {
		t.Errorf("depois: %.2f chaves por entrada, o alvo é 0,5", chD)
	}
	if ovD < 0.35 || ovD > 0.55 {
		t.Errorf("depois: %.2f ovos por entrada, a 0176 promete ~0,45", ovD)
	}
	if moD < 0.8 || moD > 1.0 || moD <= moA {
		t.Errorf("moedas de 5Mi por entrada: %.2f antes, %.2f depois; a 0176 promete ~0,9 e mais que antes", moA, moD)
	}
}
