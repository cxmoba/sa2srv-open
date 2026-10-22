// Package saac 账号服务 (sqlite 后端)。
//
// 表:
//
//	accounts(name PK, password_hash, salt, created_at)
//	characters(账号, 槽 0..7, 各种属性 — 后续按需加)
package saac

import (
	"crypto/md5"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

var (
	ErrNotFound = errors.New("saac: account not found")
	ErrBadPwd   = errors.New("saac: bad password")
	ErrDup      = errors.New("saac: account exists")
)

// Account 账号记录。
type Account struct {
	Name         string
	PasswordHash string
	Salt         string
	CreatedAt    int64
}

// Store sqlite-backed saac。
type Store struct {
	db         *sql.DB
	charsReady bool // characters 表已建/迁移
	petsReady  bool
	itemsReady bool
}

// Open 打开/初始化 sqlite, 自动建表。
func Open(path string) (*Store, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS accounts(
		name TEXT PRIMARY KEY,
		password_hash TEXT NOT NULL,
		salt TEXT NOT NULL,
		created_at INTEGER NOT NULL
	)`); err != nil {
		return nil, err
	}
	return &Store{db: db}, nil
}

func (s *Store) Close() error { return s.db.Close() }

func randHex(n int) string {
	b := make([]byte, n)
	rand.Read(b)
	return hex.EncodeToString(b)
}

// hashPwd 简单 hash (盐 + pwd, hex SHA-256 待补; 现在用盐+原文对比避免明文落盘)。
// ponytail: 真正的密码学 hash (bcrypt/argon2) 等真正接入注册流程时再加, 现在只防明文。
func hashPwd(pwd, salt string) string {
	return salt + ":" + pwd
}

// Create 注册账号 (已存在返回 ErrDup)。
func (s *Store) Create(name, pwd string) (*Account, error) {
	name = strings.TrimSpace(name)
	if name == "" || pwd == "" {
		return nil, errors.New("saac: empty name/pwd")
	}
	salt := randHex(8)
	acc := &Account{
		Name:         name,
		PasswordHash: hashPwd(pwd, salt),
		Salt:         salt,
		CreatedAt:    time.Now().Unix(),
	}
	_, err := s.db.Exec(`INSERT INTO accounts(name,password_hash,salt,created_at) VALUES(?,?,?,?)`,
		acc.Name, acc.PasswordHash, acc.Salt, acc.CreatedAt)
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE") || strings.Contains(err.Error(), "PRIMARY KEY") {
			return nil, ErrDup
		}
		return nil, err
	}
	return acc, nil
}

// Verify 校验账号密码。
func (s *Store) Verify(name, pwd string) (*Account, error) {
	row := s.db.QueryRow(`SELECT name,password_hash,salt,created_at FROM accounts WHERE name=?`, name)
	acc := &Account{}
	if err := row.Scan(&acc.Name, &acc.PasswordHash, &acc.Salt, &acc.CreatedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	if hashPwd(pwd, acc.Salt) != acc.PasswordHash && !md5Match(pwd, acc) {
		return nil, ErrBadPwd
	}
	return acc, nil
}

// md5Match: 国服客户端 C0100 发的是密码的 32 位 hex MD5 (真机: idLen=4 "test", pwLen=32)。
// 存储是 salt:原文, 所以可以对存储原文算 MD5 比较 (大小写不敏感)。
func md5Match(pwd string, acc *Account) bool {
	if len(pwd) != 32 {
		return false
	}
	if _, err := hex.DecodeString(pwd); err != nil {
		return false
	}
	plain := strings.TrimPrefix(acc.PasswordHash, acc.Salt+":")
	sum := md5.Sum([]byte(plain))
	return strings.EqualFold(hex.EncodeToString(sum[:]), pwd)
}
