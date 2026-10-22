package handler

// 怪物/宠物数据表接入 (internal/gamedata, 仿 SA1 enemybase + enemy)。
//   - /fight [id]: 按当前地图遇敌表 (encount.txt) 选 enemy 实例 → 随机等级 + 个体成长 → HP/攻防 (SA1 公式)
//   - /pet [id]:   同上但只选 PETFLG=1 的实例, 等级 = LV_MIN, 成长系数存库
//   - 名字: 国服 enemy.bin 中文名 (UTF-8 直接上线), S0462 +0xc/+0x62, S0B10 单位 +0x27

import (
	"log"
	"math/rand"
	"time"

	"sa2srv/internal/gamedata"
	"sa2srv/internal/saac"
)

// Data 启动时由 main 载入; nil = 退回旧的固定数值 (测试用)。
var Data *gamedata.DB

var rng = rand.New(rand.NewSource(time.Now().UnixNano()))

// EXP 倍率: SA1 敌怪经验 (lv3 ≈ 10) 远小于本服角色升级曲线 (playerNext = 2000*lv), 角色经验 ×100;
// 宠物升级曲线 (10*lv²) 与 SA1 量级相当, ×1。
var expMul, petExpMul = envInt("SA2_EXPMUL", 100), envInt("SA2_PETEXPMUL", 1)

// monster 本场战斗的野怪。
type monster struct {
	e    *gamedata.Enemy
	lv   int
	st   gamedata.Stats
	name string
	gfx  int
}

func pickEnemy(mapID, id int, petOnly bool) *gamedata.Enemy {
	if Data == nil {
		return nil
	}
	if id > 0 {
		if e := Data.Enemies[id]; e != nil {
			return e
		}
		log.Printf("sa2: enemy id %d not found, random pick", id)
	}
	var f func(*gamedata.Enemy) bool
	if petOnly {
		f = func(e *gamedata.Enemy) bool { return e.PetFlg == 1 || e.Base.PetFlg == 1 }
	}
	return Data.Pick(rng, mapID, f)
}

func newMonster(e *gamedata.Enemy) *monster {
	lv := e.RollLevel(rng)
	st := e.Base.Calc(e.Base.RollGrowth(rng), lv)
	return &monster{e: e, lv: lv, st: st, name: e.DisplayName(), gfx: e.CGNO()}
}

func growthOf(p *saac.Pet) gamedata.Growth {
	return gamedata.Growth{Vit: p.GVit, Str: p.GStr, Tgh: p.GTgh, Dex: p.GDex}
}

func petBase(p *saac.Pet) *gamedata.Base {
	if Data == nil || p.Temp == 0 {
		return nil
	}
	return Data.Bases[p.Temp]
}

// petCalc 宠物派生值 (数据表; 无模板时 ok=false)。
func petCalc(p *saac.Pet) (gamedata.Stats, bool) {
	b := petBase(p)
	if b == nil {
		return gamedata.Stats{}, false
	}
	return b.Calc(growthOf(p), p.Level), true
}

// elem: SA1 属性 0..100 (合计 100) → S0462 字节 (÷10, 合计 10; 旧模板合计 10..20 显示正常)。
func elem(v int) int {
	if v <= 0 {
		return 0
	}
	return v / 10
}

// newPetFromEnemy: SA1 ENEMY_createPet 简化版。
func newPetFromEnemy(e *gamedata.Enemy) saac.Pet {
	b := e.Base
	g := b.RollGrowth(rng)
	n := e.DisplayName()
	p := saac.Pet{Name: n, Species: n, Gfx: e.CGNO(), Level: e.LvMin, EnemyID: e.ID, Temp: b.TempNo,
		GVit: g.Vit, GStr: g.Str, GTgh: g.Tgh, GDex: g.Dex,
		Earth: elem(b.Earth), Water: elem(b.Water), Fire: elem(b.Fire), Wind: elem(b.Wind)}
	st := b.Calc(g, p.Level)
	p.MaxHP, p.HP = st.MaxHP, st.MaxHP
	return p
}

// migratePet: 旧英文模板宠物 (Temp 0) → 按 CGNO 找种族模板; 名字换成国服中文名。成功返回 true。
func migratePet(p *saac.Pet) bool {
	if Data == nil || p.Temp != 0 {
		return false
	}
	var best *gamedata.Base
	for _, b := range Data.Bases {
		if b.CGNO == p.Gfx && (best == nil || b.TempNo < best.TempNo) {
			best = b
		}
	}
	name := Data.NameForCGNO(p.Gfx)
	if best != nil {
		g := best.AvgGrowth()
		p.Temp, p.GVit, p.GStr, p.GTgh, p.GDex = best.TempNo, g.Vit, g.Str, g.Tgh, g.Dex
		p.Earth, p.Water, p.Fire, p.Wind = elem(best.Earth), elem(best.Water), elem(best.Fire), elem(best.Wind)
		if best.CNName != "" {
			name = best.CNName
		}
		st := best.Calc(g, p.Level)
		p.MaxHP = st.MaxHP
		p.HP = st.MaxHP
	}
	if name == "" {
		return best != nil
	}
	name = gamedata.TruncUTF8(name, gamedata.MaxNameBytes)
	changed := best != nil || p.Name != name || p.Species != name
	p.Name, p.Species = name, name
	return changed
}
