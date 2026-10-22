package saac

import "fmt"

// Char 角色 (最小字段; 出生点 map/x/y 为子格坐标, 见 mapdata 头 gridW/gridH)。
type Char struct {
	ID      uint32
	Account string
	Slot    int
	Name    string
	Map     int
	X, Y    int
	// 成长 (0 = 未设置, 登录时用默认值): 等级 / 当前等级内 EXP / 场景 HP
	Level, Exp, HP int
}

func (s *Store) ensureChars() error {
	if s.charsReady {
		return nil
	}
	_, err := s.db.Exec(`CREATE TABLE IF NOT EXISTS characters(
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		account TEXT NOT NULL, slot INTEGER NOT NULL, name TEXT NOT NULL,
		map INTEGER NOT NULL, x INTEGER NOT NULL, y INTEGER NOT NULL,
		UNIQUE(account, slot))`)
	if err != nil {
		return err
	}
	// 迁移: 旧库无 level/exp/hp 列 (已存在时 ALTER 报错, 忽略)
	for _, col := range []string{"level", "exp", "hp"} {
		_, _ = s.db.Exec(`ALTER TABLE characters ADD COLUMN ` + col + ` INTEGER NOT NULL DEFAULT 0`)
	}
	// id 从 0x100 起, 避开 Dummy 的 0x15
	_, _ = s.db.Exec(`INSERT OR IGNORE INTO sqlite_sequence(name,seq) VALUES('characters',255)`)
	s.charsReady = true
	return nil
}

func (s *Store) ListChars(account string) ([]Char, error) {
	if err := s.ensureChars(); err != nil {
		return nil, err
	}
	rows, err := s.db.Query(`SELECT id,slot,name,map,x,y,level,exp,hp FROM characters WHERE account=? ORDER BY slot`, account)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Char
	for rows.Next() {
		c := Char{Account: account}
		if err := rows.Scan(&c.ID, &c.Slot, &c.Name, &c.Map, &c.X, &c.Y, &c.Level, &c.Exp, &c.HP); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func (s *Store) CreateChar(c *Char) error {
	if err := s.ensureChars(); err != nil {
		return err
	}
	r, err := s.db.Exec(`INSERT INTO characters(account,slot,name,map,x,y) VALUES(?,?,?,?,?,?)`,
		c.Account, c.Slot, c.Name, c.Map, c.X, c.Y)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrDup, err)
	}
	id, _ := r.LastInsertId()
	c.ID = uint32(id)
	return nil
}

func (s *Store) DeleteChar(account string, slot int) error {
	if err := s.ensureChars(); err != nil {
		return err
	}
	_, err := s.db.Exec(`DELETE FROM characters WHERE account=? AND slot=?`, account, slot)
	return err
}

func (s *Store) SavePos(id uint32, m, x, y int) error {
	_, err := s.db.Exec(`UPDATE characters SET map=?,x=?,y=? WHERE id=?`, m, x, y, id)
	return err
}

// SaveStats 存角色等级 / EXP / 场景 HP。
func (s *Store) SaveStats(id uint32, level, exp, hp int) error {
	if err := s.ensureChars(); err != nil {
		return err
	}
	_, err := s.db.Exec(`UPDATE characters SET level=?,exp=?,hp=? WHERE id=?`, level, exp, hp, id)
	return err
}
