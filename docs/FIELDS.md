# SA2 Field Reference

Dedicated offset tables for character and pet packets/structs.
**Sizes/offsets from SA2 client; meanings from SA1 analogy, Pal UI, or live tests.**

Confidence: **high** = RE + live confirm · **med** = RE or strong analogy · **guess** = unconfirmed.

Last updated: 2026-10-12.

---

## 1. Character

### 1.1 S 0x0230 CharLogin (0xFD bytes)

OnCharLogin `0x413410`. Payload copied to `player+0x942` (base for CharParam).
Built from ServerDummy `0x41c54d..0x41c629` (`charLoginData`).

| Off | Type | Meaning | Evidence | Conf |
|-----|------|---------|----------|------|
| +0x00 | u32 | char object id | ServerDummy; live id=0x100 | high |
| +0x04 | u8 | Level | CharParam param 1; `/lv`; overhead Lv.N | high |
| +0x05 | u32 | ? (formerly guessed EXP; not what the status window shows) | param 21; ServerDummy 4000; server keeps it = EXP | low |
| +0x09 | u32 | **EXP (status window 经验值, current)** | param 2; CN live 2026-10-17: ServerDummy 1000 shown as 1000/3000; after fix 4300/10000, +delta after battle | high |
| +0x0d | u32 | **NEXT EXP (status window)** | param 3; same live test | high |
| +0x11 | u16 | HP (current) | param 4; UI `player+0x953`; live 150 | high |
| +0x13 | u16 | HP max | param 5; `player+0x955`; live 200 | high |
| +0x15 | u16 | ? (atk-ish) | param 6; ServerDummy 260 | guess |
| +0x17 | u16 | ? | param 7; 250 | guess |
| +0x19 | u16 | ? | param 8; 270 | guess |
| +0x1b | u16 | ? | param 9; 280 | guess |
| +0x1d | u16 | ? | param 10; 100 | guess |
| +0x1f | u16 | ? | param 11; 210 | guess |
| +0x21 | u16 | ? | param 12; 220 | guess |
| +0x23 | u16 | ? | param 13; 230 | guess |
| +0x25 | u16 | ? | param 14; 240 | guess |
| +0x27 | u8 | ? | param 15; 50 | guess |
| +0x28 | u8 | ? | param 16; 30 | guess |
| +0x29 | u8 | ? | param 17; 60 | guess |
| +0x2a | u8 | ? | param 18; 70 | guess |
| +0x2b | u16 | ? | param 19 | guess |
| +0x2d | u8 | ? | param 27; ServerDummy 2 | guess |
| +0x2e | u16 | ? | param 34 | guess |
| +0x30 | u16 | ? | param 35 | guess |
| +0x32 | u32 | **石币 (拥有石币)** | param 20; 背包窗 / 商店窗; 默认 1050 | high |
| +0x36 | u16 | ? | param 29 | guess |
| +0x38 | u16 | ? | param 31 | guess |
| +0x3a | u16 | ? | param 30 | guess |
| +0x3c | u16 | ? | param 32 | guess |
| +0x3e | u16 | ? | param 33 | guess |
| +0x40 | u16 | HUD chat enable | → `player+0x982`; 0 disables 0x13885; we send 1 | high |
| +0x42 | char[] | Name (MBCS) | → `player+0x984`; live "Tester" | high |
| +0x52..+0xFC | — | **TBD** | rest of 0xFD blob | — |

Params **0, 22–26, 28, 36+** are no-ops in CharParam table `0x68cd08`.

### 1.2 S 0x0420 CharParam

OnCharParam `0x40e8e0` → `0x420aa0`. **Additive** deltas (not absolute set).

| Off | Type | Meaning | Evidence | Conf |
|-----|------|---------|----------|------|
| +0 | u16 | n (entry count) | parser | high |
| +2 | — | n × 12-byte entries | | |

**Per entry (12 bytes):**

| Off | Type | Meaning | Conf |
|-----|------|---------|------|
| +0 | u32 | target char/pet id | high |
| +4 | u16 | apply (0 = skip) | high |
| +6 | u16 | param index | high |
| +8 | s32 | delta | high |

**Param index → 0x0230 field** (self path `player` vtbl+0x228 @ `0x492e20`):

