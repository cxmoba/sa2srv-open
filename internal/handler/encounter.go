package handler

// 野外随机遇敌 (2026-10-18)。
//   - 只在有遇敌表 (data/encount.txt) 且不是安全图 (data/mapflags.txt "safe") 的地图。
//   - 客户端不是每步都报位置 → 按两次上报的切比雪夫距离算步数, 每步掷一次。
//   - 每步几率 encpct (knob, 默认 4%); 遇敌后至少走 encmin 步 (默认 8) 才可能再遇。
//   - 触发 = startFightN(地图遇敌表随机, 1..SA2_MONMAX 只)。

import (
	"bufio"
	"log"
	"math/rand"
	"os"
	"strconv"
	"strings"

	"sa2srv/internal/server"
)

var (
	MapFlagFile = "data/mapflags.txt"
	safeMaps    map[int]bool
)

func loadMapFlags() {
	if safeMaps != nil {
		return
	}
	safeMaps = map[int]bool{}
	f, err := os.Open(MapFlagFile)
	if err != nil {
		log.Printf("sa2: mapflags: %v", err)
		return
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		l := strings.TrimSpace(sc.Text())
		if l == "" || l[0] == '#' {
			continue
		}
		p := strings.Split(l, ",")
		m, err := strconv.Atoi(strings.TrimSpace(p[0]))
		if err != nil {
			continue
		}
		for _, fl := range p[1:] {
			if strings.TrimSpace(fl) == "safe" {
				safeMaps[m] = true
			}
		}
	}
}

func encounterMap(m int) bool {
	loadMapFlags()
	if safeMaps[m] {
		return false
	}
	return Data != nil && len(Data.Encount[m]) > 0
}

// stepEncounter: 合法移动后调用 (steps = 本次移动步数)。返回 true = 开战了。
func stepEncounter(c server.Conn, s *session, m, steps int) bool {
	if steps <= 0 || s.inBattle || !encounterMap(m) || (s.char != nil && isFollower(s.char.ID)) {
		return false
	}
	pct := knob("encpct", 4)
	minSteps := knob("encmin", 8)
	for i := 0; i < steps; i++ {
		s.encSteps++
		if s.encSteps < minSteps {
			continue
		}
		if rand.Intn(100) < pct {
			log.Printf("sa2: random encounter map=%d after %d steps (%d%%/step)", m, s.encSteps, pct)
			s.encSteps = 0
			if err := startFightN(c, s, 0, 0); err != nil {
				log.Printf("sa2: encounter start: %v", err)
			}
			return true
		}
	}
	return false
}
