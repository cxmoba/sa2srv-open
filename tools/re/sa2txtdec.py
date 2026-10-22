import sys
def tables():
    t1=[];a=0x3c
    for _ in range(256):
        t1.append(a); a=((((a*0x55)&0xff)+0x68)*a+0xc9)&0xff
    t2=[];d=0x65
    for _ in range(256):
        t2.append(d); al=((d*0x6b)&0xff); al=(al+0x49)&0xff; al=(al*d)&0xff; d=(al-0x7f)&0xff
    return t1,t2
def dec(b):
    t1,t2=tables(); i=j=0; o=bytearray(b)
    for n in range(len(o)):
        o[n]^=t1[i]^t2[j]; i+=1
        if i==256: i=0; j=(j+1)&0xff
    return bytes(o)
if __name__=='__main__':
    d=dec(open(sys.argv[1],'rb').read()); sys.stdout.buffer.write(d)
