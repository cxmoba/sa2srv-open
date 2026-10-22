package handler

import (
	"bytes"
	"testing"

	"sa2srv/internal/gamedata"
	"sa2srv/internal/saac"
)

func withData(t *testing.T) {
	db, err := gamedata.Load("../../data", "../gamedata/testdata/enemy_cn.csv")
	if err != nil {
		t.Fatal(err)
	}
	old := Data
	Data = db
	t.Cleanup(func() { Data = old })
}

func TestFightUsesEnemyTable(t *testing.T) {
	withData(t)
	c := newConn("t-fight-data", clientCN)
	s := getSess(c.addr)
	s.char = &saac.Char{ID: 0x100, Name: "T", Map: 100, X: 10, Y: 10}
	s.level = 4
	if err := startFight(c, s, 140); err != nil {
		t.Fatal(err)
	}
	if s.mon == nil || s.mon.name != "Rocky" || s.monHP != s.mon.st.MaxHP {
		t.Fatalf("mon %+v hp %d", s.mon, s.monHP)
	}
	var b0b10 []byte
	for _, f := range c.frames(t) {
		if f.Cmd == SV_BattleInit {
			b0b10 = f.Payload
		}
	}
	e := b0b10[0xc+0x65:] // 第 2 单位 = 野怪
	if !bytes.HasPrefix(e[0x27:], append([]byte("Rocky"), 0)) || int(e[0x1c]) != s.mon.lv {
		t.Fatalf("unit name/lv % x", e[0x1c:0x47])
	}
	s.monHP, s.hp = 0, 50
	endBattle(c, s)
	if want := s.mon.e.ExpAt(s.mon.lv) * expMul; s.exp != want && s.level == 4 {
		t.Fatalf("exp %d want %d", s.exp, want)
	}
}

func TestPetMigrateAndNew(t *testing.T) {
	withData(t)
	p := saac.Pet{Name: "Kuku", Species: "Kukuru", Gfx: 25010, Level: 3}
	if !migratePet(&p) || p.Name != "Lizard" || p.Temp == 0 || p.MaxHP < 10 {
		t.Fatalf("%+v", p)
	}
	e := pickEnemy(100, 0, true)
	np := newPetFromEnemy(e)
	if np.Name == "" || len(np.Name) > 31 || np.Temp == 0 || np.MaxHP != np.HP {
		t.Fatalf("%+v", np)
	}
	b := petInfo(&np)
	if !bytes.HasPrefix(b[0xc:], append([]byte(np.Name), 0)) || !bytes.HasPrefix(b[0x62:], append([]byte(np.Name), 0)) {
		t.Fatalf("petinfo name % x", b[0xc:0x2c])
	}
}

func TestMultiMonsterBattle(t *testing.T) {
	withData(t)
	h := &Handler{}
	c := newConn("t-multi", clientCN)
	s := getSess(c.addr)
	s.char = &saac.Char{ID: 0x100, Name: "T", Map: 100, X: 10, Y: 10}
	s.level = 30
	if err := startFightN(c, s, 140, 3); err != nil {
		t.Fatal(err)
	}
	if len(s.foes) != 3 || s.foes[2].id != monsterID+2 || s.foes[2].bid != bidMonster|2<<8 {
		t.Fatalf("foes %+v", s.foes)
	}
	var b0b10 []byte
	for _, f := range c.frames(t) {
		if f.Cmd == SV_BattleInit {
			b0b10 = f.Payload
		}
	}
	if b0b10[0xb] != 4 {
		t.Fatalf("units %d", b0b10[0xb])
	}
	// 角色点第 3 只怪
	ord := make([]byte, 0x38)
	put32(ord, 0, 0x100)
	put32(ord, 0x10, monsterID+2)
	h.onBattlePacket(c, 0x0b20, ord)
	hp0, hp2 := s.foes[0].hp, s.foes[2].hp
	tok := make([]byte, 4)
	for i := 0; i < 200 && !s.ended; i++ {
		put32(tok, 0, s.syncTok)
		s.syncWait = true
		if i == 0 {
			h.onBattlePacket(c, 0x0bd0, tok)
			if s.foes[2].hp >= hp2 && s.foes[0].hp != hp0 {
				t.Fatalf("player hit wrong target: %d/%d %d/%d", s.foes[0].hp, hp0, s.foes[2].hp, hp2)
			}
			continue
		}
		h.onBattlePacket(c, 0x0bd0, tok)
	}
	if !s.ended {
		t.Fatal("battle never ended")
	}
	for _, f := range s.foes {
		if s.hp > 0 && f.alive() {
			t.Fatalf("won with live foe %+v", f)
		}
	}
}
