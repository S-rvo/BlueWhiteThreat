#!/usr/bin/env python3
"""Reserve weak training advisories and an independent blind challenge test.
No NLP score or AI label is read. The original 400-document campaign is excluded.
"""
import csv,json,random,shutil
from pathlib import Path
from automatic_campaign import ROOT, fingerprints, near, lines, write_lines
from annotate_autoeval import atomic_json,sha

def main():
    base=ROOT/'data/automatic-v2-2026-10-19'
    output=base/'retraining'
    if output.exists():raise ValueError('reserve_already_exists')
    refs=list(csv.DictReader((ROOT/'training_data.csv').open()))
    refs+=json.loads((ROOT/'../../docs/validation/nlp-synthetic-fixtures-2026-10-19.json').read_text())
    refs+=lines(base/'development.jsonl')+lines(base/'campaign/validation.jsonl')+lines(base/'campaign/test.jsonl')
    banned={r['id'] for r in csv.DictReader((ROOT/'data/reviewed_labels.csv').open())}
    binary=Path('/tmp/bwt-evaldocuments')
    prior=list(fingerprints(binary,refs).values())
    pool=lines(base/'advisory-reserve-expanded.jsonl')+lines(base/'general-reserve.jsonl')
    prints=fingerprints(binary,pool)
    pool.sort(key=lambda x:x['id']);random.Random(43).shuffle(pool)
    eligible={'en_advisory':[],'en_general':[]};seen=[];groups=set();excluded=0
    for d in pool:
        if d['id'] in banned or d['group'] in groups or any(near(prints[d['id']],p) for p in prior+seen):
            excluded+=1;continue
        # Only structured, CVE-identified reviewed advisories are weak positives.
        if d['stratum']=='en_advisory' and not d['group'].startswith('cve:'):continue
        eligible[d['stratum']].append(d);seen.append(prints[d['id']]);groups.add(d['group'])
    counts={k:len(v) for k,v in eligible.items()}
    if counts['en_advisory']<200 or counts['en_general']<100:raise ValueError('insufficient_novel_reserve: '+json.dumps(counts))
    train=eligible['en_advisory'][:100]
    test=eligible['en_advisory'][100:200]+eligible['en_general'][:100]
    random.Random(44).shuffle(test)
    output.mkdir(mode=0o700)
    write_lines(output/'weak-training.jsonl',[{**d,'label':1,'label_authority':'weak positive from reviewed CVE advisory metadata; not independent AI truth'} for d in train])
    write_lines(output/'test.jsonl',test)
    shutil.copyfile(__file__,output/'preparation.snapshot.py')
    atomic_json(output/'reserve-plan.json',{'seed':43,'shuffle_seed':44,'eligible':counts,'excluded':excluded,'weak_training':100,'test':200,
      'test_strata':{'en_advisory':100,'en_general':100},'source_distribution':'balanced challenge, not production prevalence',
      'reference_includes_original_400_documents':True,'weak_labels_used_only_for_training':True,'model_scored':False,
      'reference_sha256':sha(json.dumps(refs,sort_keys=True).encode()),'hashes':{p.name:sha(p.read_bytes()) for p in output.iterdir() if p.is_file()}},exclusive=True)
    print(json.dumps({'eligible':counts,'test':200,'weak_training':100,'directory':str(output)}))
if __name__=='__main__':main()
