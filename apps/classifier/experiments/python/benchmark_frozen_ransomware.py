#!/usr/bin/env python3
"""Local batch inference benchmark; no document text or predictions persisted."""
import json,pathlib,hashlib,time,resource
import numpy as np,joblib,sklearn
p=pathlib.Path('apps/classifier/data/iteration-2026-10-25-source-label-audit/ransomware-specialist-candidate');m=p/'candidate.joblib';rows=[json.loads(x) for x in open('apps/classifier/data/iteration-2026-10-23-representation-comparison/development.jsonl') if json.loads(x)['source']=='ransomware.live'];texts=[r['text'] for r in rows];model=joblib.load(m)
# Warm-up and repeated batches; use held-in corpus only for runtime, not metric evaluation.
model.predict_proba(texts[:8]);times=[]
for batch_size in [1,16,64,256]:
 ts=[]
 for start in range(0,len(texts),batch_size):
  batch=texts[start:start+batch_size];t=time.perf_counter();scores=model.predict_proba(batch)[:,1];ts.append(time.perf_counter()-t);assert len(scores)==len(batch) and np.isfinite(scores).all()
 times.append({'batch_size':batch_size,'p50_batch_ms':float(np.percentile(ts,50)*1000),'p95_batch_ms':float(np.percentile(ts,95)*1000),'docs_per_second':float(len(texts)/sum(ts))})
r={'artifact_sha256':hashlib.sha256(m.read_bytes()).hexdigest(),'artifact_bytes':m.stat().st_size,'bench_documents':len(texts),'runtime':'local CPU, warm model process; batch timing includes sklearn predict_proba','results':times,'max_rss_mb':resource.getrusage(resource.RUSAGE_SELF).ru_maxrss/(1024*1024),'environment':{'scikit_learn':sklearn.__version__},'warning':'same development corpus used only as workload; these are not quality metrics; excludes Python service startup and full production integration','promotion':False};(p/'benchmark.json').write_text(json.dumps(r,indent=2)+'\n');print(json.dumps(r,indent=2))
