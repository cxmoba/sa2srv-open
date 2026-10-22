package handler

// 最小 PvE 战斗 (2026-10-12 逆向, 见 PACKETS.md "Battle")。
// S 0x0B10 BattleInit (0x40d6f0 -> 0x4231b0 换场景 -> 回调 0x4fc9a0 -> 0x423300):
//   头 0x0c: +0 u16 mapNo, +3 u8 mode (8/9 特殊), +4 s16, +6 s16 (0x47a0d0(map,+4,+6) 建战场), +0xa u8 我方 side, +0xb u8 cnt
//   UI: cnt * 0x63 条目 (拷到对象 +0x90c, 0x4562d0): +0 u32 id, +4 u8 side, +0x17 u32 HP, +0x1b u8 SP, +0x1d u16 CND,
//   +0x21 u32 maxHP?, +0x4b 起 24 字节 CHAR_POS: +0x4b u32 id(宠物归属查找键), +0x4f u16 map, +0x51 s16 x, +0x53 s16 y,
//   +0x55 u8 dir, +0x56 u8, +0x57 u16 形象
//   CN: cnt * 0x65 条目 (0x58fcc0 复制 0x65, 循环 0x42894c/0x429095 +0x65); +0x00..+0x4a 同 UI,
//   新增 2 字节在 +0x4b..+0x4c (语义未知, 填 0), CHAR_POS 整体后移到 +0x4d (map +0x51, x +0x53, y +0x55, dir +0x57, 形象 +0x59)。
//   id==自己 -> 0xb24; 否则查 player pet 表(0x4ca600): 命中且 mode!=8 -> 0xb18 己方宠;
//   side==我方非宠 -> 0xb14; 否则敌人 0x4ad530。

import (
	"log"
	"os"
	"strconv"
	"strings"

	"sa2srv/internal/mbcs"
	"sa2srv/internal/server"
)

const (
	SV_BattleInit         = 0x0b10
	SV_BattleEnd          = 0x0be0
	bidPlayer, bidMonster = 0, 1       // 战场 ID (BID) = 条目 +4 u32; 动作 ActID/PasID 用它 (地图 +0x398 map 以 BID 为键)
	monsterID             = 0x60000001 // 类型 6 = 怪物 (ServerDummy 0x419620 用 0x60000015)
	battleBG              = 12000      // ServerDummy 同款; MAPCG_NO: 战斗背景 (不能是已加载的地图号, 否则 0x47a0d0 返回普通地图对象 -> vtbl+0xf0 崩溃)
)

// bUnit: BID at entry+4 is u32 key for battle map +0x398 (479340).
// Low byte of BID must equal side (ally check cmp [entry+4], header+0xa).
// Unique BIDs: player 0x0000, monster 0x0001, pet 0x0200 (side 0).
type bUnit struct {
	id, bid   uint32
	side      byte
	hp, maxHP uint32
	x, y      int16
	dir       byte
	gfx       uint16
	orderable bool   // entry+7 → obj+0x913; 0 uses filter-flag path in 47a280
	name      string // +0x27 MBCS\0 (CN 0x590050: strlen → 宽字符 → obj+0x840 显示名; 两版 +0..+0x4a 相同)
	lv        int    // +0x1c (头顶 "LV.n"; 待真机确认)
}

const (
	rowFront = 0
	rowBack  = 1
)

// formPos: 战斗阵型槽位 → 格坐标 (0x0b10 单位 +0x51/+0x53 是显式格坐标, 无槽位字段)。
// 双方沿 y 轴对阵: side 0 (我方) 在 +y, 面向 -y (dir 7); side 1 在 -y, 面向 +y (dir 3)。
// 前排离中线近, 后排远; 一排沿 x 一字排开, 间距 slotGap, 以中心居中 (n = 该排单位数)。
// 距离按地图高度缩放: 前排 h*18% (≥2), 后排 = 前排 + rowGap。
var slotGap, rowGap = envInt("SA2_SLOTGAP", 6), envInt("SA2_ROWGAP", 5)
var frontPct = envInt("SA2_FRONTPCT", 18)
var monN = envInt("SA2_MONN", 1)
var monGfx = envInt("SA2_MONGFX", 27010) // 真实小怪 (25010 种类的色 2); 旧 29000 = 巨型 Boss

func formPos(cx, cy int16, h, side, row, slot, n int) (int16, int16) {
	front := h * frontPct / 100
	if front < 2 {
		front = 2
	}
	d := front + row*rowGap
	x := int(cx) + (2*slot-(n-1))*slotGap/2
	if side == 1 {
		d = -d
		x = int(cx) - (2*slot-(n-1))*slotGap/2 // 镜像
	}
	return int16(x), int16(int(cy) + d)
}

