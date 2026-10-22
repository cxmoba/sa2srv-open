# SA2 封包表 (按功能整理)
> **Field offsets:** see [FIELDS.md](FIELDS.md) for character (0x0230/0x0420/0x0410) and pet (0x0462/0x0465/0x0b10 unit) tables.


帧: `[u16 plainLen][u16 wireLen][u16 cmd][payload][u16 cksum]`, 全小端, 见 proto/wire.go。
C = 客户端→服务器, S = 服务器→客户端。置信度: 高 = 真机抓包或反汇编直接读出; 中 = 反汇编推断; 低 = 猜测。

## 登录 / 选角 (已真机跑通)

| cmd | 方向 | 布局 | 证据 | 置信 |
|---|---|---|---|---|
| 0x0010 | C | char[8] "2.138" + char[8] 客户端版本 | 0x415cbd, 真机 | 高 |
| 0x0010 | S | u32 res(0=OK) u32 seed u16 codecType(0=明文) | 0x414640 | 高 |
| 0x0020 | C | 空 (选服状态探测) | 0x415ccf, 真机 | 高 |
| 0x0020 | S | u16 cnt + cnt×0x1C; 记录 +0x14 u32 人数 +0x18 u32 上限 | 0x4f4fc0/0x4f4600, 真机显示 Smooth | 高 |
| 0x0100 | C | u32 ? u8 loginType u16 idLen u16 pwLen id pw | 0x41487d, 真机 | 高 |
| 0x0100 | S | u32 result 1=OK, 5=u16 len+文本 | 0x415d40 | 高 |
| 0x0220 | C/S | S: u16 cnt + cnt×0xE3; 条目 +0 u16 槽 **+2 u32 charId** +0x36 名字 | 0x428c90, 真机回传 | 高(id) 中(名字) |
| 0x0200 | C/S | C: u16 slot + 0x82 结构(名字 @+0x32); S: u32 1=OK | 0x40d080 / 0x410d90 | 中 |
| 0x0230 | C/S | C: u32 charId; S: 0xFD 角色数据 (照抄 ServerDummy) | 0x410cf0 / 0x413410, 真机 | 高 |
| 0x0A03 | C/S | C: 8 字节; S: u16 ret u16 mapId [mapdata]; ret≠0 = 用本地地图 | 0x40d770, 真机进图 | 高 |
| 0x0900 | C/S | C: 空; S: u16 1, u16 15, u16 0, u32 60 | 0x40d530 | 中 |

## (1) 移动同步

| cmd | 方向 | 布局 | 证据 | 置信 |
|---|---|---|---|---|
| 0x0410 CHAR_POS | C/S | +0 u32 id (高 4 位=对象类型) +4 u16 area(地图) +6 s16 x +8 s16 y (子格) +0xa u8 dir +0xb u8 act +0xc u16 type(形象) +0xe u16 effect +0x10 u32 ? +0x14 u16 ? +0x16 u16 ? (共 0x18) | 格式串 "CHAR_POS id,x,y,area,dir,act,type,effect" @0x5f6500 (0x429500); 真机 C0x0410: `00010000 6400 9700 9800 04 00 3275 0000 00000000 a8759f02` | 高(到 +0xe) 低(+0x10 起) |
| 0x0412 | C(/S) | 0x0410 的增量: u8 flags, u32 id, [f&1 u16 area], f&2 ? (u16 x,u16 y) : (s8 dx,s8 dy), [f&4 u8 dir] [f&8 u8 act] [f&0x10 u16 type] [f&0x20 u16 effect] [f&0x40 u32 +0x10] [f&0x80 u16,u16 +0x14] | 收 0x57540c..0x575536 (还原成 0x0410), 发 0x574a67; 真机每 60s: `00 00010000 00 00` | 高 |
| 0x0415 | S | u32 id → 移除对象 | 0x40ccb0 → 0x422740 ("CharOut 0x%x") | 中 |

客户端行为:
- 客户端自己寻路、本地走, 在世界状态 ([+0xe8]==3) 时用 SendMove (0x40d480) 发 0x0410; SASock 发送层把相对上次发送 (缓存 +0x62088) 的变化压成 0x0412
- 收到 S0x0410: 0x40cc30 → 0x429500。id==自己: 更新自己位置, area 变化 → 切图 (0x5090f0); id≠自己: 按 area 查地图表 0x6a75dc, 创建/更新该对象。并在 id==[+0x274] 时清 "待确认" 标记 → 服务器应回自己的 0x0410 作确认
- 收包缓存 (+0x6208c) 和发包缓存 (+0x62088) 分开; 服务器只发完整 0x0410, 不发 0x0412 (否则其他人的 0x0410 会污染接收增量基准)
- 可走性: 客户端本地判断 (mapdata tile + chipinfo, 见 sa2_maprev 文档), 服务器目前只校验 地图一致 + 坐标在 gridW×gridH 内, 越界回送上次确认位置作纠正

服务器实现 (internal/handler/move.go): 0x0410/0x0412 → 解压 → 校验 → 存库 → 回自己 0x0410 → 广播给同图其他会话 0x0410; 进世界时互发 0x0410 (出现), 断线广播 0x0415 (消失)。

未知/待真机确认: 客户端每走一步发一次还是只发终点; dir/act 取值; 别人的 type 字段需填形象 id 否则可能不显示 (现用 30002)。

## Movement ack fix (2026-10-11)
- Server must NOT echo a valid C 0x0410/0x0412 back to the mover. An S 0x0410 with id==self is a forced reposition (0x429500 -> [obj+0x1d4]) and cancels the client's path, so the walk stopped after 1 tile. ServerDummy (0x41c6c0) only replies 0x0410 on a warp tile (0x5ad1e0/0x4fd900). The server now replies to self only to correct an invalid position. Other players still get the broadcast.

## 0x0600 Chat (C->S SendChat 0x4163d0 vtbl+0x100, built at 0x49c9e7; S->C OnMessage 0x415fd0) — confidence: layout high, channel meaning medium
| off | type | field |
|---|---|---|
| 0x00 | u32 | speaker charId (high nibble checked as &0xf0000000 = system) |
| 0x04 | u16 | channel (client chat mode byte [ui+0x614]) |
| 0x06 | u32 | ARGB color = table 0x60b1bc[channel]: 0,1 white; 2 0xff9acd32; 3 0xff00ffff |
| 0x0A | char[0x20] | speaker name (copied from player+0x984) |
| 0x2A | u16 | text length |
| 0x2C | char[] | text, MBCS, NUL-terminated |
S 0x0600 has the same layout. The client shows it in the chat log, plus a speech bubble over the speaker found by id (vtbl+0x41c).
Server relay: ch1 goes to everyone (shout, guessed); other channels go to the same map (party/guild not implemented). There is no client whisper packet, so the server treats text `/w <name> <msg>` as a whisper.

