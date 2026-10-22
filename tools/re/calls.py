from lib import *
import struct,sys
tgt=int(sys.argv[1],16)
code=img[0x1000:0x1f0000]
res=[]
for i in range(len(code)-5):
    if code[i]==0xe8:
        rel=struct.unpack_from('<i',code,i+1)[0]
        a=IB+0x1000+i
        if a+5+rel==tgt: res.append(a)
for a in res:
    # disasm backwards window
    ins=list(md.disasm(img[a-IB-40:a-IB+5],a-40))
    ins=[x for x in ins if x.address<=a]
    pushes=[x.op_str for x in ins if x.mnemonic=='push'][-3:]
    print(hex(a),hex(func_start(a) or 0),pushes)
