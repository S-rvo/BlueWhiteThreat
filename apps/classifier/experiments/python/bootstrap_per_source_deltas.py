import json,pathlib,collections
import numpy as np
p=pathlib.Path('apps/classifier/data/iteration-2026-10-25-source-label-audit');rows=[json.loads(x) for x in open('apps/classifier/data/iteration-2026-10-23-representation-comparison/development.jsonl')]; y=np.array([r['label'] for r in rows]);src=np.array([r['source'] for r in rows]);a=np.load(p/'shared-scores.npy');b=np.load(p/'separate-scores.npy')
parent=list(range(len(rows)))
def f(i):
 while parent[i]!=i:parent[i]=parent[parent[i]];i=parent[i]
 return i
seen={}
for i,r in enumerate(rows):
 provenance=r.get('group','')
 if r['source']=='ransomware.live' and r.get('source_actor'):provenance='ransomware-actor:'+r['source_actor'].strip().lower()
 for key in (r['content_component'],'provenance:'+provenance if provenance else ''):
  if key:
   if key in seen:parent[f(i)]=f(seen[key])
   else:seen[key]=i
groups=np.array([f(i) for i in range(len(rows))]);rng=np.random.default_rng(20261025)
def f1(ix,s):
 pred=s[ix]>=.5;yy=y[ix];tp=np.sum(pred&(yy==1));fp=np.sum(pred&(yy==0));fn=np.sum(~pred&(yy==1));return 2*tp/(2*tp+fp+fn) if 2*tp+fp+fn else 0
result={'replicates':5000,'seed':20261025,'resampling':'connected content/provenance groups with replacement within source; paired predictions; threshold fixed 0.5','sources':{}}
for source in sorted(set(src)):
 ix=np.where(src==source)[0];gs=np.unique(groups[ix]);members={g:ix[groups[ix]==g] for g in gs};d=[]
 for _ in range(5000):
  sampled=rng.choice(gs,size=len(gs),replace=True);bi=np.concatenate([members[g] for g in sampled]);d.append(f1(bi,b)-f1(bi,a))
 obs=f1(ix,b)-f1(ix,a);result['sources'][source]={'observed_f1_delta_separate_minus_shared':float(obs),'group_bootstrap_95pct_ci':[float(x) for x in np.quantile(d,[.025,.975])],'groups':len(gs),'bootstrap_probability_delta_gt_0':float(np.mean(np.array(d)>0))}
result['caveat']='Conditional uncertainty for these OOF predictions and historical labels; not independent model validation; fold/model training variation is not resampled.';(p/'paired-group-bootstrap.json').write_text(json.dumps(result,indent=2)+'\n');print(json.dumps(result,indent=2))