## S 0x00E0 UI flags / HUD create (handler 0x40d2c0). Confidence: high for bit2
Payload: u32 flags.
- bit0: reset windows (0x566720, 0x539390).
- bit1: if game mode [0x68b86c]==5, scene refresh (0x500f20).
- bit2: create the HUD main window 0x1388 (if mode != 1; it holds the chat edit box, menus and status) via 0x53c640(…,0x1388,0x4000,…) then vtbl+0x204. If mode == 0, also create window 0x1387.
Without it, the field shows only the compass and minimap and Enter does nothing. The HUD 0x1388 is also what the 0x0230 UI dispatch (0x5547a5 → 0x553df0) and the chat code (0x49c960, edit text at +0x2b4) operate on.
Server: sent once per char login, on the first C 0x0900 (or C 0x0A03) after entering the world. The client sends 0x0A03 only sometimes, so the server cannot rely on it. While the scene is loading, 0x428d00 lets only 0x0900/0x0A03 through and queues everything else.

## HUD / chat button gating (2026-10-12; confirmed live with winedbg memory reads)
- The in-world HUD is the stone compass window 0x1388 (vtable 0x624938, created by S 0x00E0 bit2; live window list at [0x6a7850] holds 0x1388 and 0x1387). The client has no separate bottom bar.
- Compass controls (message map 0x6948a0, init 0x5e9770): 0x13881 opens menu window 0x13ba, 0x13883 opens 0x1f4a, 0x13884 opens 0x142a, 0x13885 opens the chat window 0x13c4 (handler 0x525f70).
- On S 0x0230, the UI dispatch (0x5547a5) calls 0x553df0(player+0x982 != 0): 0 disables control 0x13885 (vtbl+0x10c), 1 enables it (vtbl+0x114).
- player+0x982 comes from 0x0230 payload +0x40 (name +0x42 lands at player+0x984; live: player=0x29f70a8, +0x984="Tester", +0x982=0). The server now sends +0x40 = 1.

## ROOT CAUSE of "no chat / UI never reacts" (2026-10-12)
- `StoneAge2.exe` (and patched10..27) stubs the client UI-notify function **0x554700** with `ret` + NOPs (0x554700..0x55473f). patched9 instead NOPs the call sites.
- Every packet handler calls `0x554700(cmd, data)` to notify UI windows: 0x0230 enables HUD buttons, 0x0A00 sets the map-loaded flag 0x800, plus 0x0100/0x0710/0x0E10/0x0E0E... As a result no UI window ever reacts to server packets.
- Correction to the earlier note: 0x13881..0x1388b belong to the **Basic controls menu 0x13ba** (0x13881 → Status 0x13c4, 0x13882 → 0x13ce, 0x13883 → 0x13f6 ...), not to the compass. Live: the compass 0x1388 has a single child, a type-5 image 88x104.
- The chat sender 0x49c900 is a method of the generic edit-box control (vtable 0x60b3a8, control factory type 1 at 0x567c28).
- Fix: `Client/StoneAge2_ui.exe` = StoneAge2.exe with 0x554700..0x55473f restored from patched9 (original prologue). `tools/run_client_wine.sh` uses it by default (`SA2EXE=StoneAge2.exe` gives the old behaviour).

## Chat input under Wine — ROOT CAUSE + channels (2026-10-12, confirmed live)
- Not a server gate, not IME/DirectInput text. Keys reach the game: WndProc 0x4085b0 gets WM_KEYDOWN (→0x407fb0) and WM_CHAR (→0x408000 → focus->vtbl+0xe8). Text goes ONLY to the UI focus `[[[0x6a4dd0]+0xe4]+0xdc]` (SetFocus 0x429890).
- Edit control (vtable 0x6283e8, type 8) WM_CHAR = 0x53e5e0 (inserts chars; Enter sends via 0x49c900 → C 0x0600). IME result (WM_IME_COMPOSITION GCS_RESULTSTR) = 0x53ecc0; paste = 0x553cc0.
- Chat window 0x1392 (vtable 0x61ed08) has two edits: 0xc3501 (left, x≈15: whisper target name) and 0xc3502 (text, x≈159, y≈571 in UI coords; drawn at game ≈(155..505, 541..556)). Pressing Enter only focuses the window 0x1392 itself, whose vtbl+0xe8 is a no-op (0x5ad220) → chars dropped. Earlier tests clicked game x≈75 (= the name field) and/or used fast xdotool clicks.
- xdotool fast `click` is often missed: mouse is polled through DirectInput (TmnSystemWrapper/DINPUT8, coop level 6). Use mousedown, sleep 0.2, mouseup.
- Working method: hold-click game (300,548) [screen 865,890 with client origin 565,342] → type → Enter. Script: tools/chat_send.sh.
- Channels (C 0x0600 +0x04): 0 = All (white ffffffff), 3 = Party (cyan ff00ffff), 1 = Whisper (red ffff0000; +0x0A name field = RECIPIENT from the left box), Guild not sent without a guild (expect 2, yellow-green). S->C: name = speaker.

## S 0x0420 CharParam (2026-10-12, verified on real client)
Handler 0x40e8e0 → 0x420aa0. Payload: `u16 n`, then n × 12 bytes: `+0 u32 charId, +4 u16 apply (0 = skip), +6 u16 param, +8 s32 delta`.
- id == self ([0x6a74e8] vtbl+0x2c): player vtbl+0x228 (0x492e20) → table 0x68cd08[param] → player vtbl[0x22c+4*param](apply, delta). Every setter **adds** delta (no absolute set).
- player+0x942 = start of S 0x0230 payload. param → field: 1 u8 +0x04 level (also mirrors to [player+0x90c]+0x24, refreshes overhead "Lv.N"), 21 u32 +0x05, 2 u32 +0x09, 3 u32 +0x0d, 5 u16 +0x13, 4 u16 +0x11, 6..14 u16 +0x15..+0x25, 15..18 u8 +0x27..+0x2a, 19 u16 +0x2b, 20 u32 +0x32, 27 u8 +0x2d, 34/35 u16 +0x2e/+0x30, 29/31/30/32/33 u16 +0x36/+0x38/+0x3a/+0x3c/+0x3e. 0, 22..26, 28, 36+ are no-ops.
- id != self: 0x4cc170 (pets?) / 0x501480 (other chars) via the same vtbl+0x228.
- C 0x0420 (empty, "SendCharParam") → server replies n=0.
- "Lv.?" overhead was not a missing packet: it showed "Lv.3" (from 0x0230 +0x04) once the UI-notify fix was in; it appears on mouse hover over the character.
- Server: GM chat `/lv N` sends a level delta (internal/handler/charparam.go).
## S 0x0401 CharaInfo (not implemented)
0x40d5c0: u32 id; 0x5034c0(id, data+4) switches on id>>28 (object type 0..7); type-specific struct copied (e.g. 0x2f*4+1 bytes, name at +0x31). Needed for NPC/other objects.

