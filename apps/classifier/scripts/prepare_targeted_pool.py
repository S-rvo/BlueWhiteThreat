#!/usr/bin/env python3
"""Select an unlabelled enrichment stream from a frozen random export.

This sampler never changes the production crawler and never assigns labels.
Its output is development-only; the untouched random stream remains the source
for prevalence and independent evaluation.
"""
import argparse, hashlib, json, re
from pathlib import Path

STRONG = re.compile(r"\b(initial access|access broker|ransomware|data exfiltrat(?:ion|ed)|stolen (?:data|credentials|files)|victim data|ransom demand)\b", re.I)
AMBIGUOUS = re.compile(r"\b(leak|database|access|credentials|dump|files)\b", re.I)
SECONDARY = re.compile(r"\b(victim|breach|compromis(?:e|ed)|exfiltrat(?:e|ed|ion)|malware|extortion|publish|sale|corporate|employee|customer records)\b", re.I)

def normalized(text): return re.sub(r"\s+", " ", text).strip().lower()
def load_hashes(paths):
    out=set()
    for path in paths:
        with path.open() as f:
            for line in f:
                d=json.loads(line); text=d.get("text","")
                if text: out.add(hashlib.sha256(normalized(text).encode()).hexdigest())
    return out

def main():
    ap=argparse.ArgumentParser(); ap.add_argument('--input',type=Path,required=True); ap.add_argument('--exclude',type=Path,action='append',default=[]); ap.add_argument('--out',type=Path,required=True); ap.add_argument('--limit',type=int,default=500); a=ap.parse_args()
    if a.limit < 1: raise ValueError('positive_limit_required')
    excluded=load_hashes(a.exclude); selected=[]; counts={'strong':0,'ambiguous_with_context':0}; seen=set()
    for line in a.input.open():
        d=json.loads(line); text=d.get('text',''); h=hashlib.sha256(normalized(text).encode()).hexdigest()
        if not text or h in excluded or h in seen: continue
        reason = 'strong' if STRONG.search(text) else 'ambiguous_with_context' if AMBIGUOUS.search(text) and SECONDARY.search(text) else ''
        if not reason: continue
        selected.append({**d,'sampling_stream':'targeted_development_enrichment','sampling_reason':reason,'automatic_label':None}); counts[reason]+=1; seen.add(h)
        if len(selected)>=a.limit: break
    a.out.parent.mkdir(parents=True,exist_ok=True)
    with a.out.open('x') as f:
        for d in selected: f.write(json.dumps(d,ensure_ascii=False)+'\n')
    a.out.chmod(0o600)
    print(json.dumps({'selected':len(selected),'reasons':counts,'labels_assigned':False,'production_crawler_changed':False}))
if __name__=='__main__': main()
