package saac

// 背包物品 (最小实现, 2026-10-17): 每角色最多 40 格 (国服背包 "n/40")。
// 字段对应 S 0x04b0 条目 (见 FIELDS.md "物品"): 物品号 / 数量 / 位置码 (0x13 食物 0x14 素材 0x15 其他, 1..0x12 装备类)。
type Item struct {
	CharID uint32
	Slot   int // 0..39
	ItemNo int
	Count  int
	Loc    int
	Opt    int // S04b0 +0xa 选项: bit4 0x10 已装备
}

const OptEquipped = 0x10

func (it *Item) Equipped() bool { return it.Opt&OptEquipped != 0 }

// ItemID: 类型 4 对象 ID (高 nibble 4), 由角色 ID 和格子派生。
func (it *Item) ItemID() uint32 { return 0x40000000 | (it.CharID&0xfffff)<<8 | uint32(it.Slot) }

func (s *Store) ensureItems() error {
	if s.itemsReady {
		return nil
	}
	if _, err := s.db.Exec(`CREATE TABLE IF NOT EXISTS items(
		char_id INTEGER NOT NULL, slot INTEGER NOT NULL,
		item_no INTEGER NOT NULL, count INTEGER NOT NULL, loc INTEGER NOT NULL,
		PRIMARY KEY(char_id, slot))`); err != nil {
		return err
	}
	if err := s.ensureChars(); err != nil {
		return err
	}
	_, _ = s.db.Exec(`ALTER TABLE characters ADD COLUMN gold INTEGER NOT NULL DEFAULT -1`)
	_, _ = s.db.Exec(`ALTER TABLE items ADD COLUMN opt INTEGER NOT NULL DEFAULT 0`)
	s.itemsReady = true
	return nil
}

func (s *Store) ListItems(charID uint32) ([]Item, error) {
	if err := s.ensureItems(); err != nil {
		return nil, err
	}
	rows, err := s.db.Query(`SELECT slot,item_no,count,loc,opt FROM items WHERE char_id=? ORDER BY slot`, charID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Item
	for rows.Next() {
		it := Item{CharID: charID}
		if err := rows.Scan(&it.Slot, &it.ItemNo, &it.Count, &it.Loc, &it.Opt); err != nil {
			return nil, err
		}
		out = append(out, it)
	}
	return out, rows.Err()
}

// MoveItem 改格子 (先删旧格, 再存新格)。
func (s *Store) MoveItem(it *Item, newSlot int) error {
	if err := s.ensureItems(); err != nil {
		return err
	}
	if _, err := s.db.Exec(`DELETE FROM items WHERE char_id=? AND slot=?`, it.CharID, it.Slot); err != nil {
		return err
	}
	it.Slot = newSlot
	return s.SaveItem(it)
}

// SaveItem 插入或覆盖一格; Count<=0 删除该格。
func (s *Store) SaveItem(it *Item) error {
	if err := s.ensureItems(); err != nil {
		return err
	}
	if it.Count <= 0 {
		_, err := s.db.Exec(`DELETE FROM items WHERE char_id=? AND slot=?`, it.CharID, it.Slot)
		return err
	}
	_, err := s.db.Exec(`INSERT OR REPLACE INTO items(char_id,slot,item_no,count,loc,opt) VALUES(?,?,?,?,?,?)`,
		it.CharID, it.Slot, it.ItemNo, it.Count, it.Loc, it.Opt)
	return err
}

// Gold 石币; 未设置 (-1) 时返回 def。
func (s *Store) Gold(charID uint32, def int) (int, error) {
	if err := s.ensureItems(); err != nil {
		return def, err
	}
	g := -1
	if err := s.db.QueryRow(`SELECT gold FROM characters WHERE id=?`, charID).Scan(&g); err != nil {
		return def, err
	}
	if g < 0 {
		return def, nil
	}
	return g, nil
}

func (s *Store) SaveGold(charID uint32, g int) error {
	if err := s.ensureItems(); err != nil {
		return err
	}
	_, err := s.db.Exec(`UPDATE characters SET gold=? WHERE id=?`, g, charID)
	return err
}
