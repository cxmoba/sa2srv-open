#!/usr/bin/env python3
"""两个会话同图: A 移动 (0x0410 / 0x0412 增量 / 越界), 检查 A 收到确认/纠正, B 收到广播, A 断开 B 收到 0x0415。
用法: move_test.py [port]"""
import socket, struct, sys, time
port = int(sys.argv[1]) if len(sys.argv) > 1 else 7778
def cks(b): return sum(((x - 256 if x > 127 else x) + (i & 3)) for i, x in enumerate(b)) & 0xffff
def pkt(cmd, pl=b""):
    n = 8 + len(pl); body = struct.pack("<HHH", n, n, cmd) + pl
    return body + struct.pack("<H", cks(body))
class C:
    def __init__(s, uid, pw):
        s.s = socket.create_connection(("127.0.0.1", port), timeout=2)
        s.tx(0x0411, struct.pack("<H", len(uid)) + uid + struct.pack("<H", len(pw)) + pw); s.rx()  # 旧注册命令, 已存在也无妨
        s.tx(0x0010, b"2.138\0\0\0" + b"1.000\0\0\0"); s.rx()
        s.tx(0x0100, struct.pack("<IBHH", 0, 0, len(uid), len(pw)) + uid + pw); assert s.rx()[1][:4] == b"\1\0\0\0"
        s.tx(0x0220); c, p = s.rx(); cnt = struct.unpack_from("<H", p)[0]
        if cnt == 0:
            body = bytearray(0x82); body[0x32:0x32 + len(uid)] = uid
            s.tx(0x0200, struct.pack("<H", 0) + bytes(body)); s.rx(); c, p = s.rx()
        s.id = struct.unpack_from("<I", p, 2 + 2)[0]
        s.tx(0x0230, struct.pack("<I", s.id))
        s.pos = None
        for _ in range(3):
            c, p = s.rx()
            if c == 0x0410: s.pos = p
    def tx(s, c, pl=b""): s.s.sendall(pkt(c, pl))
    def rx(s):
        h = b""
        while len(h) < 6: h += s.s.recv(6 - len(h))
        _, wl, c = struct.unpack("<HHH", h); r = b""
        while len(r) < wl - 6: r += s.s.recv(wl - 6 - len(r))
        return c, r[:-2]
    def xy(s, p): return struct.unpack_from("<Ihhh", p)
def show(tag, c, p): print(f"  {tag}: S0x{c:04x} id=0x{struct.unpack_from('<I',p)[0]:x} map/x/y={struct.unpack_from('<Hhh',p,4)}" if c == 0x410 else f"  {tag}: S0x{c:04x} {p.hex()}")

a = C(b"walkA", b"pw"); b = C(b"walkB", b"pw")
print("A id=0x%x pos=%s  B id=0x%x" % (a.id, a.xy(a.pos), b.id))
c, p = a.rx(); show("A sees B appear", c, p)
c, p = b.rx(); show("B sees A appear", c, p)
m, x, y = struct.unpack_from("<Hhh", a.pos, 4)
print("A full move -> (x+3,y)")
a.tx(0x0410, a.pos[:6] + struct.pack("<hh", x + 3, y) + a.pos[10:]); show("B bcast", *b.rx())
print("A delta move dx=-1 dy=+2 dir=2")
a.tx(0x0412, struct.pack("<BIbbB", 4, a.id, -1, 2, 2)); show("B bcast", *b.rx())
print("A out-of-map move (9999,9999)")
a.tx(0x0410, a.pos[:6] + struct.pack("<hh", 9999, 9999) + a.pos[10:]); show("A correction", *a.rx())
def chat(ch, t): return struct.pack("<IHI", a.id, ch, 0xffffffff) + b"walkA".ljust(32, b"\0") + struct.pack("<H", len(t)) + t + b"\0"
print("A says hello (ch0)"); a.tx(0x0600, chat(0, b"hello")); show("A echo", *a.rx()); show("B hears", *b.rx())
print("A whispers B"); a.tx(0x0600, chat(0, b"/w walkB psst")); show("B whisper", *b.rx()); show("A confirm", *a.rx())
print("A disconnects"); a.s.close(); show("B", *b.rx()); print("OK")
