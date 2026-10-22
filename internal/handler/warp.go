package handler

// 换图 / 传送点 (2026-10-18)。
//
// 客户端: 收到 id==自己 的 S 0x0410 且 area 与当前不同 → 0x429500 → 0x5090f0 载入新地图 (本地 mapdata)。
// 服务器: data/warp.txt 定义传送格; 合法移动落在传送格 → changeMap:
//   1) 旧图其他人收 S 0x0415 (消失)  2) 自己收 S 0x0410 (新 area/x/y)  3) 存库 (地图+坐标)
//   4) 新图: 和同图玩家互发 0x0410, 刷新该图 NPC。
// 落地格本身若也是传送格, 要先离开再踩回来才触发 (session.warpHold)。

import (
	"bufio"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"sa2srv/internal/cmd"
	"sa2srv/internal/mapdata"
	"sa2srv/internal/server"
)

type warpDef struct{ m, x, y, tm, tx, ty, r int }

var (
	warps     []warpDef
	warpByMap map[int][]*warpDef
	// WarpFromMapdata: 另外从 MapDir 各图 JUMP/JUM2 段生成传送 (warp.txt 手填的优先)。
	WarpFromMapdata = true
)

// WarpFile: 传送点表路径 (相对 sa2srv 目录)。
var WarpFile = "data/warp.txt"

func loadWarps() {
	if warps != nil {
		return
	}
	warps = []warpDef{}
	if f, err := os.Open(WarpFile); err != nil {
		log.Printf("sa2: warp table: %v", err)
	} else {
		sc := bufio.NewScanner(f)
		for sc.Scan() {
			if w, ok := parseWarp(sc.Text()); ok {
				warps = append(warps, w)
			}
		}
		f.Close()
	}
	hand := len(warps)
	if WarpFromMapdata {
		warps = append(warps, mapdataWarps(warps)...)
	}
	indexWarps()
	log.Printf("sa2: warp table %d entries (%d from warp.txt, %d from mapdata JUMP/JUM2)", len(warps), hand, len(warps)-hand)
}

func nearHand(hand []warpDef, m int, j mapdata.Jump) *warpDef {
	var best *warpDef
	bd := 9
	for i := range hand {
		w := &hand[i]
		if w.m != m || w.tm != j.ToMap {
			continue
		}
		if d := max(abs(w.x-j.X), abs(w.y-j.Y)); d < bd {
			best, bd = w, d
		}
	}
	return best
}

var genWarp map[[3]int]*warpDef

func indexWarps() {
	warpByMap = map[int][]*warpDef{}
	genWarp = map[[3]int]*warpDef{}
	for i := range warps {
		w := &warps[i]
		if w.r < 0 { // 生成的: 恰好一格
			k := [3]int{w.m, w.x, w.y}
			if genWarp[k] == nil {
				genWarp[k] = w
			}
			continue
		}
		warpByMap[w.m] = append(warpByMap[w.m], w)
	}
}

