# sa2srv-open

A from-scratch game server emulator, written in Go, that speaks the network protocol of the
**StoneAge 2** (石器时代 2) PC client (2008, discontinued). It exists for **protocol research,
software preservation and interoperability**: the official servers are gone, and this project
documents how the client talks to a server and implements a compatible server from scratch.

> **No game assets or game data are included.** This repository holds only original source code,
> original documentation of the protocol and file formats, and tools that read data from a client
> *you* legally own. It is not affiliated with, endorsed by, or connected to the owners of the
> StoneAge / StoneAge 2 IP or any official operator. All trademarks belong to their owners.

## Status

Tested against the Simplified-Chinese (CN) client build (`StoneAge2_nogg.exe`, 2010) running under
Wine; an older EU/NA UI build is partly supported (`-client ui`). Implemented so far:

- **Login & characters** – version/handshake, server list, register/login (MD5 password from the
  CN client), create / select character, SQLite persistence (`modernc.org/sqlite`, no cgo).
- **World** – map entry using the client's own `mapdata`, movement sync and broadcast, chat
  (all / whisper / party), NPC dialogue windows, other players appearing/disappearing.
- **Maps & warps** – exits are generated at startup from the client's `mapdata/*.bin` exit
  (JUMP) and arrival (JUM2) sections plus a small hand-made `data/warp.txt`.
- **Battles** – turn engine with the client's action packets: attack, guard, escape, critical hits,
  misses, KO, multiple monsters, **multiple pets per player**, **party battles** (every member gets
  their own battle screen and orders, shared EXP), random encounters per map, EXP and level-ups.
- **Pets** – pet list, battle/rest state, age, EXP, **capture** (uses the client's built-in
  capture animation), pet skills (damage / heal) with the SA2 pet energy (FP) cost.
- **Items** – bag, equip/unequip with stat effects, use (food heals player or pet), discard, feeding pets,
  battle items, monster drops, **NPC shop** (buy/sell), gold.
- **Party & trade** – invite / accept / leave / kick; player-to-player trade of items and gold.
- **Docs** – packet tables ([docs/PACKETS.md](docs/PACKETS.md)), character/pet field offsets
  ([docs/FIELDS.md](docs/FIELDS.md)), client data file formats
  ([docs/DATA_FORMATS_CN.md](docs/DATA_FORMATS_CN.md), [docs/DATAFILES_CN.md](docs/DATAFILES_CN.md)).

Not done yet: walkability checks on the server, quests, warehouse, friends, PK, guilds, and plenty of
content. Most code comments and docs are in Chinese; English summaries are welcome as PRs.

## Build

Requires Go 1.25+.

```bash
go build -o bin/sa2srv ./cmd/sa2srv
GOOS=windows GOARCH=amd64 go build -o bin/sa2srv_win64.exe ./cmd/sa2srv   # cross-compile
go vet ./... && go test ./...
```

## Run

```bash
./bin/sa2srv -addr :7777 -mapdir /path/to/your/client/mapdata
```

| flag | default | meaning |
|---|---|---|
| `-addr` | `:8090` | TCP listen address |
| `-db` | `saac.db` | SQLite file for accounts, characters, pets, items |
| `-seed` | `test:1234` | create this `name:password` account if missing (local testing only) |
| `-mapdir` | `../Client/mapdata` | the `mapdata` folder of your client (spawn points, warps, battle backgrounds) |
| `-spawnmap` | `100` | map id for new characters |
| `-client` | `cn` | `cn` (CN client, reference), `ui` (EU/NA UI build), `auto` (detect by version string) |
| `-datadir` | `data` | monster / item / skill tables (see below) |
| `-cnnames` | `data_cn/enemy.csv` | optional monster name index generated from your client |

Point your client at the server (the CN client takes the account in its login box; the UI build
takes `-u <id> -p <pw> -a 127.0.0.1:7777:Local:ok` on the command line). `tools/run_client_wine.sh`
shows how to start a client under Wine.

