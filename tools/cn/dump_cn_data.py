#!/usr/bin/env python3
"""Dump SA2 CN client data files (enemy/link/pth/pthinfo/instdrd/sa2ngword) to sa2srv/data_cn/.
Usage: dump_cn_data.py [cn_data_dir] [out_dir]   See sa2srv/DATAFILES_CN.md."""
import sys,os,struct,csv,json
sys.path.insert(0,os.path.dirname(__file__))
from sa2crypt import decrypt
src=sys.argv[1] if len(sys.argv)>1 else os.path.join(os.environ.get('SA2_CLIENT','../Client'),'data')
out=sys.argv[2] if len(sys.argv)>2 else os.path.join(os.path.dirname(__file__),'../../data_cn')
os.makedirs(out,exist_ok=True)
P=lambda f:os.path.join(src,f)

def enemy():
    d=bytes(b^0xff for b in open(P('enemy.bin'),'rb').read()); R=0x10c; rows=[]
    for k in range(len(d)//R):
        r=d[k*R:(k+1)*R]; eid,=struct.unpack_from('<I',r,0)
        name=r[4:0x104].decode('utf-16le','replace').split('\x00')[0]
        fam,cg=struct.unpack_from('<II',r,0x104)
        rows.append((eid,name,fam,cg,cg//1000-25 if 25000<=cg<29000 else '',cg%1000))
    with open(os.path.join(out,'enemy.csv'),'w',newline='',encoding='utf-8') as f:
        w=csv.writer(f); w.writerow(['id','name','base_species','cgno','color','species_from_cg']); w.writerows(rows)
    return len(rows)

def ini(fn):
    t=decrypt(open(P(fn),'rb').read()).decode('gbk','replace')
    open(os.path.join(out,fn.replace('.bin','.ini')),'w',encoding='utf-8').write(t)
    return t.count('\n[')+t.startswith('[')

def instdrd():
    d=open(P('instdrd.bin'),'rb').read(); assert d[:4]==b'IRD\0'
    total,nsec=struct.unpack_from('<IH',d,4); o=10; res={}
    for _ in range(nsec):
        tag=d[o:o+4].rstrip(b'\0').decode(); size,=struct.unpack_from('<I',d,o+4); cnt,=struct.unpack_from('<H',d,o+8)
        body=d[o+10:o+size]; rs=len(body)//cnt
        res[tag]={'count':cnt,'recsize':rs,'rows':[list(struct.unpack_from('<'+'H'*(rs//2),body,k*rs)) for k in range(cnt)] if rs%2==0 else body.hex()}
        o+=size
    json.dump(res,open(os.path.join(out,'instdrd.json'),'w'),indent=0)
    return {k:v['count'] for k,v in res.items()}

def ngword():
    w=open(P('sa2ngword.bin'),'rb').read(); assert w[:4]==b'NGWD'
    o=14; g=[]
    while o<len(w):
        l,=struct.unpack_from('<H',w,o); g.append(w[o+2:o+2+2*l].decode('utf-16le','replace')); o+=2+2*l
    open(os.path.join(out,'sa2ngword.txt'),'w',encoding='utf-8').write('\n'.join(g)+'\n')
    return len(g)

print('enemy',enemy())
for f in ['link.bin','pth.bin','pthinfo.bin']: print(f,ini(f))
print('instdrd',instdrd()); print('ngword',ngword())