// battleLayout: S0B10 单位条目布局 (按客户端协议)。
type battleLayout struct {
	stride, pos int // 条目长度, CHAR_POS 起点
}

func layoutFor(k clientKind) battleLayout {
	if k == clientUI {
		return battleLayout{0x63, 0x4b}
	}
	return battleLayout{0x65, 0x4d}
}

const unitNameOff, unitNameMax = 0x27, 0x20

func battleInit(k clientKind, m uint16, px, py int16, us []bUnit) []byte {
	L := layoutFor(k)
	b := make([]byte, 0xc+L.stride*len(us))
	put16(b, 0, m)
	if hdr2 >= 0 {
		put16(b, 2, uint16(hdr2))
	}
	put16(b, 4, uint16(px))
	put16(b, 6, uint16(py))
	b[8], b[9] = 1, 1 // ServerDummy
	if hdr8 >= 0 {
		put16(b, 8, uint16(hdr8))
	}
	// b[3]: BattlemapInit Flag (1 = 先制 "Make a surprise attack!")
	b[0xa] = 0
	b[0xb] = byte(len(us))
	for i, u := range us {
		e := b[0xc+L.stride*i : 0xc+L.stride*(i+1)]
		put32(e, 0, u.id)
		// +4 u32 BID (BatInit "BID%x"); low byte = side for ally check at 0x42370b
		bid := u.bid
		if bid == 0 && u.side != 0 {
			bid = uint32(u.side)
		}
		put32(e, 4, bid)
		if u.orderable {
			e[7] = 1 // → obj+0x913; non-zero takes 47a280 path A
		}
		put32(e, 0x13, u.maxHP) // +0x13 最大HP, +0x17 当前HP (0x45176f: [+0x17] > [+0x13] -> "HP MAX ERROR")
		put32(e, 0x17, u.hp)
		e[0x1b] = 10
		e[0x1c] = 1
		if u.lv > 0 && unitLvField {
			e[0x1c] = byte(min(u.lv, 255))
		}
		e[0x25] = 2
		if u.name != "" {
			copy(e[unitNameOff:unitNameOff+unitNameMax-1], mbcs.Fit(k.charset(), u.name, unitNameMax-1))
		}
		// CHAR_POS: +0 id (宠物归属查找键), +4 map, +6 x, +8 y, +0xa dir, +0xc 形象
		put32(e, L.pos+0, u.id)
		put16(e, L.pos+4, m)
		put16(e, L.pos+6, uint16(u.x))
		put16(e, L.pos+8, uint16(u.y))
		e[L.pos+0xa] = u.dir
		put16(e, L.pos+0xc, u.gfx)
	}
	return b
}

// 测试用环境变量: SA2_MONHP / SA2_HP / SA2_BG (强制战斗图)
var monMax, hpMax, forceBG, endMode = envInt("SA2_MONHP", 18), envInt("SA2_HP", 100), envInt("SA2_BG", 0), envInt("SA2_ENDMODE", 1)
var fastMode = envInt("SA2_FAST", 0)
var unitLvField = envInt("SA2_UNITLV", 1) == 1
var monPas = envInt("SA2_MONPAS", 0)
var hdr8, noNudge = envInt("SA2_HDR8", -1), envInt("SA2_NONUDGE", 0) == 1
var winCnt = envInt("SA2_CNT", 1)
var camDX, camDY = envInt("SA2_CAMDX", 0), envInt("SA2_CAMDY", 0)
var noCND, syncV = envInt("SA2_NOCND", 1) == 1, envInt("SA2_SYNCV", 1) == 1
var syncSeq uint32

// sendSync: S 0x0BD0 同步请求。客户端在空闲时把首 u32 原样回显为 C 0x0BD0 (CN 0x59c1f8 / UI 0x47decf)。
// 记录待确认 token, onBattlePacket 只接受匹配且未消费过的回显 (重复/迟到/错 token 不推进)。
func sendSync(c server.Conn, s *session) error {
	b := make([]byte, 4)
	syncSeq++
	if syncSeq == 0 {
		syncSeq = 1
	}
	if !syncV { // 旧测试开关: 发 0 token
		s.syncTok = 0
	} else {
		s.syncTok = syncSeq
	}
	put32(b, 0, s.syncTok)
	s.syncWait = true
	return send(c, 0x0bd0, b)
}

var hdr2 = envInt("SA2_HDR2", -1)
var unitHPv = envInt("SA2_UNITHP", 0)

func curFieldHPOr(s *session) int {
	if unitHPv > 0 {
		return unitHPv
	}
	return curFieldHP(s)
}

