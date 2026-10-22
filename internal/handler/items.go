package handler

// 物品系统 (功能骨架, 2026-10-17 第 4 轮)。物品定义 = data/items.txt (占位小表, 号/名/图标/价取自国服 item.bin)。
//
// 客户端发送 (国服 exe 逆向):
//   C 0x04b2 SendItemUse (0x410b20): u32 物品 id + u32 目标 id (角色 / 宠物)。拖物品到人物/宠物上时发
//            (0x51f390 / 0x5a4040 / 0x5b0b90 → 网络对象 vtbl+0x15c)。
//   C 0x04b1 SendItemDrop (0x410bb0): u32 物品 id —— 丢弃。
//   C 0x14b0 SendEquipOff (0x410c00): u32 物品 id —— 卸下装备。
// 服务器回:
//   S 0x04b0 改过的条目 (数量减少 / 选项 bit4 装备) ; S 0x04b1 u32 id 删除 (客户端 0x410b70 "OnItemUse")。
//   HP: 角色 0x0420 param 4 增量; 宠物重发 0x0462。攻/防总数: 0x0420 param 11 / 12 增量 (状态窗 "物品" 列 = 总数 - 基础)。
// 装备窗 (0x4af6e4): 遍历角色物品, 选项 bit4 + 位置码 1..0x12 → 表 0x6716c8 定装备位 (1→4, 2→3, 3..10→2, 其他→0/1), 图标 = 条目 +6。
// 背包窗 (0x4adba0): 选项 bit4 的条目不显示 (已装备)。

import (
	"bufio"
	"fmt"
	"log"
	"math/rand"
	"os"
	"strconv"
	"strings"

	"sa2srv/internal/cmd"
	"sa2srv/internal/saac"
	"sa2srv/internal/server"
)

const (
	CV_ItemUse  uint16 = 0x04b2
	CV_ItemDrop uint16 = 0x04b1
	CV_EquipOff uint16 = 0x14b0
	// C 0x0463 宠物指令: u32 宠物id + u32 子命令 + u32 参数 + u32 角色id。
	// 子命令 8 = 把物品喂给宠物 (背包拖到上方宠物头像, 0x51efd0 → 网络 vtbl+0x19c), 参数 = 物品 id。
	CV_PetCmd     uint16 = 0x0463
	petCmdUseItem        = 8
	ParamAtkTot          = 11 // S0230 +0x1f
	ParamDefTot          = 12 // S0230 +0x21
)

// itemBase: 国服全表 (data/items_cn.txt, LoadItemsCN); 有它时 LoadItems 在其上覆盖, 否则替换内建表。
var itemBase map[int]*itemDef

// LoadItemsCN 读 data/items_cn.txt (国服 item.bin 生成的全表, Tab 分隔), 作为底表并立即生效; 返回条数。
func LoadItemsCN(path string) (int, error) {
	defs, err := readItems(path)
	if err != nil {
		return 0, err
	}
	itemBase = defs
	itemDefs = make(map[int]*itemDef, len(defs))
	for k, v := range defs {
		itemDefs[k] = v
	}
	return len(defs), nil
}

// LoadItems 读 data/items.txt (手调表); 有国服底表时同号覆盖, 否则替换内建表; 返回本文件条数。
func LoadItems(path string) (int, error) {
	defs, err := readItems(path)
	if err != nil {
		return 0, err
	}
	all := map[int]*itemDef{}
	for k, v := range itemBase {
		all[k] = v
	}
	for k, v := range defs {
		all[k] = v
	}
	itemDefs = all
	return len(defs), nil
}

// readItems: 列 no name kind val loc stack price icon drop desc…; 行里有 Tab 时按 Tab 分 (名字/说明可含空格), 否则按空白分。
func readItems(path string) (map[int]*itemDef, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	defs := map[int]*itemDef{}
	sc := bufio.NewScanner(f)
	ln := 0
	for sc.Scan() {
		ln++
		t := strings.TrimSpace(strings.TrimPrefix(sc.Text(), "\ufeff"))
		if t == "" || t[0] == '#' {
			continue
		}
		var fs []string
		if strings.Contains(t, "\t") {
			fs = strings.Split(t, "\t")
		} else {
			fs = strings.Fields(t)
		}
		if len(fs) < 9 {
			return nil, fmt.Errorf("%s:%d: 列不够 (%d)", path, ln, len(fs))
		}
		n := make([]int, 9)
		for _, i := range []int{0, 3, 4, 5, 6, 7, 8} {
			v, err := strconv.ParseInt(strings.TrimSpace(fs[i]), 0, 32)
			if err != nil {
				return nil, fmt.Errorf("%s:%d: 第 %d 列: %v", path, ln, i+1, err)
			}
			n[i] = int(v)
		}
		d := &itemDef{no: n[0], name: fs[1], kind: fs[2], val: n[3], loc: n[4], stack: max(n[5], 1),
			price: n[6], icon: n[7], drop: n[8]}
		if len(fs) > 9 {
			d.desc = strings.TrimSpace(fs[9])
			if !strings.Contains(t, "\t") {
				d.desc = strings.Join(fs[9:], " ")
			}
		}
		defs[d.no] = d
	}
	return defs, sc.Err()
}