| Param | 0x0230 off | Type | Meaning | Conf |
|-------|------------|------|---------|------|
| 1 | +0x04 | u8 | Level (+ mirrors overhead) | high |
| 21 | +0x05 | u32 | ? (server sends EXP deltas here too) | low |
| 2 | +0x09 | u32 | EXP (status window) | high |
| 3 | +0x0d | u32 | NEXT EXP | high |
| **4** | **+0x11** | **u16** | **HP current** | **high** |
| 5 | +0x13 | u16 | HP max | high |
| 6–14 | +0x15..+0x25 | u16 | 体质/攻基础/敏基础/灵基础/?/攻总/防总/敏总/灵总 (see §1.1) | high |
| 15–18 | +0x27..+0x2a | u8 | 地/水/火/风 ×10 | high |
| 19 | +0x2b | u16 | ? | guess |
| 20 | +0x32 | u32 | **石币** | high |
| 27 | +0x2d | u8 | ? | guess |
| 29/31/30/32/33 | +0x36/+0x38/+0x3a/+0x3c/+0x3e | u16 | ? | guess |
| 34/35 | +0x2e/+0x30 | u16 | ? | guess |

C 0x0420 empty ("SendCharParam") → server replies n=0.

### 1.3 C/S 0x0410 CHAR_POS (0x18 bytes)

Format string @ `0x5f6500`; handler `0x429500`.

| Off | Type | Meaning | Evidence | Conf |
|-----|------|---------|----------|------|
| +0 | u32 | object id (high nibble = type: 0 player, 1 NPC, 2 pet, 6 monster, …) | live / `0x427500` | high |
| +4 | u16 | area / map id | live Halbol=100 | high |
| +6 | s16 | x (tile) | live | high |
| +8 | s16 | y (tile) | live | high |
| +0xa | u8 | dir | live | high |
| +0xb | u8 | act | live | high |
| +0xc | u16 | type / CGNO (graphic) | live 30002 field / 31002 battle | high |
| +0xe | u16 | effect | format string | med |
| +0x10 | u32 | ? | TBD | guess |
| +0x14 | u16 | ? | TBD | guess |
| +0x16 | u16 | ? | TBD | guess |

C 0x0412 = flagged delta of the same fields (see PACKETS.md).

### 1.4 C 0x0401 SendInfo / S 0x0441 (NPC type-1)

| Packet | Layout | Conf |
|--------|--------|------|
| C 0x0401 | u32 id (hover/appear) | high |
| S 0x0441 | u32 id + 0x28 type-1 info: +0 u32 id, +4 u32 ?, +8 char[0x20] name (MBCS) + trailing string | med |

S 0x0401 CharaInfo (`0x40d5c0`): u32 id; type-specific struct by `id>>28` — **TBD per type**.

### 1.5 Related (brief)

| Packet | Notes | Conf |
|--------|-------|------|
| S 0x00E0 | u32 flags; bit2 = create HUD 0x1388 | high |
| S 0x0415 | u32 id = CharOut (despawn) | high |
| S 0x0441 | NPC name plate (above) | med |
| C 0x0480 / S 0x0480 | NPC event lock/unlock | high |
| S 0x0620/21/22 | Dialog | high |

---

## 2. Pet

### 2.1 S 0x0462 OnPetInfo

Handlers: `0x40ed10` → `0x4cf750` / create `0x4daee0`.
Header **0x9c** copied to pet object `+0x918` (vector entry at `player+0xaac` stores `&pet[+0x960]` where `[0]=PET_ID`).

**ID rule:** high nibble must be **2** (`0x2xxxxxxx`). Server scheme: `0x20000000 | (charId&0xffffff)<<4 | slot` (Tester 0x100 → 0x20001000..0x20001004). Type 3 allocates nothing → null `+0x918` crash.

