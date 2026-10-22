package handler

// 宠物技能 (2026-10-18, 国服)。
//   - S 0x0462 +0x4c: 8×u16 技能号 (0 = 空)。
//   - C 0x0530 u16 技能号 (SendBattleSkillInfo 0x411980) → S 0x0530 技能资料 (OnBattleSkillInfo 0x411950 → 0x502010):
//       +0 u16 no, +2 MBCS 名字 (0x21), +0x23 u8, +0x24 u8, +0x41 MBCS 说明 (0x48), +0x89 u8。共 0x8a。
//   - 技能表 data/petskill.txt (SA1 petskill 子集)。

import (
	"bufio"
	"fmt"
	"log"
	"math/rand"
	"os"
	"strconv"
	"strings"

	"sa2srv/internal/server"
)

type petSkill struct {
	id         int
	name, desc string
	fn         string // Mighty / Heal
	tgt        int    // S0530 +0x24 选目标方式
	mul, evade int    // Mighty: 伤害倍数, 命中惩罚 %
	heal       int    // Heal: 回复量
	fp         int    // 消耗
}

var (
	PetSkillFile = "data/petskill.txt"
	petSkills    map[int]*petSkill
)

func loadPetSkills() {
	if petSkills != nil {
		return
	}
	petSkills = map[int]*petSkill{}
	f, err := os.Open(PetSkillFile)
	if err != nil {
		log.Printf("sa2: petskill: %v", err)
		return
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		if k := parsePetSkill(sc.Text()); k != nil {
			petSkills[k.id] = k
		}
	}
	log.Printf("sa2: pet skills %d", len(petSkills))
}

func parsePetSkill(l string) *petSkill {
	l = strings.TrimSpace(l)
	if l == "" || l[0] == '#' {
		return nil
	}
	p := strings.Split(l, ",")
	if len(p) < 6 {
		return nil
	}
	id, err := strconv.Atoi(p[0])
	if err != nil {
		return nil
	}
	k := &petSkill{id: id, name: p[1], desc: p[2], fn: p[3], mul: 1}
	k.fp, _ = strconv.Atoi(strings.TrimSpace(p[5]))
	// 选项: "键值" 连写 (倍2 回避30) 或 "键 值" 分写 (HP 30) 都认。
	f := strings.Fields(p[4])
	for i := 0; i < len(f); i++ {
		key, num := f[i], ""
		if j := strings.IndexAny(key, "-0123456789"); j > 0 {
			key, num = key[:j], key[j:]
		} else if i+1 < len(f) {
			i++
			num = f[i]
		}
		v, _ := strconv.Atoi(num)
		switch key {
		case "倍":
			k.mul = v
		case "回避":
			k.evade = v
		case "HP":
			k.heal = v
		case "目标":
			k.tgt = v
		}
	}
	return k
}

func findPetSkill(id int) *petSkill { loadPetSkills(); return petSkills[id] }

// defaultPetSkills: 宠物技能栏 (DB 暂无技能列, 每只宠物都给这两招; 以后按 enemy 表的技能列换掉)。
func defaultPetSkills() []int {
	loadPetSkills()
	var ids []int
	for _, id := range []int{40, 661} {
		if petSkills[id] != nil {
			ids = append(ids, id)
		}
	}
	return ids
}

func skillInfo(c server.Conn, k *petSkill) []byte {
	b := make([]byte, 0x8a)
	put16(b, 0, uint16(k.id))
	copy(b[2:2+0x20], fitText(c, k.name, 0x20))
	b[0x23] = byte(k.fp)                                    // 消耗气力 (真机: 菜单 "消耗气力 N" 读 +0x23)
	b[0x24] = byte(knob(fmt.Sprintf("skt%d", k.id), k.tgt)) // 选目标方式 (客户端 0x567dc8: 0 默认敌方单体; 1/3/4/5/6 → 光标模式 4/7/8/9/10)
	copy(b[0x41:0x41+0x47], fitText(c, k.desc, 0x47))
	b[0x89] = byte(knob("sk89", 0))
	return b
}

func onSkillInfo(c server.Conn, p []byte) error {
	if len(p) < 2 {
		return nil
	}
	no := int(p[0]) | int(p[1])<<8
	k := findPetSkill(no)
	log.Printf("sa2: C 0x0530 skill info no=%d known=%v", no, k != nil)
	if k == nil {
		return nil
	}
	return send(c, 0x0530, skillInfo(c, k))
}

