package handler

// 背包 + 商店 (最小实现, 2026-10-17 国服逆向+实测), 详见 PACKETS.md "物品 / 商店":
//   S 0x04b0 物品列表 (0x411750 → 0x4a60d0 → 角色 vtbl+0x484 = 0x4d2c80): u16 n + n×0x12
//       +0 u32 物品对象 id (高 nibble 4) +4 u16 物品号 +6 u16 图标 CGNO (0x4adbc2 建格子) +8 u16 位置码 +0xa u16 选项 (bit4 已装备, bit6 锁定) +0x10 u16 数量
//       位置码 → 背包页 (表 0x6714a0): 1..0x12 装备页, 0x13 食物, 0x14 素材, 0x15 其他, 0 不显示。最多 40 条。
//   S 0x04b1 / 0x04b2 u32 id: 删除物品 (0x410b70)。
//   C 0x0510 u16 物品号 (或 u32 对象 id) → S 0x0510 u32 id + 0x14a 物品资料 (0x411650 → 0x4a7180, 客户端物品缓存只来自这里)
//       +0 u16 物品号 +4 名字 MBCS +0x44 u16 装备类 (1..0x12) +0xc6 说明 MBCS (0x78)。+0x13e u32 价格 +0x142 u32 卖价 (>0 可卖) +0x148 flags。图标不在这里, 在 S04b0 条目 +6。
//   石币 = S 0x0230 +0x32 u32 (CharParam 20, 背包窗 "拥有石币")。
//   S 0x04a0 商店菜单 (0x40fc30): u32 flags (bit0 买 bit1 卖, 3 = 买/卖/离开 三选) + u16 len + 文本; 同时建商店窗 0x5b6b。
//   S 0x04a4 商品列表 (0x477410): u16 n + n×0x4c: +0 u16 序号 +2 u16 物品号 +8 u32 价格。
//   S 0x04a5 可卖列表 (0x477470): u16 n + n×8: u32 物品对象 id, u32 卖价。
//   C 0x04a7 买: u16 商品序号 + u16 物品号 (+ u32 数量); C 0x04a6 卖: u32。 S 0x04a8 结果 u16 (非 0 = 成功)。

import (
	"log"

	"sa2srv/internal/cmd"
	"sa2srv/internal/saac"
	"sa2srv/internal/server"
)

const (
	SV_ItemList   uint16 = 0x04b0
	SV_ItemDel    uint16 = 0x04b1
	CV_ItemInfo   uint16 = 0x0510
	SV_ShopMenu   uint16 = 0x04a0
	SV_ShopGoods  uint16 = 0x04a4
	SV_ShopSale   uint16 = 0x04a5
	CV_ShopSell   uint16 = 0x04a6
	CV_ShopBuy    uint16 = 0x04a7
	SV_ShopResult uint16 = 0x04a8
	ParamGold            = 20 // S0230 +0x32
	goldInit             = 1050
	invMax               = 40
	itemRecSize          = 0x14a
	merchantID    uint32 = 0x10000002
)

// itemDef: 物品定义 (data/items.txt, 见 items.go LoadItems); 下面是没有数据文件时的内建表 (占位名字)。
type itemDef struct {
	no         int
	name, desc string
	kind       string // food / weapon / armor / misc
	val        int    // food: 回复 HP; weapon: 攻击+; armor: 防御+
	icon       int    // item.bin +0x11a → S04b0 +6
	price      int
	loc        int // S04b0 位置码
	stack      int
	drop       int // 战斗掉落千分比
}

var itemDefs = map[int]*itemDef{
	20625: {no: 20625, name: "Apple", desc: "A common apple. HP+10", kind: "food", val: 10, icon: 35722, price: 2, loc: 0x13, stack: 99, drop: 300},
	20626: {no: 20626, name: "Big Apple", desc: "A big apple. HP+20", kind: "food", val: 20, icon: 35724, price: 4, loc: 0x13, stack: 99},
	20315: {no: 20315, name: "Meat", desc: "Plain meat. HP+40", kind: "food", val: 40, icon: 35652, price: 60, loc: 0x13, stack: 99},
	1:     {no: 1, name: "Worn Stone Axe", desc: "Axe Lv1 ATK+6", kind: "weapon", val: 6, icon: 35000, price: 130, loc: 3, stack: 1},
	2:     {no: 2, name: "Small Stone Axe", desc: "Axe Lv1 ATK+10", kind: "weapon", val: 10, icon: 35001, price: 310, loc: 3, stack: 1},
	10082: {no: 10082, name: "Straw Hat", desc: "Hat Lv1 DEF+2", kind: "armor", val: 2, icon: 35404, price: 24, loc: 1, stack: 1},
}

// 商人卖的东西 (顺序 = S04a4 序号)。
var shopGoods = []int{20625, 20626, 20315, 1, 2, 10082}

func sellPrice(d *itemDef) int { return max(d.price/2, 1) }