func isEquip(d *itemDef) bool {
	return d != nil && d.loc >= 1 && d.loc <= 0x12 && (d.kind == "weapon" || d.kind == "armor")
}

// equipSlot: 装备窗位置 (同 0x6716c8 表), 饰品返回 0。
func equipSlot(loc int) int {
	switch {
	case loc == 1:
		return 4
	case loc == 2:
		return 3
	case loc >= 3 && loc <= 10:
		return 2
	}
	return 0
}

// equipBonus: 已装备物品的攻/防加成。
func equipBonus(s *session) charStats {
	var b charStats
	for _, it := range s.items {
		d := itemDefs[it.ItemNo]
		if d == nil || !it.Equipped() {
			continue
		}
		switch d.kind {
		case "weapon":
			b.Atk += d.val
		case "armor":
			b.Def += d.val
		}
	}
	return b
}

// playerStats: 基础 (等级) + 装备。
func playerStats(s *session) charStats {
	st, b := statsAt(s.level), equipBonus(s)
	st.Atk += b.Atk
	st.Def += b.Def
	return st
}

// sendItem 发一条改过的 S04b0 条目。
func sendItem(c server.Conn, it *saac.Item) error { return send(c, SV_ItemList, itemListPacket(it)) }

// removeItem: 从背包拿掉 n 个; 用完发 S04b1, 否则发新数量。
func removeItem(c server.Conn, s *session, it *saac.Item, n int) error {
	it.Count -= n
	saveItem(it)
	if it.Count > 0 {
		return sendItem(c, it)
	}
	for i, x := range s.items {
		if x == it {
			s.items = append(s.items[:i], s.items[i+1:]...)
			break
		}
	}
	b := make([]byte, 4)
	put32(b, 0, it.ItemID())
	return send(c, SV_ItemDel, b)
}

func findPetByID(s *session, id uint32) *saac.Pet {
	for i := range s.pets {
		if s.pets[i].PetID() == id {
			return &s.pets[i]
		}
	}
	return nil
}

// useItem: 吃 (回 HP) 或 装备。target = 角色或宠物 id (0 = 自己)。
func useItem(c server.Conn, s *session, it *saac.Item, target uint32) error {
	d := itemDefs[it.ItemNo]
	if d == nil {
		return sysMsg(c, 0xffff4040, "未知的物品。")
	}
	if isEquip(d) {
		if it.Equipped() {
			return setEquip(c, s, it, false)
		}
		return setEquip(c, s, it, true)
	}
	if d.kind != "food" {
		return sysMsg(c, 0xffff4040, "这个不能使用。")
	}
	if p := findPetByID(s, target); p != nil {
		if p.HP >= p.MaxHP {
			return sysMsg(c, 0xffffff80, p.Name+" 的体力已经满了。")
		}
		old := p.HP
		p.HP = min(p.HP+d.val, p.MaxHP)
		if p.Battle && s.hasPet {
			s.petHP = p.HP
		}
		savePet(p)
		log.Printf("sa2: use %q on pet %q HP %d -> %d", d.name, p.Name, old, p.HP)
		if err := sendPetInfo(c, p); err != nil {
			return err
		}
		_ = sysMsg(c, 0xffffff80, fmt.Sprintf("%s 吃了%s, 体力 +%d。", p.Name, d.name, p.HP-old))
		return removeItem(c, s, it, 1)
	}
	old := curFieldHP(s)
	if old >= fieldHPMax {
		return sysMsg(c, 0xffffff80, "体力已经满了。")
	}
	nhp := min(old+d.val, fieldHPMax)
	s.fieldHP = nhp
	saveCharStats(s)
	log.Printf("sa2: use %q on self HP %d -> %d", d.name, old, nhp)
	if err := send(c, cmd.SV_CharParam, charParamPacket(paramDelta{s.char.ID, ParamHP, int32(nhp - old)})); err != nil {
		return err
	}
	_ = sysMsg(c, 0xffffff80, fmt.Sprintf("吃了%s, 体力 +%d。", d.name, nhp-old))
	return removeItem(c, s, it, 1)
}

// setEquip 装上/卸下; 同装备位已有的先卸下 (饰品位最多 2 个)。
func setEquip(c server.Conn, s *session, it *saac.Item, on bool) error {
	return setEquipX(c, s, it, on, true)
}

