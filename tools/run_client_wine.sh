#!/bin/sh
# SA2EXE 默认 StoneAge2_ui.exe = StoneAge2.exe 但恢复 UI 通知函数 0x554700 (patched10+ 把它改成 ret, 导致 HUD/聊天/状态窗收不到任何包)
# 在 Wine 下启动客户端并把本地 sa2srv 塞进服务器列表 (2026-10-11 逆向):
#   命令行 -u <ID> -p <PW> -a <ip:port:名字:备注> (-a 可重复; 值里不能含空格或 '-')
#   解析: 0x40b6xx 调 0x407c00 取 "-u"/"-p"/"-a"; -a 交给 0x4f5d40 按 ':' 切分 → 服务器条目
#   (+0x38 ip 串, +0x54 u16 port, +0x00 名字, +0x1c 备注), 选中后拷到 0x68b6e0/0x68b720 再连接
export DISPLAY=${DISPLAY:-:1} WINEPREFIX=${WINEPREFIX:-$HOME/.wine-sa2} WINEDEBUG=${WINEDEBUG:--all}
cd "$(dirname "$0")/../../Client" && exec wine "${SA2EXE:-StoneAge2_ui.exe}" -u "${1:-test}" -p "${2:-1234}" -a "${3:-127.0.0.1:7777:SA2Local:ok}"
