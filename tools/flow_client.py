#!/usr/bin/env python3
"""模拟 StoneAge2.exe 的真实登录流程 (客户端帧格式, 见 docs/PACKETS.md):
C0x0010 version -> S0x0010 -> C0x0100 login -> S0x0100 -> C0x0220 -> S0x0220 -> C0x0230 -> S0x0230/0x0410/0x0900
用法: flow_client.py [host] [port] [id] [pw]"""
import socket, struct, sys
host = sys.argv[1] if len(sys.argv) > 1 else "127.0.0.1"
port = int(sys.argv[2]) if len(sys.argv) > 2 else 8090
uid = (sys.argv[3] if len(sys.argv) > 3 else "test").encode()
pw = (sys.argv[4] if len(sys.argv) > 4 else "1234").encode()

def cks(b): return sum(((x - 256 if x > 127 else x) + (i & 3)) for i, x in enumerate(b)) & 0xffff
def pkt(cmd, pl=b""):
    n = 8 + len(pl); body = struct.pack("<HHH", n, n, cmd) + pl
    return body + struct.pack("<H", cks(body))
def recv(s):
    h = b""
    while len(h) < 6: h += s.recv(6 - len(h))
    pl_, wl, cmd = struct.unpack("<HHH", h); rest = b""
    while len(rest) < wl - 6: rest += s.recv(wl - 6 - len(rest))
    return cmd, rest[:-2]

s = socket.create_connection((host, port), timeout=3)
def x(cmd, pl, n=1):
    s.sendall(pkt(cmd, pl)); out = []
    for _ in range(n):
        c, p = recv(s); out.append((c, p))
        print(f"  C0x{cmd:04x} -> S0x{c:04x} len={len(p)} {p[:24].hex()}{'...' if len(p) > 24 else ''}")
    return out
print("version:"); r = x(0x0010, b"2.138".ljust(8, b"\0") + b"1.000".ljust(8, b"\0"))
res, seed, typ = struct.unpack_from("<IIH", r[0][1]); print(f"  res={res} seed=0x{seed:08x} codec={typ}")
print("login:"); r = x(0x0100, struct.pack("<IBHH", 0, 0, len(uid), len(pw)) + uid + pw)
code = struct.unpack_from("<I", r[0][1])[0]; print("  result =", code, r[0][1][6:].decode(errors="replace") if code == 5 else "")
if code not in (1, 2): sys.exit(1)
print("charlist:"); r = x(0x0220, b"")
cnt = struct.unpack_from("<H", r[0][1])[0]
if cnt == 0:
    print("create char:")
    body = bytearray(0x82); nm = b"Tester"; body[0x32:0x32 + len(nm)] = nm
    r = x(0x0200, struct.pack("<H", 0) + bytes(body), n=2)
    print("  create result =", struct.unpack_from("<I", r[0][1])[0]); r = r[1:]
    cnt = struct.unpack_from("<H", r[0][1])[0]
e = r[0][1][2:2 + 0xe3]
cid = struct.unpack_from("<I", e, 2)[0]; print(f"  cnt={cnt} charId=0x{cid:x} name={e[0x36:0x46].split(b'\0')[0]}")
print("char login:"); x(0x0230, struct.pack("<I", cid), n=3)
print("map req:"); x(0x0a03, struct.pack("<II", 0, 0))
print("probe:"); x(0x0020, b"")
print("move:"); x(0x0410, struct.pack("<IHHH", cid, 100, 151, 152).ljust(0x18, b"\0"))
print("OK")
