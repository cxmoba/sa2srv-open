"""Minimal static-RE helper for the CN client (StoneAge2_nogg.exe): disasm, wide-string xrefs, function start."""
import os, re, struct, pefile, capstone
P = os.environ.get('SA2_EXE', os.path.join(os.environ.get('SA2_CLIENT', '../Client'), 'StoneAge2_nogg.exe'))
pe = pefile.PE(P, fast_load=True); IB = pe.OPTIONAL_HEADER.ImageBase
img = pe.get_memory_mapped_image()
md = capstone.Cs(capstone.CS_ARCH_X86, capstone.CS_MODE_32)
TS, TE = 0x401000, 0x620000
code = img[TS-IB:TE-IB]
def dis(va, n=60):
    out = []
    for i in md.disasm(img[va-IB:va-IB+n*12], va):
        out.append(f"{i.address:08x}: {i.mnemonic} {i.op_str}")
        if len(out) >= n: break
    return "\n".join(out)
def xrefs(va): return [TS+m.start() for m in re.finditer(re.escape(struct.pack('<I', va)), code)]
def fstart(va):
    o = va-IB
    while o > 0x1000:
        if img[o-1] == 0xcc and img[o] != 0xcc: return IB+o
        o -= 1
def wstr(s):
    """all occurrences of wide string s -> (string start va, full string)"""
    r = []
    for m in re.finditer(re.escape(s.encode('utf-16-le')), img):
        o = m.start()
        while o > 1 and img[o-2:o] != b'\0\0' and m.start()-o < 200: o -= 2
        r.append((IB+o, img[o:o+200].decode('utf-16-le', 'replace').split('\0')[0]))
    return r