// mapdataWarps: 客户端 mapdata 的 JUMP 格 {x,y,目标图,目标落点号} + 目标图 JUM2 落点 → 传送 (半径 0, 每格一条)。
// 跳过: 同图跳 (悬崖等), 落点号 0, 目标图没有该落点 (多为城镇图, 如 100 没有 JUMP/JUM2 段, 靠 warp.txt)。
// 生成的传送 (每格一条, r = -1 表示"恰好这一格", 查 genWarp 哈希)。
//
// 规则 (2026-10-21):
//  1. 客户端 mapdata 的 JUMP 格 = 出口区 (同目标图的相邻格连成一块)。落点号非 0 且目标图 JUM2 有该落点 → 用它;
//     落点号 0 或找不到 → 用同一块里最近的有落点的格的落点; 整块都没有 → 借本图同目标、8 格内手填传送的落点。
//  2. 只在带过渡标记 (地砖低 4 位) 的格触发; 块内无标记则整块触发。实测标记集合 == JUMP 集合。
//     例: 地图 2 → 4 (萨尼那岛): 出口列 x=1231 (落点 4000..4004), 1232..1242 落点号 0 —— 客户端走到 1242 就停下等换图。
//     跳过同图跳 (悬崖等)。
func mapdataWarps(hand []warpDef) []warpDef {
	fs, err := os.ReadDir(MapDir)
	if err != nil {
		log.Printf("sa2: mapdata warps: %v", err)
		return nil
	}
	type key struct{ m, id int }
	jumps := map[int][]mapdata.Jump{}
	files := map[int]string{}
	lands := map[key]mapdata.Land{}
	for _, f := range fs {
		if !strings.HasSuffix(f.Name(), ".bin") {
			continue
		}
		p := filepath.Join(MapDir, f.Name())
		id, js, ls, err := mapdata.LoadLinks(p)
		if err != nil {
			continue
		}
		if len(js) > 0 {
			jumps[id], files[id] = js, p
		}
		for _, l := range ls {
			if _, ok := lands[key{id, l.ID}]; !ok {
				lands[key{id, l.ID}] = l
			}
		}
	}
	type pt = [2]int
	type dest struct {
		tm, tx, ty int
		ok         bool
	}
	var out []warpDef
	nDrop, nNoFlag := 0, 0
	for m, js := range jumps {
		cell := map[pt]mapdata.Jump{}
		for _, j := range js {
			if j.ToMap != m {
				cell[pt{j.X, j.Y}] = j
			}
		}
		if len(cell) == 0 {
			continue
		}
		// 1. 每格自己的落点
		got := map[pt]dest{}
		for c, j := range cell {
			if j.ToID == 0 {
				continue
			}
			if l, ok := lands[key{j.ToMap, j.ToID}]; ok {
				got[c] = dest{j.ToMap, l.X, l.Y, true}
			} else if w := nearHand(hand, m, j); w != nil {
				got[c] = dest{j.ToMap, w.tx, w.ty, true}
			}
		}
		// 2. 分块 (同目标图, 8 邻接连通)。块内没落点的格 → 同块里欧氏距离最近的有落点格;
		//    平手取 y 小、再 x 小 (确定性; 以前用多源 BFS + map 遍历, 切比雪夫距离平手时随机,
		//    导致 2→4 落点在 (52,55..63) 之间乱跳)。地图 2 出口 x=1231 每两行一个落点 4000..4004,
		//    同一行往右延伸的 0 号格 (1232..1242) 因此用同一行的落点。
		zone := map[pt]int{}
		var zones [][]pt
		for c0, j0 := range cell {
			if _, ok := zone[c0]; ok {
				continue
			}
			id := len(zones)
			zone[c0] = id
			comp := []pt{c0}
			for i := 0; i < len(comp); i++ {
				c := comp[i]
				for dx := -1; dx <= 1; dx++ {
					for dy := -1; dy <= 1; dy++ {
						n := pt{c[0] + dx, c[1] + dy}
						if j, ok := cell[n]; ok && j.ToMap == j0.ToMap {
							if _, seen := zone[n]; !seen {
								zone[n] = id
								comp = append(comp, n)
							}
						}
					}
				}
			}
			zones = append(zones, comp)
		}
		for _, comp := range zones {
			var src []pt
			for _, c := range comp {
				if got[c].ok {
					src = append(src, c)
				}
			}
			for _, c := range comp {
				if got[c].ok || len(src) == 0 {
					continue
				}
				best, bd := src[0], -1
				for _, s := range src {
					dx, dy := s[0]-c[0], s[1]-c[1]
					d := dx*dx + dy*dy
					if bd < 0 || d < bd || d == bd && (s[1] < best[1] || s[1] == best[1] && s[0] < best[0]) {
						best, bd = s, d
					}
				}
				got[c] = got[best]
			}
		}
		// 整块没落点: 借手填
		for c, j := range cell {
			if !got[c].ok {
				if w := nearHand(hand, m, j); w != nil {
					got[c] = dest{j.ToMap, w.tx, w.ty, true}
				}
			}
		}
		// 3. 触发格 = 带过渡标记的格 (地砖低 4 位, mapdata.LoadFlags)。全部 92 张有 JUMP 段的图里,
		//    标记格集合与 JUMP 格集合完全相同 (20408 格), 标记只是"这格是出口"的位图, 不含目的地。
		//    保险起见: 某块里一个标记格都没有 → 整块 JUMP 格都触发 (不回退)。
		if _, _, flg, err := mapdata.LoadFlags(files[m]); err == nil && len(flg) > 0 {
			for _, comp := range zones {
				any := false
				for _, c := range comp {
					if flg[c] {
						any = true
						break
					}
				}
				if !any {
					nNoFlag += len(comp)
					continue
				}
				for _, c := range comp {
					if !flg[c] {
						delete(got, c)
						nDrop++
					}
				}
			}
		}
		for c, d := range got {
			if d.ok {
				out = append(out, warpDef{m, c[0], c[1], d.tm, d.tx, d.ty, -1})
			}
		}
	}
	log.Printf("sa2: mapdata warps: %d cells (unflagged JUMP cells dropped %d; zones without flags kept whole %d cells)", len(out), nDrop, nNoFlag)
	return out
}