// 战斗中: 宠物 C0B20 act 3 + "+0x18 技能名 (0x20 字节, 客户端原样回传 S0530 的名字)" = 用技能; 名字空 = 普通攻击。
// 国服不传技能号, 只能按名字找。
func skillByName(name string) *petSkill {
	loadPetSkills()
	for _, k := range petSkills {
		if k.name == name {
			return k
		}
	}
	return nil
}

// petSkillStep: 出战宠用技能。返回 sent 同 runStep。
//
//	Mighty: 伤害 ×mul, 命中率 -evade%; Heal: 给己方单体 (目标 = 角色或宠物自己) 回 heal 点。
//	FP (宠物 "气力" 0x0462 +0x4a) 不够 → 改普通攻击。
//	S0B30: act knob skact (默认 actAttack 3, 伤害) / skhealact (默认 0x9a 道具通道, ActEfe 0x71 加血发光)。
func petSkillStep(c server.Conn, s *session, k *petSkill, tgt uint32) (bool, bool, error) {
	bp := battlePet(s)
	if bp == nil {
		return false, false, nil
	}
	if bp.Hunger < k.fp {
		log.Printf("sa2: pet skill %q: FP %d < %d → normal attack", k.name, bp.Hunger, k.fp)
		_ = sysMsg(c, 0xffff4040, "气力不足, 改为普通攻击。")
		return false, false, nil
	}
	bp.Hunger -= k.fp
	switch k.fn {
	case "Heal":
		target, heal := uint32(petBID), 0
		if s.char != nil && tgt == s.char.ID && s.hp > 0 {
			target = bidPlayer
			heal = min(s.hp+k.heal, fieldHPMax) - s.hp
			s.hp += heal
		} else {
			heal = min(s.petHP+k.heal, bp.MaxHP) - s.petHP
			s.petHP += heal
			bp.HP = s.petHP
		}
		savePet(bp)
		_ = sendPetInfo(c, bp)
		log.Printf("sa2: pet skill %q heals %s +%d (pet hp %d, char hp %d, FP left %d)", k.name, map[bool]string{true: "Tester", false: "pet"}[target == bidPlayer], heal, s.petHP, s.hp, bp.Hunger)
		a := bAct{actor: unitKey(c, uint32(petBID)), target: unitKey(c, target), act: byte(knob("skhealact", actItem)),
			num: uint16(knob("itemefx", itemEfx)), efe: uint16(knob("skhealefe", 0x71)), pas: byte(knob("skhealpas", 0))}
		a.passHP = int32(heal)
		a.setCnd = connKind(c) == clientCN
		if err := send(c, 0x0b30, battleAction(a)); err != nil {
			return true, true, err
		}
		s.syncN++
		return true, true, sendSync(c, s)
	case "Mighty":
		f := foeByID(s, tgt)
		if f == nil || !f.alive() {
			f = firstAlive(s)
		}
		if f == nil {
			return false, true, nil
		}
		atk, _, agi := petBattleStats(s)
		_, def, fagi := f.stats()
		r := rollHit(atk, def, agi, fagi)
		if !r.miss && rand.Intn(100) < k.evade {
			r = hitRoll{dmg: 0, miss: true}
		}
		r.dmg *= k.mul
		f.hp -= r.dmg
		s.monHP = sumFoeHP(s)
		savePet(bp)
		_ = sendPetInfo(c, bp)
		log.Printf("sa2: pet skill %q on monster #%d for %d (x%d, miss=%v; its hp %d, FP left %d)", k.name, f.bid>>8, r.dmg, k.mul, r.miss, max(f.hp, 0), bp.Hunger)
		a := r.act(c, uint32(petBID), f.bid, 1, f.hp <= 0)
		a.act = byte(knob("skact", actAttack))
		if !r.miss && knob("skefe", 0) != 0 {
			a.efe = uint16(knob("skefe", 0))
		}
		if err := send(c, 0x0b30, battleAction(a)); err != nil {
			return true, true, err
		}
		if s.monHP <= 0 && endMode > 0 {
			s.syncN, s.syncSent, s.queue = 0, false, nil
			endBattle(c, s)
			return true, true, nil
		}
		s.syncN++
		return true, true, sendSync(c, s)
	}
	return false, false, nil
}