// itemRecord: S 0x0510 的 0x14a 物品资料 (名字/说明/装备类/价格)。
func itemRecord(c server.Conn, d *itemDef) []byte {
	b := make([]byte, itemRecSize)
	put16(b, 0, uint16(d.no))
	// +2 u16 图标 CGNO: 国服新 UI 的图片控件用占位图号 0x1a 表示"物品图标", setImage 0x54ba30 → 0x4a6a20(物品号) 取本资料 +2。
	// 背包/商店格子靠它画图标, 没有就是空格子 (也拖不动)。
	put16(b, 2, uint16(d.icon))
	putText(c, b[4:0x42], 0x3e, d.name)
	putText(c, b[0xc6:0x13c], 0x76, d.desc)
	put32(b, 0x13e, uint32(d.price))      // 0x559b5b 提示框 %d (买价)
	put32(b, 0x142, uint32(sellPrice(d))) // 0x54abce: >0 才能卖
	if d.loc >= 1 && d.loc <= 0x12 {
		put16(b, 0x44, uint16(d.loc)) // 装备类
	}
	// +0x5e 起 8 条 12 字节属性: +0 u16 类型, +4 s16 数值。客户端 0x5575c0 按类型加总已装备物品:
	// 6 体质, 0xb 攻击力, 0xc 防御力, 0xd 敏捷, 0xe 灵力 (装备信息窗 / 右键卸下时本地重算状态窗)。
	switch d.kind {
	case "weapon":
		put16(b, 0x5e, 0xb)
		put16(b, 0x62, uint16(int16(d.val)))
	case "armor":
		put16(b, 0x5e, 0xc)
		put16(b, 0x62, uint16(int16(d.val)))
	}
	return b
}

func loadInventory(s *session) {
	s.items, s.gold = nil, goldInit
	if store == nil || s.char == nil {
		return
	}
	if its, err := store.ListItems(s.char.ID); err == nil {
		for i := range its {
			it := its[i]
			if d := itemDefs[it.ItemNo]; d != nil {
				it.Loc = d.loc // 位置码跟定义走 (旧库存可能是旧值)
			}
			s.items = append(s.items, &it)
		}
	} else {
		log.Printf("sa2: load items: %v", err)
	}
	if g, err := store.Gold(s.char.ID, goldInit); err == nil {
		s.gold = g
	}
}

func itemEntry(b []byte, it *saac.Item) {
	put32(b, 0, it.ItemID())
	put16(b, 4, uint16(it.ItemNo))
	if d := itemDefs[it.ItemNo]; d != nil {
		put16(b, 6, uint16(d.icon)) // 图标 (0x4adbc2 → 0x57c730 建格子)
	}
	put16(b, 8, uint16(it.Loc))
	opt := uint16(it.Opt)
	if d := itemDefs[it.ItemNo]; d != nil && d.kind == "food" {
		// 0x02 = 地图上可使用: 拖到人物身上时 0x51f390 要求 位置码 0x13 且 此位 才发 C04b2;
		// 0x04 = 战斗中可使用 (旧背包 0x4add3a 战斗模式没此位就灰掉)。
		opt |= 0x02 | 0x04
	}
	put16(b, 0xa, opt)
	put16(b, 0x10, uint16(it.Count))
}

func itemListPacket(its ...*saac.Item) []byte {
	b := make([]byte, 2+0x12*len(its))
	put16(b, 0, uint16(len(its)))
	for i, it := range its {
		itemEntry(b[2+0x12*i:], it)
	}
	return b
}

func sendInventory(c server.Conn, s *session) error {
	if len(s.items) == 0 {
		return nil
	}
	return send(c, SV_ItemList, itemListPacket(s.items...))
}

func saveItem(it *saac.Item) {
	if store != nil {
		if err := store.SaveItem(it); err != nil {
			log.Printf("sa2: save item: %v", err)
		}
	}
}

// addGold 改石币并发 CharParam 20 增量。
func addGold(c server.Conn, s *session, d int) error {
	s.gold += d
	if store != nil && s.char != nil {
		_ = store.SaveGold(s.char.ID, s.gold)
	}
	return send(c, cmd.SV_CharParam, charParamPacket(paramDelta{s.char.ID, ParamGold, int32(d)}))
}

// giveItem 放进背包 (能叠就叠), 返回变动的格子; 满了返回 nil。
func giveItem(s *session, d *itemDef, n int) *saac.Item {
	for _, it := range s.items {
		if it.ItemNo == d.no && it.Count+n <= d.stack {
			it.Count += n
			return it
		}
	}
	if len(s.items) >= invMax {
		return nil
	}
	used := map[int]bool{}
	for _, it := range s.items {
		used[it.Slot] = true
	}
	slot := 0
	for used[slot] {
		slot++
	}
	it := &saac.Item{CharID: s.char.ID, Slot: slot, ItemNo: d.no, Count: n, Loc: d.loc}
	s.items = append(s.items, it)
	return it
}

func findItem(s *session, id uint32) (int, *saac.Item) {
	for i, it := range s.items {
		if it.ItemID() == id {
			return i, it
		}
	}
	return -1, nil
}

