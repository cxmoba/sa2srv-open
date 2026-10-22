#!/usr/bin/env python3
"""组队多人测试 (脚本客户端): A 邀请 B → B 同意 → 双方收 S04e1/S04e0; A 走 → 旁观者 C 看到 B 跟着挪;
A 踩洞口传送点 → B 也被带进 10006; B 离队 → S04e2。用法: party_test.py [port]"""
import sys, struct
sys.path.insert(0, __import__("os").path.dirname(__file__))
from sa2client import C, pos
port = int(sys.argv[1]) if len(sys.argv) > 1 else 7777
def u32s(p): return struct.unpack_from("<%dI" % (len(p) // 4), p)
def login(u):
    c = C(u, b"pw", port); return c
a, b, w = login(b"walkA"), login(b"walkB"), login(b"walkC")
for x, (cx, cy) in ((a, (170, 166)), (b, (172, 166)), (w, (168, 170))):
    x.say(f"/warp 100 {cx} {cy}")
for x in (a, b, w):
    for c, p in x.drain(1.5):
        if c == 0x0410 and pos(p)[0] == x.id: x.pos = p
a.tx(0x14e0, struct.pack("<I", b.id))
p = b.wait(0x14e1); print("B invited by", p and hex(u32s(p)[0])); assert p and u32s(p)[0] == a.id
b.tx(0x14e1, struct.pack("<II", a.id, 0))
r = a.wait(0x14e2); print("A S14e2", r and r.hex()); assert r and u32s(r)[1] == 0
j = a.wait(0x04e1); print("A S04e1 member/leader", j and [hex(v) for v in u32s(j)]); assert j and u32s(j) == (b.id, a.id)
l = a.wait(0x04e0); print("A S04e0 n =", l and struct.unpack_from("<H", l)[0]); assert l and len(l) == 2 + 2 * 0x18
w.drain(0.5)
a.move(100, 171, 166); a.move(100, 174, 163)
seen = [pos(p) for c, p in w.drain(1) if c == 0x0410 and pos(p)[0] == b.id]
print("C sees B follow:", seen); assert seen and seen[-1][2:] == (171, 166)
a.move(100, 175, 160); a.move(100, 176, 159)
pa = a.wait(0x0410, pred=lambda p: pos(p)[0] == a.id)
pb = b.wait(0x0410, pred=lambda p: pos(p)[0] == b.id)
print("A ->", pa and pos(pa), " B ->", pb and pos(pb)); assert pa and pb and pos(pb)[1] == 10006
b.tx(0x04e2, struct.pack("<I", b.id))
o = a.wait(0x04e2); print("A S04e2", o and [hex(v) for v in u32s(o)]); assert o
print("OK")
