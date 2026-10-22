package mapdata

import (
	"os"
	"path/filepath"
	"testing"
)

// 真实国服地图 (不在就跳过): SA2_MAPDATA=<client>/mapdata go test ./internal/mapdata
var cnDir = os.Getenv("SA2_MAPDATA")

func load(t *testing.T, n string) *Map {
	if cnDir == "" {
		t.Skip("SA2_MAPDATA not set")
	}
	p := filepath.Join(cnDir, n)
	if _, err := os.Stat(p); err != nil {
		t.Skipf("no %s", p)
	}
	m, err := Load(p)
	if err != nil {
		t.Fatal(err)
	}
	return m
}

func TestRealMaps(t *testing.T) {
	m := load(t, "00100.bin") // 哈鲁保村: 250×250 子格 = 125×125 地砖 (旧版写死的尺寸)
	if m.ID != 100 || m.Size != 125 || m.Rows != 125 || len(m.Tiles) != 15625 || m.Sections["OBJ1"] == 0 {
		t.Fatalf("100: %+v", m.Sections)
	}
	m = load(t, "00001.bin") // 1420×1100 子格 → 710×550 地砖
	if m.ID != 1 || m.Size != 710 || m.Rows != 550 || len(m.Jumps) != 1766 || len(m.Lands) != 53 {
		t.Fatalf("1: size %d×%d jumps %d lands %d", m.Size, m.Rows, len(m.Jumps), len(m.Lands))
	}
	m = load(t, "10006.bin")
	if m.Size != 20 || m.Rows != 19 || len(m.Jumps) != 18 || len(m.Lands) != 5 {
		t.Fatalf("10006: %d×%d %d %d", m.Size, m.Rows, len(m.Jumps), len(m.Lands))
	}
	if j := m.Jumps[2]; j != (Jump{1, 16, 100, 180}) {
		t.Fatalf("10006 jump[2]=%+v", j)
	}
	if l := m.Lands[1]; l != (Land{1, 2, 17, 1}) {
		t.Fatalf("10006 land[1]=%+v", l)
	}
	// 地砖号 (高 16 位) 应大多在 chipinfo 里: 抽查不为全 0
	nz := 0
	for _, g := range load(t, "00001.bin").Tiles {
		if g != 0 {
			nz++
		}
	}
	if nz < 300000 {
		t.Fatalf("map1 nonzero tiles %d", nz)
	}
}
