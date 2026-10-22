package handler

// 角色基础属性 + 伤害公式 (2026-10-17 第 3 轮, 服务器自定, 简单可调)。
//
// S0230 / 状态窗 (国服真机对照): +0x15 体质, +0x17 攻击力(基础), +0x19 敏捷(基础), +0x1b 灵力(基础),
// +0x1f 攻击力(总数), +0x21 防御力(总数), +0x23 敏捷(总数), +0x25 灵力(总数)。"物品" 列 = 总数 - 基础。
// 0x0420 param 6..14 = +0x15..+0x25 (每个 u16), 升级时发增量。
//
// 伤害 (三方通用): dmg = atk² / (atk + def) × (0.90..1.10), 至少 1。
// 闪避: 几率 = clamp(5 + (目标敏捷 - 攻方敏捷), 0, 25) %, 闪避时伤害 0。

import "math/rand"

type charStats struct{ Vit, Atk, Def, Agi, Spi int }

// statsAt: 角色每级成长 (无装备): 攻 10+3Lv, 防 Lv+5, 敏 8+2Lv, 体 10+2Lv, 灵 10+2Lv。
// 防御基础 = 等级+5 是客户端自己的算法 (0x5575c0 类型 0xc: player+0x982 等级 + 5), 跟着它走, 状态窗才对得上。
func statsAt(lv int) charStats {
	if lv < 1 {
		lv = 1
	}
	return charStats{Vit: 10 + 2*lv, Atk: 10 + 3*lv, Def: lv + 5, Agi: 8 + 2*lv, Spi: 10 + 2*lv}
}

// putCharStats 写进 S0230 负载。
func putCharStats(b []byte, st charStats) {
	put16(b, 0x15, uint16(st.Vit))
	put16(b, 0x17, uint16(st.Atk))
	put16(b, 0x19, uint16(st.Agi))
	put16(b, 0x1b, uint16(st.Spi))
	put16(b, 0x1f, uint16(st.Atk))
	put16(b, 0x21, uint16(st.Def))
	put16(b, 0x23, uint16(st.Agi))
	put16(b, 0x25, uint16(st.Spi))
}

// putCharStatsTot 只改 "总数" 列 (+0x1f..+0x25), 基础列留给 putCharStats。
func putCharStatsTot(b []byte, st charStats) {
	put16(b, 0x1f, uint16(st.Atk))
	put16(b, 0x21, uint16(st.Def))
	put16(b, 0x23, uint16(st.Agi))
	put16(b, 0x25, uint16(st.Spi))
}

// statDeltas: 等级 from→to 时状态窗要加的 0x0420 增量。
func statDeltas(id uint32, from, to int) []paramDelta {
	a, b := statsAt(from), statsAt(to)
	d := func(p uint16, x, y int) paramDelta { return paramDelta{id, p, int32(y - x)} }
	return []paramDelta{
		d(6, a.Vit, b.Vit), d(7, a.Atk, b.Atk), d(8, a.Agi, b.Agi), d(9, a.Spi, b.Spi),
		d(11, a.Atk, b.Atk), d(12, a.Def, b.Def), d(13, a.Agi, b.Agi), d(14, a.Spi, b.Spi),
	}
}

// calcDamage: 返回伤害 (0 = 闪避) 和是否闪避。
func calcDamage(atk, def, atkAgi, defAgi int) (int, bool) {
	dodge := min(max(5+defAgi-atkAgi, 0), 25)
	if rand.Intn(100) < dodge {
		return 0, true
	}
	if atk < 1 {
		atk = 1
	}
	if def < 0 {
		def = 0
	}
	d := atk * atk * (90 + rand.Intn(21)) / (100 * (atk + def))
	return max(d, 1), false
}
