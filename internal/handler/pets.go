package handler

// 宠物持久化 + S 0x0462 / C 0x0465 / S 0x0466 (字段见 FIELDS.md §2)。
//   - 每角色最多 5 只 (slot 0..4), 表 pets (saac/pets.go)
//   - 登录进世界后发全部 0x0462; GM /pet 加到下一个空槽
//   - C 0x0465 slot → 设为出战宠 (DB 持久) → S 0x0466 u32 petId (obj+0x91d |= 0x20)
//   - /fight 只带出战宠 (BID 0x0200); 胜利加 EXP, EXP>=Next 升级, 重发 0x0462 刷新

import (
	"log"

	"sa2srv/internal/mbcs"
	"sa2srv/internal/saac"
	"sa2srv/internal/server"
)

var store *saac.Store

const maxPets = 5

var petInfoLen = envInt("SA2_PETLEN", 0xa0)

// 宠物数值来自数据表 (enemies.go / internal/gamedata): 攻/防/敏 = SA1 公式 (种族成长 × 个体系数 × 等级)。
// 无模板 (Data 为空或旧数据) 时退回旧的按等级线性公式。
func petStats(p *saac.Pet) (atk, def, spd, spi int) {
	if st, ok := petCalc(p); ok {
		return st.Atk, st.Def, st.Agi, 20 + p.Level
	}
	return 8 + 3*p.Level + p.Slot, 6 + 2*p.Level + p.Slot, 7 + 2*p.Level, 20 + p.Level
}

func petNext(lv int) int { return 10 * lv * lv } // 服务器定义: Lv1 10, Lv2 40, Lv3 90 ...

func loadPets(s *session) {
	s.pets = nil
	if store == nil || s.char == nil {
		return
	}
	ps, err := store.ListPets(s.char.ID)
	if err != nil {
		log.Printf("sa2: load pets: %v", err)
		return
	}
	s.pets = ps
	for i := range s.pets {
		p := &s.pets[i]
		if p.Gfx >= 29000 { // 旧占位 29xxx (巨型 Boss) → 小形象
			p.Gfx = 25000
		}
		if migratePet(p) { // 旧英文模板 → 数据表种族 + 国服中文名
			log.Printf("sa2: pet slot %d migrated -> %q temp=%d gfx=%d", p.Slot, p.Name, p.Temp, p.Gfx)
			savePet(p)
		}
	}
}

func battlePet(s *session) *saac.Pet {
	ps := battlePets(s)
	if len(ps) == 0 {
		return nil
	}
	return ps[0]
}

// battlePets: 出战宠列表 (C 0x1463 可标多只 Battle; C 0x0465 出战数上限, 默认 1)。
func battlePets(s *session) []*saac.Pet {
	if petMode == 0 || s == nil {
		return nil
	}
	cap := s.battleCnt
	if cap <= 0 {
		cap = 1
	}
	if cap > 5 {
		cap = 5
	}
	var out []*saac.Pet
	for i := range s.pets {
		if s.pets[i].Battle {
			out = append(out, &s.pets[i])
			if len(out) >= cap {
				break
			}
		}
	}
	return out
}

func savePet(p *saac.Pet) {
	if store != nil {
		if err := store.SavePet(p); err != nil {
			log.Printf("sa2: save pet: %v", err)
		}
	}
}

