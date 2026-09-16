package handler

// parcelaDeForca diz o quanto um personagem é build de Força, em milésimos:
// 1000 é Força pura, 500 é meio a meio e 0 é Destreza pura. É a régua das skills
// de Força da Huntress (Invisibilidade e Troca de Espíritos), decidida pelo Marco
// em 16/09/2026.
//
//	f = 0,5 + (STR − DEX) ÷ (STR + DEX)    limitado a [0, 1]
//
// A Força pura começa em 3× a Destreza, e não só com Destreza zero: um personagem
// de 1600 de Força e 500 de Destreza já joga como Força pura, e a parcela simples
// STR÷(STR+DEX) o deixava em 0,76. Do outro lado é simétrico: a Destreza pura
// começa em 3× a Força.
//
//	1600 / 500 → 1000    2000 / 1000 → 833    1500 / 1000 → 700
//	1000 / 1000 → 500    500 / 1600 → 0
//
// Sem atributo nenhum não há build para ler, e conta como meio a meio.
func parcelaDeForca(str, dex int) int {
	if str < 0 {
		str = 0
	}
	if dex < 0 {
		dex = 0
	}
	if str+dex == 0 {
		return 500
	}
	f := 500 + 1000*(str-dex)/(str+dex)
	if f < 0 {
		return 0
	}
	if f > 1000 {
		return 1000
	}
	return f
}