| Off | Type | Meaning | Evidence | Conf |
|-----|------|---------|----------|------|
| +0 | u32 | PET_ID | create / `0x4ca600` lookup | high |
| +4 | u16 | slot 0..4 (max 5) | SA1 MAX_PET=5; live | high |
| +8 | u32 | flags: bit0 valid (cleared on remove `0x4ca680`); **bit16 0x10000 = 休息 Rest** (clear = 战斗 Battle); bit4 0x10 local anim state | CN `0x534f70` (C 0x1463 role 1 sets / role 0 clears); CN detail panel "状态 战斗/休息" verified live `cn_r3_state_*.png` | high |
| +0xc | MBCS\0 | name (to +0x2e) | strlen `5771d2`+MB2WC → `+0x96c`; UTF-16 truncates to 1 char; Pal "Buddy" | high |
| +0x2c | u16 | ? | getter `0x4d9fa5` | guess |
| +0x2e | u16 | CGNO / graNo | 0 → "Dino Graphic Res Errer"; live 29000 | high |
| +0x30 | u16 | Level | Pal Lv 001; SA1 level | high |
| +0x34 | u32 | EXP | Pal / SA1 exp | high |
| +0x38 | u32 | NextEXP | Pal 0/10; SA1 maxExp | high |
| +0x3c | u16 | **Endurance (HP) cur** | live probe `/petprobe` (value=offset → Pal Endurance 60/62) `pal_probe.png`; was mislabelled Age | high |
| +0x3e | u16 | **Endurance (HP) max** | same probe | high |
| +0x40 | u16 | **Attack** | probe → Attack 64 | high |
| +0x42 | u16 | **Defense** | probe → Defens 66 | high |
| +0x44 | u16 | **Speed** | probe → Speed 68 | high |
| +0x46 | u16 | **Spirit** | probe → Spirit 70 (Mood bar also moves; Mood looks derived) | high |
| +0x48 | u16 | ? (not shown in Pal) | probe 72 not displayed | guess |
| +0x4a | u8 | **Guts cur 气力** (max fixed 100) | probe → Guts 74/100; CN setter `0x4e30cd` writes a byte; CN probe 66 → 气力 66 | high |
| +0x4b | u8 | ? (compared with 20 / 100 at `0x4e58ec`/`0x529d73`; raising it raises 心情: 0→88, 10→90, 77→107 with other fields fixed) | CN probe | med |
| +0x4c | 8×u16 | skill IDs (国服: 每个号登录时 C0530 查询 → S0530, 战斗宠物菜单 "技能" 列出) | CN live 2026-10-18 | high |
| +0x5c | u32 | **年龄 Age** (CN panel prints only the first 2 decimal digits: 500→"50", 1234→"12", 1→"1") | CN probe `cn_r3_probe*` | high |
| +0x60 | u16 | ? | getter | guess |
| +0x62 | MBCS\0 | species / race | → `+0x988`; Pal "Buddy" | high |
| +0x82 | u8 | ? | getter | guess |
| +0x83 | u8 | earth | SA1 earth; Pal Attr | med |
| +0x84 | u8 | water | SA1 water | med |
| +0x85 | u8 | fire | SA1 fire | med |
| +0x86 | u8 | wind | SA1 wind | med |
| +0x88 | u16 | **Loyalty** (Pal "Loy") | probe → Loy 136 | high |
| +0x8c | u32 | ? (not shown in CN panel) | getter | guess |
| +0x98 | u16 | ? (not shown) | getter | guess |
| +0x9a | u16 | number next to the egg icon in CN pet detail | CN probe → 154 | high |
| +0x9c.. | — | **TBD** (we send len 0xa0) | | |

S 0x1460 OnPartyPetInfo = same body; S 0x1461 OnDelPartyPetInfo.

**Age** = +0x5c (server: starts at 1, +1 per won battle the pet took part in, cap 99, persisted in `pets.age`). **DP**: the CN pet panel has no DP field (SA1 dp is a character stat); `pets.dp` stays unused. **心情 Mood** is derived by the client (+0x4b and other fields), not sent directly. Rest/Battle = +0x08 bit16; the server marks every non-battle pet Rest and re-sends 0x0462 for pets whose role changed.

Element display on NA Pal may remap order vs +0x83..+0x86 — verify before trusting Attr numbers.

### 2.2 Pet C packets (client vtable `0x5f4880`, names from log format strings) / S 0x0466

