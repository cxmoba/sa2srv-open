package handler

// 多怪战斗 + 回合队列 (2026-10-17 第 3 轮)。
//
//   - 每只怪一个 foe: 战场 ID monsterID+i, BID bidMonster|i<<8 (条目 +7=0, 动作键 = BID), 各自 HP/攻防敏/经验。
//   - 回合: 计时结束的 C0BD0 → 建队列 [角色, 宠物(活着), 每只活着的怪] → 每个 C0BD0 (动作播完) 执行下一条,
//     一条 S0B30 + 一条 S0BD0。死掉的行动者跳过; 目标死了改打第一只活着的怪。
//   - 目标: C0B20 +0 = 行动者 id, +0x10 = 目标 id (国服点哪只怪就是哪只的 0x6000000x); 不合法时打第一只活着的。
//   - 怪 HP 到 0: 该条动作 CND 0x20 (倒下); 全部倒下 → 胜利 (endMode>0 时当场结算); 经验 = 每只怪经验之和。

import (
	"fmt"
	"log"
	"math/rand"
	"sort"
	"strings"

	"sa2srv/internal/saac"
	"sa2srv/internal/server"
)

type foe struct {
	m         *monster // nil = 无数据表
	id, bid   uint32
	hp, maxHP int
	name      string
	lv, gfx   int
	captured  bool // 被捉走 (不算经验/掉落)
}

func (f *foe) alive() bool { return f.hp > 0 }

func (f *foe) stats() (atk, def, agi int) {
	if f.m != nil {
		return f.m.st.Atk, f.m.st.Def, f.m.st.Agi
	}
	return 8, 5, 5
}

var monMaxN = envInt("SA2_MONMAX", 3)

// fightCount: /fight 指定 ID 时默认 SA2_MONN 只 (1); 随机遇敌 1..SA2_MONMAX 只。
func fightCount(enemyID int) int {
	if enemyID > 0 {
		return max(monN, 1)
	}
	if monMaxN <= 1 {
		return 1
	}
	return 1 + rand.Intn(monMaxN)
}

// newFoes: 第 i 只从遇敌表独立抽 (指定 ID 时全是同一种)。
func newFoes(mapID int, enemyID, n int) []*foe {
	fs := make([]*foe, 0, n)
	for i := 0; i < n; i++ {
		f := &foe{id: monsterID + uint32(i), bid: uint32(bidMonster) | uint32(i)<<8, hp: monMax, gfx: monGfx, lv: 1}
		if e := pickEnemy(mapID, enemyID, false); e != nil {
			f.m = newMonster(e)
			f.hp, f.gfx, f.name, f.lv = f.m.st.MaxHP, f.m.gfx, f.m.name, f.m.lv
		}
		if k := knob("monhp", 0); k > 0 {
			f.hp = k
		}
		f.maxHP = f.hp
		fs = append(fs, f)
	}
	return fs
}

func sumFoeHP(s *session) int {
	t := 0
	for _, f := range s.foes {
		if f.alive() {
			t += f.hp
		}
	}
	return t
}

func firstAlive(s *session) *foe {
	for _, f := range s.foes {
		if f.alive() {
			return f
		}
	}
	return nil
}

func foeByID(s *session, id uint32) *foe {
	for _, f := range s.foes {
		if f.id == id {
			return f
		}
	}
	return nil
}

// pickTarget: 行动者在 C0B20 选的怪 (活着), 否则第一只活着的。
func pickTarget(s *session, actor uint32) *foe {
	if f := foeByID(s, s.orderTgt[actor]); f != nil && f.alive() {
		return f
	}
	return firstAlive(s)
}

type turnStep struct {
	kind   int // 0 角色, 1 宠物, 2 怪, 3 队员单位 (partybattle.go)
	ally   *ally
	foe    *foe
	pet    *saac.Pet // kind 1
	petBid uint32
	agi    int
}

// 指令 act (C0B20 +8 / S0B30 +8), 国服真机抓包:
//
//	0x00 等待   0x03 攻击 (宠物技能目标方式 0 也用它, +0x18 = 技能名)   0x08 防御   0x58 捕捉 (+0x10 = 目标怪)   0x81 宠物技能   0x99 逃跑   0x9a 道具
const (
	actWait    = 0x00
	actAttack  = 0x03
	actGuard   = 0x08
	actCapture = 0x58
	actEscape  = 0x99
	actSkill   = 0x81 // 宠物技能 (目标方式非 0): +9 u16 技能号, +0x10 目标, +0x18 名字
)

// orderOf: 行动者本回合的指令; 没收到 C0B20 (计时到) = 攻击。
func orderOf(s *session, id uint32) byte {
	if a, ok := s.orderAct[id]; ok {
		return a
	}
	return actAttack
}

