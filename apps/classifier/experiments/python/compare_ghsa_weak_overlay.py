import json, pathlib, hashlib, collections
import numpy as np
from sklearn.model_selection import StratifiedGroupKFold
from sklearn.feature_extraction.text import TfidfVectorizer
from sklearn.pipeline import FeatureUnion
from sklearn.linear_model import LogisticRegression
from sklearn.metrics import average_precision_score,precision_recall_curve
root=pathlib.Path('apps/classifier/data'); src=root/'iteration-2026-10-23-representation-comparison/development.jsonl'; rows=[json.loads(x) for x in src.read_text().splitlines()]; out=root/'iteration-2026-10-25-source-label-audit'; overlay={json.loads(x)['id']:json.loads(x)['proposed_weak_label'] for x in (out/'proposed-ghsa-overlay.jsonl').read_text().splitlines()}; assert len(overlay)==1213 and len(rows)==4287
# Same connected groups and deterministic folds.
parent=list(range(len(rows)))
def find(i):
 while parent[i]!=i:parent[i]=parent[parent[i]];i=parent[i]
 return i
seen={}
for i,r in enumerate(rows):
 prov=r.get('group','')
 if r['source']=='ransomware.live' and r.get('source_actor'):prov='ransomware-actor:'+r['source_actor'].strip().lower()
 for key in (r['content_component'],'provenance:'+prov if prov else ''):
  if key:
   if key in seen:parent[find(i)]=find(seen[key])
   else:seen[key]=i
G=np.array([find(i) for i in range(len(rows))]); strata=np.array([r['source']+':'+str(r['label']) for r in rows]);fold=np.zeros(len(rows),int)
for k,(_,te) in enumerate(StratifiedGroupKFold(5,shuffle=True,random_state=20261023).split(np.zeros(len(rows)),strata,G)):fold[te]=k
texts=[r['text'] for r in rows]; sources=np.array([r['source'] for r in rows]); reps=np.array([r.get('sampling_stream')!='targeted_development_enrichment' for r in rows]); ys={'historical':np.array([r['label'] for r in rows]),'ghsa_weak_overlay':np.array([overlay.get(r['id'],r['label']) for r in rows])};scores={k:np.zeros(len(rows)) for k in ys}
for f in range(5):
 tr=fold!=f;te=~tr
 vec=FeatureUnion([('word',TfidfVectorizer(ngram_range=(1,2),min_df=2,max_features=20000,sublinear_tf=True)),('char',TfidfVectorizer(analyzer='char',ngram_range=(3,5),min_df=2,max_features=30000,sublinear_tf=True))])
 Xtr=vec.fit_transform([texts[i] for i in np.where(tr)[0]]);Xte=vec.transform([texts[i] for i in np.where(te)[0]])
 for name,y in ys.items():
  model=LogisticRegression(C=10,class_weight='balanced',max_iter=2000,random_state=20261023);model.fit(Xtr,y[tr]);scores[name][te]=model.predict_proba(Xte)[:,1]
 print('fold',f+1,flush=True)
def metrics(y,s,t):
 p=s>=t;tp=int((p&(y==1)).sum());fp=int((p&(y==0)).sum());fn=int((~p&(y==1)).sum());tn=int((~p&(y==0)).sum());pr=tp/(tp+fp) if tp+fp else 0;re=tp/(tp+fn) if tp+fn else 0
 return {'precision':pr,'recall':re,'f1':2*pr*re/(pr+re) if pr+re else 0,'tp':tp,'fp':fp,'fn':fn,'tn':tn}
results={}
for name,y in ys.items():
 by={}
 for source in sorted(set(sources)):
  ix=reps&(sources==source);pr,re,ts=precision_recall_curve(y[ix],scores[name][ix]);eligible=np.flatnonzero(re[:-1]>=.9);j=max(eligible,key=lambda z:(pr[z],ts[z]));by[source]={'threshold_oof':float(ts[j]),**metrics(y[ix],scores[name][ix],ts[j])}
 results[name]={'positive_counts':{s:int(sum((sources==s)&(y==1))) for s in sorted(set(sources))},'average_precision':float(average_precision_score(y[reps],scores[name][reps])),'thresholds_and_metrics_on_same_oof':by,'metric_selection_bias':'thresholds selected on these same grouped OOF predictions; descriptive only'}
res={'documents':len(rows),'overlay_records':len(overlay),'changed_labels':int(np.sum(ys['historical']!=ys['ghsa_weak_overlay'])),'same_group_folds':True,'fold_assignment_sha256':hashlib.sha256(fold.tobytes()).hexdigest(),'model':'word+character TF-IDF logistic regression C=10, class_weight=balanced','results':results,'ground_truth':'none; weak-label sensitivity diagnostic only','independent_test':False,'reserve_read':False,'production_promotion':False}
(out/'sensitivity-comparison.json').write_text(json.dumps(res,indent=2)+'\n');print(json.dumps(res,indent=2))