| Cmd | Client fn | Log string | Body | Conf |
|-----|-----------|------------|------|------|
| C 0x0460 | `0x40eb30` | `SendPetDrop %x` | u32 petId | high (RE) |
| C 0x0461 | `0x40ec70` (callers `0x4e83ff`, `0x52aabf`) | `SendPetOrder cnt = %d` | u16 cnt + cnt×{u32 id, u16 order, u16 status} from `player+0xab0` | high (RE) |
| C 0x0463 | `0x40f530` | `SdPetAct owner%x pet%x act%d opt%d` | 4×u32 | high (RE) |
| C 0x0464 | `0x40f5a0` | `SendPetName 0x%x, %d, %s` | u32 id, u16 len, MBCS name | high (RE) |
| **C 0x0465** | `0x411c80` ← `0x4c9960` ← `0x4e5340` | `SendPetBattle cnt = %d` | **u8 = player+0x9f4 battle-count setting**, NOT a slot (corrected) | high (RE) |
| C 0x0466 | `0x412730` | `SendPetLock %08x` | u32 petId (C 0x0466 ≠ S 0x0466) | high (RE) |
| **C 0x1463** | `0x413250` | `SendPetRole`-ish | **u32 petId, u32 role: 0 = Battle, 1 = Rest** (Pal → Role → Rest/Battle) | **high (live)** |

Live: Pal → Kuku → Role → Rest sent `01 10 00 20 01 00 00 00`, then Battle sent `… 00 00 00 00`. Client updates the label itself; clicking the role it already shows sends nothing.
**Server:** C 0x1463 role 0 → that pet becomes the single battle pet (DB `pets.battle`) and the server sends S 0x0466; role 1 on the battle pet → no battle pet.

**S 0x0466** — `0x40d1d0` → `0x422370(id)` → `or byte [obj+0x91d], 0x20`.

| Off | Type | Meaning | Evidence | Conf |
|-----|------|---------|----------|------|
| +0 | u32 | pet object id | live: pet enters order list | high |

Sent on login (battle pet), on role select, and before 0x0b10 on `/fight`.

### 2.3 Battle unit entry (S 0x0b10, 0x63 bytes each)

Header 0x0c + cnt×0x63. Copy `0x4562d0` → object `+0x90c`.

**Header:**

| Off | Type | Meaning | Conf |
|-----|------|---------|------|
| +0 | u16 | battle map / BG id | high |
| +3 | u8 | init flag (1 = surprise) | med |
| +4 | s16 | camera x | high |
| +6 | s16 | camera y | high |
| +0xa | u8 | our side | high |
| +0xb | u8 | unit count | high |
| +2,+8..+9 | — | **TBD** / experiments | guess |

**Per unit (0x63):**

| Off | Type | Meaning | Evidence | Conf |
|-----|------|---------|----------|------|
| +0 | u32 | object id | type nibble; pet must be `0x2xxxxxxx` | high |
| **+4** | **u32** | **BID** | BatInit `"BID%x"`; map key at battle+0x398 (`479340`). **Low byte = side** (ally check `cmp [entry+4], header+0xa`) | **high** |
| +7 | u8 | orderable | → obj+0x913; 0 → 47a280 path B (filter flag); ≠0 → path A | high |
| +0x13 | u32 | max HP | "HP MAX ERROR" if cur>max | high |
| +0x17 | u32 | current HP | | high |
| +0x1b | u8 | SP | → obj+0x927 | med |
| +0x1c | u8 | **level** (shown as "LV.n" under the name on hover) | CN live 2026-10-17: monster lv3 → "LV.3" | high |
| +0x1d | u16 | CND | → obj+0x929; bits `0xe0` gate some order paths | med |
| +0x21 | u32 | ? | → +0x92d | guess |
| +0x25 | u8 | ? → 0x58d1e0(v,1) | we set 2 | guess |
| **+0x27** | char[0x20] | **display name** MBCS\0 (CN: UTF-8) | CN 0x590050: if [+0x27]≠0 → convert → obj+0x840; live: "乌力斯坦" / "贝恩达斯" shown on hover. EU UI client renders CJK as boxes | high |
| +0x47 | — | **not owner** | was misdoc; see +0x4b | — |
| **+0x4b** | **u32** | **pet id for ownership** | ebx=entry+4 → `[ebx+0x47]`; `0x4ca600` | **high** |
| +0x4f | u16 | map? | we put bg id | med |
| +0x51 | s16 | **tile x** (explicit, no slot/row index) | `0x456310`: obj+0x95d/+0x95f → vfunc+0x400 (x<<16, y<<16); live: changing x/y moves sprites | high |
| +0x53 | s16 | **tile y** | same | high |
| +0x55 | u8 | dir (side 0 = 7, side 1 = 3) | → obj+0x961 → vfunc+0x45c, +0xa44/+0xa45 | high |
| +0x56 | u8 | ? | | guess |
| +0x57 | u16 | CGNO | | high |
| other | — | **TBD** | | |