func shopResult(c server.Conn, ok bool) error {
	r := []byte{0, 0}
	if ok {
		r[0] = 1
	}
	return send(c, SV_ShopResult, r)
}

func openShop(c server.Conn, s *session) error {
	msg := encText(c, "欢迎光临！要买点什么吗？")
	b := make([]byte, 6+len(msg)+1)
	put32(b, 0, 3) // 买 + 卖
	put16(b, 4, uint16(len(msg)+1))
	copy(b[6:], msg)
	if err := send(c, SV_ShopMenu, b); err != nil {
		return err
	}
	g := make([]byte, 2+0x4c*len(shopGoods))
	put16(g, 0, uint16(len(shopGoods)))
	for i, no := range shopGoods {
		e := g[2+0x4c*i:]
		put16(e, 0, uint16(i))
		put16(e, 2, uint16(no))
		put32(e, 8, uint32(itemDefs[no].price))
	}
	if err := send(c, SV_ShopGoods, g); err != nil {
		return err
	}
	s.shopOpen = true
	return sendSaleList(c, s)
}

func sendSaleList(c server.Conn, s *session) error {
	var es [][2]uint32
	for _, it := range s.items {
		if d := itemDefs[it.ItemNo]; d != nil && !it.Equipped() {
			es = append(es, [2]uint32{it.ItemID(), uint32(sellPrice(d))})
		}
	}
	b := make([]byte, 2+8*len(es))
	put16(b, 0, uint16(len(es)))
	for i, e := range es {
		put32(b, 2+8*i, e[0])
		put32(b, 6+8*i, e[1])
	}
	return send(c, SV_ShopSale, b)
}

func (h *Handler) onItemPacket(c server.Conn, cm uint16, p []byte) (bool, error) {
	s := getSess(c.RemoteAddr())
	switch cm {
	case CV_ItemInfo:
		var id uint32
		no := 0
		switch {
		case len(p) >= 4:
			id = le32(p)
			if _, it := findItem(s, id); it != nil {
				no = it.ItemNo
			} else if it := anyItem(id); it != nil {
				no = it.ItemNo // 交易窗口里对方的物品
			}
		case len(p) >= 2:
			no = int(p[0]) | int(p[1])<<8
		}
		d := itemDefs[no]
		log.Printf("sa2: C 0x0510 item info id=0x%x no=%d known=%v", id, no, d != nil)
		if d == nil {
			return true, nil
		}
		b := make([]byte, 4+itemRecSize)
		put32(b, 0, id)
		copy(b[4:], itemRecord(c, d))
		return true, send(c, CV_ItemInfo, b)
	case CV_ShopBuy:
		if s.char == nil || len(p) < 4 {
			return true, nil
		}
		// C04a7 (国服实测): u16 商品序号 + u16 物品号 [+ u32 数量, 8 字节版]
		idx, no, n := int(p[0])|int(p[1])<<8, int(p[2])|int(p[3])<<8, 1
		if len(p) >= 8 {
			n = int(le32(p[4:]))
		}
		log.Printf("sa2: shop buy % x (idx %d no %d n %d) gold %d", p, idx, no, n, s.gold)
		if n <= 0 || n > 99 {
			n = 1
		}
		if idx < 0 || idx >= len(shopGoods) || shopGoods[idx] != no || !s.shopOpen {
			return true, shopResult(c, false)
		}
		d := itemDefs[shopGoods[idx]]
		cost := d.price * n
		if cost > s.gold {
			_ = sysMsg(c, 0xffff4040, "石币不够。")
			return true, shopResult(c, false)
		}
		it := giveItem(s, d, n)
		if it == nil {
			_ = sysMsg(c, 0xffff4040, "背包满了。")
			return true, shopResult(c, false)
		}
		saveItem(it)
		if err := addGold(c, s, -cost); err != nil {
			return true, err
		}
		if err := send(c, SV_ItemList, itemListPacket(it)); err != nil {
			return true, err
		}
		log.Printf("sa2: bought %d x %q for %d, gold %d", n, d.name, cost, s.gold)
		if err := shopResult(c, true); err != nil {
			return true, err
		}
		return true, sendSaleList(c, s)
	case CV_ShopSell:
		if s.char == nil || len(p) < 4 {
			return true, nil
		}
		id := le32(p)
		i, it := findItem(s, id)
		log.Printf("sa2: shop sell 0x%x found=%v", id, it != nil)
		if it == nil || !s.shopOpen || itemDefs[it.ItemNo] == nil || it.Equipped() {
			return true, shopResult(c, false)
		}
		d := itemDefs[it.ItemNo]
		gain := sellPrice(d) * it.Count
		it.Count = 0
		saveItem(it)
		s.items = append(s.items[:i], s.items[i+1:]...)
		if err := send(c, SV_ItemDel, p[:4]); err != nil {
			return true, err
		}
		if err := addGold(c, s, gain); err != nil {
			return true, err
		}
		log.Printf("sa2: sold %q for %d, gold %d", d.name, gain, s.gold)
		return true, shopResult(c, true)
	}
	return false, nil
}
