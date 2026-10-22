package handler

// 组队战斗 (最小版): 队长遇敌/开打时, 同图队员的角色 (和出战宠) 作为我方单位一起上场。
// 只有队长的客户端进战斗画面; 队员单位由服务器 AI 控制 (每回合打一只活着的怪), 怪也会打队员。
// 阵型 (两排): 角色在后排一字排开, 各自的宠物站在主人正前方 (前排同一列); 怪在对面前排。
// 队员单位 BID: 角色 0x0400 + i*0x200, 宠物 +0x100 (低字节 0 = 我方); 条目 +7 = 0 (不可下指令) → 动作键用裸 BID。
import (
	"fmt"
	"log"
	"math/rand"

	"sa2srv/internal/saac"
	"sa2srv/internal/server"
)

type ally struct {
	owner     *session
	id, bid   uint32
	pet       *saac.Pet // nil = 角色
	hp, maxHP int
	name      string
	guard     bool // 本回合防御
	gone      bool // 逃跑离开
}

func (a *ally) alive() bool { return a.hp > 0 && !a.gone }

func (a *ally) stats() (atk, def, agi int) {
	if a.pet != nil {
		at, d, g, _ := petStats(a.pet)
		return at, d, g
	}
	st := playerStats(a.owner)
	return st.Atk, st.Def, st.Agi
}

// partyAllies: 队长 s 的同图队员 → 我方单位 (不含队长自己)。
func partyAllies(s *session) []*ally {
	var r []*ally
	for i, id := range partyFollowers(s.char.ID) {
		e := entryByID(id)
		// 上一场已结算 (ended) 但客户端还没报回场景位置的队员也算空闲 (真机: 队员窗口不在前台时战后不一定马上发 0x0410)
		if e == nil || e.sess.char.Map != s.char.Map || (e.sess.inBattle && !e.sess.ended) {
			continue
		}
		ms := e.sess
		hp := curFieldHP(ms)
		bid := uint32(0x400 + i*0x200)
		r = append(r, &ally{owner: ms, id: id, bid: bid, hp: hp, maxHP: fieldHPMaxOr(), name: ms.char.Name})
		if knob("partypets", 1) != 0 {
			for j, bp := range battlePets(ms) {
				if bp.HP > 0 {
					r = append(r, &ally{owner: ms, id: bp.PetID(), bid: bid + 0x100 + uint32(j)*0x20, pet: bp, hp: bp.HP, maxHP: bp.MaxHP, name: bp.Name})
				}
			}
		}
	}
	return r
}

// allyUnits: 阵型 + S0B10 条目。chars = 我方角色数 (含队长), 队长 slot 0。
func allyUnits(c server.Conn, s *session, cx, cy int16, h, chars int) []bUnit {
	var us []bUnit
	slot := 0
	petSlot := map[*session]int{}
	for _, a := range s.allies {
		gfx := uint16(31002)
		if e := entryByID(a.owner.char.ID); e != nil {
			if g := uint16(e.pos[0xc]) | uint16(e.pos[0xd])<<8; g >= 30000 && g < 31000 {
				gfx = g + 1000
			}
		}
		u := bUnit{a.id, a.bid, 0, uint32(a.hp), uint32(a.maxHP), 0, 0, 7, gfx, false, a.name, a.owner.level}
		row, col := rowBack, slot
		if a.pet != nil {
			u.gfx, u.lv, row = uint16(a.pet.Gfx), a.pet.Level, rowFront
			col = petSlot[a.owner]
			petSlot[a.owner] = col + 1
		} else {
			slot++
		}
		u.x, u.y = formPos(cx, cy, h, 0, row, col, chars)
		us = append(us, u)
	}
	return us
}

func allyByBID(s *session, bid uint32) *ally {
	for _, a := range s.allies {
		if a.bid == bid {
			return a
		}
	}
	return nil
}

func alliesAlive(s *session) int {
	n := 0
	for _, a := range s.allies {
		if a.alive() {
			n++
		}
	}
	return n
}

// allyStep: 队员单位出手 (AI: 打一只活着的怪)。
func allyStep(c server.Conn, s *session, a *ally) (bool, error) {
	if !a.alive() {
		return false, nil
	}
	if handled, sent, err := allyOrderStep(c, s, a); handled {
		return sent, err
	}
	f := firstAlive(s)
	var alive []*foe
	for _, x := range s.foes {
		if x.alive() {
			alive = append(alive, x)
		}
	}
	if t := foeByID(s, s.orderTgt[a.id]); t != nil && t.alive() {
		f = t // 队员自己点的怪
	} else if len(alive) > 0 {
		f = alive[rand.Intn(len(alive))]
	}
	if f == nil {
		return false, nil
	}
	atk, _, agi := a.stats()
	_, def, fagi := f.stats()
	r := rollHit(atk, def, agi, fagi)
	f.hp -= r.dmg
	s.monHP = sumFoeHP(s)
	log.Printf("sa2: ally %q (bid 0x%x) hits monster #%d for %d (its hp %d, all %d)", a.name, a.bid, f.bid>>8, r.dmg, max(f.hp, 0), s.monHP)
	if err := send(c, 0x0b30, battleAction(r.act(c, a.bid, f.bid, 1, f.hp <= 0))); err != nil {
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

// foeHitsAlly: 怪选中队员时 (概率 = 活着的队员 / 我方活着的单位数)。返回 true = 已处理。
func foeHitsAlly(c server.Conn, s *session, f *foe) (bool, error) {
	n := alliesAlive(s)
	if n == 0 {
		return false, nil
	}
	own := 0
	if s.hp > 0 {
		own++
	}
	if s.hasPet && s.petHP > 0 {
		own++
	}
	if own > 0 && rand.Intn(n+own) >= n {
		return false, nil
	}
	var alive []*ally
	for _, a := range s.allies {
		if a.alive() {
			alive = append(alive, a)
		}
	}
	a := alive[rand.Intn(len(alive))]
	ma, _, mg := f.stats()
	_, d, g := a.stats()
	r := rollHit(ma, d, mg, g)
	if a.guard && r.dmg > 0 {
		r.dmg = max(r.dmg/2, 1)
		r.guard = true
	}
	r.dmg = min(r.dmg, a.hp)
	a.hp -= r.dmg
	log.Printf("sa2: monster #%d hits ally %q for %d (hp %d)", f.bid>>8, a.name, r.dmg, a.hp)
	if err := send(c, 0x0b30, battleAction(r.act(c, f.bid, a.bid, byte(monPas), a.hp <= 0))); err != nil {
		return true, err
	}
	s.syncN++
	return true, sendSync(c, s)
}

func allyName(st turnStep) string { return fmt.Sprintf("ally(0x%x)", st.ally.bid) }
