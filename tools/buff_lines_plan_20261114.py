#!/usr/bin/env python3
"""Plan for tools/buff_lines_20261114.py (2026-11-14): reads the audit dump of internal/entity/bufflines_audit_test.go
(BUFFLINES_DB=<db copy> BUFFLINES_OUT=<json> go test -run TestBuffLinesAudit ./internal/entity/) and writes the moves:
    python3 tools/buff_lines_plan_20261114.py audit.json plan.json > plan.txt
The tool's buff_lines_20261114.json is the union of the plans of both shards."""
import json,sys,collections
CL="WAR RAN PAL SK MNK BRD RGE DRD SHA CL MAG NEC ENC WIZ ALC".split()
NONPRIMARY={'FR','CR','PR','DR','LR','AR','HoT','PoT'}
d=json.load(open(sys.argv[1]))
S={s['id']:s for s in d['spells']}
# Bladecoat (self) and Bladecoat + (a target's) have each other's stack types: swapped first
SWAP={700707:2, 901177:1}
for i,st in SWAP.items():
    if i in S:
        s=S[i]; f0=s['line'].split(); assert f0[0]==f"stack{3-st}", s['line']
        s['line']=' '.join([f'stack{st}']+f0[1:]); s['stack']=st
        # its relations change: drop them; the audit after the swap is the check
d['pairs']=[p for p in d['pairs'] if p['A'] not in SWAP and p['B'] not in SWAP]
def cls(s): return tuple(sorted({o['class'] for o in (s['owners'] or [])}))
def parts(s):
    f=s['line'].split()
    if f[0].startswith('stack'): return int(f[0][5:]), f[1], set(f[2:])
    return 0, f[0], {k for k in f[1:] if not k.startswith('^')}
def primary(keys): return {k[1:] for k in keys if k.startswith('+') and k[1:] not in NONPRIMARY}
ref={(p['A'],p['B']) for p in d['pairs'] if p['Kind']=='refuses'}
rel=set()
for p in d['pairs']:
    if p['Kind']=='drops' and (p['A'],p['B']) in ref: continue
    rel.add(frozenset((p['A'],p['B'])))
# clusters
par={}
def f(x):
    while par.setdefault(x,x)!=x: x=par[x]
    return x
def gkey(s):
    st,b,_=parts(s); return (cls(s),st,b)
for r in rel:
    a,b=tuple(r); A,B=S[a],S[b]
    if gkey(A)==gkey(B): par[f(a)]=f(b)
cl=collections.defaultdict(list)
for s in d['spells']: cl[f(s['id'])].append(s['id'])
# conflicts
move=set(); why=collections.defaultdict(set)
for r in rel:
    a,b=tuple(r); A,B=S[a],S[b]
    ca,cb=cls(A),cls(B)
    if not ca or not cb or set(ca)&set(cb): continue
    if A['scope']==0 and B['scope']==0: continue
    sa,_,ka=parts(A); sb,_,kb=parts(B)
    shared=primary(ka&kb)
    if not shared: continue
    for X,Y,sx in ((A,B,sa),(B,A,sb)):
        if X['workbook'] and sx in (1,2,3):
            move.add(f(X['id'])); why[f(X['id'])].add(Y['id'])
# a cluster may keep its stack type when every partner it conflicts with moves and wants to less: one linked to a plain
# (EQOA-Data words) spell of its own family, which can take no stack number (3: the druid's skins with Vitae Blessing,
# Tunare's Blessing's CM twin; the paladin's group AC with Protectorate, Hero Guard II's twin), one with a damage
# shield or a proc of the classes' one slot (2), self buffs alone (1)
PLAINFAMILY={(700828,701252),(901013,700055)}
def plainlinked(root):
    ids=set(cl[root])
    for w,p in PLAINFAMILY:
        if w in ids: return True
    return False
def selfonly(root): return all(S[i]['scope']==0 for i in cl[root])
def sharedfx(root): return any(k in ('shield','proc','defproc') for i in cl[root] for k in parts(S[i])[2])
def stay(root): return 3 if plainlinked(root) else 2 if sharedfx(root) else 1 if selfonly(root) else 0
changed=True
while changed:
    changed=False
    for root in sorted(move,key=lambda r:-stay(r)):
        if stay(root) and all(f(y) in move and stay(f(y))<stay(root) for y in why[root]):
            move.discard(root); changed=True; break
# within a class, different families (the audit's c list): Concentration and Inspiration, the alchemist's two
# level 60 CMs, apart from the group thinking and Unified Intent CM lines
SPLIT={900127,700719}
# Pure Heart, every class's level 60 CM (+1200 max HP and power, group), a family of its own: it refused the cleric's
# Group Order (shared max HP)
OWN={901847}
plan={}
used={s['stack'] for s in d['spells']}
for root in sorted(move):
    ids=sorted(cl[root])
    for i in ids:
        if i in SPLIT and S[i]['workbook']:
            plan[i]=i; print(f"  split {i} {S[i]['name']} -> {i}")
    ids=[i for i in ids if i not in SPLIT]; new=ids[0]
    if new in used and new not in [S[i]['stack'] for i in ids]: print('COLLIDE',new,file=sys.stderr)
    members=[i for i in ids if S[i]['workbook'] and parts(S[i])[0] in (1,2,3)]
    for i in members: plan[i]=new
    X=S[root]
    print(f"[{'/'.join(CL[c] for c in cls(X))}] stack{parts(X)[0]}->{new}: "+", ".join(f"{i} {S[i]['name']}" for i in members))
    print("     vs "+", ".join(sorted({f"{'/'.join(CL[c] for c in cls(S[y]))} {S[y]['name']}" for y in why[root]})))
# the bard's Concerto line (Concerto, Symphony, Rhapsody, Harmony and their Perfect forms) one line apart from the
# Power songs, with Symphony (stack 171 since 2026-11-14, the owner's fix of Symphony over Power Dance)
CONCERTO=[700058,700852,196,701012,700714,701083]
for i in CONCERTO:
    if i in S:
        assert parts(S[i])[0]==3, (i, S[i]['line']); plan[i]=171
for i,st in SWAP.items():
    if i in S: plan[i]=st
# Bladecoat + joins its single-target coats (Thistlecoat + to Spikecoat +) wherever they go
if 901177 in S: plan[901177]=plan.get(901097,1)
for i in OWN:
    if i in S: assert parts(S[i])[0] in (1,2,3); plan[i]=i
out=[{'id':i,'name':S[i]['name'],'old':(3-SWAP[i]) if i in SWAP else parts(S[i])[0],'new':plan[i],
      'classes':[CL[c] for c in cls(S[i])]} for i in sorted(plan)]
json.dump(out,open(sys.argv[2],'w'),indent=0)
print(len(plan),'spells',len(move),'clusters',file=sys.stderr)