func fieldHPMaxOr() int {
	if unitHPv > 0 || os.Getenv("SA2_HP") != "" {
		return unitHP()
	}
	return fieldHPMax
}

func unitHP() int {
	if unitHPv > 0 {
		return unitHPv
	}
	return hpMax
}

// 场景 HP: charLoginData +0x11 = 150 (cur), +0x13 = 200 (max) —— 客户端 0x54c156/0x54c163 成对读 player+0x953/+0x955 (HP/最大HP)。
// 0x0420 param 4 -> +0x11, param 5 -> +0x13 (charparam 映射)。SA2_HP 环境变量仍可强制战斗 HP (测试)。
const (
	fieldHPInit = 150
	fieldHPMax  = 200
	ParamHP     = 4
)

func curFieldHP(s *session) int {
	if os.Getenv("SA2_HP") != "" {
		return hpMax
	}
	if s.fieldHP <= 0 {
		s.fieldHP = fieldHPInit
	}
	return s.fieldHP
}

// writeBackHP: 战斗结束把 HP 写回场景 (失败按 1 HP 处理)。
func writeBackHP(c server.Conn, s *session) {
	old := curFieldHP(s)
	nhp := s.hp
	if nhp < 1 {
		nhp = 1
	}
	s.fieldHP = nhp
	saveCharStats(s)
	log.Printf("sa2: field HP %d -> %d (S 0x0420 param %d delta %d)", old, nhp, ParamHP, nhp-old)
	if nhp != old {
		_ = send(c, 0x0420, charParamPacket(paramDelta{s.char.ID, ParamHP, int32(nhp - old)}))
	}
}

var fieldPosReply = envInt("SA2_POSREPLY", 0) == 1
var petMode, petBID, petGfx = envInt("SA2_PET", 1), envInt("SA2_PETBID", 0x200), envInt("SA2_PETGFX", 29000)

func envInt(k string, d int) int {
	if v, err := strconv.Atoi(os.Getenv(k)); err == nil {
		return v
	}
	return d
}

// startFight: /fight [敌怪ID]。野怪从数据表选 (当前地图遇敌表), HP/攻击按 SA1 公式; 无数据表时用 SA2_MONHP/SA2_MONGFX。
func startFight(c server.Conn, s *session, enemyID int) error { return startFightN(c, s, enemyID, 0) }

