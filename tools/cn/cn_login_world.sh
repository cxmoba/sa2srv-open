#!/bin/sh
# 重启国服客户端 ($SA2_CLIENT, 需自备 run_cn_wine.sh 启动脚本) 并自动进游戏: GG 更新框 Cancel -> 输账号密码 -> 登录 -> 选服 -> 开始 -> 关新手教程。
# 坐标按 DISPLAY=:1 1920x1200、窗口居中 (游戏区左上约 559,311)。约 110s。账号/密码可用 $1 $2 覆盖。
export DISPLAY=${DISPLAY:-:1}
H(){ xdotool mousemove $1 $2; sleep .3; xdotool mousedown 1; sleep .25; xdotool mouseup 1; }
U=${1:-test}; P=${2:-1234}
pkill -f "[S]toneAge2_nogg.exe"; sleep 2
cd "${SA2_CLIENT:-../Client}" && (setsid bash run_cn_wine.sh "$U" "$P" 127.0.0.1:7777:SA2CN:ok > /tmp/cn_client.log 2>&1 &)
sleep ${WAIT:-45}
H 178 148; sleep 12                                   # nProtect GameGuard Update -> Cancel
H 955 694; sleep .5; xdotool type --delay 150 "$U"    # 用户名 (CN 登录框不吃 -u/-p)
H 955 731; sleep .5; xdotool type --delay 150 "$P"    # 密码 (客户端发 MD5 hex)
H 900 788; sleep 8                                    # 登录
H 797 403; sleep .15; H 797 403; sleep 10             # 选服 SA2CN
H 718 793; sleep 15                                   # 选角 开始
H 1253 427; sleep 2                                   # 关新手教程
