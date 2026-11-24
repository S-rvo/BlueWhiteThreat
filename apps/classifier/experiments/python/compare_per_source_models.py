#!/usr/bin/env python3
"""Development-only shared vs source-specific grouped OOF comparison at fixed 0.5."""
import json,pathlib,hashlib
import numpy as np
from sklearn.model_selection import StratifiedGroupKFold
from sklearn.feature_extraction.text import TfidfVectorizer
from sklearn.pipeline import FeatureUnion
from sklearn.linear_model import LogisticRegression
from sklearn.metrics import average_precision_score
root=pathlib.Path('apps/classifier/data/iteration-2026-10-23-representation-comparison');rows=[json.loads(x) for x in (root/'development.jsonl').read_text().splitlines()];texts=[r['text'] for r in rows];y=np.array([r['label'] for r in rows]);src=np.array([r['source'] for r in rows]);
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
G=np.array([find(i) for i in range(len(rows))]);fold=np.zeros(len(rows),int)
for k,(_,te) in enumerate(StratifiedGroupKFold(5,shuffle=True,random_state=20261023).split(np.zeros(len(rows)),np.array([r['source']+':'+str(r['label']) for r in rows]),G)):fold[te]=k
shared=np.load(root/'oof-word_char-10.npy');per=np.zeros(len(rows))
for source in sorted(set(src)):
 ix=np.where(src==source)[0];localfold=fold[ix]
 for f in range(5):
  tr=ix[localfold!=f];te=ix[localfold==f]
  vec=FeatureUnion([('word',TfidfVectorizer(ngram_range=(1,2),min_df=2,max_features=20000,sublinear_tf=True)),('char',TfidfVectorizer(analyzer='char',ngram_range=(3,5),min_df=2,max_features=30000,sublinear_tf=True))]);Xtr=vec.fit_transform([texts[i] for i in tr]);Xte=vec.transform([texts[i] for i in te]);model=LogisticRegression(C=10,class_weight='balanced',max_iter=2000,random_state=20261023);model.fit(Xtr,y[tr]);per[te]=model.predict_proba(Xte)[:,1]

def summarize(score,ix):
 yy=y[ix];ss=score[ix];pred=ss>=.5;tp=int((pred&(yy==1)).sum());fp=int((pred&(yy==0)).sum());fn=int((~pred&(yy==1)).sum());tn=int((~pred&(yy==0)).sum());p=tp/(tp+fp) if tp+fp else 0;r=tp/(tp+fn) if tp+fn else 0
 return {'documents':len(ix),'positives':int(yy.sum()),'average_precision':float(average_precision_score(yy,ss)),'threshold_fixed_0_5':{'precision':p,'recall':r,'f1':2*p*r/(p+r) if p+r else 0,'tp':tp,'fp':fp,'fn':fn,'tn':tn}}
res={}
for s in sorted(set(src)):
 ix=np.where(src==s)[0];res[s]={'shared':summarize(shared,ix),'separate':summarize(per,ix)}
np.save('apps/classifier/data/iteration-2026-10-25-source-label-audit/shared-scores.npy',shared);np.save('apps/classifier/data/iteration-2026-10-25-source-label-audit/separate-scores.npy',per)
out={'input_sha256':hashlib.sha256((root/'development.jsonl').read_bytes()).hexdigest(),'shared_oof_sha256':hashlib.sha256((root/'oof-word_char-10.npy').read_bytes()).hexdigest(),'protocol':'same grouped 5-fold IDs as shared model; each per-source model fits only that source in each fold; fixed C=10, threshold=.5; historical labels; no threshold selection','results':res,'warning':'No independent truth; historical labels noisy; in-source grouped evaluation only','promotion':False,'reserve_read':False};pathlib.Path('apps/classifier/data/iteration-2026-10-25-source-label-audit/per-source-models.json').write_text(json.dumps(out,indent=2)+'\n');print(json.dumps(out,indent=2))
