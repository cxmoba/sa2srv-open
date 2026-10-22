#!/usr/bin/env python3
"""Compare tile low-nibble flags vs JUMP cells vs JUM2 arrival cells per CN map."""
import struct, sys, collections, csv, os
MD=os.path.join(os.environ.get('SA2_CLIENT','../Client'),'mapdata')  # SA2_CLIENT = your client dir
def load(m):
    d=open(f'{MD}/{m:05d}.bin','rb').read()
    W,H=struct.unpack_from('<HH',d,0xC)
    tw,th=W//2,H//2
    flags={}; lowvals=collections.Counter()
    for ty in range(th):
        row=struct.unpack_from(f'<{tw}I',d,40+ty*tw*4)
        for tx,v in enumerate(row):
            lo=v&0xffff
            if lo: lowvals[lo]+=1
            n=lo&0xf
            for b in range(4):
                if n>>b&1: flags[(tx*2+(b&1),ty*2+(b>>1))]=lo
    return W,H,flags,lowvals
J=collections.defaultdict(dict); J2=collections.defaultdict(dict)
for r in csv.DictReader(open('data_cn/map_jump.csv')): J[int(r['map'])][(int(r['x']),int(r['y']))]=(int(r['to_map']),int(r['to_id']))
for r in csv.DictReader(open('data_cn/map_jum2.csv')): J2[int(r['map'])][(int(r['x']),int(r['y']))]=int(r['id'])
maps=[int(a) for a in sys.argv[1:]] or [1,2,3,4,5,6,10006,100,101]
for m in maps:
    if not os.path.exists(f'{MD}/{m:05d}.bin'): continue
    W,H,F,lv=load(m)
    js=J[m]; jx={c for c,(t,_) in js.items() if t!=m}; arr=set(J2[m])
    fs=set(F)
    print(f'map {m} {W}x{H}: flagged={len(fs)} JUMP(other map)={len(jx)} JUMP(same map)={len(js)-len(jx)} JUM2={len(arr)}')
    print(f'  flag&JUMP={len(fs&jx)} flag-only={len(fs-set(js))} JUMP-only={len(jx-fs)} flag&JUM2={len(fs&arr)} flag&sameJUMP={len(fs&(set(js)-jx))}')
    print('  low16 values:',lv.most_common(8))
    # by destination: flagged cells per toMap
    bt=collections.Counter(js[c][0] for c in fs&set(js)); print('  flagged JUMP by toMap:',dict(bt))
    # flag-only cells: nearest JUMP distance
    if fs-set(js):
        ds=collections.Counter()
        for c in fs-set(js):
            d=min((max(abs(c[0]-a),abs(c[1]-b)) for a,b in js),default=999); ds[min(d,20)]+=1
        print('  flag-only cells: Chebyshev dist to nearest JUMP:',sorted(ds.items()))
