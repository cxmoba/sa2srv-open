# 中文客户端 (SA2China) 数据文件格式

来源: 国服 (SA2China) 客户端安装目录的 data/。CN exe 构建时间 2010-12-27 (PE 时间戳),
原客户端 exe 为 2008-06-30。解析/导出: `tools/cn/dump_cn_data.py` (输出 `data_cn/`, 不随仓库发布),
加解密: `tools/cn/sa2crypt.py`。

## enemy.bin — 任务追踪用怪物名表 (高置信)
- 整个文件逐字节 XOR 0xFF (同 item.bin)。loader 0x5a9ef0: 打开 `enemy.bin` (UTF-16 名, 0x69aef8) -> 0x4236d0(0xff) 解码,
  记录数 = size / 0x10c (乘 0x7a44c6b>>3 即 /268)。查找 0x5aa020: 线性按 u32 id 匹配。
- 记录 0x10C (268) 字节, 共 1970 条 (id 0..5291, 4197 重复一次):

| 偏移 | 类型 | 含义 |
|---|---|---|
| +0x000 | u32 | 怪物/任务目标 id |
| +0x004 | wchar[128] | 名称 UTF-16LE, 0 结尾 (后面有残留垃圾, 忽略) |
| +0x104 | u32 | 基础种族号 (多数 = CGNO%1000; 亚种/+A 变体可不同) |
| +0x108 | u32 | CGNO (=(25+颜色)*1000+种族, 同 FIELDS.md) |

- **没有数值** (无 HP/攻防/等级/掉落/技能)。exe 用法: 0x5afb76/0x5afd16 把名字填入任务文本 `打败 %s (%d,%d)` /
  `捕捉 %s 打败 %s (%d,%d)`; 0x5b001e 用 +0x108 CGNO 画图标 (0x46ef80)。即任务/图鉴显示用的客户端表, 怪物数值仍在服务器端。
- id 分段: 0xxx 293, 1xxx 174, 2xxx 630, 3xxx 201, 4xxx 380, 5xxx 292 (5xxx 多为 "名字+A" 强化版)。654 个不重复名字。

## link.bin / pth.bin / pthinfo.bin — 自动寻路 (高置信: 格式; 中: 语义)
- 加密 INI 文本 (GBK/ASCII)。通用 loader 0x4cc6b0 (`data/%s.bin`), 解密 0x4cbe70, 表初始化 0x4cbbf0:
  `plain[k] = c[k] ^ A[k&0xff] ^ B[(k>>8)&0xff]`,
  A0=0x3C, A' = ((A*0x55&0xff)+0x68)*A+0xC9; B0=0x65, B' = ((B*0x6B+0x49)&0xff)*B-0x7F (全部 &0xff)。对称, 加密同函数。
  该 cipher 可能也用于其它 `data/*.bin` INI 类文件。
- link.ini: 65 个 `[linkN]` (N 如 525/526/531/53202 — 疑为目标地图/区域号), `amount=` + `pathK=<path id>`。
- pth.ini: 183 个 `[path<id>]`, id 形如 `<mapid><5位序号>` (10000001 = 地图100 #1, 1000600002 = 地图10006 #2), `pathinfoK=<idx>`。
- pthinfo.ini: 191 个 `[pathinfoN]`: mapid, mapx, mapy, mapflag (1 = 换图点/传送), mapleave (flag=1 时为打包值如 0x10064387, 含义未解)。
- 即: 地图间 "去某处" 的点序列, 客户端自动寻路 (PthInfo 调试串 `MX,MY(%d,%d)%d`)。服务器暂不需要; 可作为地图传送点坐标的参考数据。

## instdrd.bin — 随机迷宫生成数据 (中置信)
- 未加密。头: `IRD\0` u32 总长 u16 段数(6); 每段: tag[4] u32 段长(含 10 字节段头) u16 记录数 + 记录。
- INI 73×6B (u16 id 1950.., u16, u16) / CAT 5×6B / CHI 15×22B (id + 10 个地图块 chip 号) / OBJ 13×42B (id + 20 个物件图号 14200..) /
  WAL 8×31B / HIT 8×69B (碰撞掩码)。loader 0x4c5222。导出为 instdrd.json。用途推测: 副本/随机迷宫 (instance dungeon) 地形拼装。

## sa2ngword.bin — 屏蔽词表 (高置信)
- 未加密。`NGWD` u32 0x100 u16 2 u16 BOM(FEFF) u16 37(未知) 然后重复 [u16 字数][UTF-16 字]; 共 5621 个词, 绝大多数是韩文
  (韩服遗留), 中文仅约 27 个。导出 sa2ngword.txt。服务器可选用于聊天过滤。

## 其它差异
- 与原客户端同名但内容不同: item.bin (720000 -> 2583784 B, 待重新解析 CN 物品名), namelist.bin (6748 -> 17638), event_w.bin,
  mapposlist.bin, sa2gui.bin, fontwdat.bin; soundlist.bin 相同。原客户端独有 caution.bin。
- CN exe 新增: qq_chat/new_guild/compose_formula/itemdivide/luatimer 等界面数据, GameGuard + TmnSystemWrapper, version.xml 服务器列表
  (端口 13101 等)。**协议是否与原 exe 一致尚未验证** (未反汇编 CN 的收包分发/ServerDummy); 本服务器仍只针对原 exe/StoneAge2_ui.exe。
