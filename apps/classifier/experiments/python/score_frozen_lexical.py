#!/usr/bin/env python3
"""Score JSONL with a trusted local private candidate. Unknown source abstains."""
import argparse, hashlib, json
from pathlib import Path
import joblib


def load_candidate(directory):
    manifest=json.loads((directory/'manifest.json').read_text());path=directory/'candidate.joblib'
    if hashlib.sha256(path.read_bytes()).hexdigest()!=manifest['candidate_sha256']:raise ValueError('candidate hash mismatch')
    return joblib.load(path)  # Local trusted artifacts only: pickle is executable.


def score(candidate, rows):
    if not rows:return []
    probabilities=candidate['pipeline'].predict_proba([r['text'] for r in rows])[:,1];out=[]
    for r,p in zip(rows,probabilities):
        t=candidate['source_thresholds'].get(r.get('source',''))
        out.append({'id':r.get('id'),'score':float(p),'relevant':bool(p>=t) if t is not None else None,'threshold':t,'status':'scored' if t is not None else 'unknown_source_abstention','version':candidate['version']})
    return out


def main():
    p=argparse.ArgumentParser();p.add_argument('--candidate',type=Path,required=True);p.add_argument('--input',type=Path,required=True);p.add_argument('--output',type=Path,required=True);a=p.parse_args()
    rows=[json.loads(l) for l in a.input.read_text().splitlines() if l.strip()];candidate=load_candidate(a.candidate)
    with a.output.open('x') as f:
        for row in score(candidate,rows):f.write(json.dumps(row)+'\n')

if __name__=='__main__':main()