func petID(s *session) uint32 {
	if bp := battlePet(s); bp != nil {
		return bp.PetID()
	}
	return 0
}

// buildQueue: 本回合出手顺序 = 敏捷 + 0..敏捷/4 随机, 高的先 (SA1 BATTLE 排序思路)。
// 防御指令在回合开始就生效 (s.guard), 不占出手顺序之外的时间。
func buildQueue(s *session) {
	s.guard = [2]bool{}
	var q []turnStep
	jit := func(a int) int { return a + rand.Intn(max(a/4, 1)+1) }
	if s.hp > 0 {
		q = append(q, turnStep{kind: 0, agi: jit(playerStats(s).Agi)})
		s.guard[0] = orderOf(s, s.char.ID) == actGuard
	}
	for i, bp := range battlePets(s) {
		hp := s.petHPs[bp.PetID()]
		if hp <= 0 {
			continue
		}
		_, _, ag, _ := petStats(bp)
		bid := petBIDAt(i)
		q = append(q, turnStep{kind: 1, pet: bp, petBid: bid, agi: jit(ag)})
		if i == 0 {
			s.guard[1] = orderOf(s, bp.PetID()) == actGuard
		}
	}
	for _, f := range s.foes {
		if f.alive() {
			_, _, ag := f.stats()
			q = append(q, turnStep{kind: 2, foe: f, agi: jit(ag)})
		}
	}
	for _, a := range s.allies {
		a.guard = a.alive() && s.orderAct[a.id] == actGuard
		if a.alive() {
			_, _, ag := a.stats()
			q = append(q, turnStep{kind: 3, ally: a, agi: jit(ag)})
		}
	}
	sort.SliceStable(q, func(i, j int) bool { return q[i].agi > q[j].agi })
	if knob("fixorder", 0) != 0 { // 旧顺序: 角色 → 宠物 → 怪
		sort.SliceStable(q, func(i, j int) bool { return q[i].kind < q[j].kind })
	}
	s.queue = q
	var ord []string
	for _, st := range q {
		ord = append(ord, fmt.Sprintf("%s(%d)", stepName(st), st.agi))
	}
	log.Printf("sa2: turn order %s guard=%v", strings.Join(ord, " > "), s.guard)
}

func stepName(st turnStep) string {
	switch st.kind {
	case 0:
		return "Tester"
	case 1:
		return "pet"
	case 3:
		return allyName(st)
	}
	return fmt.Sprintf("mon#%d", st.foe.bid>>8)
}

// sideDown: 我方全灭 (角色倒下且没有活着的宠物)。
func sideDown(s *session) bool {
	if s.hp > 0 || alliesAlive(s) > 0 {
		return false
	}
	for _, bp := range battlePets(s) {
		if s.petHPs[bp.PetID()] > 0 {
			return false
		}
	}
	return true
}

// stepTurn: 执行队列里下一条能执行的动作; 队列空 → 回合结束 (结算或下一回合 0BD1)。
func stepTurn(c server.Conn, s *session) error {
	for len(s.queue) > 0 && !s.escaped {
		st := s.queue[0]
		s.queue = s.queue[1:]
		sent, err := runStep(c, s, st)
		if err != nil || sent {
			return err
		}
	}
	s.syncN, s.syncSent, s.orders = 0, false, 0
	if s.monHP <= 0 || sideDown(s) || s.escaped {
		endBattle(c, s)
		return nil
	}
	s.orderTgt, s.orderAct, s.petSkill = nil, nil, nil
	return send(c, 0x0bd1, orderReq())
}

