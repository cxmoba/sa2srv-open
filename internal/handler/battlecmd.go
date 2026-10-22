package handler

// 战斗指令 (2026-10-17 第 6 轮): 防御 / 逃跑 / 捕捉 / 暴击。
//
// C0B20 +8 act (国服抓包): 0x00 等待, 0x03 攻击, 0x08 防御 (目标=自己), 0x58 捕捉 (+0xf=5, +0x10=怪 id),
// 0x99 逃跑, 0x9a 道具。未知 act 一律跳过 + 日志 (不再当攻击)。
//
// 客户端 S0B30 动作分派 0x587a90 (act-3 跳表 0x587ebc/0x587e70):
//   act 8    → 0x58ea40 防御姿势 (ActEfe 0x97 时走 0x586ad0)
//   act 0x99 → 0x58f630 逃跑 (同组单位一起跑); ActEfe 0x97 → 0x586ad0 (motion 0x1a, 失败踉跄)
//   act 0x58 → 0x587d86 捕捉 (Pas 0xa4 → 0x4563d0)
// S0BE0 EndFlag: 0 胜利结算; &0xf!=0 无结算; 其中 &7!=0 (且 !=0xf) 额外开 UI 0xfc (全员倒下提示) —— 逃跑发 8。

import (
	"fmt"
	"log"
	"math/rand"

	"sa2srv/internal/gamedata"
	"sa2srv/internal/server"
)

// hitRoll: 一次普通攻击的结果。
type hitRoll struct {
	dmg               int
	miss, crit, guard bool
}

// rollHit: SA1 风格: 闪避 5%+敏捷差 (≤25%); 暴击 3%+敏捷差/4 (≤20%) → 伤害 ×1.5 且无视一半防御。
func rollHit(atk, def, atkAgi, defAgi int) hitRoll {
	critPct := min(max(3+(atkAgi-defAgi)/4, 1), 20)
	if k := knob("critpct", -1); k >= 0 {
		critPct = k
	}
	if rand.Intn(100) < critPct {
		d, _ := calcDamageNoDodge(atk, def/2)
		return hitRoll{dmg: max(d*3/2, 1), crit: true}
	}
	if k := knob("misspct", -1); k >= 0 {
		if rand.Intn(100) < k {
			return hitRoll{dmg: 0, miss: true}
		}
	}
	d, miss := calcDamage(atk, def, atkAgi, defAgi)
	return hitRoll{dmg: d, miss: miss}
}

func calcDamageNoDodge(atk, def int) (int, bool) {
	if atk < 1 {
		atk = 1
	}
	if def < 0 {
		def = 0
	}
	return max(atk*atk*(90+rand.Intn(21))/(100*(atk+def)), 1), false
}

// act: 转成 S0B30 条目。
// 国服伤害字/Miss/暴击图 (0x458fa0 按 CND 分支, 0x457a90 按 ActEfe 分支; 与 SA1 BCF 同位):
//
//	CND bit2=4 / ActEfe 0x60 → 效果 0x57 CG 0x58a8 暴击星
//	CND bit0=1 / ActEfe 0x5c → 效果 0x53 CG 0x58a4 Miss 图
//	CND bit3=8 / ActEfe 0x5f → 效果 0x56 CG 0x58a7 防御
//	CND bit4=16 / ActEfe 0x5e → 效果 0x55 CG 0x58a6 反击
//
// Pas 仍可用 knob critpas/misspas/guardpas 覆盖 (0=默认受击)。
func (r hitRoll) act(c server.Conn, actor, target uint32, pas byte, ko bool) bAct {
	switch {
	case r.crit && knob("critpas", 0) != 0:
		pas = byte(knob("critpas", 0))
	case r.miss && knob("misspas", 0) != 0:
		pas = byte(knob("misspas", 0))
	case r.guard && knob("guardpas", 0) != 0:
		pas = byte(knob("guardpas", 0))
	}
	a := hitAct(c, actor, target, r.dmg, pas, ko)
	switch {
	case r.miss:
		a.efe = uint16(knob("missefe", 0x5c))
		a.num = 0
		a.passHP = 0
		a.cnd |= uint16(knob("misscnd", 1)) // BCF_DEATH 位在国服作 Miss 图
	case r.crit:
		a.efe = uint16(knob("critefe", 0x60))
		a.cnd |= uint16(knob("critcnd", 4)) // BCF_KAISHIN
	case r.guard:
		a.efe = uint16(knob("guardefe", 0x5f))
		a.cnd |= uint16(knob("guardcnd", 8)) // BCF_GUARD
	}
	if ko {
		if k := knob("kocnd", koCND); k != 0 {
			a.cnd |= uint16(k) // 倒下 0x20 可与暴击位并存
		}
	}
	return a
}