// petInfo 构造 S 0x0462 (0x40ed10 → 已有: 0x4cf750 更新 / 新: 0x4daee0)。
func petInfo(p *saac.Pet) []byte {
	b := make([]byte, petInfoLen)
	put32(b, 0, p.PetID())
	put16(b, 4, uint16(p.Slot))
	// +0x08 flags: bit0 有效; bit16 (0x10000) = 休息 (国服 0x534f70: C 0x1463 role 1 置位 / role 0 清除,
	// 宠物详情右下 "状态 战斗/休息" 读它)。非出战宠一律标休息。
	flags := uint32(1)
	if !p.Battle {
		flags |= petFlagRest
	}
	put32(b, 8, flags)
	putMBCSFit(b, 0xc, 0x20, p.Name)
	put16(b, 0x2e, uint16(p.Gfx))
	put16(b, 0x30, uint16(p.Level))
	put32(b, 0x34, uint32(p.Exp))
	put32(b, 0x38, uint32(p.NextExp))
	// Pal 实测 (/petprobe, shots/pal_probe.png): +0x3c/+0x3e Endurance(HP) cur/max,
	// +0x40 Attack, +0x42 Defense, +0x44 Speed, +0x46 Spirit, +0x4a Guts(cur, max 固定 100), +0x88 u16 Loyalty
	put16(b, 0x3c, uint16(p.HP))
	put16(b, 0x3e, uint16(p.MaxHP))
	atk, def, spd, spi := petStats(p)
	put16(b, 0x40, uint16(atk))
	put16(b, 0x42, uint16(def))
	put16(b, 0x44, uint16(spd))
	put16(b, 0x46, uint16(spi))
	for i, id := range defaultPetSkills() { // +0x4c 8×u16 技能号 (petskill.go)
		if i < 8 {
			put16(b, 0x4c+2*i, uint16(id))
		}
	}
	b[0x4a] = byte(clampInt(p.Hunger, 0, 100)) // 气力 u8 (国服 setter 0x4e30cd 按 byte 写; +0x4b 另一 u8, 影响心情)
	put32(b, 0x5c, uint32(p.Age))              // 年龄 u32 (国服详情页只显示十进制前两位: 500→"50", 1234→"12")
	put16(b, 0x88, uint16(p.Loyalty))
	putMBCSFit(b, 0x62, 0x20, p.Species)
	b[0x83], b[0x84], b[0x85], b[0x86] = byte(p.Earth), byte(p.Water), byte(p.Fire), byte(p.Wind)
	return b
}

const petFlagRest = 0x10000

func clampInt(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

func sendPetInfo(c server.Conn, p *saac.Pet) error {
	log.Printf("sa2: S 0x0462 pet %q id=0x%x slot=%d gfx=%d lv=%d exp=%d/%d hp=%d/%d battle=%v",
		p.Name, p.PetID(), p.Slot, p.Gfx, p.Level, p.Exp, p.NextExp, p.HP, p.MaxHP, p.Battle)
	return send(c, 0x0462, petInfo(p))
}

// sendAllPets: 登录后发全部宠物, 出战宠再发 0x0466。
func sendAllPets(c server.Conn, s *session) {
	loadPets(s)
	for i := range s.pets {
		_ = sendPetInfo(c, &s.pets[i])
	}
	if bp := battlePet(s); bp != nil {
		_ = sendPetBattleFlag(c, bp.PetID())
	}
}

// sendPetBattleFlag: S 0x0466 (0x40d1d0) → 0x422370(id) → or byte [obj+0x91d], 0x20.
func sendPetBattleFlag(c server.Conn, id uint32) error {
	b := make([]byte, 4)
	put32(b, 0, id)
	log.Printf("sa2: S 0x0466 PetBattleFlag id=0x%x", id)
	return send(c, 0x0466, b)
}

// addPet: GM /pet [敌怪ID] → 下一个空槽 (省略 ID: 当前地图遇敌表里可当宠物的随机一种)。
func addPet(c server.Conn, s *session, enemyID int) error {
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
		log.Printf("sa2: /pet: all %d slots full", maxPets)
		return send(c, 0x0600, chatPacket(c, 0, 0, 0xffffff00, "System", "宠物栏已满 Pal slots full (5)"))
	}
	var p saac.Pet
	if e := pickEnemy(s.char.Map, enemyID, true); e != nil {
		p = newPetFromEnemy(e)
	} else { // 无数据表
		p = saac.Pet{Name: "Pet", Species: "Pet", Gfx: 25000, Level: 1, MaxHP: 9 + 3*slot, Earth: 5, Water: 5}
		p.HP = p.MaxHP
	}
	p.CharID, p.Slot = s.char.ID, slot
	p.Exp, p.NextExp, p.Age, p.Loyalty, p.Hunger = 0, petNext(p.Level), 1, 50, 100
	p.Battle = battlePet(s) == nil && petMode != 0 // 第一只自动出战
	savePet(&p)
	s.pets = append(s.pets, p)
	if err := sendPetInfo(c, &p); err != nil {
		return err
	}
	if p.Battle {
		return sendPetBattleFlag(c, p.PetID())
	}
	return nil
}

