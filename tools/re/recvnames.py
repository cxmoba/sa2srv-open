from lib import *
import pickle,struct,re
pairs=pickle.load(open('pairs.pkl','rb'))
def wstr(s):
    b=img[s-IB:s-IB+200]; e=0
    while e+1<len(b) and b[e:e+2]!=b'\0\0': e+=2
    t=b[:e].decode('utf-16le','replace')
    if t and all(32<=ord(ch)<127 for ch in t): return t
    e=b.find(b'\0'); t=b[:e].decode('latin1')
    return t if t and all(32<=ord(ch)<127 for ch in t) and len(t)>3 else None
def firststr(fn,n=400):
    for i in md.disasm(img[fn-IB:fn-IB+n*6],fn):
        if i.mnemonic=='push' and i.op_str.startswith('0x5f') or i.mnemonic=='push' and i.op_str.startswith('0x6'):
            try:
                s=wstr(int(i.op_str,16))
                if s: return s
            except: pass
        if i.mnemonic=='ret': break
    return ''
for a,c,f in pairs:
    if func_start(a)!=0x417ba5 and not (0x417ba5<=a<0x419000): continue
    l=list(md.disasm(img[f-IB:f-IB+16],f))[:3]
    s=' ; '.join(i.mnemonic+' '+i.op_str for i in l)
    tgt=f
    if l and l[0].mnemonic=='jmp': tgt=int(l[0].op_str,16)
    print(f"0x{c:04x} {f:08x} {s[:50]:50} {firststr(tgt)[:60]!r}")
