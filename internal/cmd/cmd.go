// Package cmd 定义 SA2 命令码常量。
//
// SA2 客户端二进制逆向 (2026-08-20): 包 [u16le seq][u16le len][u16le cmd][payload].
// 命令码为 uint16 LE; 从 SASock callback 表提取, 精确语义待上线流量确认。
package cmd

// 客户端 -> 服务器
const (
	C_Login      uint16 = 0x0410 // 登录
	C_Register   uint16 = 0x0411 // 注册 (待客户端确认; 占位)
	C_Logout     uint16 = 0x0412 // 登出
	C_CharList   uint16 = 0x0498 // 角色列表
	C_CharSelect uint16 = 0x049C // 选角
	C_Ping       uint16 = 0x0800 // 心跳
	C_Move       uint16 = 0x0808 // 移动
	C_Chat       uint16 = 0x0818 // 聊天
	C_BattleCmd  uint16 = 0x0850 // 战斗指令
	C_ItemUse    uint16 = 0x02E8 // 物品使用
	C_NpcTalk    uint16 = 0x0B60 // NPC 对话
	C_PetCmd     uint16 = 0x0B1C // 宠物操作
	C_Map        uint16 = 0x0808 // 切图 (与 C_Move 相同 0x0808, 暂共用)
)

// 服务器 -> 客户端
const (
	S_LoginResult uint16 = 0x0402 // 登录结果 (u8 ok)
	S_Pong        uint16 = 0x0802 // 心跳响应
	S_RegResult   uint16 = 0x0404 // 注册结果 (u8 ok)
)

// ---- 2026-10-11 静态逆向确认 (StoneAge2.exe, 见 docs/PACKETS.md) ----
// 客户端 -> 服务器 (名字取自客户端调试字符串 SendXxx)
const (
	CV_Version    uint16 = 0x0010 // 连上后首包: char[8] 协议版本 "2.138" + char[8] 客户端版本 (0x415cbd)
	CV_Reconnect  uint16 = 0x0020 // 重连时代替 0x0010, 无负载 (0x415ccf)
	CV_Login      uint16 = 0x0100 // u32 ?, u8 loginType, u16 idLen, u16 pwLen, id, pw (OnVersion 0x41487d)
	CV_CharCreate uint16 = 0x0200 // u16 slot + 0x82 结构, 名字 MBCS @结构+0x32 (0x40d080)
	CV_CharDelete uint16 = 0x0210 // u16 idx  "SendCharDelete"
	CV_CharList   uint16 = 0x0220 // 无负载  "SendCharList"
	CV_CharLogin  uint16 = 0x0230 // u32 charId "SendLogin id=%d"
	CV_CharOut    uint16 = 0x0415 // u16 no   "SendCharOut"
	CV_Move       uint16 = 0x0410 // 0x18 位置结构 (ServerDummy 0x41c6c0 读 +6/+8 = x/y)
	CV_Chat       uint16 = 0x0600 // 0x2C 头 + MBCS 文本\0  SendChat 0x4163d0 (vtable+0x100), 调用 0x49ca4e
	CV_MoveDelta  uint16 = 0x0412 // 0x0410 的增量压缩形式 (SASock 0x574a67 发 / 0x575402 收)
	CV_CharParam  uint16 = 0x0420 // 无负载  "SendCharParam"
	CV_WorldTime  uint16 = 0x0900 // 无负载  "SendWorldTime"
	CV_Map        uint16 = 0x0A03 // 8 字节  "SendMap %d,%d"
	CV_PetBattle  uint16 = 0x0465 // 1 byte = player+0x9f4 battle-count setting ("SendPetBattle cnt=%d"), NOT a slot
	CV_PetOrder   uint16 = 0x0461 // u16 cnt + cnt*{u32 id,u16 order,u16 status} (Pal reorder)
	CV_PetRole    uint16 = 0x1463 // u32 petId, u32 role 0=Battle 1=Rest (Pal Role menu)
)

// 宠物 (2026-10-12)
const (
	SV_PetInfo       uint16 = 0x0462 // OnPetInfo 0x9c+ header
	SV_PartyPetInfo  uint16 = 0x1460
	SV_DelPartyPet   uint16 = 0x1461
	SV_PetBattleFlag uint16 = 0x0466 // sets pet/obj +0x91d bit0x20 (battle select ack)
)

// 服务器 -> 客户端 (名字取自客户端 OnXxx, 派发表在 0x417ba5 起注册, 145 项)
const (
	SV_Version    uint16 = 0x0010 // u32 res(0=OK), u32 seed, u16 codecType (0=明文)  OnVersion 0x414640
	SV_LoginCnt   uint16 = 0x0020 // OnLoginCnt 0x414600
	SV_Login      uint16 = 0x0100 // u32 result: 1/2=成功, 0/4=错误提示, 5=u16 len+自定义文本  OnLogin 0x415d40
	SV_CharCreate uint16 = 0x0200
	SV_CharDelete uint16 = 0x0210
	SV_CharList   uint16 = 0x0220 // u16 cnt + cnt*0xE3 字节角色条目  OnCharList 0x40d440 -> 0x428c90
	SV_CharLogin  uint16 = 0x0230 // 0xFD 字节角色数据  OnCharLogin 0x413410
	SV_ObjPos     uint16 = 0x0410 // 0x18 字节 自己/对象位置 (SASock 0x5753bd 缓存), 0x0412 = 增量
	SV_UIFlags    uint16 = 0x00E0 // u32 flags, bit2=创建 HUD 窗口 0x1388/0x1387  0x40d2c0
	SV_CharParam  uint16 = 0x0420 // u16 n + n*{u32 id, u16 apply(!=0), u16 param, s32 delta} 加到属性上  OnCharParam 0x40e8e0
	SV_Message    uint16 = 0x0600 // 同 C0x0600 布局  OnMessage 0x415fd0
	SV_CharOut    uint16 = 0x0415 // u32 id: 移除对象 (0x40ccb0 -> 0x422740 "CharOut 0x%x")
	SV_WorldTime  uint16 = 0x0900 // 10 字节  WorldTime 0x40d530
	SV_MapRet     uint16 = 0x0A03 // u16 ret, u16 mapId [, mapdata]  OnMap 0x40d770
	SV_Map        uint16 = 0x0A00 // "Map - (%d) Layer%d %s" 0x416290 (未解析)
)