// setEquipX: sendDelta=false 用于客户端右键卸下 (C14b0): 客户端已按物品资料 +0x5e 属性自己重算了状态窗, 再发增量会扣两次。
func setEquipX(c server.Conn, s *session, it *saac.Item, on, sendDelta bool) error {
	d := itemDefs[it.ItemNo]
	if !isEquip(d) || it.Equipped() == on {
		return nil
	}
	before := equipBonus(s)
	var changed []*saac.Item
	if on {
		slot, acc := equipSlot(d.loc), 0
		for _, x := range s.items {
			xd := itemDefs[x.ItemNo]
			if x == it || !x.Equipped() || !isEquip(xd) || equipSlot(xd.loc) != slot {
				continue
			}
			if slot == 0 && acc == 0 {
				acc++
				continue
			}
			x.Opt &^= saac.OptEquipped
			changed = append(changed, x)
		}
		it.Opt |= saac.OptEquipped
	} else {
		it.Opt &^= saac.OptEquipped
	}
	changed = append(changed, it)
	for _, x := range changed {
		saveItem(x)
	}
	if err := send(c, SV_ItemList, itemListPacket(changed...)); err != nil {
		return err
	}
	after := equipBonus(s)
	id := s.char.ID
	if sendDelta {
		if err := send(c, cmd.SV_CharParam, charParamPacket(
			paramDelta{id, ParamAtkTot, int32(after.Atk - before.Atk)},
			paramDelta{id, ParamDefTot, int32(after.Def - before.Def)})); err != nil {
			return err
		}
	}
	verb := "卸下"
	if on {
		verb = "装备"
	}
	st := playerStats(s)
	log.Printf("sa2: %s %q -> atk %d def %d", verb, d.name, st.Atk, st.Def)
	return sysMsg(c, 0xffffff80, fmt.Sprintf("%s了%s (攻击 %d 防御 %d)。", verb, d.name, st.Atk, st.Def))
}

// dropItem: 丢弃整格 (装备中的先卸下)。
func dropItem(c server.Conn, s *session, it *saac.Item) error {
	if it.Equipped() {
		if err := setEquip(c, s, it, false); err != nil {
			return err
		}
	}
	log.Printf("sa2: drop item %d x%d slot %d", it.ItemNo, it.Count, it.Slot)
	return removeItem(c, s, it, it.Count)
}

func itemAtSlot(s *session, slot int) *saac.Item {
	for _, it := range s.items {
		if it.Slot == slot {
			return it
		}
	}
	return nil
}

// moveItem: 背包内移动 / 交换 / 叠加 (服务器格子; 客户端条目 id 随格子变, 所以先删旧 id 再发新条目)。
func moveItem(c server.Conn, s *session, from, to int) error {
	a := itemAtSlot(s, from)
	if a == nil || to < 0 || to >= invMax || from == to {
		return sysMsg(c, 0xffff4040, "移动失败。")
	}
	b := itemAtSlot(s, to)
	del := []uint32{a.ItemID()}
	var upd []*saac.Item
	switch {
	case b == nil:
		_ = store.MoveItem(a, to)
		upd = append(upd, a)
	case b.ItemNo == a.ItemNo && itemDefs[a.ItemNo] != nil && itemDefs[a.ItemNo].stack > 1: // 叠加
		d := itemDefs[a.ItemNo]
		n := min(a.Count, d.stack-b.Count)
		b.Count += n
		saveItem(b)
		upd = append(upd, b)
		if n < a.Count {
			a.Count -= n
			saveItem(a)
			upd = append(upd, a)
			del = nil
		} else {
			a.Count = 0
			saveItem(a)
			for i, x := range s.items {
				if x == a {
					s.items = append(s.items[:i], s.items[i+1:]...)
					break
				}
			}
		}
	default: // 交换
		del = append(del, b.ItemID())
		a.Count, b.Count = -a.Count, -b.Count // 先删两格
		saveItem(a)
		saveItem(b)
		a.Count, b.Count = -a.Count, -b.Count
		a.Slot, b.Slot = to, from
		saveItem(a)
		saveItem(b)
		upd = append(upd, a, b)
	}
	for _, id := range del {
		x := make([]byte, 4)
		put32(x, 0, id)
		if err := send(c, SV_ItemDel, x); err != nil {
			return err
		}
	}
	if len(upd) > 0 {
		return send(c, SV_ItemList, itemListPacket(upd...))
	}
	return nil
}