// runStep 返回 true = 发了一条动作 (等下一个 C0BD0)。
func runStep(c server.Conn, s *session, st turnStep) (bool, error) {
	if sideDown(s) {
		return false, nil
	}
	var actor uint32
	var atk, agi int
	switch st.kind {
	case 0:
		if s.hp <= 0 { // 倒下的角色不行动 (宠物还能打)
			return false, nil
		}
		switch a := orderOf(s, s.char.ID); a {
		case actItem:
			id, tgt := s.orderItem, s.orderItemTgt
			s.orderItem, s.orderItemTgt = 0, 0
			if id == 0 {
				return false, nil
			}
			return battleItem(c, s, id, tgt)
		case actGuard:
			return guardStep(c, s, bidPlayer)
		case actWait:
			log.Printf("sa2: Tester waits")
			return false, nil
		case actEscape:
			return escapeStep(c, s)
		case actCapture:
			return captureStep(c, s)
		case actAttack:
		default:
			log.Printf("sa2: Tester order act=0x%02x 未实现 → 跳过 (不当攻击)", a)
			return false, nil
		}
		ps := playerStats(s) // 含装备
		actor, atk, agi = bidPlayer, ps.Atk, ps.Agi
	case 1:
		bp := st.pet
		if bp == nil {
			bp = battlePet(s)
		}
		if bp == nil {
			return false, nil
		}
		hp := s.petHPs[bp.PetID()]
		if hp <= 0 {
			return false, nil
		}
		bid := st.petBid
		if bid == 0 {
			bid = uint32(petBID)
		}
		switch a := orderOf(s, bp.PetID()); a {
		case actGuard:
			return guardStep(c, s, bid)
		case actWait:
			log.Printf("sa2: pet %q waits", bp.Name)
			return false, nil
		case actAttack, actSkill:
			if k := s.petSkill; k != nil && bp == battlePet(s) {
				s.petSkill = nil
				if sent, done, err := petSkillStep(c, s, k, s.petSkillTgt); done || err != nil {
					return sent, err
				}
			}
		default:
			log.Printf("sa2: pet %q order act=0x%02x 未实现 → 跳过", bp.Name, a)
			return false, nil
		}
		actor = bid
		atk, _, agi, _ = petStats(bp)
	case 2:
		return true, foeTurn(c, s, st.foe)
	case 3:
		return allyStep(c, s, st.ally)
	}
	who := s.char.ID
	if st.kind == 1 {
		if st.pet != nil {
			who = st.pet.PetID()
		} else {
			who = petID(s)
		}
	}
	f := pickTarget(s, who)
	if f == nil {
		return false, nil
	}
	_, def, fagi := f.stats()
	r := rollHit(atk, def, agi, fagi)
	f.hp -= r.dmg
	s.monHP = sumFoeHP(s)
	log.Printf("sa2: %s hits monster #%d (0x%x) for %d (atk %d vs def %d, miss=%v crit=%v; its hp %d, all %d)",
		stepName(st), f.bid>>8, f.id, r.dmg, atk, def, r.miss, r.crit, max(f.hp, 0), s.monHP)
	if err := send(c, 0x0b30, battleAction(r.act(c, actor, f.bid, 1, f.hp <= 0))); err != nil {
		return true, err
	}
	if s.monHP <= 0 && endMode > 0 {
		s.syncN, s.syncSent, s.queue = 0, false, nil
		endBattle(c, s)
		if endMode == 2 {
			return true, sendSync(c, s)
		}
		return true, nil
	}
	s.syncN++
	return true, sendSync(c, s)
}

// foeTarget: 怪选目标。活着的角色/宠物里挑: 默认各半, 但更爱打血少 (比例低) 的那个 (60%)。
// knob montgt: 1 总打宠, 2 总打人 (测试)。
func foeTarget(s *session) (toPet bool, ok bool) {
	charOK, petOK := s.hp > 0, s.hasPet && s.petHP > 0
	switch {
	case !charOK && !petOK:
		return false, false
	case !petOK:
		return false, true
	case !charOK:
		return true, true
	}
	switch knob("montgt", monTarget) {
	case 1:
		return true, true
	case 2:
		return false, true
	}
	bp := battlePet(s)
	pr := float64(s.petHP) / float64(max(bp.MaxHP, 1))
	cr := float64(s.hp) / float64(fieldHPMax)
	weak := pr < cr // 宠物更虚
	if rand.Intn(100) < 60 {
		return weak, true
	}
	return !weak, true
}