## NPC talk / dialog (2026-10-12, verified on real client: talk -> page1 -> "Yes" -> page2 -> "Yes" -> close, re-talk OK)
- Spawn: S 0x0410 with id high nibble **1** = CNPC (0x427500 switch id>>28: 0 player, 1 CNPC 0x4d2130 "CNPC memory cannot be acquired", 2.., 7, 8). Then S 0x0441 (0x40d570): `u32 id` + 0x28-byte type-1 info (`+0 u32 id, +4 u32 ?, +8 char[0x20] name` → drawn yellow over head) + MBCS string. The client also asks C 0x0401 `u32 id` (SendInfo, on hover/appear) → answer with the same 0x0441. (Hovering yourself also sends C 0x0401 id=self.)
- Talk: right-click the NPC (within 6 tiles; player 0x4cd290, target kind vtbl+0x98 == 2) → **C 0x0480 SendEvent `u32 npcId`**. SendEvent sets an input lock ([0x6a4e0c] flag 0x20, 0x42a7a0). **Server must answer S 0x0480 `u32 id` (OnEventData 0x40d660 → 0x42a800 clears the lock)**, otherwise mouse/keyboard are dead while the dialog is up and no further 0x0480 is sent.
- S 0x0620 OnDLG (0x416c00): `+0 u32 charid (NPC; 0 → system dialog 0x4ae720, else 0x4d29f0), +4 u32 dlgId, +8 u32 buttons (bit mask, max 3; bit0 "Yes", bit1 "Accept"; labels table 0x60e1bc), +0xc u32 default button bit, +0x10 u16 disptime in seconds (×30 frames; **0 = time out at once**), +0x12 char[0x22] title (MBCS, shown as the name tab), +0x34 MBCS text\0`. Opening hides the chat window.
- C 0x0621 SendDlgRet: `u32 charid, u32 dlgId, u32 buttonBit`. On timeout the client sends `button=0` and then `button=1<<default`.
- S 0x0622 OnDLGEnd (0x413d40): `u32 charid, u32 dlgId` closes window (registry key 0x5708/0x56af) with +0x3c0 == dlgId.
- Wine note: ScreenToClient puts the client origin at (560,313), the picture is drawn at (565,342) → logical = drawn + (5,29). Drawn-coordinate clicks still hit buttons (rects are offset the same way).
- Server: internal/handler/npc.go (test NPC "Elder" 0x10000001 at Halbol 147,147; 2-page dialog).

## Battle (2026-10-12, 真机验证: /fight -> 指令 -> 结束回 Halbol)

触发: GM 聊天 `/fight` (internal/handler/battle.go)。流程 (S=服务端, C=客户端):

1. S 0x0B10 BattleInit (0x40d6f0 -> 0x4231b0 换场景 -> 回调 0x4fc9a0 -> 0x423300)
   - 头 0x0c: +0 u16 MAPCG_NO (战斗背景地图号; **不能等于当前已加载地图**, 否则 0x47a0d0 返回普通地图对象,
     0x475eb0 调 vtbl+0xf0 崩溃 "Runtime Error"; 用 4 = North Macca 可用), +3 u8 BattlemapInit Flag (1 = "Make a surprise attack!"),
     +4 s16 / +6 s16 镜头中心 (格), +0xa u8 我方 side, +0xb u8 cnt
   - cnt × 0x63 条目 (拷进对象 +0x90c, 0x4562d0 "BatInit ID,BID,HP,CND,SP"): +0 u32 id, +4 u8 side, +0x17 u32 HP, +0x1b u8 SP,
     +0x1d u16 CND, +0x21 u32 (maxHP?), +0x47 u32 owner, +0x4f u16, +0x51 s16 x, +0x53 s16 y, +0x55 u8 dir, +0x56 u8, +0x57 u16 CGNO (graphic/NNNNN.anz; 不存在 -> "File Not Found CGNOxxxx")
   - id==自己 -> 0xb24 对象 [0x6a6140]; side==我方: 玩家 0xb14 / 宠物 0xb18; 其它 -> 敌人 (0x4ad530)
2. C 0x0B01 (0x18 字节, 自己的 objPos) -> 服务端回 S 0x0BD1 (BATTLE_SYNC_CMD, 0x474bb0; 现在发 4 个 0)。指令菜单 + 30 秒倒计时出现。
3. Attack -> 敌人列表窗 -> 点敌人: C 0x0B20 SendBattleOrder 0x38: +0 u32 actor, +8 u8 act (3 = 攻击), +0xf u8 (5), +0x10 u32 target, +0x14 u8 1
   然后 C 0x0B90 SendBattleOrderEnter (u32 id, u32 0)。超时不选也会发 0x0B90。服务端回显 S 0x0B20 (OnBattleOrder 0x40ef80)。