// startFightN: n>0 指定野怪只数 (/fight ID N)。
func startFightN(c server.Conn, s *session, enemyID, n0 int) error {
	battleMu.Lock()
	defer battleMu.Unlock()
	s.battleHost, s.noDrops, s.leaveAfter = nil, false, nil
	for _, bp := range battlePets(s) {
		_ = sendPetInfo(c, bp)
		_ = sendPetBattleFlag(c, bp.PetID())
	}
	ch := s.char
	bg := pickBattleBG(int(ch.Map))
	if forceBG > 0 {
		bg = bgInfo{forceBG, 36, 36}
	}
	cx, cy := int16(bg.w/2+camDX), int16(bg.h/2+camDY) // 按图尺寸缩放 ServerDummy 的 36x36 布局 (镜头 25,25; 玩家 y=36 在小图会出界, 取 32)
	n := fightCount(enemyID)
	if n0 > 0 {
		n = min(n0, 5)
	}
	s.foes = newFoes(ch.Map, enemyID, n)
	s.mon = s.foes[0].m
	f0 := s.foes[0]
	mhp := f0.maxHP
	us := []bUnit{
		{ch.ID, bidPlayer, 0, uint32(curFieldHPOr(s)), uint32(fieldHPMaxOr()), 0, 0, 7, 31002, true, ch.Name, s.level},
		{f0.id, f0.bid, 1, uint32(f0.hp), uint32(f0.maxHP), 0, 0, 3, uint16(f0.gfx), false, f0.name, f0.lv},
	}
	s.ended, s.orders, s.orderActors, s.hasPet, s.orderTgt, s.queue = false, 0, nil, len(battlePets(s)) > 0, nil, nil
	s.orderItem, s.orderItemTgt = 0, 0
	s.orderAct, s.guard, s.escaped, s.escTries = nil, [2]bool{}, false, 0
	s.inBattle, s.monHP, s.rewarded, s.syncN, s.hp, s.syncSent = true, sumFoeHP(s), false, 0, curFieldHP(s), false
	_ = mhp
	log.Printf("sa2: battle start %q vs %d monster(s), bg %v", ch.Name, len(s.foes), bg)
	for _, f := range s.foes {
		if f.m != nil {
			log.Printf("sa2:   monster #%d id=0x%x enemy %d %q (temp %d) lv%d hp=%d atk=%d def=%d agi=%d gfx=%d",
				f.bid>>8, f.id, f.m.e.ID, f.name, f.m.e.TempNo, f.lv, f.hp, f.m.st.Atk, f.m.st.Def, f.m.st.Agi, f.gfx)
		}
	}
	s.petHPs = map[uint32]int{}
	bps := battlePets(s)
	s.hasPet = len(bps) > 0
	for i, bp := range bps {
		hp := bp.HP
		if hp <= 0 {
			hp = 1
		}
		s.petHPs[bp.PetID()] = hp
		if i == 0 {
			s.petHP = hp // 兼容旧单宠路径
		}
		bid := petBIDAt(i)
		us = append(us, bUnit{bp.PetID(), bid, 0, uint32(hp), uint32(bp.MaxHP), 0, 0, 7, uint16(bp.Gfx), true, bp.Name, bp.Level})
		log.Printf("sa2: battle include pet %q id=0x%x bid=0x%x gfx=%d hp=%d/%d", bp.Name, bp.PetID(), bid, bp.Gfx, hp, bp.MaxHP)
	}
	// 阵型: 角色后排, 宠物前排同 slot (主人正前方); 野怪对面前排。组队时队员角色在后排依次往右排 (partybattle.go)。
	s.allies = partyAllies(s)
	chars := 1
	for _, a := range s.allies {
		if a.pet == nil {
			chars++
		}
	}
	us[0].x, us[0].y = formPos(cx, cy, bg.h, 0, rowBack, 0, chars)
	us[1].x, us[1].y = formPos(cx, cy, bg.h, 1, rowFront, 0, len(s.foes))
	nPet := len(battlePets(s))
	for i := 0; i < nPet; i++ {
		us[2+i].x, us[2+i].y = formPos(cx, cy, bg.h, 0, rowFront, i, max(chars, nPet))
	}
	us = append(us, allyUnits(c, s, cx, cy, bg.h, chars)...)
	// 其余野怪: 前排一字排开, 每只自己的 HP/形象/名字/等级
	for i, f := range s.foes[1:] {
		m := bUnit{f.id, f.bid, 1, uint32(f.hp), uint32(f.maxHP), 0, 0, 3, uint16(f.gfx), false, f.name, f.lv}
		m.x, m.y = formPos(cx, cy, bg.h, 1, rowFront, i+1, len(s.foes))
		us = append(us, m)
	}
	for _, u := range us {
		log.Printf("sa2: formation id=0x%x side=%d (%d,%d) dir=%d", u.id, u.side, u.x, u.y, u.dir)
	}
	k := connKind(c)
	b := battleInit(k, uint16(bg.id), cx, cy, us)
	log.Printf("sa2: 0x0b10 units=%d pet=%v layout=%s stride=0x%x len=%d", b[0xb], s.hasPet, k, layoutFor(k).stride, len(b))
	err := send(c, SV_BattleInit, b)
	startPartyMembers(c, s, uint16(bg.id), cx, cy, us)
	return err
}

// S 0x0B30 OnBattleAction (0x40f030): u8 btlflag (0 BAF_NONE, 1 PETENTER, 2 SIM_ATK ...);
// flag 0: u8 cnt + cnt*0x4b 动作 (管理器 0x47dac3): +4 u32 ActID, +8 u8 Act, +0xa u16 ActNum, +0x10 u16 ActEfe,
// +0x36 u32 PasID, +0x3a u8 Pas, +0x44 u16 CND, +0x46 u8 cap
type bAct struct {
	actor, target uint32
	act           byte
	num, efe      uint16
	pas           byte
	passHP        int32 // +0x3b 目标 HP 增量 (国服 0x58bf50: 负数 = 扣血, 正数 = 回血, 封顶 maxHP)
	cnd           uint16
	item          uint32 // +0xc 物品 ID (act 0x9a)
	setCnd        bool   // 国服: +0x44 写 cnd (覆盖 ServerDummy 的 0x1e37); 欧版保持旧行为
}

var passHPOn, koCND = envInt("SA2_PASSHP", 1), envInt("SA2_KOCND", 0x20)

// unitKey: 动作 ActID/PasID 要用客户端里的 BID 键。BattleInit 条目 +4 u32 BID 的最高字节和 +7 "可下指令" 重叠,
// 所以国服里角色/宠物的键是 BID|0x01000000 (C0B20 里也是 0x01000000 / 0x01000200), 怪物 (+7=0) 不变。
// 之前发裸 BID 0 / 0x200 → 0x599ec0 查不到单位 → 动作直接跳过 (不播动画、不扣血)。
func unitKey(c server.Conn, bid uint32) uint32 {
	if connKind(c) == clientCN && knob("bidhi", 1) != 0 {
		if bid == bidPlayer || isOwnSideBID(bid) {
			return bid | 0x01000000
		}
	}
	return bid
}

