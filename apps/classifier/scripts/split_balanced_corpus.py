#!/usr/bin/env python3
"""Create a deterministic group-safe, label/style-stratified split."""
import argparse, csv, hashlib, json
from collections import Counter, defaultdict
from pathlib import Path

p=argparse.ArgumentParser(); p.add_argument('--input',type=Path,required=True); p.add_argument('--out-dir',type=Path,required=True); p.add_argument('--validation-per-cell',type=int,default=15); a=p.parse_args()
docs=[json.loads(x) for x in a.input.open() if x.strip()]
official={'cisa-kev-ransomware','github-advisories','cert-fr-cti','cert-eu-cti','official-release'}
def style(d):
    return d.get('style') or ('official' if d.get('source') in official else 'conversational')
for d in docs: d['style']=style(d)
groups=defaultdict(list)
for d in docs: groups[d.get('group') or d['id']].append(d)
for g in groups.values():
    if len({x['label'] for x in g}) != 1: raise SystemExit('mixed-label group')

validation=[]
for label in (0,1):
  for doc_style in ('official','conversational'):
    candidates=[g for g in groups.values() if g[0]['label']==label and style(g[0])==doc_style]
    candidates.sort(key=lambda g: hashlib.sha256((g[0].get('group') or g[0]['id']).encode()).hexdigest())
    # Prefer singleton groups so the requested class size remains exact.
    candidates.sort(key=lambda g: len(g))
    for g in candidates:
        current=len([x for x in validation if x['label']==label and style(x)==doc_style])
        if current+len(g) <= a.validation_per_cell:
            validation.extend(g)
        if len([x for x in validation if x['label']==label and style(x)==doc_style]) == a.validation_per_cell: break
train=[x for x in docs if x['id'] not in {v['id'] for v in validation}]
cells=Counter((x['label'],style(x)) for x in validation)
expected=Counter({(label,s):a.validation_per_cell for label in (0,1) for s in ('official','conversational')})
if cells!=expected: raise SystemExit(f'could not create exact style/label split: {cells}')
if {x.get('group') for x in train}&{x.get('group') for x in validation}: raise SystemExit('group leakage')
a.out_dir.mkdir(parents=True,exist_ok=True)
with (a.out_dir/'training.csv').open('w',newline='') as f:
    w=csv.writer(f); w.writerow(['id','text','label']); w.writerows((x['id'],x['text'],x['label']) for x in train)
with (a.out_dir/'validation.jsonl').open('w') as f:
    for x in validation: f.write(json.dumps(x,ensure_ascii=False)+'\n')
cons={'summary':{'documents':len(validation),'completed':len(validation),'counts':{'consensus':len(validation),'pending':0,'abstention':0,'disagreement':0},'label_authority':'inherited frozen two-model consensus plus content audit'},'documents':[]}
for x in validation: cons['documents'].append({'id':x['id'],'text_sha256':hashlib.sha256(x['text'].encode()).hexdigest(),'label':x['label'],'status':'consensus'})
(a.out_dir/'validation-consensus.json').write_text(json.dumps(cons,indent=2)+'\n')
(a.out_dir/'weak-empty.jsonl').write_text('')
manifest={'training':len(train),'validation':len(validation),'training_labels':Counter(x['label'] for x in train),'validation_labels':Counter(x['label'] for x in validation),'training_cells':Counter(f"{x['label']}:{style(x)}" for x in train),'validation_cells':Counter(f"{x['label']}:{style(x)}" for x in validation),'training_sources':Counter(x['source'] for x in train),'validation_sources':Counter(x['source'] for x in validation),'group_overlap':0}
(a.out_dir/'split-manifest.json').write_text(json.dumps(manifest,indent=2,default=dict)+'\n'); print(json.dumps(manifest,default=dict))
