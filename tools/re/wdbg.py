import subprocess,os,re
env=dict(os.environ,WINEPREFIX=os.environ.get('WINEPREFIX',os.path.expanduser('~/.wine-sa2')),DISPLAY=os.environ.get('DISPLAY',':1'),WINEDEBUG='-all')
def rd(addrs,n=1):
    """read n dwords at each addr; returns list of lists"""
    cmds='attach 0x20\n'+''.join(f'x /{n}x 0x{a:x}\n' for a in addrs)+'detach\nquit\n'
    out=subprocess.run(['winedbg'],input=cmds,capture_output=True,text=True,env=env,timeout=60).stdout
    vals=[]
    for blk in out.split('Wine-dbg>')[2:2+len(addrs)]:
        ws=re.findall(r'(?:^|\s)([0-9a-f]{8})(?=\s|$)',blk.split(':',1)[1] if n>1 else blk)
        vals.append([int(w,16) for w in ws][:n])
    return vals
