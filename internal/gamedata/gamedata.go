// Package gamedata 怪物/宠物数据表, 仿 SA1 gmsv 的 enemybase + enemy 两张表 (char/enemy.c)。
//
//   - enemybase.txt (SA1 enemybasefile, ENEMYTEMP): 种族模板 = 成长率。
//     字段 (逗号分隔, # 开头为注释, 与 SA1 原文件逐列一致, 可直接放入 SA1 的 enemybase1.txt):
//     1..6 名字, ATOMFIXNAME1..5 (素材名, 本服不用)
//     7.. TEMPNO, INITNUM, LVUPPOINT, BASEVITAL, BASESTR, BASETGH, BASEDEX, MODAI, GET,
//     EARTHAT, WATERAT, FIREAT, WINDAT, POISON, PARALYSIS, SLEEP, STONE, DRUNK, CONFUSION,
//     PETSKILL1..7, RARE, CRITICAL, COUNTER, SLOT, IMGNUMBER, PETFLG, SIZE,
//     ATOM(BASEADD,FIXMIN,FIXMAX)x5, LIMITLEVEL [, FUSIONCODE]
//   - enemy.txt (SA1 enemyfile = enemy1.txt, ENEMY): 实例 = 同一种族在不同区域/等级的切片。
//     1 名字(备注), 2 TACTICSOPTION, [3 ACT_CONDITION (SA1 _BATTLENPC_WARP_PLAYER, 按列数自动识别)]
//     然后 ID, TEMPNO, LV_MIN, LV_MAX, CREATEMAXNUM, CREATEMINNUM, TACTICS, EXP, DUELPOINT, STYLE, PETFLG,
//     ITEM1..10, ITEMPROB1..10
//   - sa2map.txt (本服新增, 可选): TEMPNO,CGNO[,CN 名字 id] —— SA1 IMGNUMBER 是 SA1 图号, 在 SA2 无意义;
//     这里给种族指定 SA2 CGNO 和国服 enemy.bin 里的显示名。IMGNUMBER 已在 25000..28999 时可省略。
//   - encount.txt (本服新增): MAP,ENEMY_ID,WEIGHT —— 地图遇敌表 (SA1 是 encount + group 两层, 这里简化成一层)。
//   - data_cn/enemy.csv (国服 enemy.bin 导出): id,name,base_species,cgno,... —— 只用作名字/形象索引。
//
// 文件可以是 UTF-8 或 GBK (SA1 原文件为 GB2312/GBK, 按行自动识别)。
//
// 数值公式照搬 SA1 ENEMY_createEnemy + CHAR_complianceParameter:
//
//	成长系数 g = BASExxx + rand(-2..2), 再把 10 点随机分到 4 项
//	属性(×100) = ((lv-1)*LVUPPOINT + INITNUM) * g
//	MaxHP = (4*VIT + STR + TGH + DEX) / 100
//	攻击 = STR/100 + 0.1*TGH/100 + 0.1*VIT/100 + 0.05*DEX/100
//	防御 = TGH/100 + 0.1*STR/100 + 0.1*VIT/100 + 0.05*DEX/100
//	敏捷 = DEX/100
//	EXP (enemy.EXP == -1 时) = enemybaseexptbl[lv-1] + (rank + alpha) * lv
package gamedata

import (
	"bufio"
	"bytes"
	"encoding/csv"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"

	"sa2srv/internal/mbcs"
)

// Base SA1 ENEMYTEMP (enemybase 一行)。
type Base struct {
	Name                           string
	TempNo, InitNum, LvUpPoint     int
	Vital, Str, Tgh, Dex           int
	ModAI, Get                     int
	Earth, Water, Fire, Wind       int // SA1: 0..100, 合计 100
	Poison, Paralysis, Sleep       int
	Stone, Drunk, Confusion        int
	Skills                         [7]int
	Rare, Critical, Counter, Slot  int
	ImgNumber, PetFlg, Size, Limit int
	CGNO                           int    // SA2 形象 (sa2map 或 IMGNUMBER)
	CNName                         string // 国服显示名 (sa2map 指定的 cn id, 否则按 CGNO 默认名)
}

// Enemy SA1 ENEMY (enemy1 一行)。
type Enemy struct {
	Name, TacticsOption string
	ID, TempNo          int
	LvMin, LvMax        int
	Tactics, Exp, Duel  int
	Style, PetFlg       int
	Items, ItemProbs    [10]int
	Base                *Base
}

