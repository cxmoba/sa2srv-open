from lib import *
import re,struct
pairs=[]
for m in re.finditer(rb'\x66\xc7\x44\x24(.)(..)\xc7\x44\x24(.)(....)',img[0x1000:0x1f0000],re.S):
    a=IB+0x1000+m.start(); cmd=struct.unpack('<H',m.group(2))[0]; fn=struct.unpack('<I',m.group(4))[0]
    if 0x401000<=fn<0x5f0000: pairs.append((a,cmd,fn))
# also with disp32 forms: 66 c7 84 24 d32 imm16 c7 84 24 d32 imm32
for m in re.finditer(rb'\x66\xc7\x84\x24(....)(..)\xc7\x84\x24(....)(....)',img[0x1000:0x1f0000],re.S):
    a=IB+0x1000+m.start(); cmd=struct.unpack('<H',m.group(2))[0]; fn=struct.unpack('<I',m.group(4))[0]
    if 0x401000<=fn<0x5f0000: pairs.append((a,cmd,fn))
pairs.sort()
import collections
grp=collections.defaultdict(list)
for a,c,f in pairs: grp[func_start(a)].append((c,f))
for k,v in grp.items(): print(hex(k or 0),len(v))
import pickle; pickle.dump(pairs,open('pairs.pkl','wb'))
