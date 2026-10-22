#!/bin/bash
# /fight 后下两条 Attack 指令 (玩家, 宠物); 用窗口相对坐标 (见 FIELDS.md §2.4)
export DISPLAY=${DISPLAY:-:1}
WID=$(xdotool search --name StoneAge | head -1)
wc(){ xdotool mousemove --window $WID $1 $2; sleep .35; xdotool mousedown 1; sleep .22; xdotool mouseup 1; sleep .4; }
wc 660 66; sleep 1.2; wc 400 120; sleep 1.5
wc 660 ${PETATKY:-66}; sleep 1.2; wc 400 120
