#!/usr/bin/env python3
"""Decode remaining CN client data (see DATA_FORMATS_CN.md).
item.bin, namelist.bin, soundlist.bin, map/chipinfo.bin, mapdata/*.bin (header + JUMP/JUM2/EVEN), gui/*.bin (index only),
hit/*.hit (index only).  Usage: dump_cn_more.py [CN client root] [out dir]"""
import os, sys, struct, csv, json
ROOT = sys.argv[1] if len(sys.argv) > 1 else os.environ.get('SA2_CLIENT', '../Client')
OUT = sys.argv[2] if len(sys.argv) > 2 else os.path.join(os.path.dirname(os.path.abspath(__file__)), '../../data_cn')
def ws(b): return b.decode('utf-16-le', 'replace').split('\0')[0]
def rd(*p): return open(os.path.join(ROOT, *p), 'rb').read()
def wcsv(name, head, rows):
    with open(os.path.join(OUT, name), 'w', encoding='utf-8', newline='') as f:
        w = csv.writer(f); w.writerow(head); w.writerows(rows)
    return len(rows)

def item():
    """item.bin: XOR 0xFF, 0x128-byte records, UTF-16LE strings."""
    d = bytes(b ^ 0xff for b in rd('data', 'item.bin')); R = 0x128
    rows = []
    for k in range(len(d) // R):
        r = d[k*R:(k+1)*R]; H = lambda o: struct.unpack_from('<H', r, o)[0]; I = lambda o: struct.unpack_from('<i', r, o)[0]
        req = [(H(o), H(o+2)) for o in range(0x3a, 0x4e, 4)]
        mods = [(H(o+2), I(o+4)) for o in range(0x58, 0x90, 8)]
        rows.append(dict(no=H(0), name=ws(r[2:0x34]), loc=H(0x34), cat=H(0x36), x38=H(0x38),
                         req=[list(x) for x in req if x[0] != 0x53 or x[1]],
                         eff=H(0x50), eff_val=I(0x54), mods=[list(m) for m in mods if m[0] not in (0, 0x53)],
                         x90=I(0x90), x94=H(0x94), x96=H(0x96), desc=ws(r[0x98:0x11a]),
                         icon=H(0x11a), price=struct.unpack_from('<I', r, 0x11c)[0], x120=I(0x120), x124=I(0x124)))
    json.dump(rows, open(os.path.join(OUT, 'item.json'), 'w', encoding='utf-8'), ensure_ascii=False, indent=0)
    cols = ['no', 'name', 'loc', 'cat', 'x38', 'eff', 'eff_val', 'mods', 'req', 'icon', 'price', 'x120', 'x124', 'x94', 'x96', 'desc']
    return wcsv('item.csv', cols, [[json.dumps(x[c]) if c in ('mods', 'req') else x[c] for c in cols] for x in rows])

def namelist():
    d = rd('data', 'namelist.bin'); mg, sz, ver, n = struct.unpack_from('<4sIII', d)
    R = (len(d) - 16) // n
    return wcsv('namelist.csv', ['map', 'name'], [(struct.unpack_from('<H', d, 16+i*R)[0], ws(d[18+i*R:16+(i+1)*R])) for i in range(n)])

def soundlist():
    d = rd('data', 'soundlist.bin'); mg, sz, ver, n = struct.unpack_from('<4sIII', d); p = 16; rows = []
    for _ in range(n):
        gid, c = struct.unpack_from('<HH', d, p); p += 4
        rows.append((gid, c, ' '.join(str(x) for x in struct.unpack_from('<%dH' % c, d, p)))); p += 2*c
    return wcsv('soundlist.csv', ['group', 'n', 'sounds'], rows)

def chipinfo():
    d = rd('map', 'chipinfo.bin'); mg, sz, ver, hdr, n = struct.unpack_from('<4sIHHH', d)
    return wcsv('chipinfo.csv', ['tile', 'a', 'b', 'c', 'd'], [struct.unpack_from('<H4B', d, 14+6*i) for i in range(n)])

def secs(d):
    p = struct.unpack_from('<I', d, 4)[0]; out = {}
    while p + 8 <= len(d):
        tag = d[p:p+4]; ln = struct.unpack_from('<I', d, p+4)[0]
        if ln < 8: break
        out.setdefault(tag.decode('latin1'), d[p:p+ln]); p += ln
    return out

def mapdata():
    md = os.path.join(ROOT, 'mapdata'); hdrs = []; jump = []; jum2 = []; even = []
    for f in sorted(os.listdir(md)):
        d = open(os.path.join(md, f), 'rb').read()
        off, ver, mid, w, h, ts, area = struct.unpack_from('<IHHHHIH', d, 4)
        s = secs(d)
        hdrs.append((f, mid, w, h, area, ' '.join('%s:%d' % (k, len(v)) for k, v in s.items())))
        for tag, out, fmt, sz in (('JUMP', jump, '<4H', 8), ('JUM2', jum2, '<HHHIB', 11), ('EVEN', even, '<4H', 8)):
            if tag in s:
                v = s[tag]; n = struct.unpack_from('<I', v, 10)[0]
                out += [(mid,) + struct.unpack_from(fmt, v, 14+sz*i) for i in range(n)]
    wcsv('mapdata_index.csv', ['file', 'map', 'w', 'h', 'area', 'sections'], hdrs)
    wcsv('map_jump.csv', ['map', 'x', 'y', 'to_map', 'to_id'], jump)
    wcsv('map_jum2.csv', ['map', 'id', 'x', 'y', 'u32', 'dir'], jum2)
    wcsv('map_even.csv', ['map', 'x', 'y', 'v1', 'v2'], even)
    # resolved warps: exit cells grouped by (map,to_map,to_id) -> landing JUM2 of to_map with that id
    land = {}
    for m, i, x, y, u, dr in jum2: land.setdefault((m, i), (x, y, dr))
    grp = {}
    for m, x, y, tm, ti in jump:
        if ti and tm != m: grp.setdefault((m, tm, ti), []).append((x, y))
    rows = []
    for (m, tm, ti), cells in sorted(grp.items()):
        xs = [c[0] for c in cells]; ys = [c[1] for c in cells]; L = land.get((tm, ti))
        rows.append((m, min(xs), min(ys), max(xs), max(ys), len(cells), tm, ti) + (L if L else ('', '', '')))
    wcsv('map_warps_resolved.csv', ['map', 'x0', 'y0', 'x1', 'y1', 'cells', 'to_map', 'to_id', 'to_x', 'to_y', 'to_dir'], rows)
    return len(hdrs), len(jump), len(jum2), len(even), len(rows), sum(1 for r in rows if r[8] != '')

def gui():
    g = os.path.join(ROOT, 'data', 'gui'); rows = []
    for f in sorted(os.listdir(g)):
        d = open(os.path.join(g, f), 'rb').read(); rows.append((f[:-4], len(d), d[:4].decode('latin1').strip('\0'), struct.unpack_from('<I', d, 4)[0]))
    return wcsv('gui_index.csv', ['window', 'bytes', 'magic', 'ver'], rows)

def hit():
    h = os.path.join(ROOT, 'hit'); rows = []
    for f in sorted(os.listdir(h)):
        d = open(os.path.join(h, f), 'rb').read(); mg, sz, ver, hdr, n = struct.unpack_from('<4sIHHH', d)
        rows.append((f[:-4], len(d), n))
    return wcsv('hit_index.csv', ['gfx', 'bytes', 'n'], rows)

if __name__ == '__main__':
    for fn in (item, namelist, soundlist, chipinfo, mapdata, gui, hit): print(fn.__name__, fn())
