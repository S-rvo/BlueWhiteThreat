#!/usr/bin/env python3
"""Freeze a non-production ransomware.live TF-IDF candidate for future prospective evaluation."""
import hashlib,json,pathlib,platform
import numpy as np, sklearn, joblib
from sklearn.feature_extraction.text import TfidfVectorizer
from sklearn.pipeline import FeatureUnion,Pipeline
from sklearn.linear_model import LogisticRegression
root=pathlib.Path('apps/classifier/data/iteration-2026-10-23-representation-comparison');rows=[json.loads(x) for x in (root/'development.jsonl').read_text().splitlines() if json.loads(x)['source']=='ransomware.live'];assert len(rows)==1453
texts=[r['text'] for r in rows];y=np.array([r['label'] for r in rows]);assert len(set(y))==2
pipeline=Pipeline([('features',FeatureUnion([('word',TfidfVectorizer(ngram_range=(1,2),min_df=2,max_features=20000,sublinear_tf=True)),('char',TfidfVectorizer(analyzer='char',ngram_range=(3,5),min_df=2,max_features=30000,sublinear_tf=True))])),('classifier',LogisticRegression(C=10,class_weight='balanced',max_iter=2000,random_state=20261023))]);pipeline.fit(texts,y)
out=pathlib.Path('apps/classifier/data/iteration-2026-10-25-source-label-audit/ransomware-specialist-candidate');out.mkdir(exist_ok=False);model=out/'candidate.joblib';joblib.dump(pipeline,model,compress=3);loaded=joblib.load(model);probe=texts[:min(32,len(texts))];a=pipeline.predict_proba(probe)[:,1];b=loaded.predict_proba(probe)[:,1];assert np.array_equal(a,b)
manifest={'candidate':'ransomware.live source-specific lexical logistic regression','training_documents':len(rows),'positive':int(y.sum()),'negative':int((y==0).sum()),'training_corpus_sha256':hashlib.sha256((root/'development.jsonl').read_bytes()).hexdigest(),'script_sha256':hashlib.sha256(pathlib.Path(__file__).read_bytes()).hexdigest(),'model_sha256':hashlib.sha256(model.read_bytes()).hexdigest(),'threshold':0.5,'threshold_changed':False,'features':'word TF-IDF (1,2) + char TF-IDF (3,5), sublinear TF','C':10,'class_weight':'balanced','versions':{'python':platform.python_version(),'numpy':np.__version__,'scikit_learn':sklearn.__version__,'joblib':joblib.__version__},'serialization_parity_probe_documents':len(probe),'serialization_max_abs_probability_error':float(np.max(np.abs(a-b))),'training_labels':'historical noisy labels; not independently validated','production':False,'promotion':False,'reserve_read':False,'next_gate':'prospective independent corpus with frozen labels, threshold 0.5, source/content group separation; do not tune on test'}
(out/'manifest.json').write_text(json.dumps(manifest,indent=2)+'\n');print(json.dumps(manifest,indent=2))
