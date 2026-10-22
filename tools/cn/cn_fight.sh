#!/bin/sh
# 国服客户端: 聊天发 /fight, 角色+宠物各下一次 攻击 (菜单第 1 项 -> 点怪), 等战斗结束。
# 怪的位置随战斗背景变; 点歪了也没关系, 计时到点服务端照样推进。
export DISPLAY=${DISPLAY:-:1}
LOG=${LOG:-sa2srv.log}   # server log file
H(){ xdotool mousemove $1 $2; sleep .3; xdotool mousedown 1; sleep .25; xdotool mouseup 1; }
H 760 898; sleep .5; xdotool type --delay 120 "/fight"; xdotool keydown Return; sleep .12; xdotool keyup Return
sleep 9
H 1265 376; sleep 1.2; H 819 505; sleep 2
H 1265 376; sleep 1.2; H 819 505
for i in $(seq 1 12); do sleep 5; tail -40 "$LOG" | grep -q "battle end" && break; done
sleep 8