4. S 0x0B30 OnBattleAction (0x40f030): u8 btlflag (0 BAF_NONE, 1 PETENTER, 2 SIM_ATK, 3..7 CAPTURE/CALLFRIEND/EARTH...),
   flag 0: u8 cnt + cnt × 0x4b (管理器 0x47dac3, 消息队列 [0x6a60a0]): +4 u32 ActID, +8 u8 Act (0x81 skill, 0x9a/0xa5/0xa6 特殊, 其它通用),
   +0xa u16 ActNum, +0x10 u16 ActEfe, +0x36 u32 PasID, +0x3a u8 Pas, +0x44 u16 CND, +0x46 u8 cap
   **国服 (2026-10-17 第 3 轮)**: 处理在 0x599ec0 (消息 0xb30, 0x423f50 入队)。ActID(+4)/PasID(+0x36) 用 map vtbl+0x58 查单位,
   键 = BattleInit 条目 +4 读成 u32, 它的最高字节就是 +7 "可下指令" → 角色 **0x01000000**、宠物 **0x01000200**、怪 1。
   以前发 0 / 0x200 查不到 → 角色和宠物的动作整条跳过 (不播动画, 0bd0 立刻回)。
   +0x3b s32 **PassHP** = 目标 HP 增量 (0x58bf50: 加到单位 +0x927, 夹在 0..maxHP), 发 -伤害; +0x3f u8 PassSP。
   +0x44 u16 **CND** = 目标动作后的状态 (条目同时拷进攻方 +0xa2c 和受方 +0xa5c, 0x58983b 写进单位 +0x92d):
   0 = 正常, **0x20 = 倒下** (0x58f36f ==0x20; 选目标时 test 0xe0 跳过), 0x40 另一种 (未用)。ServerDummy 的 0x1e37 国服不再发。
   宠物 CND 0x20: 播被击倒姿势后从战场消失, 聊天框出现客户端自己的 "<宠物名> ...." 提示, 之后回合只给角色下指令。
5. S 0x0BE0 OnBattleEnd (0x40d730 -> 0x424200 -> 0x47a930): +0 u8 EndFlag。**客户端只在本回合倒计时结束后处理**;
   早到的会被忽略, 所以服务端每次 C 0x0B90 都补发。flag&0xf!=0 无结算窗 (flag 1 = "All members have fainted... stay using EXP?" 败北提示);
   flag 0 = 胜利结算, 后接 0x92 字节/角色结构 (eTCMD_BATTLE_END_ CharaCnt/LevelCnt), 未实现。
6. C 0x0BF0 (u8 0) = 回到场景 -> 服务端发 S 0x0410 objPos (原地图/坐标) + 重新 spawnNpcs -> 回到 Halbol, 不崩。

其它: S 0x0BC0 TCMD_BATTLE_POS (0x40d140): u8 cnt + cnt × 0x1c {+0 u32 BatID, +0xa s16 X, +0xc s16 Y, +0x10 u16 CGNO};
S 0x0BD0 Synchronize (0x411400, u32); S 0x0B70/0B80 (ASSISTING/FORMATION) 未用。
已知问题: 战斗中角色/怪物精灵不显示 (只显示头顶 "思考" 气泡, 位置正确; 敌人列表能列出怪物), 所以攻击动画看不到; 原因未查明 (怀疑缺 0x0BC0/进场动作)。

### 战斗背景 (S 0x0b10 +0 MAPCG_NO) — 2026-10-12
- 与 SA1 相同: 战斗场地是独立小地图。mapdata 10000-13999 = 20~40 格见方小图 (共 ~460 张), 每 100 号一组, 组长 xx00 为 36x36。
- mapdata 头: +0xa u16 id, +0xc/+0xe w/h, **+0x14 u16 区域/图块集号**。场景图与战斗图共用该号 (Halbol 100 -> 区域 100 -> 10000..10017 组; 1000 段=103, 2000=124, 11000 段=121/123/138, 12000 段=143/149/155/161/165 ...)。尚未在客户端代码里查 场景->战斗 映射表 (待查); 目前按区域号由服务端选。
- 实现 battlebg.go: 启动时扫 mapdata, 按当前场景图区域号随机选一张; 无匹配回退 12000。单位/镜头坐标按图尺寸缩放 ServerDummy 的 36x36 布局。
- 实测 10010(24x24) 等多张正常, 无崩溃 (shots/battle_bg_10010.png, battle_bg_random_win.png)。
- 胜利: S 0x0be0 flag 0 + 0x92 块 (+2 为角色 ID) 有效, 回合计时结束后客户端处理, 显示 "has leveled up!" 并回场景 (shots/battle_win_levelup.png)。已知问题: 回场景后有一次地图显示为 North Macca 且看不到人物, 需重查回场景的 0x0410/地图重载。


### 战斗 2026-10-12 第三轮
- **S 0x0bd0 同步值必须每次不同** (客户端回显 C 0x0bd0 同值; 重复值时第 2 条 S 0x0b30 不播放并立刻回 bd0)。现用递增序号 -> 每回合玩家攻击 + 怪物反击都能播放 (shots/battle_monster_counter.png, battle_player_attack_2nd_action.png, battle_turn_sheet.png)。
- 动作 +0x44 CND=0x37 (ServerDummy 值) = 目标进入倒地/无法行动状态 -> 被打后下回合自动跳过指令。已去掉 (SA2_NOCND=1 默认)。
- 回合 30s: 客户端在指令阶段把 S 包排队 (0x47da34 循环: 0x475a70 忙检测; 0xbe0/0xbd1 在 [mgr+0x3cc]!=0 时视为忙)。C 0x0b90 后直接发动作/回显 S 0x0b90/改 S 0x0bd1 负载 都不能提前结束指令阶段; 未解决。已做: 致命一击后立即发 0x0be0 (省掉一次同步)。
- S 0x0BD1 = OnBattleOrderRequest (0x40eef0, 注册表 0x418c48)。C 0x0bf0 由失败弹窗 (0x425528, 回调 0x4ade20) 发送; 胜利不发任何包。
- **胜利后黑屏 / 小地图 "North Macca"**: namelist.bin 记录 = u16 id + UTF-16 名, id 1 = North Macca (地图 00001), 100 = Halbol。客户端回场景后地图显示为 id 1, 但上报的 C 0x0410 仍是 area 100。试验: 0x0b10 头 +8 改 100、胜利块 CharaCnt 改 2 均无变化 -> 不是这两个字段; 0x0be0 胜利块其余全 0, 未发现目标地图/事件字段。修复: 回场景后服务器发自己的 0x0410 (强制定位) 即恢复 (调试指令 /rt 2 验证; /rt 1 = S 0x0A03 无效)。服务器不知道何时回场景, 故 be0 后 2..45s 内多次发送 (fieldNudge)。shots/battle_win_return_halbol.png
- 战斗背景铺满: 只选 34~40 格的战斗图 + 镜头放图中心, 单位在镜头 ±4 格。shots/battle_bg_fill_10006.png
- 调试环境变量: SA2_MONHP SA2_HP SA2_BG SA2_ENDMODE SA2_FAST SA2_BD1 SA2_MONFIRST SA2_MONPAS SA2_HDR8 SA2_NONUDGE SA2_CNT SA2_CAMDX/DY SA2_NOCND SA2_SYNCV。
- 待办: 指令阶段提前结束; 场景 HP 回写 (0x0420 中 HP 的 param 号未知); 失败回场景本轮未复测。