**Ownership path:** id≠self → lookup `+0x4b` in `player+0xaac` → owned pet object size 0xb18 (`0x4235d4`). Mode byte header+3==8 skips.

**Order list (`0x41f620(kind,mode)` + `0x47a280`):** mode from header+3. kind 0=self, 1=ally, 2=pet. Mode 0: allow all; mode 2/3/6: pets only; mode 1: exclude pets. With entry+7=0: need filter≠0 and `(CND&0xe0)==0`, then `479400`.

**BID (live):** player `0x0000`, monster `0x0001`, pet `0x0200` (low byte side=0). **Collision on BID=0 for player+pet was why pet never entered the order list.** ActID/PasID in 0x0b30 use BID (`pet hits … bid=512`).

**Formation (server-side; the client has no slot field):** the two sides face each other along the y axis. Side 0 is at +y facing −y (dir 7); side 1 is at −y facing +y (dir 3). On screen +y runs down-right, so a row is a line along x (a down-left diagonal). Rows: front = h×18% from the centre (min 2), back = front + 5 tiles. Units in a row are centred on the camera x, 6 tiles apart; side 1 is mirrored. Characters stand in the back row and each pet in the front row at its owner's slot; wild monsters stand in the front row. `formPos()`; env `SA2_FRONTPCT/SA2_ROWGAP/SA2_SLOTGAP`, `SA2_MONN` (extra monsters, formation test only). Shots: `formation_nopet.png`, `formation_pet.png`.

### 2.5 Pet / monster graphics (species → CGNO)

**No species table ships with the client.** I checked `data/*.bin` (item.bin is the item table: byte-inverted, 0xf0-byte records, UTF-16 names), `data/gui/*`, namelist.bin (map names) and the exe's strings. Species names, stats and battle/field/portrait ids are all server-side, as in SA1. The client only maps CGNO → graphic file.

| CGNO range | File | Content | Use |
|---|---|---|---|
| 20000–20611 | `graphic/20xxx.anz` (112 parts) | human characters / NPCs | players, NPCs |
| **25000–25061** | `graphic/25xxx.anz` | **creatures, normal size** (32–90 px wide) | **pets / monsters, colour 0** |
| 26000–26061 / 27000–27061 / 28000–28061 | `26xxx/27xxx/28xxx.anz` | same creatures, colour variants 1/2/3 (seen: 25000 brown / 26000 red / 27000 black-white / 28000 grey) | colour variants |
| 29000–29012 | `graphic/29xxx.ana` (1 big ANZ each, 2.6 MB, 250–440 px) | giant boss models | **not** regular pets (old placeholder) |
| 30000–33xxx | `.anz` | player bodies (30002 / 31002 = Tester) | characters |

**Rule:** `CGNO = (25 + colour) × 1000 + species`, species 0..61, colour 0..3.
**One CGNO drives everything:** the Pal list portrait, the Pal detail sprite and the battle sprite all come from the same 0x0462 +0x2e / 0x0b10 +0x57 CGNO. Live: 25000 shows a piglet in both the portrait and the sprite, 25010 a green dino. Field walk-with should use the same id (assumed, not verified).
Species/CGNO now come from the data tables (`data/enemybase.txt` + `data/sa2map.txt`, see README "Game data"); the old English templates were removed and stored pets are migrated to Chinese names by CGNO on login. `SA2_MONGFX` is only used when no data table is loaded. Shots: `realpet_pal_list.png`, `realpet_pal.png`, `realpet_battle.png`.
Contact sheets for picking ids: generate with `/tmp/sheet.py` (uses `tools/sa2_ana.py`).

### 2.4 C 0x0b20 SendBattleOrder (0x38)