// isOwnSideBID: 我方角色/宠的裸 BID (低字节 0)。队长宠 0x200+i*0x20; 队员角色 0x400+i*0x200, 其宠 +0x100+j*0x20。
func isOwnSideBID(bid uint32) bool {
	if bid == uint32(petBID) {
		return true
	}
	if bid >= 0x200 && bid < 0x400 && (bid-0x200)%0x20 == 0 {
		return true // 队长多宠
	}
	return false
}

func petBIDAt(i int) uint32 { return uint32(petBID) + uint32(i)*0x20 }

// hitAct: 普通攻击一条; +0x3b 带目标 HP 增量 (-dmg), 目标倒下时 CND 试验值 SA2_KOCND。
func hitAct(c server.Conn, actor, target uint32, dmg int, pas byte, ko bool) bAct {
	a := bAct{actor: unitKey(c, actor), target: unitKey(c, target), act: actCode, num: uint16(dmg), pas: pas}
	if knob("passhp", passHPOn) != 0 {
		a.passHP = -int32(dmg)
	}
	// CND 位由 hitRoll.act 设 (暴击/Miss/防御/倒下); 这里只保证国服写 +0x44。
	a.setCnd = connKind(c) == clientCN
	_ = ko
	return a
}

// knob: 调试开关, 战斗中可改: 设了 SA2_KNOBDIR 时读 $SA2_KNOBDIR/<name> (整数) 覆盖默认值。
// 现有: monhp (怪 HP), mondmg (怪每击伤害), montgt (0 随机/1 打宠/2 打人), passhp, kocnd, bidhi。
var knobDir = os.Getenv("SA2_KNOBDIR")

func knob(name string, d int) int {
	if knobDir == "" {
		return d
	}
	b, err := os.ReadFile(knobDir + "/" + name)
	if err != nil {
		return d
	}
	v, err := strconv.Atoi(strings.TrimSpace(string(b)))
	if err != nil {
		return d
	}
	return v
}

func battleAction(as ...bAct) []byte {
	b := make([]byte, 2+0x4b*len(as))
	b[0] = 0
	b[1] = byte(len(as))
	for i, a := range as {
		e := b[2+0x4b*i:]
		put32(e, 4, a.actor)
		e[8] = a.act
		put16(e, 0xa, a.num)
		put16(e, 0x10, a.efe)
		put32(e, 0x36, a.target)
		e[0x3a] = a.pas
		put32(e, 0x3b, uint32(a.passHP))
		if dummyAct { // ServerDummy 0x419f10 的单条动作
			e[0xc] = 0x6e
			if a.item != 0 {
				put32(e, 0xc, a.item)
			}
			put16(e, 0x14, 0xcf)
			if !noCND {
				e[0x44], e[0x45], e[0x47] = 0x37, 0x1e, 0xf
			}
		}
		if a.efe != 0 || a.cnd != 0 || a.passHP != 0 {
			log.Printf("sa2: S0B30 act=%d efe=0x%x num=%d pas=0x%x passHP=%d cnd=0x%x", a.act, a.efe, a.num, a.pas, a.passHP, a.cnd)
		}
		if a.setCnd {
			// +0x44 u16 CND = 目标动作后的状态 (同一条目拷进攻方 +0xa2c / 受方 +0xa5c, 国服 0x58983b 写进单位 +0x92d)。
			// 0x20 = 倒下 (0x58f36f 比较 ==0x20); 0x40 另一种 (逃跑?); ServerDummy 的 0x1e37 不再用。
			put16(e, 0x44, a.cnd)
		}
	}
	return b
}

