#!/usr/bin/env python3
"""从 SA1 gmsv 原始数据 (GBK) 生成 sa2srv 的 data/ 表 (UTF-8, 列格式不变)。

用法: python3 tools/sa1/import_sa1.py [SA1_DATA_DIR] [OUT_DIR]
  SA1_DATA_DIR 默认 $SA1_DATA 或 ../sa1/gmsv/data (enemybase1.txt / enemy1.txt / encount.txt / group1.txt)
  OUT_DIR      默认 data

映射规则 (SA1 种族与 SA2 不一一对应):
  1. SA1 enemybase 名字与国服 enemy.bin (data_cn/enemy.csv) 名字完全相同 -> 取该名字的 CGNO
     (同名多条时: id<1000 的原始种族段优先, 再取最小 id)。
  2. 名字对不上的种族不导出 (SA1 IMGNUMBER 是 SA1 图号, 在 SA2 里没有对应图)。
  3. enemy1 只保留 TEMPNO 被导出的实例行; encount 只取 SA1 floor 100 (法兰城外/新手区, Lv1-6) 的区域,
     按 区域出现率 x 组内概率 展开成 SA2 map 100 的 enemy 权重 (SA2 map 100 = 哈尔波 新手村);
     floor 100 里也有 Lv10-20 的区域, 只保留 LV_MAX <= MAXLV (默认 6) 的实例。
"""
import csv, os, sys, collections

src = sys.argv[1] if len(sys.argv) > 1 else os.environ.get('SA1_DATA', '../sa1/gmsv/data')
out = sys.argv[2] if len(sys.argv) > 2 else 'data'
here = os.path.dirname(os.path.abspath(__file__))
MAXLV = int(os.environ.get('MAXLV', '6'))
cn_path = os.path.join(here, '..', '..', 'data_cn', 'enemy.csv')

def rd(name):
    with open(os.path.join(src, name), 'rb') as f:
        for raw in f:
            try:
                l = raw.decode('utf-8')
            except UnicodeDecodeError:
                l = raw.decode('gbk', errors='replace')
            l = l.rstrip('\r\n')
            if not l.strip() or l.startswith('#'):
                continue
            yield l

def num(s, d=0):
    s = s.strip()
    try:
        return int(float(s)) if s else d
    except ValueError:
        return d

cn = collections.defaultdict(list)
for r in csv.DictReader(open(cn_path, encoding='utf-8')):
    cn[r['name'].strip()].append((int(r['id']) >= 1000, int(r['id']), int(r['cgno'])))
cnpick = {n: sorted(v)[0] for n, v in cn.items()}

bases, sa2map = [], []
temps = set()
total = 0
for l in rd('enemybase1.txt'):
    f = l.split(',')
    if len(f) < 37:
        continue
    total += 1
    name, temp = f[0].strip(), num(f[6], -1)
    p = cnpick.get(name)
    if not p or not (25000 <= p[2] < 29000):
        continue
    bases.append(l)
    temps.add(temp)
    sa2map.append(f'{temp},{p[2]},{p[1]}')

enemies, eids = [], {}
for l in rd('enemy1.txt'):
    f = l.split(',')
    st = 3 if len(f) >= 34 else 2
    if len(f) < st + 11:
        continue
    eid, temp = num(f[st]), num(f[st + 1])
    if temp in temps:
        enemies.append(l)
        eids[eid] = max(num(f[st + 2]), num(f[st + 3]))  # LV_MAX

groups = {}
for l in rd('group1.txt'):
    f = l.split(',')
    if len(f) < 24:
        continue
    gid = num(f[1])
    ids = [num(x, -1) for x in f[4:14]]
    pr = [num(x) for x in f[14:24]]
    groups[gid] = [(i, p) for i, p in zip(ids, pr) if i > 0 and p > 0]

w = collections.Counter()
for l in rd('encount.txt'):
    f = l.split(',')
    if len(f) < 30 or num(f[1]) != 100:
        continue
    gs = [num(x, -1) for x in f[10:20]]
    ps = [num(x) for x in f[20:30]]
    for g, p in zip(gs, ps):
        if g <= 0 or p <= 0 or g not in groups:
            continue
        gp = groups[g]
        tot = sum(x[1] for x in gp) or 1
        for eid, q in gp:
            if eid in eids and eids[eid] <= MAXLV:
                w[eid] += p * q * 100 // tot

os.makedirs(out, exist_ok=True)
hdr = '# 由 tools/sa1/import_sa1.py 从 SA1 {} 生成 (GBK -> UTF-8, 列格式与 SA1 相同); 只保留名字能对上国服 enemy.bin 的种族\n'
with open(os.path.join(out, 'enemybase.txt'), 'w', encoding='utf-8') as fo:
    fo.write(hdr.format('enemybase1.txt'))
    fo.write('# 名字,素材1..5,TEMPNO,INITNUM,LVUPPOINT,BASEVITAL,BASESTR,BASETGH,BASEDEX,MODAI,GET,EARTHAT,WATERAT,FIREAT,WINDAT,'
             'POISON,PARALYSIS,SLEEP,STONE,DRUNK,CONFUSION,PETSKILL1..7,RARE,CRITICAL,COUNTER,SLOT,IMGNUMBER,PETFLG,SIZE,ATOM...,LIMITLEVEL[,FUSIONCODE]\n')
    fo.write('\n'.join(bases) + '\n')
with open(os.path.join(out, 'enemy.txt'), 'w', encoding='utf-8') as fo:
    fo.write(hdr.format('enemy1.txt'))
    fo.write('# 备注名,TACTICSOPTION,ACT_CONDITION,ID,TEMPNO,LV_MIN,LV_MAX,CREATEMAXNUM,CREATEMINNUM,TACTICS,EXP,DUELPOINT,STYLE,PETFLG,ITEM1..10,ITEMPROB1..10\n')
    fo.write('\n'.join(enemies) + '\n')
with open(os.path.join(out, 'sa2map.txt'), 'w', encoding='utf-8') as fo:
    fo.write('# SA1 TEMPNO -> SA2 CGNO, 国服 enemy.bin 名字 id (import_sa1.py 按名字完全匹配生成; 可手工增改)\n')
    fo.write('\n'.join(sa2map) + '\n')
with open(os.path.join(out, 'encount.txt'), 'w', encoding='utf-8') as fo:
    fo.write('# MAP,ENEMY_ID,WEIGHT — SA2 map 100 <- SA1 floor 100 新手区 (encount.txt 区域 x group1.txt 组), import_sa1.py 生成\n')
    for eid, x in sorted(w.items()):
        fo.write(f'100,{eid},{x}\n')
print(f'enemybase: {len(bases)}/{total} species matched CN names; enemy rows: {len(enemies)}; map100 encounter enemies: {len(w)}')