// rollDrops: 战斗胜利后每只怪按 drop 千分比掉物品 (占位)。
func rollDrops(c server.Conn, s *session, nFoes int) {
	if s.char == nil {
		return
	}
	var got []*saac.Item
	var names []string
	for i := 0; i < max(nFoes, 1); i++ {
		for _, d := range itemDefs {
			if d.drop > 0 && rand.Intn(1000) < d.drop*knob("dropmul", 1) {
				if it := giveItem(s, d, 1); it != nil {
					saveItem(it)
					got = append(got, it)
					names = append(names, d.name)
				}
				break // 每只怪最多一件
			}
		}
	}
	if len(got) == 0 {
		return
	}
	log.Printf("sa2: battle drops %v", names)
	_ = send(c, SV_ItemList, itemListPacket(got...))
	_ = sysMsg(c, 0xffffff80, "得到了 "+strings.Join(names, "、")+"。")
}

func (h *Handler) onItemUse(c server.Conn, cm uint16, p []byte) (bool, error) {
	s := getSess(c.RemoteAddr())
	if s == nil || s.char == nil {
		return false, nil
	}
	switch cm {
	case CV_ItemUse:
		if len(p) < 4 {
			return true, nil
		}
		id, tgt := le32(p), uint32(0)
		if len(p) >= 8 {
			tgt = le32(p[4:])
		}
		_, it := findItem(s, id)
		log.Printf("sa2: C 0x04b2 item use id=0x%x target=0x%x found=%v", id, tgt, it != nil)
		if it == nil {
			return true, nil
		}
		return true, useItem(c, s, it, tgt)
	case CV_ItemDrop:
		if len(p) < 4 {
			return true, nil
		}
		_, it := findItem(s, le32(p))
		log.Printf("sa2: C 0x04b1 item drop id=0x%x found=%v", le32(p), it != nil)
		if it == nil {
			return true, nil
		}
		return true, dropItem(c, s, it)
	case CV_PetCmd:
		if len(p) < 12 || le32(p[4:]) != petCmdUseItem {
			if len(p) >= 8 {
				log.Printf("sa2: C 0x0463 pet cmd pet=0x%x sub=%d (未实现)", le32(p), le32(p[4:]))
			}
			return true, nil
		}
		pet, id := le32(p), le32(p[8:])
		_, it := findItem(s, id)
		log.Printf("sa2: C 0x0463 pet use item pet=0x%x id=0x%x found=%v", pet, id, it != nil)
		if it == nil {
			return true, nil
		}
		return true, useItem(c, s, it, pet)
	case CV_EquipOff:
		if len(p) < 4 {
			return true, nil
		}
		_, it := findItem(s, le32(p))
		log.Printf("sa2: C 0x14b0 equip off id=0x%x found=%v", le32(p), it != nil)
		if it == nil {
			return true, nil
		}
		return true, setEquipX(c, s, it, false, knob("offdelta", 0) == 1)
	}
	return false, nil
}

// itemGM: /use 格 [pet] | /drop 格 | /equip 格 | /unequip 格 | /mv 格 格 | /items
func itemGM(c server.Conn, s *session, msg string) (bool, error) {
	f := strings.Fields(msg)
	if len(f) == 0 || s.char == nil {
		return false, nil
	}
	arg := func(i int) int {
		if i < len(f) {
			v, _ := strconv.Atoi(f[i])
			return v
		}
		return -1
	}
	get := func() *saac.Item { return itemAtSlot(s, arg(1)) }
	switch f[0] {
	case "/items":
		var b strings.Builder
		for _, it := range s.items {
			name := "?"
			if d := itemDefs[it.ItemNo]; d != nil {
				name = d.name
			}
			e := ""
			if it.Equipped() {
				e = "(装)"
			}
			fmt.Fprintf(&b, "%d:%s%sx%d ", it.Slot, name, e, it.Count)
		}
		st := playerStats(s)
		fmt.Fprintf(&b, "| 石币 %d 攻 %d 防 %d HP %d", s.gold, st.Atk, st.Def, curFieldHP(s))
		return true, sysMsg(c, 0xffffff80, b.String())
	case "/use":
		it := get()
		if it == nil {
			return true, sysMsg(c, 0xffff4040, "那格没有东西。")
		}
		tgt := s.char.ID
		if len(f) > 2 && f[2] == "pet" {
			if bp := battlePet(s); bp != nil {
				tgt = bp.PetID()
			}
		}
		return true, useItem(c, s, it, tgt)
	case "/drop":
		if it := get(); it != nil {
			return true, dropItem(c, s, it)
		}
		return true, sysMsg(c, 0xffff4040, "那格没有东西。")
	case "/equip", "/unequip":
		if it := get(); it != nil {
			return true, setEquip(c, s, it, f[0] == "/equip")
		}
		return true, sysMsg(c, 0xffff4040, "那格没有东西。")
	case "/mv":
		return true, moveItem(c, s, arg(1), arg(2))
	}
	return false, nil
}
