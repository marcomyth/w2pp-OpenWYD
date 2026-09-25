package world

// Enquanto a Batalha Real do Coliseu corre, quem está na arena aparece para os
// outros como "??????", sem capa e sem guilda (GetCreateMob, GetFunc.cpp:1186;
// SendScore, SendFunc.cpp:1284). O pacote de criação é montado em muitos
// lugares, e o que todos têm à mão é o mundo — por isso a caixa mora aqui, e
// quem a liga e desliga é o evento (handler/coliseu.go).

type anonimato struct {
	ligado         bool
	x1, y1, x2, y2 int16
}

// SetAnonimato liga a caixa (bordas incluídas) ou, com ligado false, desliga.
// Loop-only.
func (w *World) SetAnonimato(ligado bool, x1, y1, x2, y2 int16) {
	w.anonimato = anonimato{ligado: ligado, x1: x1, y1: y1, x2: x2, y2: y2}
}

// Anonimo diz se quem está em (x, y) deve aparecer sem nome, capa e guilda.
// Loop-only.
func (w *World) Anonimo(x, y int16) bool {
	a := w.anonimato
	return a.ligado && x >= a.x1 && x <= a.x2 && y >= a.y1 && y <= a.y2
}