func (h *Handler) onBattlePacket(c server.Conn, cm uint16, p []byte) (bool, error) {
	s := getSess(c.RemoteAddr())
	theHandler = h
	battleMu.Lock()
	defer battleMu.Unlock()
	if ok, err := onMemberBattle(c, s, cm, p); ok {
		return true, err
	}
	switch cm {
	case 0x0b20:
		// C0B20 两版均 0x38 字节; 客户端用 +4 查 actor (首 dword 语义未定)。
		if len(p) != 0x38 {
			log.Printf("sa2: C 0x0b20 len %d != 0x38", len(p))
		}
		s.orders++
		var actor uint32
		if len(p) >= 8 {
			actor = le32(p[4:])
			s.orderActors = append(s.orderActors, actor)
		}
		if len(p) >= 0x14 { // +0 行动者 id, +0x10 目标 id
			if s.orderTgt == nil {
				s.orderTgt = map[uint32]uint32{}
			}
			s.orderTgt[le32(p)] = le32(p[0x10:])
			if s.orderAct == nil {
				s.orderAct = map[uint32]byte{}
			}
			s.orderAct[le32(p)] = p[8]
		}
		// 道具: +8 act 0x9a, +9 u16 物品号, +0xb u32 物品 id, +0x10 目标 id (角色 0x100 / 宠物 0x2000xxxx), 后面是物品名。
		if len(p) >= 0x14 && p[8] == actItem && s.char != nil && le32(p) == s.char.ID {
			s.orderItem, s.orderItemTgt = le32(p[0xb:]), le32(p[0x10:])
		}
		// 宠物技能: act 3 + +0x18 技能名 (petskill.go)
		// 宠物技能两种: 目标方式 0 (敌方单体) 的技能客户端发 act 3 + 名字; 其它目标方式发 act 0x81 + +9 u16 技能号 (名字也在 +0x18)。
		if len(p) >= 0x38 && p[8] == actSkill && s.char != nil && le32(p) != s.char.ID {
			s.petSkill, s.petSkillTgt = findPetSkill(int(le16(p[9:]))), le32(p[0x10:])
			log.Printf("sa2: pet order skill no=%d -> %v tgt=0x%x", le16(p[9:]), s.petSkill != nil, s.petSkillTgt)
		}
		if len(p) >= 0x38 && p[8] == actAttack && s.char != nil && le32(p) != s.char.ID {
			s.petSkill, s.petSkillTgt = nil, le32(p[0x10:])
			if nm := cstrBytes(p[0x18:0x38]); len(nm) > 0 {
				s.petSkill = skillByName(decText(c, nm))
				log.Printf("sa2: pet order skill %q -> %v", decText(c, nm), s.petSkill != nil)
			}
		}
		log.Printf("sa2: battle order #%d actor=0x%x % x", s.orders, actor, p)
		return true, send(c, 0x0b20, p) // 回显 (OnBattleOrder 0x40ef80)
	case 0x0b90: // SendBattleOrderEnter: 点完指令 + 回合计时到点各一次
		log.Printf("sa2: battle order enter (phase %d)", s.syncN)
		// 指令阶段 (syncN==0): C0B90 = 本客户端指令下完 (或计时到点)。
		// 旧逻辑只发一次 S0BD0 且等 C0BD0 才开打: 指令窗还开着时早到的 S0BD0 被客户端丢掉,
		// 要再等一整轮 30s 计时才会第二次 C0B90/C0BD0 → 体感 ~60s。PACKETS.md: 每次 C0B90 都补发。
		// 现: 每次补发 S0BD0; 全员就绪后短延迟强制开打 (C0BD0 先到则取消)。
		if s.syncN != 0 || s.ended || !s.inBattle {
			return true, nil
		}
		switch fastMode {
		case 1:
			_ = send(c, 0x0b90, p)
			return true, nil
		case 2:
			buildQueue(s)
			return true, stepTurn(c, s)
		}
		_ = sendSync(c, s)
		if holdTurn(s) { // 组队: 还有队员没下完
			return true, nil
		}
		armTurnStart(c, s)
		return true, nil
	case 0x0bd0: // SendBattleSynchronize: 指令窗关闭后的回显 (phase 0) / 动作播放完 (phase ≥1)
		log.Printf("sa2: battle C 0x0bd0 % x (phase %d)", p, s.syncN)
		if !s.inBattle || s.ended {
			log.Printf("sa2: C 0x0bd0 ignored (inBattle=%v ended=%v)", s.inBattle, s.ended)
			return true, nil
		}
		if len(p) < 4 || !s.syncWait || le32(p) != s.syncTok {
			log.Printf("sa2: C 0x0bd0 ignored: len=%d waiting=%v want token %d", len(p), s.syncWait, s.syncTok)
			return true, nil
		}
		s.syncWait = false
		if s.syncN == 0 && len(s.queue) == 0 { // 开打
			s.turnGen++ // 取消 armTurnStart
			if holdTurn(s) {
				return true, nil
			}
			buildQueue(s)
		}
		return true, stepTurn(c, s)
	case 0x0b01:
		log.Printf("sa2: battle C 0x0b01 % x -> S 0x0bd1", p)
		return true, send(c, 0x0bd1, orderReq())
	case 0x0410, 0x0412: // 战斗结束回场景后客户端上报的首个位置 (be0 后约 3-5s) = 场景就绪。
		// 0410/0412 同等对待: 客户端压缩器不看战斗状态, 回图首包可能是增量 0412。
		if !s.ended {
			return false, nil
		}
		s.ended, s.inBattle = false, false
		if fieldPosReply {
			return true, returnToField(c, s)
		}
		spawnNpcs(c, s.char.Map) // 战斗会清掉场景 NPC
		resendOthers(c, s)       // 也会清掉同图其他玩家 (真机: 战斗后队员/路人消失)
		return false, nil
	case 0x0bf0: // 战斗结束后客户端回到场景, 重新放回原地图
		log.Printf("sa2: battle C 0x0bf0 % x -> back to map %d", p, s.char.Map)
		ch := s.char
		if !s.inBattle {
			return true, nil
		}
		s.inBattle, s.ended, s.syncWait = false, false, false
		if noNudge {
			return true, nil
		}
		if err := send(c, 0x0410, objPos(ch.ID, uint16(ch.Map), uint16(ch.X), uint16(ch.Y), 4, 30002)); err != nil {
			return true, err
		}
		spawnNpcs(c, ch.Map)
		return true, nil
	case 0x0b02, 0x0b03, 0x0b40, 0x0b50, 0x0b60, 0x0b70, 0x0b80, 0x0bc0, 0x0be0:
		log.Printf("sa2: battle C 0x%04x % x", cm, p)
		return true, nil
	}
	return false, nil
}