// Encounter 地图遇敌条目。
type Encounter struct{ EnemyID, Weight int }

// CNName 国服 enemy.bin 一条。
type CNName struct {
	ID, Species, CGNO int
	Name              string
}

// DB 全部表。
type DB struct {
	Bases   map[int]*Base // TEMPNO →
	Enemies map[int]*Enemy
	Order   []int // enemy id 按文件顺序
	Encount map[int][]Encounter
	CN      map[int]CNName // cn id →
	cnByCG  map[int]CNName // CGNO → 默认名 (id<1000 优先, 再短名, 再小 id)
	cnByNm  map[string]CNName
	Skipped int // enemy 行因 TEMPNO 不存在被跳过 (SA1 同样跳过)
}

// MaxNameBytes: S0462 名字区 0x20 (31 字节 + 0), 战斗单位名 0x20。国服表最长 27。
const MaxNameBytes = 31

func lines(path string) ([]string, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var out []string
	sc := bufio.NewScanner(bytes.NewReader(b))
	sc.Buffer(make([]byte, 64*1024), 1<<20)
	for sc.Scan() {
		raw := sc.Bytes()
		var l string
		if utf8.Valid(raw) {
			l = string(raw)
		} else {
			l = mbcs.DecodeAs(mbcs.GBK, raw)
		}
		l = strings.TrimPrefix(l, "\ufeff")
		l = strings.TrimRight(l, "\r\n \t")
		if l == "" || l[0] == '#' {
			continue
		}
		out = append(out, l)
	}
	return out, sc.Err()
}

// atoi SA1 风格: 空串 = def (SA1 enemybase 预置 -1), 其余 atoi (非法 = 0)。
func atoi(s string, def int) int {
	s = strings.TrimSpace(s)
	if s == "" {
		return def
	}
	v, err := strconv.Atoi(s)
	if err != nil {
		// atoi 语义: 前缀数字
		n := 0
		for n < len(s) && (s[n] >= '0' && s[n] <= '9' || (n == 0 && s[n] == '-')) {
			n++
		}
		v, _ = strconv.Atoi(s[:n])
	}
	return v
}

func parseBase(l string) (*Base, error) {
	f := strings.Split(l, ",")
	const str = 6
	const minInts = 31 // 至少到 IMGNUMBER
	if len(f) < str+minInts {
		return nil, fmt.Errorf("enemybase: %d fields < %d", len(f), str+minInts)
	}
	n := f[str:]
	g := func(i int) int {
		if i < len(n) {
			return atoi(n[i], -1)
		}
		return -1
	}
	b := &Base{Name: strings.TrimSpace(f[0]),
		TempNo: g(0), InitNum: g(1), LvUpPoint: g(2), Vital: g(3), Str: g(4), Tgh: g(5), Dex: g(6),
		ModAI: g(7), Get: g(8), Earth: g(9), Water: g(10), Fire: g(11), Wind: g(12),
		Poison: g(13), Paralysis: g(14), Sleep: g(15), Stone: g(16), Drunk: g(17), Confusion: g(18),
		Rare: g(26), Critical: g(27), Counter: g(28), Slot: g(29), ImgNumber: g(30), PetFlg: g(31), Size: g(32),
		Limit: g(48)}
	for i := range b.Skills {
		b.Skills[i] = g(19 + i)
	}
	if b.ImgNumber >= 25000 && b.ImgNumber < 29000 {
		b.CGNO = b.ImgNumber
	}
	return b, nil
}

func parseEnemy(l string) (*Enemy, error) {
	f := strings.Split(l, ",")
	start := 2
	if len(f) >= 3+31 { // SA1 _BATTLENPC_WARP_PLAYER: 第 3 列 ACT_CONDITION
		start = 3
	}
	if len(f) < start+11 {
		return nil, fmt.Errorf("enemy: %d fields", len(f))
	}
	n := f[start:]
	g := func(i int) int {
		if i < len(n) {
			return atoi(n[i], 0)
		}
		return 0
	}
	e := &Enemy{Name: strings.TrimSpace(f[0]), TacticsOption: f[1],
		ID: g(0), TempNo: g(1), LvMin: g(2), LvMax: g(3), Tactics: g(6), Exp: g(7), Duel: g(8), Style: g(9), PetFlg: g(10)}
	for i := 0; i < 10; i++ {
		e.Items[i], e.ItemProbs[i] = g(11+i), g(21+i)
	}
	// SA1: lv_min==0 → =lv_max; 再排序
	if e.LvMin == 0 {
		e.LvMin = e.LvMax
	}
	if e.LvMin > e.LvMax {
		e.LvMin, e.LvMax = e.LvMax, e.LvMin
	}
	if e.LvMin < 1 {
		e.LvMin, e.LvMax = 1, max(1, e.LvMax)
	}
	return e, nil
}

