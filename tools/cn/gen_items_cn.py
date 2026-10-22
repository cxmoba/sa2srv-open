#!/usr/bin/env python3
"""data_cn/item.json (dump_cn_more.py) -> data/items_cn.txt (Tab 分隔, items.go LoadItemsCN)。
kind: 装备位 1..0x12 + 效果 11 → weapon (攻击), 效果 12 / 其他装备 → armor (防御, 无效果码时 0);
效果 4 → food (回体力); 其余 misc。附加属性 (致命/反击等) 只放 extra 列, 服务器暂不用。"""
import json, os
B = os.path.join(os.path.dirname(os.path.abspath(__file__)), '../..')
it = json.load(open(os.path.join(B, 'data_cn/item.json')))
out = ['# 国服 item.bin 全表 (tools/cn/gen_items_cn.py 生成, 勿手改; data/items.txt 同号覆盖)。Tab 分隔:',
       '# no\tname\tkind\tval\tloc\tstack\tprice\ticon\tdrop\tdesc\textra(属性码:值 …, 未用)']
clean = lambda s: s.replace('\t', ' ').replace('\n', ' ').replace('\r', ' ').replace('\\n', ' ')
for x in it:
    loc, eff, v = x['loc'], x['eff'], x['eff_val']
    if 1 <= loc <= 0x12:
        k = 'weapon' if eff == 11 else 'armor'
        v = v if eff in (11, 12) else 0
    elif eff == 4: k = 'food'
    else: k, v = 'misc', 0
    stack = 1 if k in ('weapon', 'armor') else 99
    extra = ' '.join('%x:%d' % (a, b) for a, b in x['mods'])
    out.append('\t'.join(map(str, [x['no'], clean(x['name']), k, v, '0x%x' % loc, stack, x['price'], x['icon'], 0, clean(x['desc']), extra])))
open(os.path.join(B, 'data/items_cn.txt'), 'w', encoding='utf-8').write('\n'.join(out) + '\n')
print(len(it), 'items')
