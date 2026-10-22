#!/bin/sh
# 在 DISPLAY=:1 的 Wine 客户端里发一条聊天 (2026-10-12 验证可用)
# 用法: tools/chat_send.sh <文本> [tab: all|party|guild|whisper] [whisper 收件人]
# 关键: 客户端用 DirectInput 轮询鼠标, xdotool 快速 click 常被漏掉 -> 必须 mousedown/sleep/mouseup。
# 坐标按客户端区原点 565,342 (xdotool getwindowgeometry), 游戏内坐标 = 屏幕 - 原点。
export DISPLAY=${DISPLAY:-:1}
OX=${OX:-565}; OY=${OY:-342}
c(){ xdotool mousemove $(($OX+$1)) $(($OY+$2)); sleep .3; xdotool mousedown 1; sleep .2; xdotool mouseup 1; sleep .3; }
case "${2:-all}" in all) c 43 528;; party) c 115 528;; guild) c 187 528;; whisper) c 260 528;; esac
if [ -n "$3" ]; then c 75 548; xdotool type --delay 120 "$3"; fi
c 300 548                      # 文本输入框 edit 0xc3502 (窗口 0x1392), 不是左侧的 0xc3501
xdotool type --delay 120 "$1"
xdotool keydown Return; sleep .12; xdotool keyup Return