// LoadCN 读 data_cn/enemy.csv。
func (db *DB) LoadCN(path string) error {
	fh, err := os.Open(path)
	if err != nil {
		return err
	}
	defer fh.Close()
	r := csv.NewReader(fh)
	r.FieldsPerRecord = -1
	recs, err := r.ReadAll()
	if err != nil {
		return err
	}
	for i, rec := range recs {
		if i == 0 || len(rec) < 4 {
			continue
		}
		c := CNName{ID: atoi(rec[0], 0), Name: strings.TrimSpace(rec[1]), Species: atoi(rec[2], 0), CGNO: atoi(rec[3], 0)}
		db.CN[c.ID] = c
		if old, ok := db.cnByCG[c.CGNO]; !ok || better(c, old) {
			db.cnByCG[c.CGNO] = c
		}
		if old, ok := db.cnByNm[c.Name]; !ok || better(c, old) {
			db.cnByNm[c.Name] = c
		}
	}
	return nil
}

// better: 同一 CGNO 默认名: 原始种族段 (id<1000) > 名字短 > id 小; 带 "+A" 的强化版最后。
func better(a, b CNName) bool {
	ka := func(c CNName) (int, int, int) {
		p := 0
		if c.ID >= 1000 {
			p = 1
		}
		if strings.Contains(c.Name, "+") || !hasCJK(c.Name) {
			p = 2
		}
		return p, len(c.Name), c.ID
	}
	a1, a2, a3 := ka(a)
	b1, b2, b3 := ka(b)
	if a1 != b1 {
		return a1 < b1
	}
	if a2 != b2 {
		return a2 < b2
	}
	return a3 < b3
}

func hasCJK(s string) bool {
	for _, r := range s {
		if r >= 0x4e00 && r <= 0x9fff {
			return true
		}
	}
	return false
}

// NameForCGNO 国服默认名 (没有则 "")。
func (db *DB) NameForCGNO(cg int) string { return db.cnByCG[cg].Name }

// Load 读 dir 下 enemybase.txt / enemy.txt / sa2map.txt / encount.txt, 以及 cnPath (可空)。
func Load(dir, cnPath string) (*DB, error) {
	db := &DB{Bases: map[int]*Base{}, Enemies: map[int]*Enemy{}, Encount: map[int][]Encounter{},
		CN: map[int]CNName{}, cnByCG: map[int]CNName{}, cnByNm: map[string]CNName{}}
	if cnPath != "" {
		if err := db.LoadCN(cnPath); err != nil {
			return nil, fmt.Errorf("cn names: %w", err)
		}
	}
	ls, err := lines(filepath.Join(dir, "enemybase.txt"))
	if err != nil {
		return nil, err
	}
	for i, l := range ls {
		b, err := parseBase(l)
		if err != nil {
			return nil, fmt.Errorf("enemybase line %d: %w", i+1, err)
		}
		db.Bases[b.TempNo] = b
	}
	if ls, err := lines(filepath.Join(dir, "sa2map.txt")); err == nil {
		for _, l := range ls {
			f := strings.Split(l, ",")
			if len(f) < 2 {
				continue
			}
			b := db.Bases[atoi(f[0], -1)]
			if b == nil {
				continue
			}
			b.CGNO = atoi(f[1], 0)
			if len(f) >= 3 && strings.TrimSpace(f[2]) != "" {
				if c, ok := db.CN[atoi(f[2], -1)]; ok {
					b.CNName = c.Name
					if b.CGNO == 0 {
						b.CGNO = c.CGNO
					}
				}
			}
		}
	}
	for _, b := range db.Bases {
		if b.CGNO == 0 { // 没有 sa2map / IMGNUMBER 不是 SA2 CGNO: SA1 名字与国服名完全相同时借用其 CGNO
			if c, ok := db.cnByNm[b.Name]; ok && c.CGNO >= 25000 && c.CGNO < 29000 {
				b.CGNO, b.CNName = c.CGNO, c.Name
			}
		}
		if b.CNName == "" && b.CGNO != 0 {
			b.CNName = db.NameForCGNO(b.CGNO)
		}
	}
	ls, err = lines(filepath.Join(dir, "enemy.txt"))
	if err != nil {
		return nil, err
	}
	for i, l := range ls {
		e, err := parseEnemy(l)
		if err != nil {
			return nil, fmt.Errorf("enemy line %d: %w", i+1, err)
		}
		e.Base = db.Bases[e.TempNo]
		if e.Base == nil { // SA1 同样丢弃找不到模板的行
			db.Skipped++
			continue
		}
		if _, dup := db.Enemies[e.ID]; !dup {
			db.Order = append(db.Order, e.ID)
		}
		db.Enemies[e.ID] = e
	}
	if ls, err := lines(filepath.Join(dir, "encount.txt")); err == nil {
		for _, l := range ls {
			f := strings.Split(l, ",")
			if len(f) < 2 {
				continue
			}
			m, id, w := atoi(f[0], 0), atoi(f[1], 0), 1
			if len(f) >= 3 {
				w = atoi(f[2], 1)
			}
			if db.Enemies[id] != nil && w > 0 {
				db.Encount[m] = append(db.Encount[m], Encounter{id, w})
			}
		}
	}
	return db, nil
}

