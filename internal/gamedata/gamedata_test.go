package gamedata

import (
	"math/rand"
	"os"
	"path/filepath"
	"testing"
)

func TestLoadShipped(t *testing.T) {
	db, err := Load("../../data", "testdata/enemy_cn.csv")
	if err != nil {
		t.Fatal(err)
	}
	if len(db.Enemies) < 4 || len(db.Encount[100]) == 0 {
		t.Fatalf("enemies %d encount %d", len(db.Enemies), len(db.Encount[100]))
	}
	e := db.Enemies[140] // example data: Rockling Lv1-2 (TEMPNO 1)
	if e == nil || e.DisplayName() != "Rocky" || e.CGNO() != 25000 || e.LvMin != 1 {
		t.Fatalf("enemy 140: %q %d", e.DisplayName(), e.CGNO())
	}
	// CN index default name: id<1000 preferred over the "+A" variant
	if db.NameForCGNO(25000) != "Rocky" || db.NameForCGNO(26000) != "Rocky Brute" || db.NameForCGNO(26001) != "" {
		t.Fatalf("cn default %q %q", db.NameForCGNO(25000), db.NameForCGNO(26000))
	}
	// without a CN index the enemybase name is used
	db2, err := Load("../../data", "")
	if err != nil {
		t.Fatal(err)
	}
	if n := db2.Enemies[140].DisplayName(); n != "Rockling" {
		t.Fatalf("no-cn name %q", n)
	}
	for _, id := range db.Order {
		if n := db.Enemies[id].DisplayName(); len(n) > MaxNameBytes || n == "" {
			t.Fatalf("name %q", n)
		}
	}
	st := e.Base.Calc(e.Base.AvgGrowth(), 1)
	if st.MaxHP < 8 || st.MaxHP > 80 || st.Atk < 1 || st.Atk > 15 {
		t.Fatalf("lv1 stats %+v", st)
	}
	if hi := e.Base.Calc(e.Base.AvgGrowth(), 50); hi.MaxHP <= st.MaxHP*5 {
		t.Fatalf("growth %+v", hi)
	}
	if x := e.ExpAt(3); x < 1 || x > 20 {
		t.Fatalf("exp %d", x)
	}
	r := rand.New(rand.NewSource(1))
	for i := 0; i < 50; i++ {
		if p := db.Pick(r, 100, nil); p == nil || p.LvMax > 6 {
			t.Fatalf("pick %+v", p)
		}
	}
}

// SA1 原文件: GBK 编码, 带 ACT_CONDITION 列, 有空列 (= -1), IMGNUMBER 为 SA1 图号, 用 sa2map 映射。
func TestSA1Format(t *testing.T) {
	d := t.TempDir()
	gbkName := []byte{0xce, 0xda, 0xc1, 0xa6} // "乌力" GBK
	base := append(append([]byte{}, gbkName...), []byte(",,,,,,101,20,4,25,25,25,25,100,50,50,50,0,0,0,0,0,0,0,0,7,,,,,,,0,0,0,7,100500,1,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,1\r\n")...)
	os.WriteFile(filepath.Join(d, "enemybase.txt"), append([]byte("#c\r\n"), base...), 0o644)
	os.WriteFile(filepath.Join(d, "enemy.txt"), []byte("x,,,7,101,0,3,1,1,1,-1,0,0,1,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0\n"), 0o644)
	os.WriteFile(filepath.Join(d, "sa2map.txt"), []byte("101,25000\n"), 0o644)
	db, err := Load(d, "testdata/enemy_cn.csv")
	if err != nil {
		t.Fatal(err)
	}
	e := db.Enemies[7]
	if e == nil || e.LvMin != 3 || e.LvMax != 3 || e.Base.Name != "乌力" || e.CGNO() != 25000 || e.Base.Skills[1] != -1 {
		t.Fatalf("%+v %+v", e, e.Base)
	}
}