func parseWarp(line string) (warpDef, bool) {
	line = strings.TrimSpace(line)
	if line == "" || line[0] == '#' {
		return warpDef{}, false
	}
	p := strings.Split(line, ",")
	if len(p) < 6 {
		return warpDef{}, false
	}
	v := make([]int, 7)
	for i := 0; i < len(p) && i < 7; i++ {
		n, err := strconv.Atoi(strings.TrimSpace(p[i]))
		if err != nil {
			return warpDef{}, false
		}
		v[i] = n
	}
	return warpDef{v[0], v[1], v[2], v[3], v[4], v[5], v[6]}, true
}

func findWarp(m, x, y int) *warpDef {
	loadWarps()
	if warpByMap == nil {
		indexWarps() // 测试直接设 warps 时
	}
	for _, w := range warpByMap[m] { // 手填的优先
		if w.m == m && abs(x-w.x) <= w.r && abs(y-w.y) <= w.r {
			return w
		}
	}
	return genWarp[[3]int{m, x, y}]
}

func abs(v int) int {
	if v < 0 {
		return -v
	}
	return v
}

// checkWarp: applyMove 接受移动后调用。返回 true = 已换图。
func (h *Handler) checkWarp(c server.Conn, s *session, m, x, y int) bool {
	w := findWarp(m, x, y)
	if w == nil {
		s.warpHold = false
		return false
	}
	if s.warpHold || s.inBattle {
		return false
	}
	log.Printf("sa2: warp hit map=%d (%d,%d) -> map=%d (%d,%d)", m, x, y, w.tm, w.tx, w.ty)
	h.changeMap(c, s, w.tm, w.tx, w.ty)
	return true
}

// changeMap: 把自己挪到 (m,x,y), 可跨图。
func (h *Handler) changeMap(c server.Conn, s *session, m, x, y int) {
	if s.char == nil {
		return
	}
	h.tradeDrop(s.char.ID) // 换图取消交易, 通知对方关窗 (同断线)
	old := s.char.Map
	worldMu.Lock()
	e := world[c.RemoteAddr()]
	var olds []*worldEntry
	for k, o := range world {
		if k != c.RemoteAddr() && o.sess.char != nil && o.sess.char.Map == old {
			olds = append(olds, o)
		}
	}
	worldMu.Unlock()
	if old != m {
		pl := make([]byte, 4)
		put32(pl, 0, s.char.ID)
		for _, o := range olds {
			_ = send(o.conn, cmd.SV_CharOut, pl)
		}
	}
	s.char.Map, s.char.X, s.char.Y = m, x, y
	s.warpHold = findWarp(m, x, y) != nil
	if h.saac != nil {
		_ = h.saac.SavePos(s.char.ID, m, x, y)
	}
	gfx := uint16(30002)
	dir := byte(4)
	if e != nil {
		gfx = uint16(e.pos[0xc]) | uint16(e.pos[0xd])<<8
		dir = e.pos[0xa]
	}
	p := objPos(s.char.ID, uint16(m), uint16(x), uint16(y), dir, gfx)
	_ = send(c, cmd.SV_ObjPos, p)
	log.Printf("sa2: change map %d -> %d (%d,%d)", old, m, x, y)
	if old == m {
		worldMu.Lock()
		if e != nil {
			copy(e.pos[:], p)
		}
		worldMu.Unlock()
		return
	}
	// 新图: 走 enterWorld 的互发逻辑 (保留 rx 基准), 再补 NPC
	h.enterWorld(c, s)
	defer h.partyWarp(s.char.ID, m, x, y) // 队长换图 → 队员跟过去
	worldMu.Lock()
	if e2 := world[c.RemoteAddr()]; e2 != nil {
		copy(e2.pos[:], p)
	}
	worldMu.Unlock()
	spawnNpcs(c, m)
}