// DisplayName: 国服名 > enemybase 名 > enemy 备注名, 截到 MaxNameBytes (不拆 UTF-8 字符)。
func (e *Enemy) DisplayName() string {
	n := e.Base.CNName
	if n == "" {
		n = e.Base.Name
	}
	if n == "" {
		n = e.Name
	}
	return TruncUTF8(n, MaxNameBytes)
}

// Mapped: 种族有 SA2 形象 (sa2map / IMGNUMBER / 名字匹配国服表)。
func (e *Enemy) Mapped() bool { return e.Base.CGNO > 0 }

// CGNO SA2 形象 (未配置时 25000)。
func (e *Enemy) CGNO() int {
	if e.Base.CGNO > 0 {
		return e.Base.CGNO
	}
	return 25000
}

func TruncUTF8(s string, n int) string {
	if len(s) <= n {
		return s
	}
	for n > 0 && !utf8.RuneStart(s[n]) {
		n--
	}
	return s[:n]
}

// Growth 个体成长系数 (SA1 gp_gear 随机后的 BASEVITAL..BASEDEX)。
type Growth struct{ Vit, Str, Tgh, Dex int }

// RollGrowth: BASE + rand(-2..2), 再随机加 10 点 (SA1 ENEMY_createEnemy)。
func (b *Base) RollGrowth(r *rand.Rand) Growth {
	g := Growth{b.Vital + r.Intn(5) - 2, b.Str + r.Intn(5) - 2, b.Tgh + r.Intn(5) - 2, b.Dex + r.Intn(5) - 2}
	for i := 0; i < 10; i++ {
		switch r.Intn(4) {
		case 0:
			g.Vit++
		case 1:
			g.Str++
		case 2:
			g.Tgh++
		case 3:
			g.Dex++
		}
	}
	return g
}

// AvgGrowth: 不随机 (宠物升级重算时若无存档系数用它): BASE + 2.5 → 取 BASE+2。
func (b *Base) AvgGrowth() Growth {
	return Growth{b.Vital + 2, b.Str + 3, b.Tgh + 2, b.Dex + 3}
}

// Stats 战斗派生值。
type Stats struct{ MaxHP, Atk, Def, Agi int }

// Calc SA1 PARAM_CAL + CHAR_complianceParameter。
func (b *Base) Calc(g Growth, lv int) Stats {
	if lv < 1 {
		lv = 1
	}
	k := (lv-1)*max(b.LvUpPoint, 0) + max(b.InitNum, 0)
	v, s, t, d := float64(k*g.Vit), float64(k*g.Str), float64(k*g.Tgh), float64(k*g.Dex)
	st := Stats{
		MaxHP: int((4*v + s + t + d) * 0.01),
		Atk:   int(s*0.01 + t*0.01*0.1 + v*0.01*0.1 + d*0.01*0.05),
		Def:   int(t*0.01 + s*0.01*0.1 + v*0.01*0.1 + d*0.01*0.05),
		Agi:   int(d * 0.01),
	}
	st.MaxHP = max(st.MaxHP, 1)
	st.Atk = max(st.Atk, 1)
	return st
}

