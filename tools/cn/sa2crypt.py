"""SA2 CN client data/*.bin INI cipher (StoneAge2.exe CN 2010-12-27, 0x4cbbf0 / 0x4cbe70).
plain[k] = c[k] ^ A[i] ^ B[j];  i++ per byte, j++ each time i wraps 256."""
def tables():
    A=[];a=0x3c
    for _ in range(256):
        A.append(a); a=((((a*0x55)&0xff)+0x68)*a+0xc9)&0xff
    B=[];d=0x65
    for _ in range(256):
        B.append(d); d=(((d*0x6b+0x49)&0xff)*d-0x7f)&0xff
    return A,B
def decrypt(buf):
    A,B=tables();out=bytearray(len(buf))
    for k,c in enumerate(buf):
        out[k]=c^A[k&0xff]^B[(k>>8)&0xff]
    return bytes(out)
encrypt=decrypt
if __name__=='__main__':
    import sys;sys.stdout.buffer.write(decrypt(open(sys.argv[1],'rb').read()))
