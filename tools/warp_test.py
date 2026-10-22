#!/usr/bin/env python3
"""换图多人测试: A、B 同在哈鲁保村; A 踩洞口传送点 → A 收到自己 0x0410 (area 10006), B 收到 0x0415 (A 消失);
B 也 /warp 进 10006 → A 看到 B 出现。用法: warp_test.py [port]"""
import sys, struct
sys.path.insert(0, __import__("os").path.dirname(__file__))
from sa2client import C, pos
port = int(sys.argv[1]) if len(sys.argv) > 1 else 7777
a = C(b"walkA", b"pw", port); b = C(b"walkB", b"pw", port)
a.say("/warp 100 175 160"); b.say("/warp 100 172 162"); a.drain(1); b.drain(1)
a.pos = None; a.move(100, 175, 160); b.drain(0.5)
print("A steps on cave mouth (176,159)")
a.move(100, 176, 159)
p = a.wait(0x0410, pred=lambda p: pos(p)[0] == a.id); print("  A self pos:", p and pos(p))
q = b.wait(0x0415); print("  B sees A leave (0x0415):", q and q.hex())
assert p and pos(p)[1] == 10006 and q and struct.unpack_from("<I", q)[0] == a.id
print("B /warp 10006 4 17")
b.say("/warp 10006 4 17")
r = a.wait(0x0410, pred=lambda p: pos(p)[0] == b.id); print("  A sees B appear:", r and pos(r))
assert r and pos(r)[1] == 10006
print("OK")