// guardStep: 防御 (效果在 buildQueue 已生效)。发一条 act 8 让客户端摆防御姿势。
func guardStep(c server.Conn, s *session, bid uint32) (bool, error) {
	log.Printf("sa2: unit 0x%x guards", bid)
	if knob("guardact", 1) == 0 {
		return false, nil
	}
	a := bAct{actor: unitKey(c, bid), target: unitKey(c, bid), act: actGuard}
	a.setCnd = connKind(c) == clientCN
	if err := send(c, 0x0b30, battleAction(a)); err != nil {
		return true, err
	}
	s.syncN++
	return true, sendSync(c, s)
}

// escapeStep: 逃跑。成功率 = 60 + (角色敏捷 - 怪平均敏捷)×2 + 本场已失败次数×10, 夹在 10..95。
// 成功: act 0x99 (角色和宠物跑出画面), 动作播完 → S0BE0 EndFlag 8 (无结算) 回地图, HP 原样保留。
// 失败: act 0x99 ActEfe 0x97 (原地踉跄), 回合继续。
func escapeStep(c server.Conn, s *session) (bool, error) {
	agi, n, sum := playerStats(s).Agi, 0, 0
	for _, f := range s.foes {
		if f.alive() {
			_, _, g := f.stats()
			sum += g
			n++
		}
	}
	avg := 0
	if n > 0 {
		avg = sum / n
	}
	pct := min(max(60+(agi-avg)*2+s.escTries*10, 10), 95)
	if k := knob("escpct", -1); k >= 0 {
		pct = k
	}
	ok := rand.Intn(100) < pct
	log.Printf("sa2: Tester tries to escape (agi %d vs avg %d, try %d, %d%%) -> %v", agi, avg, s.escTries, pct, ok)
	a := bAct{actor: unitKey(c, bidPlayer), target: unitKey(c, bidPlayer), act: actEscape}
	a.setCnd = connKind(c) == clientCN
	if ok {
		s.escaped = true
		s.queue = nil
		// 真机: S0BE0 必须紧跟在本回合最后一个 S0B30 后面发 (和胜利一样: 客户端播完动画再处理结算);
		// 等 C 0x0bd0 动画播完后再单独发 S0BE0 客户端不处理, 卡在战斗画面。
		// 动作默认 act 0x99 (ActEfe 0 = 跑步动作), knob escsucact 可换成 8 (防御) 等。
		_ = sysMsg(c, 0xffff8040, "逃跑成功!")
		a.act = byte(knob("escsucact", int(actEscape)))
		if err := send(c, 0x0b30, battleAction(a)); err != nil {
			return true, err
		}
		s.syncN, s.syncSent = 0, false
		endBattle(c, s)
		return true, nil
	} else {
		s.escTries++
		a.efe = uint16(knob("escfail", 0x97))
		_ = sysMsg(c, 0xffff8040, "逃跑失败!")
	}
	if err := send(c, 0x0b30, battleAction(a)); err != nil {
		return true, err
	}
	s.syncN++
	return true, sendSync(c, s)
}

// catchPct: SA1 风格捕捉率: 基础 20 + 怪损失 HP 比例×60 + (角色等级-怪等级)×2, 夹在 5..95。
func catchPct(s *session, f *foe) int {
	lost := 0
	if f.maxHP > 0 {
		lost = (f.maxHP - max(f.hp, 0)) * 60 / f.maxHP
	}
	return min(max(20+lost+(s.level-f.lv)*2, 5), 95)
}

func canCatch(f *foe) bool {
	return f.m != nil && f.m.e != nil && (f.m.e.PetFlg == 1 || f.m.e.Base.PetFlg == 1)
}

// captureStep: 捕捉 C0B20 act 0x58 → 目标怪。条件: 怪 PETFLG=1、宠物栏 <5。
// 成功: 怪离场 (动作 CND knob capcnd, 默认 0x20 倒下), 按 SA1 ENEMY_createPet 生成宠物进空槽 (S0462), 不给经验/掉落。
func captureStep(c server.Conn, s *session) (bool, error) {
	f := foeByID(s, s.orderTgt[s.char.ID])
	if f == nil || !f.alive() {
		f = firstAlive(s)
	}
	if f == nil {
		return false, nil
	}
	if !canCatch(f) {
		log.Printf("sa2: capture %q: not catchable (PETFLG=0)", f.name)
		_ = sysMsg(c, 0xffff8040, fmt.Sprintf("%s 不能捕捉。", f.name))
		return false, nil
	}
	if len(s.pets) >= maxPets {
		log.Printf("sa2: capture %q: pet slots full", f.name)
		_ = sysMsg(c, 0xffff8040, "宠物栏已满, 无法捕捉。")
		return false, nil
	}
	pct := catchPct(s, f)
	if k := knob("catchpct", -1); k >= 0 {
		pct = k
	}
	ok := rand.Intn(100) < pct
	log.Printf("sa2: capture %q (hp %d/%d lv %d, %d%%) -> %v", f.name, max(f.hp, 0), f.maxHP, f.lv, pct, ok)
	a := bAct{actor: unitKey(c, bidPlayer), target: f.bid, act: byte(knob("capact", actCapture)), pas: byte(knob("cappas", 0))}
	a.setCnd = connKind(c) == clientCN
	if ok {
		a.cnd = uint16(knob("capcnd", koCND))
		f.hp, f.captured = 0, true
		s.monHP = sumFoeHP(s)
		if err := catchPet(c, s, f); err != nil {
			return true, err
		}
	} else {
		_ = sysMsg(c, 0xffff8040, fmt.Sprintf("捕捉 %s 失败。", f.name))
	}
	pkt := battleAction(a)
	if connKind(c) == clientCN && knob("capflag3", 1) != 0 { // 国服自带捕捉动画 (S0B30 btlflag 3, 见 captureAct; 真机已验证)
		pkt = captureAct(c, f.bid, ok)
	}
	if err := send(c, 0x0b30, pkt); err != nil {
		return true, err
	}
	if s.monHP <= 0 && endMode > 0 {
		s.syncN, s.syncSent, s.queue = 0, false, nil
		endBattle(c, s)
		return true, nil
	}
	s.syncN++
	return true, sendSync(c, s)
}

