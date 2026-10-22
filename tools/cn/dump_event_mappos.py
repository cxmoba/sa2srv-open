#!/usr/bin/env python3
"""Decode CN client data/event_w.bin (任务表) and data/mapposlist.bin (世界地图位置表).
Usage: dump_event_mappos.py [CN data dir] [out dir]   (defaults: $SA2_CLIENT/data, ../../data_cn)
Formats: see DATA_FORMATS_CN.md.  event_w.bin: whole file XOR 0xFF (client 0x4f85c0 -> 0x4236d0(0xff)),
959 records x 0x6f0 bytes, UTF-16LE strings.  mapposlist.bin: plain, 'MAPL' header + 12-byte records (0x4c0f30)."""
import os, sys, struct, csv, json
SRC = sys.argv[1] if len(sys.argv) > 1 else os.path.join(os.environ.get('SA2_CLIENT', '../Client'), 'data')
OUT = sys.argv[2] if len(sys.argv) > 2 else os.path.join(os.path.dirname(os.path.abspath(__file__)), '../../data_cn')
R = 0x6f0
def ws(b): return b.decode('utf-16-le', 'replace').split('\0')[0]
def i32(r, o): return struct.unpack_from('<i', r, o)[0]

def event():
    e = bytes(b ^ 0xff for b in open(os.path.join(SRC, 'event_w.bin'), 'rb').read())
    assert len(e) % R == 0
    rows = []
    for k in range(len(e) // R):
        r = e[k*R:(k+1)*R]
        rows.append(dict(
            rec=k, id=i32(r, 0), title=ws(r[4:0x104]),
            npc_gfx=i32(r, 0x104), npc=ws(r[0x108:0x20c]), npc_x=i32(r, 0x20c), npc_y=i32(r, 0x210),
            text=ws(r[0x214:0x314]), goal=ws(r[0x314:0x414]),
            cond_var=i32(r, 0x414), cond_op=ws(r[0x418:0x420]), cond_val=i32(r, 0x420),
            cond2_a=i32(r, 0x424), cond2_op=ws(r[0x428:0x430]), cond2_b=i32(r, 0x430),
            req=[[i32(r, o), i32(r, o+4), i32(r, o+8)] for o in range(0x434, 0x494, 12) if any(i32(r, o+j) for j in (0, 4, 8))],
            x494=i32(r, 0x494), x498=i32(r, 0x498),
            x49c=i32(r, 0x49c), x4a0=i32(r, 0x4a0), x4a4=i32(r, 0x4a4),
            rew=[i32(r, o) for o in range(0x4a8, 0x4e8, 4)],
            start_note=ws(r[0x4e8:0x5e8]), end_note=ws(r[0x5ec:0x6ec]), x6ec=i32(r, 0x6ec)))
    with open(os.path.join(OUT, 'event_w.json'), 'w', encoding='utf-8') as f:
        json.dump(rows, f, ensure_ascii=False, indent=1)
    cols = ['rec', 'id', 'title', 'npc_gfx', 'npc', 'npc_x', 'npc_y', 'cond_var', 'cond_op', 'cond_val', 'req', 'x49c', 'x4a0', 'x4a4', 'goal', 'start_note', 'end_note']
    with open(os.path.join(OUT, 'event_w.csv'), 'w', encoding='utf-8', newline='') as f:
        w = csv.writer(f); w.writerow(cols)
        for x in rows: w.writerow([json.dumps(x[c], ensure_ascii=False) if c == 'req' else x[c] for c in cols])
    return len(rows), len({x['id'] for x in rows})

def mappos():
    d = open(os.path.join(SRC, 'mapposlist.bin'), 'rb').read()
    magic, size, ver, n = struct.unpack_from('<4sIII', d)
    assert magic == b'MAPL' and size == len(d) and 16 + 12*n == len(d)
    with open(os.path.join(OUT, 'mapposlist.csv'), 'w', newline='') as f:
        w = csv.writer(f); w.writerow(['id', 'parent', 'x', 'y', 'w', 'h'])
        for i in range(n): w.writerow(struct.unpack_from('<6H', d, 16 + 12*i))
    return n

if __name__ == '__main__':
    print('event_w records/ids', event()); print('mapposlist records', mappos())