// foeTurn: 一只怪打角色或一只出战宠。防御中伤害减半; 打到 0 → 倒下 (CND 0x20)。
func foeTurn(c server.Conn, s *session, f *foe) error {
	if !f.alive() {
		return stepTurn(c, s)
	}
	if done, err := foeHitsAlly(c, s, f); done || err != nil {
		return err
	}
	type tgt struct {
		bid      uint32
		pet      *saac.Pet
		def, agi int
		guard    bool
	}
	var opts []tgt
	if s.hp > 0 {
		ps := playerStats(s)
		opts = append(opts, tgt{bidPlayer, nil, ps.Def, ps.Agi, s.guard[0]})
	}
	for i, bp := range battlePets(s) {
		if s.petHPs[bp.PetID()] <= 0 {
			continue
		}
		_, d, g, _ := petStats(bp)
		opts = append(opts, tgt{petBIDAt(i), bp, d, g, i == 0 && s.guard[1]})
	}
	if len(opts) == 0 {
		return stepTurn(c, s)
	}
	toPetOnly := knob("montgt", monTarget)
	var pick tgt
	switch toPetOnly {
	case 1:
		for _, o := range opts {
			if o.pet != nil {
				pick = o
				break
			}
		}
		if pick.bid == 0 {
			pick = opts[0]
		}
	case 2:
		pick = opts[0] // 角色优先 (opts[0] 是角色 if alive)
		for _, o := range opts {
			if o.pet == nil {
				pick = o
				break
			}
		}
	default:
		pick = opts[rand.Intn(len(opts))]
	}
	ma, _, mg := f.stats()
	r := rollHit(ma, pick.def, mg, pick.agi)
	if k := knob("mondmg", 0); k > 0 {
		r = hitRoll{dmg: k}
	}
	if pick.guard && r.dmg > 0 {
		r.dmg = max(r.dmg/2, 1)
		r.guard = true
	}
	ko := false
	if pick.pet != nil {
		hp := s.petHPs[pick.pet.PetID()]
		r.dmg = min(r.dmg, hp)
		hp -= r.dmg
		s.petHPs[pick.pet.PetID()] = hp
		if pick.pet == battlePet(s) {
			s.petHP = hp
		}
		ko = hp <= 0
		log.Printf("sa2: monster #%d hits pet %q for %d (guard=%v; hp %d)", f.bid>>8, pick.pet.Name, r.dmg, r.guard, hp)
	} else {
		r.dmg = min(r.dmg, s.hp)
		s.hp -= r.dmg
		ko = s.hp <= 0
		log.Printf("sa2: monster #%d hits Tester for %d (guard=%v; hp %d)", f.bid>>8, r.dmg, r.guard, s.hp)
	}
	if err := send(c, 0x0b30, battleAction(r.act(c, f.bid, pick.bid, byte(monPas), ko))); err != nil {
		return err
	}
	s.syncN++
	return sendSync(c, s)
}

// actItem: C0B20 / S0B30 的 "用道具" 动作码 (客户端 0x4acd0f 写 0x9a)。
const actItem = 0x9a

// itemEfx: 战斗用道具的特效图号 (knob itemefx 可试别的)。
var itemEfx = 60020

// battleItem: 角色这回合用道具 (只支持食物回 HP)。目标 = 角色自己或出战宠物。
// S0B30 条目 act 0x9a (可用 knob itemact 改), ActNum = 特效图号 (act 0x9a 时客户端按 ActNum 读特效 0x58e870,
// 图不存在就弹 "File Not Found CGNOn" / "ReadEffectData : NULL"), PassHP = +回复量 (客户端加到目标 HP, 夹在 0..maxHP)。
func battleItem(c server.Conn, s *session, id, tgt uint32) (bool, error) {
	_, it := findItem(s, id)
	if it == nil {
		return false, nil
	}
	d := itemDefs[it.ItemNo]
	if d == nil || d.kind != "food" {
		_ = sysMsg(c, 0xffff4040, "这个不能在战斗中使用。")
		return false, nil
	}
	target, heal := uint32(bidPlayer), 0
	if bp := battlePet(s); bp != nil && s.hasPet && tgt == bp.PetID() {
		if s.petHP <= 0 {
			return false, nil
		}
		target = uint32(petBID)
		heal = min(s.petHP+d.val, bp.MaxHP) - s.petHP
		s.petHP += heal
		bp.HP = s.petHP
		savePet(bp)
		log.Printf("sa2: battle item %q on pet %q +%d (hp %d/%d)", d.name, bp.Name, heal, s.petHP, bp.MaxHP)
	} else {
		heal = min(s.hp+d.val, fieldHPMax) - s.hp
		s.hp += heal
		log.Printf("sa2: battle item %q on Tester +%d (hp %d)", d.name, heal, s.hp)
	}
	if err := removeItem(c, s, it, 1); err != nil {
		return true, err
	}
	// 客户端 0x45a860 (act 0x9a): ActEffect(+0x10)==0xa → 使用动作 (motion 0xa) + 本地扣物品 (0x458d80 按 +0xc 物品ID);
	// 其它值走 "投掷" 路径 0x599260 (处理同包后续条目)。itemID 在 +0xc。
	a := bAct{actor: unitKey(c, bidPlayer), target: unitKey(c, target), act: byte(knob("itemact", actItem)), num: uint16(knob("itemefx", itemEfx)),
		efe: uint16(knob("itemefe", 0x71)), pas: byte(knob("itempas", 0)), item: id}
	a.passHP = int32(heal)
	a.setCnd = connKind(c) == clientCN
	as := []bAct{a}
	if knob("itemchain", 0) != 0 { // 试验: 第二条 = 目标被动条目
		b := a
		b.actor = a.target
		as = append(as, b)
	}
	if err := send(c, 0x0b30, battleAction(as...)); err != nil {
		return true, err
	}
	s.syncN++
	return true, sendSync(c, s)
}
