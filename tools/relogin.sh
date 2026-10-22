#!/bin/bash
# 重启客户端并登录进世界 (DISPLAY=:1)
export DISPLAY=${DISPLAY:-:1}
c(){ xdotool mousemove $1 $2; sleep .3; xdotool mousedown 1; sleep .2; xdotool mouseup 1; }
pkill -9 -f '[S]toneAge2_ui' 2>/dev/null; sleep 2
cd "$(dirname "$0")/../.." && (setsid bash sa2srv/tools/run_client_wine.sh > /tmp/client.log 2>&1 &)
sleep 18
c 958 696; sleep 5; c 760 392; sleep .15; c 760 392; sleep 6; c 692 497; sleep 1; c 721 793; sleep 10