Live wire often 64 B (pad); meaningful body 0x38. Format string `SendBattleOrder ID%08x, %d, tgtID%08x,%d` @ `0x40efec`.

| Off | Type | Meaning | Evidence | Conf |
|-----|------|---------|----------|------|
| +0 | u32 | actor id | live Attack: `0x100` (player); pet actor `0x2xxxxxxx` **not yet seen** | high |
| +8 | u8 | act | **3 = Attack** (live); **0** after Call pal click; **8** = self-target (Care/Wait-ish, live mis-click) | high |
| +0xf | u8 | ? | Attack → 5; act0/8 → 1 | med |
| +0x10 | u32 | target id | Attack → `0x60000001` monster; act8 → self `0x100` | high |
| +0x14 | u8 | ? | often 0 in live pads | guess |
| rest | — | **TBD** | | |

**Order slots (live 2026-10-12 evening):** with unique pet BID `0x200` + S 0x0466 battle flag:
1. C 0x0b20 #1 actor=`0x100` act=3 target=monster
2. C 0x0b20 #2 actor=`0x20000001` (pet) — pet order menu (brain/AI + fangs Attack); shot `battle_pet_orderlist_ord2_head.png`
3. Server `petTurn` BID=`0x200` (`pet hits … bid=512`)

**Call pal** (battle menu) sends a C 0x0b20 with act=0. Picking the battle pet in the Pal UI is **C 0x1463** role=0 (not C 0x0465). The pet's own menu has Attack one row lower (≈715,94 window-relative); player Attack ≈ (715,65); enemy ≈ (250,250).

UI automation: `xdotool mousemove --window WID`; Attack ≈ (660,66); enemy ≈ (400,120). Pet menu Attack is lower (fangs row) — mis-click can yield act=`0x96`.

---

## 3. Open / TBD

- 0x0230 +0x05 meaning; +0x15..+0x3e exact combat/stat names
- Mood formula (client-derived; +0x4b input)
- Pet +0x48, +0x8 bits beyond bit0; S delta packet for pet EXP/HP (none found; full 0x0462 resend works = update path 0x4cf750)
- Element byte order vs NA Pal Attr icons
- 0x0b10 unit bytes between documented fields
- Pet C 0x0b20 act codes beyond 3 (seen 0x96)


## 中文客户端数据文件
见 DATAFILES_CN.md (enemy.bin 怪物名表 = id/名称/种族/CGNO, 无数值; link/pth/pthinfo 寻路; instdrd 随机迷宫; sa2ngword 屏蔽词)。

## 物品 (2026-10-17 第 4 轮)
- S04b0 条目 0x12: +0 u32 id, +4 u16 物品号, +6 u16 图标CGNO (背包 0x4adbc2→0x57c730; 装备窗口 0x4af7b8 读 node+0xe), +8 u16 位置码, +0xa u16 选项 (0x10=已装备, 背包不显示已装备), +0x10 u16 数量。客户端物品表在 char+0xb18 (std::list, node+8=条目)。
- S0510 资料 0x14a: +0 物品号, +4 名字, +0x44 装备类型(位置码), +0x4a 5×{u16 类型,u16 值} 需求 (0x4ac150; 1=等级, 2..5=属性), +0x5e 8×12 字节属性 {+0 u16 类型, +4 s16 值} (0x5575c0; 6 体力, 0xb 攻, 0xc 防, 0xd 敏, 0xe 精), +0xc6 说明, +0x13e u32 价格 (提示 0x559b5b), +0x142 u32 卖价 (>0 才可卖, 0x54abce), +0x148 标志 (bit7/0x20/0x40 未确认)。
- 客户端角色结构 = S0230 负载 + 0x97e。+0x993 体, +0x995 攻基础, +0x997 敏, +0x999 精。客户端防御基础 = 等级+5; 状态窗 "物品" 列 = 总值 − 基础。
- 服务端: 0x0420 param 11 = 攻总值, 12 = 防总值; 战斗伤害用 playerStats = statsAt(lv) + 装备加成。