Chat commands for development (no permission check – do not expose this server publicly):
`/warp map x y`, `/fight [enemyId]`, `/pet [enemyId]`, `/lv N`, `/give itemNo`, `/items`, `/pjoin`, `/battlecnt N`.

## Game data

The server starts with the tiny **hand-written example tables** in `data/` (four made-up monsters,
a few placeholder items and pet skills). For real content, generate tables from a client and/or
server data you own. Generated files are git-ignored and must not be committed.

```bash
export SA2_CLIENT=/path/to/your/SA2China            # your installed client
python3 tools/cn/dump_cn_data.py "$SA2_CLIENT/data"  # -> data_cn/enemy.csv, pth/pthinfo/link, ...
python3 tools/cn/dump_cn_more.py "$SA2_CLIENT"       # -> data_cn/item.json, namelist, mapdata index, ...
python3 tools/cn/gen_items_cn.py                     # -> data/items_cn.txt (full item table)
# optional: monster growth tables in StoneAge 1 gmsv format (enemybase1.txt / enemy1.txt / encount / group)
python3 tools/sa1/import_sa1.py /path/to/sa1/gmsv/data data   # overwrites data/enemy*.txt, writes sa2map.txt
```

| file | content |
|---|---|
| `data/enemybase.txt` | species templates (growth, elements, capture rate, sprite) – SA1 `enemybase` column layout |
| `data/enemy.txt` | spawn instances (level range, EXP, drops) – SA1 `enemy` layout |
| `data/encount.txt` | `MAP,ENEMY_ID,WEIGHT` random encounters |
| `data/mapflags.txt` | `MAP,safe` maps without encounters |
| `data/items.txt` | hand-tuned items; overrides `items_cn.txt` by item number |
| `data/petskill.txt` | pet skills |
| `data/warp.txt` | hand-made warps (take priority over the ones generated from `mapdata`) |

Formulas follow the open-source StoneAge 1 server (stats from growth × level, damage
`atk²/(atk+def)` with variance and agility-based dodge).

## Layout

```
cmd/sa2srv/        entry point
internal/proto     frame format, checksum, packet helpers
internal/handler   game logic: login flow, movement, chat, battles, pets, items, shop, party, trade, warps
internal/saac      account / character / pet / item storage (SQLite)
internal/gamedata  monster tables and stat formulas
internal/mapdata   client mapdata/*.bin reader (header, tiles, exit and arrival sections)
internal/mbcs      GBK/UTF-8 helpers
tools/cn           decoders for the CN client's data files (read from your own install)
tools/sa1          importer for StoneAge 1 server tables
tools/re           small static-analysis helpers used to document the protocol
tools/*.py, *.sh   test clients and Wine/xdotool automation used for end-to-end tests
docs/              protocol and file-format documentation
```

## 中文说明

sa2srv-open 是用 Go 从零编写的《石器时代 2》客户端协议服务端模拟器，用于协议研究、软件保存和互通性研究。
官方服务器早已关闭，本项目记录客户端与服务器之间的协议，并从零实现兼容的服务端。

- **本仓库不含任何游戏资源或游戏数据**（无客户端 exe/dll、图片、地图、物品/怪物/任务表）。`data/` 里只有手写的示例小表。
- 完整数据请用 `tools/cn/` 从你自己合法拥有的客户端生成（见上文 *Game data*），生成的文件已加入 `.gitignore`，请勿提交。
- 已实现：登录建角、地图与传送、走路聊天、NPC、战斗（组队、一人多宠、暴击/闪避/防御）、捕捉、宠物技能、物品与商店、组队、交易等。
- 本项目与《石器时代》/《石器时代 2》的版权方及任何官方运营方无关。商标归各自所有者。
- 许可证：MIT（见 [LICENSE](LICENSE)）。

## License

MIT – see [LICENSE](LICENSE). The license covers the code and documentation in this repository only;
it grants nothing regarding the game client or its data.
