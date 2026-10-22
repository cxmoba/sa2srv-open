package saac

// Pet 宠物 (每角色最多 5 只, slot 0..4)。字段对应 S 0x0462 (见 FIELDS.md §2.1)。
type Pet struct {
	CharID                   uint32
	Slot                     int
	Name, Species            string
	Gfx                      int  // +0x2e CGNO
	Level                    int  // +0x30
	Exp, NextExp             int  // +0x34 / +0x38
	Age, DP                  int  // +0x3c / +0x3e
	HP, MaxHP                int  // +0x40 / +0x42
	Hunger, Loyalty          int  // +0x44 / +0x46
	Earth, Water, Fire, Wind int  // +0x83..+0x86
	Battle                   bool // 出战宠 (C 0x0465 选中)
	// 数据表 (gamedata): 来源敌怪 id / 种族 TEMPNO / 个体成长系数 (SA1 BASEVITAL..BASEDEX 随机后)。Temp 0 = 旧数据未迁移
	EnemyID, Temp          int
	GVit, GStr, GTgh, GDex int
}

// PetID: 类型 2 对象 ID (高 nibble 2), 由角色 ID 和槽位派生, 保证唯一。
func (p *Pet) PetID() uint32 { return 0x20000000 | (p.CharID&0xffffff)<<4 | uint32(p.Slot) }

func (s *Store) ensurePets() error {
	_, err := s.db.Exec(`CREATE TABLE IF NOT EXISTS pets(
		char_id INTEGER NOT NULL, slot INTEGER NOT NULL,
		name TEXT NOT NULL, species TEXT NOT NULL, gfx INTEGER NOT NULL,
		level INTEGER NOT NULL, exp INTEGER NOT NULL, next_exp INTEGER NOT NULL,
		age INTEGER NOT NULL, dp INTEGER NOT NULL, hp INTEGER NOT NULL, max_hp INTEGER NOT NULL,
		hunger INTEGER NOT NULL, loyalty INTEGER NOT NULL,
		earth INTEGER NOT NULL, water INTEGER NOT NULL, fire INTEGER NOT NULL, wind INTEGER NOT NULL,
		battle INTEGER NOT NULL DEFAULT 0,
		PRIMARY KEY(char_id, slot))`)
	if err != nil {
		return err
	}
	if !s.petsReady { // 迁移: 旧库无数据表列
		for _, col := range []string{"enemy_id", "temp", "g_vit", "g_str", "g_tgh", "g_dex"} {
			_, _ = s.db.Exec(`ALTER TABLE pets ADD COLUMN ` + col + ` INTEGER NOT NULL DEFAULT 0`)
		}
		s.petsReady = true
	}
	return nil
}

const petCols = `slot,name,species,gfx,level,exp,next_exp,age,dp,hp,max_hp,hunger,loyalty,earth,water,fire,wind,battle,enemy_id,temp,g_vit,g_str,g_tgh,g_dex`

func (s *Store) ListPets(charID uint32) ([]Pet, error) {
	if err := s.ensurePets(); err != nil {
		return nil, err
	}
	rows, err := s.db.Query(`SELECT `+petCols+` FROM pets WHERE char_id=? ORDER BY slot`, charID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Pet
	for rows.Next() {
		p := Pet{CharID: charID}
		var b int
		if err := rows.Scan(&p.Slot, &p.Name, &p.Species, &p.Gfx, &p.Level, &p.Exp, &p.NextExp, &p.Age, &p.DP,
			&p.HP, &p.MaxHP, &p.Hunger, &p.Loyalty, &p.Earth, &p.Water, &p.Fire, &p.Wind, &b,
			&p.EnemyID, &p.Temp, &p.GVit, &p.GStr, &p.GTgh, &p.GDex); err != nil {
			return nil, err
		}
		p.Battle = b != 0
		out = append(out, p)
	}
	return out, rows.Err()
}

// SavePet 插入或覆盖一只宠物。
func (s *Store) SavePet(p *Pet) error {
	if err := s.ensurePets(); err != nil {
		return err
	}
	b := 0
	if p.Battle {
		b = 1
	}
	_, err := s.db.Exec(`INSERT OR REPLACE INTO pets(char_id,`+petCols+`) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		p.CharID, p.Slot, p.Name, p.Species, p.Gfx, p.Level, p.Exp, p.NextExp, p.Age, p.DP,
		p.HP, p.MaxHP, p.Hunger, p.Loyalty, p.Earth, p.Water, p.Fire, p.Wind, b,
		p.EnemyID, p.Temp, p.GVit, p.GStr, p.GTgh, p.GDex)
	return err
}

// SetBattlePet 只把 slot 标为出战宠 (其它清零)。slot<0 = 全部取消。
func (s *Store) SetBattlePet(charID uint32, slot int) error {
	if err := s.ensurePets(); err != nil {
		return err
	}
	_, err := s.db.Exec(`UPDATE pets SET battle=(slot=?) WHERE char_id=?`, slot, charID)
	return err
}