### 战斗 2026-10-12 第四轮
- **回场景地图的真正来源 (非传送)**: OnBattleEnd 的淡出回调 0x4fc770 按 EndFlag 分派: 0 -> 0x4204f0(结算块), 1 -> 0x425410, 2 -> 0x420680 ...
  - 胜利结算块 +0x31 起是 CHAR_POS (0x18 字节: +0x35 u16 地图, +0x37/+0x39 x,y) -> 0x4c97f0 / 0x508ef0 载入地图。以前全 0 -> 载入地图 0 (空对象, +0x160 mapId=0), 小地图名回落 namelist 第一条 "North Macca" (namelist.bin: u16 id + UTF-16 名, 1=North Macca, 100=Halbol)。
  - 失败 (flag 1) 负载 = flag + CHAR_POS (+0 id, +4 u16 地图 -> 0x4c97f0)。以前只发 1 字节 -> 读到垃圾 -> "MapFileOpenError 13750/13814/13740"。
  - 现两者都填自己的 CHAR_POS: 胜利/失败都正确回到 Halbol, 不再需要重复发 0x0410 (已删除)。战斗会清掉场景 NPC, 收到回场景后的首个 C 0x0410 时补发 NPC。shots/battle_win_return_resultblock.png, battle_lose_return.png
- **HP**: 0x0b10 单位 +0x13 = 最大HP, +0x17 = 当前HP (0x45176f: cur>max -> "HP MAX ERROR")。场景 HP = 0x0230 +0x11 u16 (150), 最大 +0x13 (200) —— 客户端 0x54c156/0x54c163 成对读 player+0x953/+0x955。0x0420 param 4 改 +0x11 (实测 150 -> 140, player+0x953 内存确认)。战斗 HP 取自场景 HP, 结束后用 param 4 写回 (失败按 1)。
- **指令阶段 30s (未解决, 结论)**: 战斗包队列处理 0x47d9b0 的闸门: 状态 [map+0x3a8]==3 跳过; 0x473780 = 窗口 0x1f42 (指令菜单, 0x516xxx 类, bd1 时由 0x474c2d 创建) 存在就跳过。[+0x3cc] 只是 +0x3c4 处 std::list 的 size, 指令阶段实测为 0, 不是阻塞原因。选完指令后菜单不关闭, 只能等计时; 客户端没有 S 0x0b90 处理函数 (注册表 0x418c..)。S 0x0b20 (0x40ef80): +4 = BID (经 map vtbl+0x58 查单位, 再 vtbl+0x5e4), +8 act, +0xf, +0x14 目标。下一步: 看 0x516xxx 指令窗在什么条件下自毁 (可能要求宠物槽也下指令)。


### 战斗 2026-10-12 第五轮: 指令槽 / 宠物
- **指令阶段提前结束 = 下满指令槽**: 无宠物时角色每回合有 2 个指令槽 (同 SA1 无宠物时角色行动两次)。第 1 个指令后菜单重新出现 (同一单位), 第 2 个 C 0x0b20 + C 0x0b90 后客户端关闭指令窗 (0x5152e0 -> 0x474cf0 无下一单位 -> 窗口 slot 0x1d8 = 0x515110: 发 C 0x0b90 并销毁 0x1f42), 队列解冻, 立刻回 C 0x0bd0 -> 服务器结算。两次 C 0x0b20 负载相同 (+0 角色 ID, +8 act, +0x10 目标 ID)。实测第 2 指令后 0s 进入结算 (shots/battle_two_orders_early_resolve.png)。服务器现按收到的指令数 (最多 2) 打两下, 再怪物反击。
- 可下指令单位列表 = 战场图 +0x3f4 std::map (节点 +0x10 单位), 0x474cf0 用 [+0x424] 索引取下一个, 0x44c160 判断能否行动。实测只有 Tester。
- 0x0b10 宠物单位 (已修正):
  - **ID 类型高 nibble 必须为 2** (`0x2xxxxxxx`): 0x50a390 类型 2 分配 0x9c 缓冲; 类型 3 空操作 → +0x918 空指针崩。旧实验 `0x30000001` 已废弃。
  - 归属: 单位条目 **+0x4b = pet id** (解析循环里 ebx=entry+4, 所以 `[ebx+0x47]` = entry+0x4b)。`0x4ca600` 在 `player+0xaac` 向量里按 id 查找; 命中且 header mode≠8 → 创建 0xb18 己方宠对象。**不是** +0x47 owner 字段。
  - side 须与玩家 side (0x0b10 头 +0xa) 相同。`0x41f620(kind, mode)` 过滤进指令列表: kind==2 为宠。
  - 仅加单位未先发 0x0462 → 宠能画在场上但不进指令表 (shots/battle_pet_unit_not_owned.png)。
- 场景宠物 S **0x0462 OnPetInfo** (0x40ed10 → 0x4cf750 / 新建 0x4daee0, 头 0x9c → pet+0x918):
  - `/pet` GM + `petID=0x20000001` + gfx≠0 → 不崩; 随后默认 `/fight` (SA2_PET=0) OK。
  - 字段见下「SA1 对照」。相关: **C 0x0465 SendPetBattle** (1 字节 slot, 对齐 SA1 KS), SendPetOrder, S 0x1460/0x1461。
- **SA1 对照** (open-source SA1 server code) — 含义取 SA1, 尺寸/偏移取 SA2:

  | 含义 (SA1 / Pal) | SA1 | SA2 0x0462 |
  |---|---|---|
  | 栏位数 | MAX_PET / CHAR_MAXPETHAVE = 5 | +4 u16 slot 0..4 |
  | 形象 | graNo | +0x2e u16 CGNO |
  | 等级 | level | +0x30 u16 |
  | EXP / Next | exp / maxExp | +0x34 u32 / +0x38 u32 |
  | HP / MaxHP | hp / maxHp | +0x40 / +0x42 u16 (Pal 耐久力) |
  | 忠诚 | ai | +0x46 u16 (Pal 忠誠度) |
  | 四元素 | earth/water/fire/wind | +0x83..+0x86 u8 |
  | 技能数 | MAX_SKILL = 7 | +0x4c 起 **8**×u16 |
  | 名 / 种族 | name, freeName (GBK 文本包) | +0xc / +0x62 **MBCS**\0 (client strlen+MB2WC; UTF-16 truncates to 1 char) |
  | Age / DP | **无** (SA1 客户端 PET 无此字段; dp 是角色声望) | +0x3c / +0x3e u16 (SA2 Pal) |
  | 饥饿条 | 无 | +0x44? (Pal meat gauge) |
  | 攻击/防御/敏捷 | atk/def/quick 在 K 包明文 | Pal 有 攻撃/防御/回復/威力; SA2 精确偏移仍待定 (+0x48 候选) |
  | 出战选择 | KS / battlePetNo / PETST | **C 0x0465** SendPetBattle cnt=slot |
  | 协议外壳 | 1-byte cmd + CRC + `|` GBK | u16 cmd, 无 CRC, 二进制 UTF-16LE |

  协议差异大, 但宠物语义高度同构。战斗无宠 = 角色 2 指令槽 (两作相同)。
