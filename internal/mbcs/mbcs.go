// Package mbcs — sa2srv 客户端文本编码。
//
// 服务端内部统一用 UTF-8 (Go string / DB)。线上字节按会话的客户端协议转换:
//   - CN (国服 StoneAge2_nogg.exe, 1.220): UnicodeToMbcs.dll 导出 SetAcp(ord3)/m2w(ord2)/w2u8(ord11)/u82w(ord12)。
//     聊天/名字/NPC/宠物文本走 w2u8/u82w → 线上是 UTF-8 (原样直通);
//     只有 S0100 自定义错误文本走 m2w + SetAcp(936) → GBK。2026-10-16 真机确认 (聊天框 UTF-8 正常、GBK 乱码;
//     登录错误框 GBK "密码错误" 正常)。
//   - UI (欧版 StoneAge2_ui.exe, 2.138): 原样字节直通 (ASCII 测试数据)。
//
// 转换必须发生在组帧 (含末尾 u16 cksum) 之前; 固定长度字段按编码后字节截断, 不拆半个字符 (Fit)。
// 兼容旧用法: SA2SRV_MBCS=gbk 时 Encode/Decode 全局走 GBK (现在 handler 按会话调用 EncodeAs/DecodeAs)。
package mbcs

import (
	"os"
	"strings"
	"unicode/utf8"

	"golang.org/x/text/encoding/simplifiedchinese"
	"golang.org/x/text/transform"
)

// Charset 线上文本编码。
type Charset int

const (
	Raw Charset = iota // 原样直通
	GBK                // CP936
)

// Target 旧全局开关 ("utf-8" 直通 / "gbk")。
var Target = targetFromEnv()

func targetFromEnv() string {
	if strings.EqualFold(os.Getenv("SA2SRV_MBCS"), "gbk") {
		return "gbk"
	}
	return "utf-8"
}

func global() Charset {
	if Target == "gbk" {
		return GBK
	}
	return Raw
}

// Encode / Decode 按全局 Target (兼容旧接口)。
func Encode(s string) []byte { return EncodeAs(global(), s) }
func Decode(b []byte) string { return DecodeAs(global(), b) }

// IsGBK 报告全局兜底是否生效。
func IsGBK() bool { return Target == "gbk" }

// EncodeAs 把 UTF-8 字符串转成线上字节。无法编码的字符替换成 '?'。
func EncodeAs(cs Charset, s string) []byte {
	if cs != GBK {
		return []byte(s)
	}
	enc := simplifiedchinese.GBK.NewEncoder()
	out := make([]byte, 0, len(s))
	for _, r := range s {
		if r < 0x80 {
			out = append(out, byte(r))
			continue
		}
		b, _, err := transform.Bytes(enc, []byte(string(r)))
		if err != nil || len(b) == 0 {
			out = append(out, '?')
			continue
		}
		out = append(out, b...)
	}
	return out
}

// DecodeAs 把线上字节转回 UTF-8。GBK 解码失败时退回原样 (若本来就是合法 UTF-8)。
func DecodeAs(cs Charset, b []byte) string {
	if cs != GBK {
		return string(b)
	}
	ascii := true
	for _, c := range b {
		if c >= 0x80 {
			ascii = false
			break
		}
	}
	if ascii {
		return string(b)
	}
	s, _, err := transform.String(simplifiedchinese.GBK.NewDecoder(), string(b))
	if err != nil || strings.ContainsRune(s, utf8.RuneError) {
		if utf8.Valid(b) {
			return string(b)
		}
	}
	return s
}

// Fit 把字符串编码后截到最多 max 字节 (不含终止 0), 不拆多字节字符。
func Fit(cs Charset, s string, max int) []byte {
	var out []byte
	for _, r := range s {
		b := EncodeAs(cs, string(r))
		if len(out)+len(b) > max {
			break
		}
		out = append(out, b...)
	}
	return out
}