// setPetRole: C 0x1463 role 0 → 标为出战 (可多只, 上限 s.battleCnt); role 1 → 休息。
func setPetRole(c server.Conn, s *session, id uint32, role uint32) error {
	idx := -1
	for i := range s.pets {
		if s.pets[i].PetID() == id {
			idx = i
			break
		}
	}
	if idx < 0 {
		log.Printf("sa2: SendPetRole: unknown pet 0x%x", id)
		return nil
	}
	cap := s.battleCnt
	if cap <= 0 {
		cap = 1
	}
	was := s.pets[idx].Battle
	if role == 0 {
		n := 0
		for i := range s.pets {
			if s.pets[i].Battle {
				n++
			}
		}
		if !was && n >= cap {
			// 满员: 清掉一只非本宠的出战, 腾位置 (旧单宠行为的温和版)
			for i := range s.pets {
				if s.pets[i].Battle && i != idx {
					s.pets[i].Battle = false
					_ = sendPetInfo(c, &s.pets[i])
					savePet(&s.pets[i])
					break
				}
			}
		}
		s.pets[idx].Battle = true
	} else {
		s.pets[idx].Battle = false
	}
	if was != s.pets[idx].Battle {
		savePet(&s.pets[idx])
		_ = sendPetInfo(c, &s.pets[idx])
	}
	if store != nil && s.char != nil {
		// DB: 把所有 Battle 标志写回 (SetBattlePet 只支持单宠; 逐只 SavePet 已够)
	}
	if s.pets[idx].Battle {
		log.Printf("sa2: battle pet + %q slot %d (cnt cap %d)", s.pets[idx].Name, s.pets[idx].Slot, cap)
		return sendPetBattleFlag(c, s.pets[idx].PetID())
	}
	log.Printf("sa2: battle pet - %q slot %d", s.pets[idx].Name, s.pets[idx].Slot)
	return nil
}

// writeBackPet: 战斗后把各出战宠 HP 存库并重发 0x0462。
func writeBackPet(c server.Conn, s *session) {
	if !s.hasPet {
		return
	}
	for _, bp := range battlePets(s) {
		hp := s.petHPs[bp.PetID()]
		if hp == 0 && bp.PetID() == petID(s) { // 兼容旧单宠字段
			hp = s.petHP
		}
		if hp < 1 {
			hp = 1
		}
		bp.HP = hp
		savePet(bp)
		_ = sendPetInfo(c, bp)
	}
}

var winExpDef, winPetExpDef = envInt("SA2_WINEXP", 1000), envInt("SA2_WINPETEXP", 6)

// winExps: 胜利经验 = 野怪 SA1 经验 (enemy.EXP 或 enemybaseexptbl 公式) × 倍率; 无数据表用固定值。
// 被捉走的怪不算 (只算打死的); 全部被捉 → 0 经验 (不能落到固定值 1000)。
func winExps(s *session) (pl, pet int) {
	x, ok := 0, false
	killed := 0
	for _, f := range s.foes { // 多怪: 经验相加
		if f.captured {
			log.Printf("sa2: exp foe %q captured -> +0 exp, no drop", f.name)
			continue
		}
		killed++
		if f.m != nil {
			e := f.m.e.ExpAt(f.m.lv)
			log.Printf("sa2: exp foe %q killed -> base %d", f.name, e)
			x, ok = x+e, true
		}
	}
	if len(s.foes) > 0 && killed == 0 {
		return 0, 0
	}
	if !ok && s.mon != nil && len(s.foes) == 0 {
		x, ok = s.mon.e.ExpAt(s.mon.lv), true
	}
	if ok {
		return max(x*expMul, 1), max(x*petExpMul, 1)
	}
	return winExpDef, winPetExpDef
}

func playerNext(lv int) int { return 2000 * lv } // 服务器定义 (0x0230 无已知 NextEXP 字段)