// S 0x0BE0 OnBattleEnd (0x40d730 -> 0x424200 -> 管理器 0x47a930):
//
//	+0 u8 EndFlag: &0xf!=0 -> 0x47ae40 无结算窗 (1="All members have fainted");
//	              0 -> 胜利, 打开 UI 0xfb (CBattleResultWin), 后接 0x92 字节块:
//	+1 起 0x92: +0 u8 CharaCnt, +1 起 u32 BID[CharaCnt] (非对齐), +0x5a u8 LevelCnt
//	            LevelCnt>0 时再跟一块 0x92 的升级信息 (LVUP)。
func endBattle(c server.Conn, s *session) {
	win := s.monHP <= 0 && !s.escaped
	var b []byte
	// 逃跑: 国服实测 EndFlag 8 (&0xf!=0, &7==0) 客户端卡在战斗画面不回地图; EndFlag 1 能回但弹 "全员气绝" 框。
	// → 默认按胜利格式发 (flag 0, CharaCnt 0 = 无经验的结算窗), knob escwin=0 改回 flag 路径 (escflag)。
	escWin := s.escaped && knob("escwin", 1) == 1
	if win || escWin {
		b = make([]byte, 1+0x92)
		b[0] = 0 // win
		if win {
			b[1] = byte(winCnt) // CharaCnt
		}
		put32(b, 2, s.char.ID) // 角色 ID (vtbl+0x2c = obj+0xc8), 不是战场 BID
		// LevelCnt at block+0x5a = payload+1+0x5a = +0x5b stays 0
		// 0x4204f0: 结算块 +0x31 起是回场景的 CHAR_POS (+0x35 u16 地图, +0x37/+0x39 x,y) -> 0x508ef0 载入地图。
		// 之前全 0 -> 地图 0 空对象 (小地图名回落到 namelist 第一条 "North Macca")。
		copy(b[1+0x31:], objPos(s.char.ID, uint16(s.char.Map), uint16(s.char.X), uint16(s.char.Y), 4, 30002))
	} else {
		// flag 1 -> 0x425410(payload+1): CHAR_POS (+0 id, +4 u16 地图 -> 0x4c97f0 换图, x, y)。之前只发 1 字节 -> 读到垃圾 -> MapFileOpenError 13xxx
		b = make([]byte, 1+objPosSize)
		b[0] = 1 // lose / fainted
		if s.escaped {
			// EndFlag &7==0 且 &0xf!=0 → 客户端只开 UI 0xfd, 不弹 0xfc ("全员倒下") —— 逃跑用 8。
			b[0] = byte(knob("escflag", 8))
		}
		copy(b[1:], objPos(s.char.ID, uint16(s.char.Map), uint16(s.char.X), uint16(s.char.Y), 4, 30002))
	}
	log.Printf("sa2: battle end win=%v escaped=%v monHP=%d hp=%d petHP=%d", win, s.escaped, s.monHP, s.hp, s.petHP)
	if s.ended { // 幂等
		return
	}
	s.ended, s.syncWait = true, false
	_ = send(c, SV_BattleEnd, b)
	writeBackHP(c, s)
	writeBackPet(c, s)
	if len(s.members) > 0 {
		s.noDrops = true
	}
	if win && !s.rewarded {
		s.rewarded = true
		rewardWin(c, s)
	}
	if len(s.members) > 0 {
		partyEnd(c, s, win)
	}
}

var dummyAct = true
var actCode byte = 3

// monStats: 野怪 攻/防/敏 (数据表 SA1 公式; 无数据表时固定 8/5/5)。
func monStats(s *session) (atk, def, agi int) {
	if s.mon != nil {
		return s.mon.st.Atk, s.mon.st.Def, s.mon.st.Agi
	}
	return 8, 5, 5
}

