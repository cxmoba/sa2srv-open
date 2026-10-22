package handler

import (
	"encoding/binary"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
)

// 战斗背景 (MAPCG_NO, S 0x0b10 +0): 同 SA1, 战斗场地是独立的小地图。
// mapdata 头: "MAP\0", +4 u32 size, +0xa u16 id, +0xc u16 w, +0xe u16 h, +0x14 u16 区域/图块集号, +0x16 u16 ?
// 10000-13999 号 = ~30-38 格见方的小图 = 战斗场地, 每 100 号一组 (组长 xx00 是 36x36, 其余 30-34)。
// 场景图与战斗图共用 +0x14 区域号 (Halbol/100 -> 100 -> 10000..10017 组), 据此按区域随机。
type bgInfo struct{ id, w, h int }

var (
	bgOnce   sync.Once
	bgByArea map[int][]bgInfo
	areaOf   map[int]int
)

func loadBG() {
	bgByArea, areaOf = map[int][]bgInfo{}, map[int]int{}
	fs, _ := os.ReadDir(MapDir)
	for _, f := range fs {
		n := f.Name()
		if !strings.HasSuffix(n, ".bin") || len(n) != 9 {
			continue
		}
		id, err := strconv.Atoi(n[:5])
		if err != nil {
			continue
		}
		d := make([]byte, 0x18)
		fh, err := os.Open(filepath.Join(MapDir, n))
		if err != nil {
			continue
		}
		_, err = fh.Read(d)
		fh.Close()
		if err != nil || string(d[:3]) != "MAP" {
			continue
		}
		w, h := int(binary.LittleEndian.Uint16(d[0xc:])), int(binary.LittleEndian.Uint16(d[0xe:]))
		area := int(binary.LittleEndian.Uint16(d[0x14:]))
		areaOf[id] = area
		if id >= 10000 && id < 14000 && w >= 34 && w <= 40 && h >= 34 && h <= 40 {
			bgByArea[area] = append(bgByArea[area], bgInfo{id, w, h})
		}
	}
}

// pickBattleBG: 按当前场景图的区域号随机选战斗图; 找不到则回退 12000 (ServerDummy)。
func pickBattleBG(fieldMap int) bgInfo {
	bgOnce.Do(loadBG)
	if l := bgByArea[areaOf[fieldMap]]; len(l) > 0 {
		return l[rand.Intn(len(l))]
	}
	return bgInfo{battleBG, 36, 36}
}

func (b bgInfo) String() string { return fmt.Sprintf("%d(%dx%d)", b.id, b.w, b.h) }