## 物品 (2026-10-17 第 5 轮)
- S0510 物品资料记录: +0 u16 物品号, **+2 u16 图标 CGNO** (国服新 UI 图片控件占位 0x1a → 0x4a6a20 取记录 +2)。
- S04b0 条目 opt (+0xa u16): 0x02 = 地图上可使用 (拖到头像才会发 C04b2), 0x04 = 战斗中可用, 0x10 = 已装备。拖放使用还要求 loc == 0x13。
- 新 UI 窗口: 列表 vector 0x719864..0x719868, 窗口 id 在 +0xe4, 查找 0x542980(id)；子控件 std::set 在 +0xfc，类型 +0xe0，id +0xe4，0x552720(类型,id)。
  背包 = 窗口 5110 (vtbl 0x6863c0, 刷新 0x51fed0, 拖放 ~0x51fff0, 模式 +0x308: 0x1e 背包/0x1f 装备)。HUD = 窗口 5000，+0x528 = 悬停头像。
- 客户端 item.bin 表 0x7b9c80 (数量 0x7b9d18, 数据 0x7b9d1c, 0x128 字节/项, +0x11a 图标), 查 0x561130 — 只给奖励/任务等列表用，背包不用它。

## 战斗 (2026-10-17 第 6 轮, 国服实测)
- S0B30 条目 (0x4b): +4 ActID, +8 act, +0xa ActNum, +0xc u32 物品ID, +0x10 ActEfe, +0x36 PasID, +0x3a Pas, +0x3b PassHP (负=扣 正=加), +0x44 CND。
- act 0x9a 道具: ActEfe 0x71 (或 0x89) → 0x458e40 给目标加 PassHP + 发光; 0xa = 只播动作 + 客户端本地扣物品, 不加血。
- act 0x99 逃跑: ActEfe 0x97 = 失败转身 (0x586ad0); ActEfe 0 = 成功跑走 (全队跑, 0x58f630)。
- act 8 防御 (0x58ea40), act 0x58/0x9b 捕捉 (0x587d86), act 3 攻击。
- 单位 HP +0x927 / 最大 +0x923 (0x58bf50 应用); 地图角色 HP player+0x953 (S0420 param 4)。
- S0BE0 EndFlag: 0 胜利 (结算块, CharaCnt 0 = 不弹结算窗, 逃跑用); 1 战败 (+CHAR_POS, 弹 UI 0xfc "全员气绝"); 8 卡死 (国服)。
- S0B30 头 flag (0x599ec0): 0 普通, 1 宠物入场, 2 合击, 3 捕捉, 4 呼叫同伴, 5/6 遁地, 7 忠犬。
  线上解析 0x4112f0: flag 0 = count + 0x4b 条目; flag 1/2/3 无 count 字节。flag 3 捕捉结构 0x13 字节: +4 ActID, +8 ActKind(0x58), +0xd PassID, +0x11 PassKind(0x5a 成功/0x57 失败), +0x12 Turn。

## S0B30 暴击/Miss (2026-10-17)
- **ActEfe (+0x10)**: 0x5c Miss, 0x5d 命中闪, 0x5e 反击, 0x5f 防御, 0x60 暴击 (0x457a90 建特效 0x53..0x57)。
- **CND (+0x44)**: 与 SA1 BCF 同位; bit0=1 Miss, bit1=2 普通, bit2=4 暴击, bit3=8 防御, bit4=16 反击; bit5=0x20 倒下 (可与暴击位并存, 例 0x24)。
- **PassHP (+0x3b)**: Miss 时必须 0 (否则仍出伤害字); 暴击仍发 -伤害。
- 伤害字颜色 (+0xa64): 1 红, 2 绿, 3 黄; 普通受击固定 1, 暴击不改颜色。

### S0530 宠物技能资料 (0x8a)
| off | type | field | evidence |
|---|---|---|---|
| +0 | u16 | 技能号 | 0x5020b7 |
| +2 | MBCS (0x21) | 名字 | 菜单 |
| +0x23 | u8 | 消耗气力 | 真机 knob 7 → 菜单 "消耗气力 7" |
| +0x24 | u8 | 选目标方式 (0 敌单体 / 1 自身 / 3..6 其它光标) | 0x567dc8, 真机 1 → act 0x81 自身 |
| +0x41 | MBCS (0x48) | 说明 | 提示框 |
| +0x89 | u8 | 标志 bit0 (0x55e781) | 未知 |
