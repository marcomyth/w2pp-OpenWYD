package handler

// A escada de add das armas que MONSTRO COMUM solta nos spots com add: o Deserto
// (deserto_armas.go), o spot dos Ciclopes (ciclopes.go) e a sala da fonte da
// Dungeon (dungeon_caveiras.go). Pedido do Marco em 30/09/2026, depois de uma
// Cruz Sagrada 72 e um Arco Divino 54 em uma hora de Verme: "precisa ficar ao
// menos 5 horas para cair algo legal e não minutos", e que a arma venha com add
// aleatório, "uma 27 de dano, por exemplo".
//
// Antes cada spot tinha a sua escada, e todas começavam no alto: o Deserto e os
// Ciclopes davam 45-72 com 30% das armas em 63 ou mais; a fonte, 45-63 com 40%
// em 54 ou mais. Os logs de produção mostram o efeito: um jogador tirou cinco
// Cruzes Sagradas do Verme numa tarde, todas com o add alto.
//
// Agora todo monstro comum sorteia a mesma escada, que começa no degrau mais
// baixo do bônus de drop e sobe até o topo antigo, e o que passa do teto do
// legado (45 de dano, 20 de magia: o SetItemBonus nunca dá mais que isso numa
// arma) é raro. Os pesos são por mil:
//
//	dano    27   36   45   54   63   72
//	magia   12   16   20   24   28   32
//	peso   420  330  206   28   12    4
//
// Com o rand() de 15 bits do MSVC, Intn(1000) dá 33 chances em 32.768 aos
// valores abaixo de 768 e 32 aos outros; os três degraus do alto ficam todos
// acima de 768 e pagam um pouco menos que o peso: 54 ou mais em 4,30% das
// armas, 63 ou mais em 1,56%, e o 72 em 0,39%.
//
// A conta das horas, com um jogador matando 300 monstros por hora (o ritmo que
// os logs de "drop table hit" dão a quem farma o Verme) e a arma a 1,5% por
// morte no Deserto (migração 0188): 4,5 armas por hora, 54+ uma vez a cada 5 h,
// 63+ a cada 14 h e o 72 a cada 55 h. Nos Ciclopes (0,93% por morte) e na fonte
// (1,16%), com menos arma por hora, o add alto demora mais ainda.
//
// Os chefes não usam esta escada: cada um tem a sua, pendendo para o alto, e o
// renascimento de horas já é a raridade deles.
var (
	addTropaFisica = []addArma{
		{420, efDamage, 27},
		{330, efDamage, 36},
		{206, efDamage, 45},
		{28, efDamage, 54},
		{12, efDamage, 63},
		{4, efDamage, 72},
	}
	addTropaMagica = []addArma{
		{420, efMagic, 12},
		{330, efMagic, 16},
		{206, efMagic, 20},
		{28, efMagic, 24},
		{12, efMagic, 28},
		{4, efMagic, 32},
	}
)