- 调试: SA2_PET/SA2_PETBID/SA2_PETGFX/SA2_PETLEN, GM 指令 /pet。


## Pets / petTurn (2026-10-12 evening)
- Pal UI: S 0x0462 MBCS name OK (`shots/pal_buddy_fixed.png`).
- `SA2_PET=1`: pet unit owned on field; 2nd C 0x0b20 actor=pet (0x20000001) after unique BID 0x200 + S 0x0466; root cause was BID collision (both side=0 → BID 0).
- Server `petTurn` verified live when orders≥2 && hasPet (`pet hits … bid=2`); screenshot `shots/battle_petturn_live.png` / earlier `battle_petturn_verified.png`.
- Reliable Attack click: `xdotool mousemove --window` (660,66) then enemy (400,120) → act=3 target=monster.
- Field reference: [FIELDS.md](FIELDS.md).

## Multi-pet build (2026-10-12)
Offsets are in FIELDS.md §2.
- DB table `pets(char_id, slot 0..4, name, species, gfx, level, exp, next_exp, age, dp, hp, max_hp, hunger(=Guts), loyalty, earth..wind, battle)` (`saac/pets.go`).
- Login: every pet goes out as S 0x0462, plus S 0x0466 for the battle pet. GM `/pet [enemyId]` fills the next free slot from the data tables (SA1 enemy/enemybase, Chinese name UTF-8 at +0xc/+0x62; 2026-10-17); the first pet becomes the battle pet automatically; a 6th gets "Pal slots full (5)".
- Battle-pet select: **C 0x1463** `{u32 petId, u32 role}` (0 = Battle, 1 = Rest) → DB + S 0x0466. C 0x0465 is only the count setting (logged).
  CN client: pet panel 任务 → 休息 (button 0x1388e, role 1) / 战斗 (0x1388f, role 0); the client flips 0x0462 +0x08 bit16 locally (`0x534f70`). The option matching the current state is greyed and sends nothing. Server keeps one battle pet and re-sends 0x0462 (+0x08 bit16 = Rest) for every pet whose role changed, so relogin and switching show the right 状态.
- `/fight [enemyId]`: monster from data/encount.txt for the current map; HP/atk from SA1 formulas; name at unit +0x27, level at +0x1c (2026-10-17). Only the battle pet goes in (BID 0x0200, HP from DB). Order: player C 0x0b20 → pet C 0x0b20 (actor = pet id) → monster, which picks player or pet (`SA2_MONTGT` 0 = random, 1 = pet, 2 = player). A KO'd pet skips its turn and gets no EXP.
- End: pet HP is saved (min 1) and S 0x0462 resent (no pet delta packet known; resending the full 0x0462 updates the Pal UI). Win: player +`SA2_WINEXP` (1000) via 0x0420 param 21; level-up when EXP ≥ 2000×Lv (param 1 +1, EXP reduced). Pet +`SA2_WINPETEXP` (6); level-up when EXP ≥ NextExp (10×Lv²): MaxHP +3, HP full.
- `SA2_PET` now defaults to 1 (`SA2_PET=0` disables pets in battle).

## Battle formation (0x0b10)
Unit +0x51/+0x53 are explicit tile x/y and +0x55 is dir; there is no slot or row field. The server lays out the formation itself (FIELDS.md §2.3): characters in the back row, each pet directly in front of its owner, wild monsters in the front row on the mirrored side, everyone in a line along x, 6 tiles apart. Distances scale with battle-map height. Verified with no pet and with pet + 3 monsters (`SA2_MONN=3`); every unit has its own tile.

## Pet/monster CGNO (2026-10-12)
Real creature sprites are `25xxx` (colour 0) to `28xxx` (colour 3), species = CGNO % 1000 (0..61). `29xxx` are giant bosses (old placeholders, removed). No client species table exists; the server picks the CGNO, and that one id drives the Pal portrait and the battle sprite. FIELDS.md §2.5.

