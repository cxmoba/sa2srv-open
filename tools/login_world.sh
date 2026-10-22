#!/bin/sh
# 重启客户端并自动进游戏 (Game Start -> SA2Local -> 选第 1 个角色 -> Start), 约 60s
export DISPLAY=${DISPLAY:-:1}
c(){ xdotool mousemove $1 $2; sleep .3; xdotool mousedown 1; sleep .2; xdotool mouseup 1; }
pkill -f "[S]toneAge2_ui.exe"; sleep 2
cd "$(dirname "$0")/../.." && (setsid bash sa2srv/tools/run_client_wine.sh > /tmp/client.log 2>&1 &)
sleep 18; c 958 696; sleep 5; c 760 392; sleep .15; c 760 392; sleep 6; c 692 497; sleep 1; c 721 793; sleep 12
