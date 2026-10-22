from lib import *
import struct,re
out=[]
for m in re.finditer(rb'\xff[\x50-\x53]\x44',img[0x1000:0x1f0000]):
    a=IB+0x1000+m.start()
    ins=[x for x in md.disasm(img[a-IB-24:a-IB+80],a-24)]
    # resync: take window ending exactly at a
    k=[j for j,x in enumerate(ins) if x.address==a]
    if not k: 
        ins=[x for x in md.disasm(img[a-IB-23:a-IB+80],a-23)]; k=[j for j,x in enumerate(ins) if x.address==a]
        if not k: continue
    k=k[0]
    pushes=[p.op_str for p in ins[max(0,k-5):k] if p.mnemonic=='push']
    if len(pushes)<3 or not pushes[-1].startswith('0x'): continue
    cmd=int(pushes[-1],16)
    if cmd<0x100 or cmd>0x2000: continue
    fmt=''
    for p in ins[k:k+16]:
        if p.mnemonic=='push' and p.op_str.startswith('0x5f'):
            s=int(p.op_str,16); b=img[s-IB:s-IB+160]
            e=0
            while e+1<len(b) and b[e:e+2]!=b'\0\0': e+=2
            fmt=b[:e].decode('utf-16le','replace'); break
    out.append((cmd,a,pushes[-3],fmt))
for cmd,a,ln,fmt in sorted(out):
    print(f"cmd=0x{cmd:04x} @{a:08x} len={ln:>5} {fmt[:70]!r}")