### 多怪 (2026-10-17, 国服实测)
- S0B10 每只怪一个单元 (stride 0x65), id 0x60000001+i, bid i<<8|1; C0B20 +0x10 = 所点怪的 id (人和宠一样)。
- 宠物菜单第一项 AI = act 0x96, 目标填宠物自己 → 服务端当普通攻击, 打第一只活怪。
- 客户端在回合计时结束前可以重下指令 (又发 C0B20 #1), 以最后一次为准。

### 商店 / 物品 (2026-10-17, 国服实测)
- S 0x04a0: u32 flags (bit0 买 bit1 卖) + u16 文本长 + 文本。flags=3 → 买/卖/返回 三按钮。
- S 0x04a4 OnShopBuyList: u16 n + n×0x4c (+0 序号 u16, +2 物品号 u16, +8 价格 u32)。客户端随后对每个物品号发 C 0x0510。
- S 0x04a5 OnShopSaleList: u16 n + n×8 (u32 物品对象 id, u32 卖价)。
- C 0x04a7 SendShopBuy: u16 序号 + u16 物品号 (4 字节; 8 字节版另有 u32 数量)。S 0x04a8 结果 u16 非 0 = 成功。
- C 0x04a6 SendShopSell: u32 物品对象 id。成功后 S 0x04b1 删物品。
- S 0x04b0: u16 n + n×0x12 (+0 id=0x40000000|(charID&0xfffff)<<8|slot, +4 物品号, +6 图标 CGNO, +8 位置码, +0xa 选项 bit4(0x10)=已装备, +0x10 数量)。位置码: 1..0x12 装备页, 0x13 食物, 0x14 素材, 0x15 其他。
- C/S 0x0510: 请求 u16 物品号 (或 u32 id); 回复 u32 id + 0x14a 资料 (+0 物品号, +4 名字, +0xc6 说明)。图标不在 0x0510 里, 在 0x04b0 +6。
- 石币: S 0x0230 +0x32 / CharParam 20。

### 物品系统 (2026-10-17 第 4 轮, 国服实测)
- C 0x04b2 SendItemUse (0x410b20): u32 物品id + u32 目标id (角色/宠物)。客户端在把物品拖到角色/宠物身上时发送; 背包拖拽在当前版本没能启动 → UI 触发未实测, 服务端已实现 (GM /use 验证)。
- C 0x04b1 SendItemDrop: u32 物品id (丢弃)。服务端已实现, UI 触发未实测 (GM /drop 验证)。
- C 0x14b0 SendEquipOff: u32 物品id。装备窗口右键已装备物品 → 发送 (实测)。客户端会自己重算 总值=基础+Σ装备资料属性, 所以服务端此时不要再发 0x0420 增量 (否则重复扣)。
- S 0x04b1 (handler 0x410b70 OnItemUse): u32 物品id → 客户端删除该物品。用完/丢弃/移动旧 id 用它; 数量变化直接重发 0x04b0。
- 装备窗口位置码→格子 (表 0x6716c8): 1=头, 2=身体, 3..10=手/武器, 其他=饰品。背包分页 (表 0x6714a0): 1..0x12 装备, 0x13 食物, 0x14 素材, 0x15 其他。
- 战斗菜单 "道具" (1265,433) 打开战斗模式背包, 但没能选中物品, 没抓到 C 0x0b20 物品指令 → 战斗用道具未支持。
- 战斗胜利掉落: 每只怪按 items.txt drop‰ × knob dropmul 掉最多一个物品, 进背包 + 聊天提示 "得到了 X"。

## 物品使用 / 丢弃 / 战斗道具 (2026-10-17, 真机验证)
- 拖物品到人物头像: C04b2 {u32 物品ID, u32 目标ID}。拖到装备区: C04b2 目标 0 (0x51fb00)。
- 拖物品到宠物头像: **C0463** {u32 宠物ID, u32 子命令 8, u32 物品ID, u32 人物ID} (0x51efd0 → net vtbl+0x19c)。
- 拖到地面: 确认框 → C04b1 {u32 物品ID}。
- 背包没有移动/整理封包 (自动排序)。
- 战斗 C0B20 道具: +0 u32 行动者, +4 0x01000000, +8 act 0x9a, +9 u16 物品号, +0xb u32 物品ID, +0xf 3, +0x10 u32 目标, 后跟物品名 (UTF-8)。
- 战斗 S0B30 act 0x9a: +0xa ActNum = 特效 CGNO (0x58e870 → 0x42b5a0 读特效文件，缺文件弹 "File Not Found CGNO")；+0x3b PassHP = +回复。
  现有特效文件: 60000.ana, 60006, 60007, 60020, 60025, 60030, 60050–60056.anz。

## 战斗指令 / 结束 (2026-10-17 第 6 轮, 国服真机)
- C 0x0b20 +8 act: 0x00 等待, 0x03 攻击, 0x08 防御 (目标=自己, +0xf=1), 0x58 捕捉 (+0xf=5, +0x10 怪 BID 0x6000000x, 先弹"选择对象"列表), 0x99 逃跑, 0x9a 道具。其他 act 服务端记 log "未实现 → 跳过"。
- 逃跑成功: S0B30 (act 0x99) 后立刻 S0BE0 (flag 0, CharaCnt 0, 回场景 CHAR_POS)。S0BE0 不能等 C0BD0 动画结束后再发 (客户端不处理, 卡在战斗)。
- 战败: S0BE0 flag 1 + CHAR_POS → 回地图 + "队伍全军覆没"框; C 0x0bf0 u8 (1=OK 当场复活, 0=取消回原地) —— 服务端都只回地图, HP=1, 不扣经验。
- 捕捉 (国服): S0B30 flag 3 = `03` + 0x13 字节结构体 (无条数字节): +4 u32 ActID 捕手, +8 u8 ActKind 0x58, +0xd u32 PassID 怪, +0x11 u8 PassKind (0x5a 成功 / 0x57 失败), +0x12 u8 Turn 0。客户端自己演整套: 跑过去→"CAPTURE!"绳圈→成功: 怪转圈被拖回+"GET!"后消失; 失败: 绳断, 捕手走回。成功另发 S0462 新宠物 + 系统消息。
  注意: S0B30 线上 flag 0 才是 `flag,count,0x4b×n`; flag 1/2/3 后面直接是结构体 (解析 0x4112f0)。按 0x4b 发 flag 3 → 客户端 0x594652 崩溃。

## 暴击 / Miss (2026-10-17)
- 暴击: S0B30 ActEfe=0x60, CND\|=0x4, PassHP=-伤害 (log: efe=0x60 cnd=0x4)。
- Miss: S0B30 ActEfe=0x5c, CND\|=0x1, PassHP=0, ActNum=0 (log: efe=0x5c cnd=0x1)。
- 图标 CG 0x58a4/0x58a8 本机 graphic 包缺失 → 真机上看不到专用 Miss/暴击图, 字段已接通。

## 换图 / 传送点 (2026-10-18, 国服真机)
- 换图 = 服务器发自己的 S 0x0410 (id==自己, area=新图, x/y=落点)。客户端 0x429500 发现 area 变了 → 0x5090f0 读本地 mapdata/<area>.bin 换图, 不需要别的包。同图内 = 瞬移。
- 客户端没有传送点表 (event_w.bin 是 0xff 填充的事件旗标位图, config/warp.tsv 是传送石界面目的地名单, 无坐标)。
- 出入口格从自动寻路表推出: data/pth.bin (路径 = 路点序列) + pthinfo.bin (路点 mapid/mapx/mapy/mapflag/mapleave) + link.bin。
  三个文件都是简单流加密的 INI 文本, 解密 = tools/re/sa2txtdec.py (国服 0x4cbbf0 建两张 256 字节表, 0x4cbe70 逐字节 xor T1[i]^T2[j])。
  跨图的相邻路点: 前一个 = 本图出入口格; 后一个 = 新图的下一个路点 (不是落点)。mapflag=1 + mapleave (0x1xxxxxxx 对象 id) = 要点 NPC 的传送 (未做)。
- 客户端位置上报是稀疏的 (起步/拐弯/停下), 且走不到图边第 0 格 → 传送格用半径 1 判定。
- 服务器: data/warp.txt + internal/handler/warp.go。落点若也在传送格范围内, 要先离开再踩回才再触发。旧图其他人收 S 0x0415, 新图互发 0x0410。坐标/地图每次移动存库, 重登回到原图原位 (已验证: 下线在 10006/1, 上线就在那)。
- GM: /warp 地图 x y。

## 随机遇敌 (2026-10-18)
- 纯服务器逻辑, 无新包: 合法移动后按步数掷骰, 命中就 startFightN (同 /fight 随机)。客户端在走路中途收到 S 0x0b10 也能正常进战斗。
- 步数 = 两次位置上报的切比雪夫距离 (上报稀疏)。knob encpct (每步 %, 默认 4), encmin (遇敌后保底步数, 默认 8)。
- data/mapflags.txt: "MAP,safe" = 不遇敌 (哈鲁保村 100、院子 10000-10006)。遇敌表 encount.txt 新增地图 1 (马加岛北部, 先沿用新手区怪)。

## 宠物技能 (2026-10-18, 国服真机)
- S0462 +0x4c 8×u16 技能号。登录后客户端对每个技能号发 C0530 u16 no, 服务器回 S0530 (解析 0x502010, 存表 0x718820, 取 0x501740):
  +0 u16 no, +2 MBCS 名字 (0x21), **+0x23 u8 消耗气力** (菜单 "消耗气力 N", 客户端 0x528c47 用它和宠物气力比), **+0x24 u8 选目标方式** (0x567dc8: 0 = 敌方单体; 1 = 自身; 3/4/5/6 = 光标模式 7/8/9/10), +0x41 MBCS 说明 (0x48), +0x89 u8 标志 (0x55e781 test bit0, 未知), 共 0x8a。
  客户端 std::map 插入不覆盖: 同一次登录内改 S0530 内容无效, 要重登。
- 战斗指令 C0B20 (+0 = 宠物 id):
  - 目标方式 0: act **0x03** (同普通攻击), +0x10 目标, **+0x18 技能名 UTF-8** (0x20 字节); 名字空 = 普通攻击。
  - 目标方式 1: act **0x81**, **+9 u16 技能号**, +0x10 = 宠物自己, +0x18 名字。选完技能不用点目标。
  - 目标方式 3: 屏幕变暗进入我方选择, 但点角色/宠物都不发指令 (限时放弃)。
- 服务器: Mighty → S0B30 act 3 (伤害×倍数); Heal → S0B30 act 0x9a + ActEfe 0x71 + PassHP (道具加血通道, 演出一般)。扣宠物气力 (S0462 +0x4a) 后重发 S0462。

## 组队 (2026-10-18, 国服真机 + 脚本客户端)
协议号国服与欧版相同 (国服去掉了日志串, 用欧版 StoneAge2.exe 的串对照)。
- 界面: 底栏 "团队" 按钮 → 光标变邀请模式 → 点别的玩家 → 确认框 "确定要邀请XX加入队伍吗？" → OK。组队后再点 "团队" 打开队伍窗口 (委任队长 / 暂离队伍 / 退出队伍 / 请离队伍)。
- C 0x14e0 u32 目标id (邀请) → S 0x14e1 u32 邀请者id 给对方 (对方弹确认框; 忙时客户端自动回拒绝)。
- C 0x14e1 u32 邀请者id, u32 答复 (**0 = 同意**)。
- S 0x14e2 u32 被邀请者, u32 结果 → 邀请者 (**0 = 同意**; 非 0 = "拒绝组队申请了。")。
- S 0x04e1 u32 成员, u32 队长 → 同图所有人 (队长=0 → 组队失败提示)。之后客户端左侧出现队员头像, 队长头顶出队长标记, **队员由客户端画成跟在队长身后走** (服务器不用逐步推队员位置给队员自己)。
- S 0x04e0 u16 n (>=2), n×0x18 {u32 id, u16 map, s16 x, s16 y, u8 dir, u8 act, u16 形象, 0…} → 队员; 长度必须 n*0x18+2。
- C 0x04e2 u32 id (退出队伍; 真机发 **0 = 自己**) / C 0x14e3 u32 id (请离)。S 0x04e2 u32 队长, u32 离队者 → 同图 (相同 = 解散, 真机提示 "解散组队了。")。
- 服务器 (internal/handler/party.go): 队长走 → 队员权威位置排成一串 (第 k 个站到第 k-1 个的旧位置) 并告诉队伍外的人; 队长换图 (传送点 / GM /warp) → 队员一起换图并在新图重发 0x04e1/0x04e0; 断线自动离队; 离队/战斗回图后重发同图其他玩家的 0x0410 (否则客户端里他们消失)。
- 组队战斗 (partybattle.go): 队长开打时同图队员的角色和出战宠一起上场。阵型两排: 角色后排从左往右 (队长 slot 0), 宠物站主人正前方; 怪在对面前排。队员单位 BID 0x400+i*0x200 (宠物 +0x100), 条目 +7=0 → 服务器 AI 控制 (打随机活怪), 怪也会打队员。只有队长客户端进战斗画面。

## 其他玩家头顶名字 (2026-10-18)
- 客户端看到别的玩家后每 6 秒发 C 0x0401 u32 id (SendInfo) 直到收到回复。回 S 0x0401: u32 id + 0xbd 字节记录 (国服 0x4bba60 拷进 0x7186c4 表): +4 名字, +0x24 u8 等级 (组队邀请框读这两个), +0x31 MBCS (收包时转码)。现在 "walkB" 正常显示; 悬停时的 "Lv.0" 还没对上。

## 交易 (2026-10-18, 国服真机 + 脚本客户端)
- C 0x1600 u32 对方id → S 0x1600 u32 申请者id 给对方 (选项 S0800 +0 bit3 不开时客户端自动回拒绝)
- C 0x1601 u32 申请者id, u32 答复 (0=同意) → S 0x1601 u32 对方id, u32 结果 (0 开交易窗口) 给双方
- C 0x1602 u32 对方id: 撤回申请
- C 0x0690 u16 n, n×u32 id (最高位 = 石币 0x80000000|金额) → 原样 S 0x0690 给对方; 对方用 C 0x0510 查物品
- C 0x06a0 u32 对方id, u32 1 → S 0x06a0 {id,0} 给对方; 两边都确认 → 交换, S 0x06a4 {对方id, 1}
- C 0x0680 u32 对方id, u32 0 → S 0x0680 给对方 (关窗)
- S/C 0x0800 选项 0x8c 字节: +0 u32 bit0 组队 bit1 对战 bit2 名片 bit3 交易; +6/+7 音量
