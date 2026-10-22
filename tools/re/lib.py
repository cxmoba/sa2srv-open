import sys
import pefile, capstone, re, struct
import os
P=os.environ.get('SA2_EXE','../Client/StoneAge2.exe')  # path to your own client exe
pe=pefile.PE(P,fast_load=True); IB=pe.OPTIONAL_HEADER.ImageBase
img=pe.get_memory_mapped_image()
md=capstone.Cs(capstone.CS_ARCH_X86,capstone.CS_MODE_32); md.detail=False
TEXT=(IB+0x1000, IB+0x1000+0x1eede0)
def va2off(va): return va-IB
def find_str(s, wide=False):
    b=s.encode('utf-16le') if wide else s.encode()
    return [IB+m.start() for m in re.finditer(re.escape(b),img)]
def xrefs(va):
    b=struct.pack('<I',va); return [IB+m.start() for m in re.finditer(re.escape(b),img[0x1000:0x1000+0x1eede0])] and [IB+0x1000+m.start() for m in re.finditer(re.escape(b),img[0x1000:0x1000+0x1eede0])]
def dis(va,n=60):
    out=[]
    for i in md.disasm(img[va-IB:va-IB+n*8],va):
        out.append(f"{i.address:08x}: {i.mnemonic} {i.op_str}")
        if len(out)>=n: break
    return "\n".join(out)
def func_start(va):
    # scan back for common prologue / int3 padding
    o=va-IB
    while o>0x1000:
        if img[o-1] in (0xcc,0xc3) and img[o] in (0x55,0x6a,0x83,0x81,0x8b,0x53,0x56,0x57,0x64,0x51):
            return IB+o
        o-=1
