// sa2srv 入口: SA2 客户端协议 server + saac 账号服务。
package main

import (
	"flag"
	"log"
	"os"
	"path/filepath"

	"sa2srv/internal/gamedata"
	"sa2srv/internal/handler"
	"sa2srv/internal/saac"
	"sa2srv/internal/server"
)

func main() {
	addr := flag.String("addr", ":8090", "listen addr")
	dbPath := flag.String("db", "saac.db", "saac sqlite path")
	seed := flag.String("seed", "test:1234", "种子账号 name:pwd (不存在则建)")
	legacy := flag.Bool("legacy", false, "用旧帧格式 [seq][len][cmd][payload] (旧 Python 脚本)")
	flag.StringVar(&handler.MapDir, "mapdir", handler.MapDir, "客户端 mapdata 目录 (出生点取地图中心)")
	flag.IntVar(&handler.SpawnMap, "spawnmap", handler.SpawnMap, "出生地图 id")
	flag.StringVar(&handler.ClientMode, "client", handler.ClientMode, "客户端协议: cn (国服, 默认) / ui (欧版 StoneAge2_ui.exe) / auto (按 C0010 版本串 1.220/2.138 识别)")
	dataDir := flag.String("datadir", "data", "怪物/宠物数据表目录 (enemybase.txt / enemy.txt / sa2map.txt / encount.txt, SA1 格式); 空 = 不载入")
	cnNames := flag.String("cnnames", "data_cn/enemy.csv", "国服 enemy.bin 导出的名字/形象索引 (可空)")
	flag.Parse()
	server.Legacy = *legacy

	store, err := saac.Open(*dbPath)
	if err != nil {
		log.Fatalf("saac open: %v", err)
	}
	defer store.Close()

	if n, p, ok := splitColon(*seed); ok {
		if _, err := store.Create(n, p); err != nil && err != saac.ErrDup {
			log.Printf("seed err: %v", err)
		}
	}

	if *cnNames != "" {
		if _, err := os.Stat(*cnNames); err != nil {
			log.Printf("gamedata: no CN name index %s (generate it with tools/cn/dump_cn_data.py); using names from enemybase.txt", *cnNames)
			*cnNames = ""
		}
	}
	if *dataDir != "" {
		db, err := gamedata.Load(*dataDir, *cnNames)
		if err != nil {
			log.Fatalf("gamedata: %v", err)
		}
		handler.Data = db
		if n, err := handler.LoadItemsCN(filepath.Join(*dataDir, "items_cn.txt")); err == nil {
			log.Printf("items: %d CN defs (items_cn.txt)", n)
		} else {
			log.Printf("items: %v", err)
		}
		if n, err := handler.LoadItems(filepath.Join(*dataDir, "items.txt")); err == nil {
			log.Printf("items: %d defs", n)
		} else {
			log.Printf("items: %v (用内建表)", err)
		}
		log.Printf("gamedata: %d enemybase, %d enemy, %d maps with encounters, %d CN names (%s)",
			len(db.Bases), len(db.Enemies), len(db.Encount), len(db.CN), *dataDir)
	}

	h := handler.New(store)
	log.Printf("sa2srv: listening on %s (db=%s, client=%s)", *addr, *dbPath, handler.ClientMode)
	if err := server.ServeTCP(*addr, h); err != nil {
		log.Fatal(err)
	}
}

func splitColon(s string) (string, string, bool) {
	for i := 0; i < len(s); i++ {
		if s[i] == ':' {
			return s[:i], s[i+1:], true
		}
	}
	return "", "", false
}
