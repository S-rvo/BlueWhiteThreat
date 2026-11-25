#!/usr/bin/env python3
"""Development-only domain transfer audit. Historical labels, no tuning/test reserve."""
import json,hashlib,pathlib
import numpy as np
from sklearn.feature_extraction.text import TfidfVectorizer
from sklearn.pipeline import FeatureUnion
from sklearn.linear_model import LogisticRegression
from sklearn.metrics import average_precision_score

root=pathlib.Path('apps/classifier/data/iteration-2026-10-23-representation-comparison')
rows=[json.loads(x) for x in (root/'development.jsonl').read_text().splitlines()]
texts=[r['text'] for r in rows]; y=np.array([r['label'] for r in rows]); src=np.array([r['source'] for r in rows]); sources=sorted(set(src)); results={}
for held in sources:
 tr=src!=held;te=~tr
 vec=FeatureUnion([('word',TfidfVectorizer(ngram_range=(1,2),min_df=2,max_features=20000,sublinear_tf=True)),('char',TfidfVectorizer(analyzer='char',ngram_range=(3,5),min_df=2,max_features=30000,sublinear_tf=True))])
 Xtr=vec.fit_transform([texts[i] for i in np.where(tr)[0]]);Xte=vec.transform([texts[i] for i in np.where(te)[0]])
 model=LogisticRegression(C=10,class_weight='balanced',max_iter=2000,random_state=20261023);model.fit(Xtr,y[tr]);score=model.predict_proba(Xte)[:,1];pred=score>=.5
 tp=int((pred&(y[te]==1)).sum());fp=int((pred&(y[te]==0)).sum());fn=int((~pred&(y[te]==1)).sum());tn=int((~pred&(y[te]==0)).sum());p=tp/(tp+fp) if tp+fp else 0;r=tp/(tp+fn) if tp+fn else 0
 results[held]={'train_documents':int(tr.sum()),'heldout_documents':int(te.sum()),'heldout_positive_count':int(y[te].sum()),'average_precision':float(average_precision_score(y[te],score)) if len(set(y[te]))>1 else None,'threshold_fixed_at_0_5':{'precision':p,'recall':r,'f1':2*p*r/(p+r) if p+r else 0,'tp':tp,'fp':fp,'fn':fn,'tn':tn}}
out={'protocol':'leave one source out; no source tuning, fixed C=10 and threshold 0.5; historical labels; no reserve','input_sha256':hashlib.sha256((root/'development.jsonl').read_bytes()).hexdigest(),'model':'word+char TF-IDF logistic regression; class_weight balanced','results':results,'warning':'source is the held-out domain; this measures domain transfer under noisy historical labels, not production accuracy','promotion':False,'reserve_read':False}
dst=pathlib.Path('apps/classifier/data/iteration-2026-10-25-source-label-audit/leave-one-source-out.json');dst.write_text(json.dumps(out,indent=2)+'\n');print(json.dumps(out,indent=2))