// petBattleStats: 出战宠 攻/防/敏 (= 宠物栏显示值 petStats)。
func petBattleStats(s *session) (atk, def, agi int) {
	if bp := battlePet(s); bp != nil {
		a, d, g, _ := petStats(bp)
		return a, d, g
	}
	return 8, 5, 5
}

// SA2_MONTGT: 0 随机 (玩家/宠物各半), 1 总打宠物, 2 总打玩家
var monTarget = envInt("SA2_MONTGT", 0)

// returnToField: S 0x0A03 ret=1 mapId (让客户端按 mapId 比较/切回场景图, 0x501b10) + 自己的 0x0410 + NPC。
func returnToField(c server.Conn, s *session) error {
	ch := s.char
	log.Printf("sa2: battle -> field map %d (%d,%d)", ch.Map, ch.X, ch.Y)
	if err := send(c, 0x0410, objPos(ch.ID, uint16(ch.Map), uint16(ch.X), uint16(ch.Y), 4, 30002)); err != nil {
		return err
	}
	spawnNpcs(c, ch.Map)
	return nil
}

var bd1Val = envInt("SA2_BD1", 0)

// S 0x0BD1 OnBattleOrderRequest (0x40eef0): 打开指令菜单 + 回合计时。payload u32 (含义待定, 试验中)。
func orderReq() []byte {
	b := make([]byte, 4)
	put32(b, 0, uint32(bd1Val))
	return b
}

// S 0x0462 OnPetInfo (0x40ed10 -> 0x4cf750 / 新建 0x4daee0):
// 头 0x9c 拷到宠物 +0x918 缓冲 (类型 2 ID = 0x2xxxxxxx; 类型 3 不分配 → 崩).
//
// SA2 offsets (client getters 0x4d9ca8..0x4da1fa) — meanings from SA1 PET + NA/JP Pal UI:
//
//	+0    u32  PET_ID          (SA1: pet index / object id)
//	+4    u16  slot 0..4       (SA1 CHAR_MAXPETHAVE / MAX_PET = 5)
//	+8    u32  flags           (bit0 cleared on remove path 0x4ca680; SA1 status/useFlag-ish)
//	+0xc  MBCS name\0          (SA1 pet.name; strlen+MB2WC → +0x96c; NOT UTF-16)
//	+0x2c u16  ?               (getter present)
//	+0x2e u16  CGNO / graNo    (SA1 graNo; 0 -> "Dino Graphic Res Errer")
//	+0x30 u16  Level           (SA1 level)
//	+0x34 u32  EXP             (SA1 exp)
//	+0x38 u32  NextEXP         (SA1 maxExp; Pal shows 0/10)
//	+0x3c u16  Age             (SA2-only in Pal; not in SA1 client PET struct)
//	+0x3e u16  DP              (SA2 Pal; SA1 "dp" is player fame, not pet)
//	+0x40 u16  HP              (SA1 hp; Pal 耐久力)
//	+0x42 u16  MaxHP           (SA1 maxHp)
//	+0x44 u16  Hunger?/gauge   (SA2 Pal meat gauge; SA1 has no pet hunger)
//	+0x46 u16  Loyalty         (SA1 ai / 忠诚)
//	+0x48 u16  ?               (atk/def/recovery/power may live here or be derived)
//	+0x4a u8 / +0x4b u8 flags  (standby/battle-ish; cf SA1 selectPetNo / battlePetNo)
//	+0x4c 8×u16 skill IDs      (SA1 MAX_SKILL / CHAR_MAXPETSKILLHAVE = 7; SA2 has 8 slots)
//	+0x5c u32  ?  +0x60 u16 ?
//	+0x62 MBCS species\0       (→ +0x988 wchar; NOT UTF-16)
//	+0x82 u8 ?  +0x83..+0x86 u8 earth/water/fire/wind (SA1 earth/water/fire/wind)
//	+0x88 u16 ?  +0x8c u32 ?  +0x98 u16 ?
//
// SA1 diffs: text+CRC+GBK vs SA2 binary UTF-16LE; SA1 atk/def/quick explicit in K packet,
// SA2 Pal also shows 攻撃/防御/回復/威力 (exact SA2 offsets still TBD beyond +0x40..+0x48).
// Battle select: SA1 KS/lssproto_KS (petarray) ↔ SA2 C 0x0465 SendPetBattle (1-byte slot).
//
// S 0x1460 OnPartyPetInfo 同体; S 0x1461 OnDelPartyPetInfo.
// Pal UI: Name/Species/Lv/Age/DP/EXP/HP/Hunger/Loyalty + 5 slots; active battle pet marked.
