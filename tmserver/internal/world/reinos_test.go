package world

import "testing"

// Na cidade dos Reinos decide o byte do legado (internal/reinos). Os bytes são
// os dos templates reais: Bruxa 0/64, Lanceiro 0/96, Rei_Harabard 15/111,
// Guarda_Real 100/100, BlackOracle (clã 6) 20/20.
func TestReinosClassificaPeloByteDoLegado(t *testing.T) {
	w := New(Config{GridDim: 4096}, slogDiscard(), nil, nil)
	casos := []struct {
		nome string
		tmpl []byte
		x, y int16
		npc  bool
	}{
		{"Bruxa azul é monstro", campoTemplate(7, 0, 64), 1706, 1686, false},
		{"Lanceiro vermelho é monstro", campoTemplate(8, 0, 96), 1708, 1766, false},
		{"Rei Harabard é monstro", campoTemplate(7, 15, 111), 1750, 1574, false},
		{"Guarda_Real continua NPC", campoTemplate(7, 100, 100), 1720, 1600, true},
		{"oráculo continua NPC", campoTemplate(6, 20, 20), 1720, 1610, true},
		{"a mesma Bruxa fora dos Reinos segue como está", campoTemplate(7, 0, 64), 2600, 1700, true},
	}
	for _, c := range casos {
		id := w.SpawnMob(c.tmpl, c.x, c.y)
		if id < 0 {
			t.Fatalf("%s: não nasceu", c.nome)
		}
		if got := w.Entity(id).NonCombatNPC; got != c.npc {
			t.Errorf("%s: NonCombatNPC = %v, want %v", c.nome, got, c.npc)
		}
	}
}

// Guarda do Reino morto entra na fila de respawn como qualquer monstro.
func TestGuardaDoReinoVolta(t *testing.T) {
	w := New(Config{GridDim: 4096}, slogDiscard(), nil, nil)
	id := w.SpawnMob(campoTemplate(7, 0, 64), 1706, 1686)
	antes := len(w.respawnQueue)
	w.DespawnMob(id, 1)
	if len(w.respawnQueue) != antes+1 {
		t.Errorf("fila de respawn = %d, want %d: o guarda morreu e não vai voltar", len(w.respawnQueue), antes+1)
	}
}

// Na cidade dos Reinos o guarda enxerga o jogador pela capa e pela marca de
// Inimigo do Reino, não pelo clã gravado.
func TestGuardaDoReinoEnxergaCapaEMarca(t *testing.T) {
	agora := uint32(5000)
	w := New(Config{GridDim: 4096, Now: func() uint32 { return agora }}, slogDiscard(), nil, nil)
	const pconn = 5
	w.sessions[pconn] = &Session{Conn: pconn, Mode: UserPlay}
	p := &Entity{ID: pconn, Mode: MobUser, HP: 100, X: 1702, Y: 1686}
	w.entities[pconn] = p
	w.grid.SetMob(1702, 1686, pconn)

	casos := []struct {
		nome        string
		capa        int16
		clanGravado uint8
		marca       uint8 // reino marcado, 0 = nenhum
		azul, verm  bool  // o guarda de cada reino ataca?
	}{
		{"sem capa", 0, 0, 0, false, false},
		{"capa vermelha", 3198, 0, 0, true, false},
		{"capa azul", 736, 0, 0, false, true},
		{"clã 8 gravado, sem capa", 0, 8, 0, false, false},
		{"sem capa, inimigo de Hekalotia", 0, 0, 7, true, false},
		{"capa branca, inimigo de Akelonia", 3199, 0, 8, false, true},
	}
	for _, c := range casos {
		p.Equip[15] = Item{Index: c.capa}
		p.Clan = c.clanGravado
		LimparInimigoDoReino(p)
		if c.marca != 0 {
			w.MarcarInimigoDoReino(p, c.marca)
		}
		if got := w.FindEnemyFromView(1706, 1686, 7) == pconn; got != c.azul {
			t.Errorf("%s: guarda azul ataca = %v, want %v", c.nome, got, c.azul)
		}
		if got := w.FindEnemyFromView(1706, 1686, 8) == pconn; got != c.verm {
			t.Errorf("%s: guarda vermelho ataca = %v, want %v", c.nome, got, c.verm)
		}
	}

	// Fora da cidade dos Reinos a regra antiga fica: clã gravado, sem capa.
	p.X, p.Y = 2602, 1700
	w.grid.ClearMob(1702, 1686)
	w.grid.SetMob(2602, 1700, pconn)
	p.Equip[15] = Item{Index: 3198}
	p.Clan = 0
	LimparInimigoDoReino(p)
	if w.FindEnemyFromView(2606, 1700, 7) == pconn {
		t.Error("fora dos Reinos a capa passou a decidir o aggro")
	}
}