// catchPet: 把捉到的怪变成宠物 (同 /pet, 但等级/种类 = 这只怪)。
func catchPet(c server.Conn, s *session, f *foe) error {
	used := map[int]bool{}
	for _, p := range s.pets {
		used[p.Slot] = true
	}
	slot := -1
	for i := 0; i < maxPets; i++ {
		if !used[i] {
			slot = i
			break
		}
	}
	if slot < 0 {
		return nil
	}
	p := newPetFromEnemy(f.m.e)
	if f.lv > p.Level {
		p.Level = f.lv
		st := f.m.e.Base.Calc(gamedata.Growth{Vit: p.GVit, Str: p.GStr, Tgh: p.GTgh, Dex: p.GDex}, p.Level)
		p.MaxHP = st.MaxHP
	}
	p.HP = min(max(f.maxHP/2, 1), p.MaxHP)
	p.CharID, p.Slot = s.char.ID, slot
	p.Exp, p.NextExp, p.Age, p.Loyalty, p.Hunger = 0, petNext(p.Level), 1, 50, 100
	savePet(&p)
	s.pets = append(s.pets, p)
	log.Printf("sa2: caught pet %q lv%d hp %d/%d -> slot %d", p.Name, p.Level, p.HP, p.MaxHP, slot)
	_ = sysMsg(c, 0xffffff80, fmt.Sprintf("捕捉到了 %s!", p.Name))
	return sendPetInfo(c, &p)
}

// captureAct: S0B30 btlflag 3 (BAF_CAPTURE)。国服客户端自带整套捕捉动画, 服务端只发这一包:
//
//	线上: b[0]=3, 后面直接是 0x13 字节结构体 (没有条数字节! 线上解析 0x4112f0 → 0x424050 拷 0x13 字节;
//	      之前按 0x4b 条目+条数发, 客户端在 0x594652 查不到单位直接崩溃)。
//	+4 u32 ActID (捕手单位键, 角色 0x01000000), +8 u8 ActKind (0x58 = 捕捉),
//	+0xd u32 PassID (被捕怪单位键), +0x11 u8 PassKind (0x5a = 成功; 0x57 = 失败), +0x12 u8 Turn (0)。
//
// 真机 (2026-10-18): 捕手跑到怪面前 → "CAPTURE!" 字 + 粉色绳圈套住怪;
//
//	成功 (0x5a): 怪被绳圈套着、转着圈被拖回捕手身边 → "GET!" → 怪消失。
//	失败 (0x57): 绳圈断开, 捕手自己走回原位, 怪留在场上。
//
// knob: capflag3=0 退回旧的 0x4b 条目 (借倒地效果); capkind/cappk/cappkfail/capturn 覆盖字段。
func captureAct(c server.Conn, target uint32, ok bool) []byte {
	// 国服线上解析 0x4112f0: b[0]=flag; flag 0 才有 b[1]=条数 + 0x4b 条目; flag 1/2/3 后面直接是结构体 (无条数字节)。
	// flag 3 结构体 0x13 字节 (0x424050 拷 0x13)。
	b := make([]byte, 1+0x13)
	b[0] = 3
	e := b[1:]
	put32(e, 4, unitKey(c, bidPlayer))
	e[8] = byte(knob("capkind", actCapture))
	put32(e, 0xd, target)
	e[0x11] = byte(knob("cappk", 0x5a))
	if !ok {
		e[0x11] = byte(knob("cappkfail", 0x57))
	}
	e[0x12] = byte(knob("capturn", 0))
	log.Printf("sa2: S0B30 flag3 capture ActID=0x%x PassID=0x%x ActKind=0x%x PassKind=0x%x Turn=%d ok=%v", unitKey(c, bidPlayer), target, e[8], e[0x11], e[0x12], ok)
	return b
}
