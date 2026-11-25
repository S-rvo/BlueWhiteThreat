#!/usr/bin/env python3
"""Score JSONL with frozen experimental ransomware candidate; never used by production."""
import argparse,hashlib,json,math
from pathlib import Path
import joblib

def digest(path):return hashlib.sha256(path.read_bytes()).hexdigest()
def score(input_path,output_path,model_dir):
 manifest=json.loads((model_dir/'manifest.json').read_text());model_path=model_dir/'candidate.joblib'
 if digest(model_path)!=manifest['model_sha256']:raise ValueError('candidate_hash_mismatch')
 if manifest.get('production') is not False or manifest.get('threshold')!=0.5:raise ValueError('candidate_manifest_policy_mismatch')
 rows=[json.loads(line) for line in input_path.read_text().splitlines() if line.strip()];ids=[r.get('id') for r in rows]
 if any(not isinstance(x,str) or not x for x in ids) or len(set(ids))!=len(ids):raise ValueError('invalid_or_duplicate_id')
 if any(not isinstance(r.get('text'),str) or not r['text'].strip() for r in rows):raise ValueError('invalid_text')
 model=joblib.load(model_path);scores=model.predict_proba([r['text'] for r in rows])[:,1]
 if len(scores)!=len(rows) or not all(math.isfinite(float(s)) and 0<=s<=1 for s in scores):raise ValueError('invalid_score')
 payload=''.join(json.dumps({'id':r['id'],'score':float(s),'threshold':0.5,'prediction':int(s>=.5),'model_sha256':manifest['model_sha256']})+'\n' for r,s in zip(rows,scores))
 with output_path.open('x') as f:f.write(payload)
 return len(rows)
def main():
 p=argparse.ArgumentParser();p.add_argument('--input',type=Path,required=True);p.add_argument('--output',type=Path,required=True);p.add_argument('--model-dir',type=Path,required=True);a=p.parse_args();print(json.dumps({'scored':score(a.input,a.output,a.model_dir),'production':False}))
if __name__=='__main__':main()
