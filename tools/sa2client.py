#!/usr/bin/env python3
"""脚本客户端 (欧版/UI 协议, 不加密), 给多人测试用: 登录→选角→进世界, 收发包, 聊天/GM 指令。
from sa2client import C; a = C(b"walkA", b"pw", port)"""
import socket, struct, time
def cks(b): return sum(((x - 256 if x > 127 else x) + (i & 3)) for i, x in enumerate(b)) & 0xffff
def pkt(cmd, pl=b""):
    n = 8 + len(pl); body = struct.pack("<HHH", n, n, cmd) + pl
    return body + struct.pack("<H", cks(body))
class C:
    def __init__(s, uid, pw, port=7777):
        s.uid = uid
        s.s = socket.create_connection(("127.0.0.1", port), timeout=2)
        s.tx(0x0411, struct.pack("<H", len(uid)) + uid + struct.pack("<H", len(pw)) + pw); s.rx()
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
            if c == 0x0410 and struct.unpack_from("<I", p)[0] == s.id: s.pos = p
    def tx(s, c, pl=b""): s.s.sendall(pkt(c, pl))
    def rx(s, timeout=2):
        s.s.settimeout(timeout); h = b""
        while len(h) < 6: h += s.s.recv(6 - len(h))
        _, wl, c = struct.unpack("<HHH", h); r = b""
        while len(r) < wl - 6: r += s.s.recv(wl - 6 - len(r))
        return c, r[:-2]
    def drain(s, t=0.5):
        out = []; end = time.time() + t
        while time.time() < end:
            try: out.append(s.rx(max(0.05, end - time.time())))
            except (socket.timeout, TimeoutError): break
        return out
    def wait(s, cmd, t=3, pred=None):
        end = time.time() + t
        while time.time() < end:
            try: c, p = s.rx(max(0.05, end - time.time()))
            except (socket.timeout, TimeoutError): break
            if c == cmd and (pred is None or pred(p)): return p
        return None
    def say(s, t):
        if isinstance(t, str): t = t.encode()
        s.tx(0x0600, struct.pack("<IHI", s.id, 0, 0xffffffff) + s.uid.ljust(32, b"\0") + struct.pack("<H", len(t)) + t + b"\0")
    def move(s, m, x, y):
        base = s.pos or (struct.pack("<IHhh", s.id, m, x, y) + bytes(14))
        s.pos = struct.pack("<IHhh", s.id, m, x, y) + base[10:]
        s.tx(0x0410, s.pos)
def pos(p): return struct.unpack_from("<IHhh", p)
