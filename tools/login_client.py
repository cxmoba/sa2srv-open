#!/usr/bin/env python3
"""Minimal SA2 test client: 0x0410 Login -> expects 0x0402 LoginResult."""
import socket, struct, sys
host, port = (sys.argv[1] if len(sys.argv)>1 else "127.0.0.1"), int(sys.argv[2]) if len(sys.argv)>2 else 8090
user, pwd = (sys.argv[3] if len(sys.argv)>3 else "test"), (sys.argv[4] if len(sys.argv)>4 else "1234")
def frame(seq, cmd, pl): return struct.pack("<HHH", seq, 6+len(pl), cmd) + pl
def recv(s):
    h=b""
    while len(h)<6: h+=s.recv(6-len(h))
    seq,ln,cmd=struct.unpack("<HHH",h); pl=b""
    while len(pl)<ln-6: pl+=s.recv(ln-6-len(pl))
    return seq,cmd,pl
s=socket.create_connection((host,port),timeout=3)
u,p=user.encode(),pwd.encode()
s.sendall(frame(1,0x0410,struct.pack("<H",len(u))+u+struct.pack("<H",len(p))+p))
seq,cmd,pl=recv(s); print(f"login  -> seq={seq} cmd=0x{cmd:04x} payload={pl.hex()}")
for c in sys.argv[5:]:
    s.sendall(frame(2,int(c,16),b"")); seq,cmd,pl=recv(s); print(f"0x{int(c,16):04x} -> cmd=0x{cmd:04x} payload={pl.hex()}")