// enemybaseexptbl SA1 include/enemyexptbl.h (lv1..200)。
var enemybaseexptbl = []int{
	1, 2, 3, 4, 5, 6, 9, 12, 15, 18, 22, 26, 30, 35, 40, 46, 52, 58, 65, 72, 79, 87,
	95, 104, 113, 122, 131, 141, 151, 162, 173, 184, 196, 208, 220, 233, 246, 260, 274, 288, 303, 318, 333, 348,
	365, 381, 398, 415, 432, 450, 468, 486, 506, 525, 545, 564, 585, 606, 627, 648, 670, 692, 714, 737, 760, 784,
	808, 832, 857, 882, 907, 933, 959, 956, 1012, 1040, 1067, 1095, 1123, 1152, 1181, 1210, 1240, 1270, 1300, 1331, 1362, 1394,
	1426, 1458, 1490, 1524, 1557, 1590, 1625, 1659, 1694, 1729, 1764, 1800,
	1836, 1872, 1909, 1946, 1983, 2021, 2059, 2097, 2136, 2175,
	2214, 2254, 2294, 2334, 2374, 2414, 2455, 2496, 2537, 2578,
	2619, 2661, 2703, 2745, 2787, 2829, 2872, 2915, 2958, 3000,
	3043, 3088, 3132, 3176, 3220, 3264, 3309, 3354, 3399, 3444,
	3489, 3535, 3581, 3627, 3673, 3719, 3765, 3812, 3859, 3906,
	3953, 4000, 4047, 4095, 4143, 4191, 4239, 4287, 4335, 4384,
	4433, 4482, 4531, 4580, 4629, 4679, 4729, 4779, 4829, 4879,
	4929, 4980, 5031, 5082, 5133, 5184, 5235, 5287, 5339, 5391,
	5443, 5495, 5547, 5599, 5652, 5705, 5758, 5811, 5864, 5917,
	5970, 6024, 6078, 6132, 6186, 6240, 6295, 6350, 6405, 6460,
}

var rankBonus = []float64{2.5, 2.0, 1.5, 1.0, 0.5, 0.0}

// Rank SA1 ENEMY_getRank: 成长总和 >=100 → 0, >=95 → 1 ... (注意: 名次越小越强, 经验加成越高)。
func (b *Base) Rank() int {
	sum := b.Vital + b.Str + b.Tgh + b.Dex
	for i, th := range []int{100, 95, 90, 85, 80, 0} {
		if sum >= th {
			return i
		}
	}
	return 5
}

// ExpAt SA1 ENEMY_getExp (enemy.EXP != -1 时直接用它)。
func (e *Enemy) ExpAt(lv int) int {
	if e.Exp != -1 {
		return e.Exp
	}
	if lv < 1 || lv > len(enemybaseexptbl) {
		return 0
	}
	b := e.Base
	pos := func(x int) int { return max(x, 0) }
	alpha := float64(pos(b.Critical)+pos(b.Counter)+pos(b.Get)+pos(b.Poison)+pos(b.Paralysis)+pos(b.Sleep)+
		pos(b.Stone)+pos(b.Drunk)+pos(b.Confusion))/100.0 + float64(pos(b.Rare))
	v := int(float64(enemybaseexptbl[lv-1]) + (rankBonus[b.Rank()]+alpha)*float64(lv))
	return max(v, 1)
}

// RollLevel LV_MIN..LV_MAX。
func (e *Enemy) RollLevel(r *rand.Rand) int { return e.LvMin + r.Intn(e.LvMax-e.LvMin+1) }

// Pick 按地图遇敌表加权随机 (表空则从全部 enemy 里均匀选)。filter 可空。
func (db *DB) Pick(r *rand.Rand, mapID int, filter func(*Enemy) bool) *Enemy {
	var cand []Encounter
	ok := func(e *Enemy) bool { return e.Mapped() && (filter == nil || filter(e)) }
	for _, en := range db.Encount[mapID] {
		if ok(db.Enemies[en.EnemyID]) {
			cand = append(cand, en)
		}
	}
	if len(cand) == 0 {
		ids := append([]int(nil), db.Order...)
		sort.Ints(ids)
		for _, id := range ids {
			if ok(db.Enemies[id]) {
				cand = append(cand, Encounter{id, 1})
			}
		}
	}
	if len(cand) == 0 {
		return nil
	}
	tot := 0
	for _, c := range cand {
		tot += c.Weight
	}
	x := r.Intn(tot)
	for _, c := range cand {
		if x < c.Weight {
			return db.Enemies[c.EnemyID]
		}
		x -= c.Weight
	}
	return db.Enemies[cand[0].EnemyID]
}