// rewardWin: 角色 EXP (0x0420 param 21) + 升级 (param 1); 出战宠 EXP/升级 (重发 0x0462)。
func rewardWin(c server.Conn, s *session) {
	if !s.expInit { // 未经选角 (脚本/测试) 才补初始化; EXP 0 是升级后合法值, 不能当哨兵
		s.level, s.exp, s.expInit = baseLevel, baseExp, true
	}
	winExp, winPetExp := winExps(s)
	if s.noDrops && s.hp <= 0 { // 组队 (SA1 _SHARE_EXP): 只有活着的单位拿经验
		winExp = 0
	}
	s.exp += winExp
	// EXP 同时改 param 21 (+0x05, 旧用法) 和 param 2 (+0x09 = 状态窗经验值); 升级时 param 3 (+0x0d NEXT) 跟着变
	id := s.char.ID
	ds := []paramDelta{{id, 21, int32(winExp)}, {id, ParamExp, int32(winExp)}}
	for s.exp >= playerNext(s.level) {
		need := playerNext(s.level)
		s.exp -= need
		ds = append(ds, paramDelta{id, 21, -int32(need)}, paramDelta{id, ParamExp, -int32(need)}, paramDelta{id, ParamLevel, 1},
			paramDelta{id, ParamNextExp, int32(playerNext(s.level+1) - need)})
		ds = append(ds, statDeltas(id, s.level, s.level+1)...)
		s.level++
		log.Printf("sa2: player level up -> %d", s.level)
	}
	log.Printf("sa2: win player exp +%d (exp %d/%d lv %d)", winExp, s.exp, playerNext(s.level), s.level)
	_ = send(c, 0x0420, charParamPacket(ds...))
	saveCharStats(s)
	nd := 0
	for _, f := range s.foes {
		if !f.captured {
			nd++
		}
	}
	if s.noDrops {
		log.Printf("sa2: party battle: drops assigned by partyEnd")
	} else if nd > 0 || len(s.foes) == 0 { // 全被捉 → 不掉落 (rollDrops 至少掷 1 次)
		rollDrops(c, s, nd)
	} else {
		log.Printf("sa2: no drops (all foes captured)")
	}

	for _, bp := range battlePets(s) {
		hp := s.petHPs[bp.PetID()]
		if !s.hasPet || hp <= 0 {
			continue // 倒下的宠不拿经验 (SA1)
		}
		bp.Exp += winPetExp
		if bp.Age < 99 {
			bp.Age++
		}
		for bp.Exp >= bp.NextExp && bp.NextExp > 0 {
			bp.Exp -= bp.NextExp
			bp.Level++
			bp.NextExp = petNext(bp.Level)
			if st, ok := petCalc(bp); ok {
				bp.MaxHP = st.MaxHP
			} else {
				bp.MaxHP += 3
			}
			bp.HP = bp.MaxHP
			log.Printf("sa2: pet %q level up -> %d", bp.Name, bp.Level)
		}
		log.Printf("sa2: win pet %q exp +%d (%d/%d lv %d)", bp.Name, winPetExp, bp.Exp, bp.NextExp, bp.Level)
		savePet(bp)
		_ = sendPetInfo(c, bp)
	}
}

// putMBCS: OnPetInfo +0xc/+0x62 是 MBCS\0 (客户端 strlen 后 MB→wchar; UTF-16 会截成 1 字)。
func putMBCS(b []byte, off int, s string) {
	for i := 0; i < len(s) && off+i < len(b)-1; i++ {
		b[off+i] = s[i]
	}
}

// petTextCS: petInfo 不带连接, 默认按 ClientMode 编码 (国服 GBK)。
func putMBCSFit(b []byte, off, max int, s string) {
	if off+max > len(b) {
		max = len(b) - off
	}
	copy(b[off:], mbcs.Fit(detectClient("").charset(), s, max-1))
}

// saveCharStats: 角色等级 / EXP / 场景 HP 存库 (战斗结算、/lv 后调用)。
func saveCharStats(s *session) {
	if store == nil || s.char == nil || s.level <= 0 {
		return
	}
	hp := s.fieldHP
	if hp <= 0 {
		hp = fieldHPInit
	}
	s.char.Level, s.char.Exp, s.char.HP = s.level, s.exp, hp
	if err := store.SaveStats(s.char.ID, s.level, s.exp, hp); err != nil {
		log.Printf("sa2: save stats: %v", err)
		return
	}
	log.Printf("sa2: saved stats %q lv=%d exp=%d hp=%d", s.char.Name, s.level, s.exp, hp)
}
