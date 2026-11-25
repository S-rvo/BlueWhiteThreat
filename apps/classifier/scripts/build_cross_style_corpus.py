#!/usr/bin/env python3
"""Build a balanced label × writing-style corpus from frozen consensus runs."""
import argparse, hashlib, json
from collections import Counter
from pathlib import Path

from build_balanced_corpus import fingerprint, similar

OFFICIAL = {"cisa-kev-ransomware", "github-advisories", "cert-fr-cti",
            "cert-eu-cti", "official-release", "cert-fr"}

def jsonl(path):
    return [json.loads(line) for line in path.open() if line.strip()]

def style(doc):
    return doc.get("style") or ("official" if doc.get("source") in OFFICIAL else "conversational")

def annotated(spec):
    pool_path, consensus_path = map(Path, spec.split("::", 1))
    pool = {str(x["id"]): x for x in jsonl(pool_path)}
    result = []
    for vote in json.loads(consensus_path.read_text())["documents"]:
        if vote.get("status") != "consensus":
            continue
        doc = pool[str(vote["id"])]
        if hashlib.sha256(doc["text"].encode()).hexdigest() != vote["text_sha256"]:
            raise SystemExit(f"changed annotated text: {vote['id']}")
        result.append(doc | {"label": vote["label"], "annotation_status": "consensus"})
    return result

def main():
    p=argparse.ArgumentParser()
    p.add_argument("--existing", type=Path, required=True)
    p.add_argument("--annotated", action="append", default=[], metavar="POOL::CONSENSUS")
    p.add_argument("--per-cell", type=int, default=50)
    p.add_argument("--out", type=Path, required=True)
    a=p.parse_args()
    candidates=jsonl(a.existing)
    for spec in a.annotated: candidates += annotated(spec)
    candidates.sort(key=lambda d: hashlib.sha256(str(d["id"]).encode()).hexdigest())
    selected=[]; prints=[]; ids=set(); groups=set(); excluded=Counter()
    counts=Counter()
    for doc in candidates:
        cell=(int(doc["label"]), style(doc))
        if counts[cell] >= a.per_cell: continue
        print_=fingerprint(doc["text"])
        if str(doc["id"]) in ids or (doc.get("group") and doc["group"] in groups): excluded["id_or_group"]+=1; continue
        if any(similar(print_, old) for old in prints): excluded["content_duplicate"]+=1; continue
        doc=dict(doc); doc["style"]=cell[1]
        selected.append(doc); prints.append(print_); ids.add(str(doc["id"])); groups.add(doc.get("group")); counts[cell]+=1
    expected=Counter({(label,s):a.per_cell for label in (0,1) for s in ("official","conversational")})
    if counts != expected:
        raise SystemExit("insufficient cells: "+json.dumps({f"{k[0]}:{k[1]}":v for k,v in counts.items()},sort_keys=True))
    a.out.parent.mkdir(parents=True,exist_ok=True)
    with a.out.open("x") as f:
        for doc in selected: f.write(json.dumps(doc,ensure_ascii=False)+"\n")
    manifest={"documents":len(selected),"per_cell":a.per_cell,"cells":{f"{k[0]}:{k[1]}":v for k,v in sorted(counts.items())},"sources":Counter(d.get("source","unknown") for d in selected),"excluded":excluded,"deduplication":"normalized exact or 5-token shingle Jaccard >= 0.80"}
    Path(str(a.out)+".manifest.json").write_text(json.dumps(manifest,indent=2,default=dict)+"\n")
    print(json.dumps(manifest,default=dict))

if __name__ == "__main__": main()
