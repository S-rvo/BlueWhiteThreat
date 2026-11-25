#!/usr/bin/env python3
"""Freeze a new blind AI-reference campaign; calibrate on validation only.
All texts, individual scores and votes stay in ignored private campaign storage.
"""
import argparse
import csv
import json
import math
from pathlib import Path
import random
import shutil
import subprocess
import tempfile
from annotate_autoeval import atomic_json, sha

ROOT = Path(__file__).resolve().parents[1]
csv.field_size_limit(16 * 1024 * 1024)
SEED = 42
TARGETS = dict(recall=.9, precision=.5, coverage=.8, per_class=50,
               per_stratum_class=10)

def lines(path):
    return [json.loads(x) for x in Path(path).read_text().split('\n') if x.strip()]

def write_lines(path, rows):
    with Path(path).open('x') as f:
        for row in rows:
            f.write(json.dumps(row, ensure_ascii=False)+'\n')
    Path(path).chmod(0o600)

def fingerprints(binary, docs):
    with tempfile.TemporaryDirectory() as td:
        inp=Path(td)/'docs.jsonl'
        write_lines(inp, docs)
        output=subprocess.check_output([str(binary), '--input',str(inp)], text=True)
    return {r['id']:r for r in (json.loads(x) for x in output.split('\n') if x)}

def near(a,b):
    return a['id']==b['id'] or a['normalized']==b['normalized'] or (int(a['fingerprint'],16)^int(b['fingerprint'],16)).bit_count() <= 10

def freeze(args):
    if args.dir.exists():
        raise ValueError('campaign_directory_already_exists')
    with (ROOT/'training_data.csv').open() as source:
        refs=list(csv.DictReader(source))
    refs += json.loads((ROOT/'../../docs/validation/nlp-synthetic-fixtures-2026-10-19.json').read_text())
    refs += lines(args.development)
    with (ROOT/'data/reviewed_labels.csv').open() as source:
        banned={r['id'] for r in csv.DictReader(source)}
    banned |= {r['id'] for r in refs}
    pool=[]
    for path in args.pool:
        pool += lines(path)
    if len({r['id'] for r in pool}) != len(pool):
        raise ValueError('duplicate_pool_id')
    rng=random.Random(SEED)
    pool.sort(key=lambda r:r['id'])
    rng.shuffle(pool)
    refprints=list(fingerprints(args.binary, refs).values())
    prints=fingerprints(args.binary,pool)
    eligible=[]; excluded={}
    for d in pool:
        if not all(d.get(k) for k in ['id','text','group','stratum','source','source_type','language','url','collected_at']):
            raise ValueError('missing_provenance')
        reason=None
        if d['id'] in banned: reason='known_id'
        elif any(near(prints[d['id']],p) for p in refprints): reason='reference_near_duplicate'
        elif any(d['group']==e['group'] or near(prints[d['id']], prints[e['id']]) for e in eligible): reason='pool_near_duplicate'
        if reason: excluded[reason]=excluded.get(reason,0)+1
        else: eligible.append(d)
    # Score-blind equal allocation across source families; the pool's source
    # and stratum metadata are fixed before annotation. No label balancing.
    strata={s:[d for d in eligible if d['stratum']==s] for s in sorted({d['stratum'] for d in eligible})}
    by_source={s:[d for d in eligible if d['source']==s] for s in sorted({d['source'] for d in eligible})}
    selected=[]; positions={s:0 for s in by_source}
    while len(selected)<400:
        progressed=False
        for source,rows in by_source.items():
            if positions[source]<len(rows):
                selected.append(rows[positions[source]])
                positions[source]+=1
                progressed=True
                if len(selected)==400: break
        if not progressed: break
    if len(selected)!=400:
        raise ValueError('insufficient_stratified_novel_pool: '+json.dumps({k:len(v) for k,v in strata.items()}))
    # Alternate each source×stratum group into the two partitions.
    validation=[];test=[]
    split_groups=sorted({(d['source'],d['stratum']) for d in selected})
    for source,stratum in split_groups:
        rows=[d for d in selected if d['source']==source and d['stratum']==stratum]
        for d in rows:
            (validation if len(validation)<=len(test) else test).append(d)
    if len(validation)!=200 or len(test)!=200:raise ValueError('invalid_split')
    rng.shuffle(validation);rng.shuffle(test)
    args.dir.mkdir(mode=0o700,parents=True)
    write_lines(args.dir/'validation.jsonl',validation)
    write_lines(args.dir/'test.jsonl',test)
    shutil.copyfile(ROOT/'model.json',args.dir/'model.json')
    (args.dir/'model.json').chmod(0o600)
    shutil.copyfile(__file__,args.dir/'automatic_campaign.snapshot.py')
    shutil.copyfile(args.binary,args.dir/'evaldocuments')
    (args.dir/'evaldocuments').chmod(0o700)
    manifest={'schema':2,'seed':SEED,'targets':TARGETS,'label_authority':'two local AI models, not human ground truth',
      'pool_documents':len(pool),'eligible':len(eligible),'exclusions':excluded,'eligible_strata':{k:len(v) for k,v in strata.items()},
      'split_strata':{name:{s:sum(d['stratum']==s for d in rows) for s in strata} for name,rows in [('validation',validation),('test',test)]},
      'hashes':{p.name:sha(p.read_bytes()) for p in args.dir.iterdir() if p.is_file()},
      'input_hashes':{str(p):sha(p.read_bytes()) for p in [*args.pool,args.development,ROOT/'training_data.csv',ROOT/'data/reviewed_labels.csv']},
      'selection':'score-blind equal round-robin across source families; SimHash distance <=10 excluded against reference and pool; group disjoint',
      'limitations':['AI consensus can share errors.','Lexical deduplication does not prove event independence.','Public Q&A and advisories are not representative of Tor forums.','French advisories are a supplement, not forum posts.'],
      'production_promotion':False}
    atomic_json(args.dir/'manifest.json',manifest,exclusive=True)
    print(json.dumps(manifest))

def wilson(success,total):
    if not total:return None
    z=1.959963984540054;p=success/total;den=1+z*z/total
    center=(p+z*z/(2*total))/den
    delta=z*math.sqrt(p*(1-p)/total+z*z/(4*total*total))/den
    return [max(0,center-delta),min(1,center+delta)]

def metrics(rows,threshold):
    tp=sum(r['label']==1 and r['score']>=threshold for r in rows)
    fp=sum(r['label']==0 and r['score']>=threshold for r in rows)
    fn=sum(r['label']==1 and r['score']<threshold for r in rows)
    tn=sum(r['label']==0 and r['score']<threshold for r in rows)
    div=lambda a,b:a/b if b else None
    return dict(n=len(rows),tp=tp,fp=fp,fn=fn,tn=tn,precision=div(tp,tp+fp),recall=div(tp,tp+fn),
        f1=div(2*tp,2*tp+fp+fn),precision_ci95=wilson(tp,tp+fp),recall_ci95=wilson(tp,tp+fn),
        transmission_rate=div(tp+fp,len(rows)))

def group_support(rows, key, minimum=TARGETS['per_stratum_class']):
    """Count reference-class support within each source or campaign stratum.

    This blocks aggregate metrics from passing when source/stratum is a proxy
    for the label and prevents interpreting one-class subgroup metrics.
    """
    result={}
    for value in sorted({r.get(key,'') for r in rows}):
        group=[r for r in rows if r.get(key,'')==value]
        positives=sum(r['label']==1 for r in group)
        negatives=sum(r['label']==0 for r in group)
        result[value]={'positive':positives,'negative':negatives,
                         'passed':positives>=minimum and negatives>=minimum}
    return result

def groups_supported(rows, key, minimum=TARGETS['per_stratum_class']):
    support=group_support(rows,key,minimum)
    return bool(support) and all(s['passed'] for s in support.values())

def verify(directory):
    manifest=json.loads((directory/'manifest.json').read_text())
    for name,digest in manifest['hashes'].items():
        if sha((directory/name).read_bytes())!=digest:raise ValueError('frozen_file_changed: '+name)
    return manifest

def scored(directory,split):
    docs=lines(directory/(split+'.jsonl'))
    annotation=directory/(split+'-annotations')/'consensus.json'
    consensus=json.loads(annotation.read_text())
    if consensus['summary']['counts']['pending']:raise ValueError('annotations_incomplete')
    byid={r['id']:r for r in consensus['documents']}
    if set(byid)!={d['id'] for d in docs}:raise ValueError('annotation_ids_mismatch')
    output=subprocess.check_output([str((directory/'evaldocuments').resolve()),'--mode','scores','--input',str(directory/(split+'.jsonl')),'--model',str(directory/'model.json')],text=True)
    scores={r['id']:r['score'] for r in (json.loads(x) for x in output.split('\n') if x)}
    rows=[]
    for d in docs:
        vote=byid[d['id']]
        if vote['text_sha256']!=sha(d['text'].encode()):raise ValueError('annotation_text_changed')
        if vote['status']=='consensus':rows.append({**d,'label':vote['label'],'score':scores[d['id']]})
    return rows,consensus,annotation

def calibrate(args):
    verify(args.dir)
    if (args.dir/'calibration.json').exists():raise ValueError('calibration_already_frozen')
    rows,consensus,path=scored(args.dir,'validation')
    thresholds=sorted({0.,1.,*[r['score'] for r in rows]})
    candidates=[(t,metrics(rows,t)) for t in thresholds]
    validation_source_supported=groups_supported(rows,'source')
    validation_strata_supported=groups_supported(rows,'stratum')
    qualified=[(t,m) for t,m in candidates if validation_source_supported and validation_strata_supported and (m['recall'] or 0)>=TARGETS['recall'] and (m['precision'] or 0)>=TARGETS['precision']]
    # Fixed objective: among recall>=.90 and precision>=.50 maximize precision,
    # break ties by recall then larger threshold. If infeasible, do not deploy.
    chosen=max(qualified,key=lambda x:(x[1]['precision'],x[1]['recall'],x[0])) if qualified else max(candidates,key=lambda x:(x[1]['f1'] or 0,x[0]))
    report={'threshold':chosen[0],'validation_metrics':chosen[1],'validation_coverage':len(rows)/200,'validation_constraints_feasible':bool(qualified),
       'validation_source_support':group_support(rows,'source'),'validation_sources_supported':validation_source_supported,
       'validation_stratum_support':group_support(rows,'stratum'),'validation_strata_supported':validation_strata_supported,
       'annotation_sha256':sha(path.read_bytes()),'annotation_plan_sha256':sha(path.with_name('plan.json').read_bytes()),
       'selection':'maximum precision subject to recall>=.90, precision>=.50; fallback maximum F1 if infeasible',
       'script_sha256':sha(Path(__file__).read_bytes()),'test_consumed':False,'production_promotion':False}
    (args.dir/'calibration-implementation.snapshot.py').write_bytes(Path(__file__).read_bytes())
    atomic_json(args.dir/'calibration.json',report,exclusive=True)
    print(json.dumps(report))

def evaluate(args):
    manifest=verify(args.dir)
    calibration_path=args.dir/'calibration.json'
    calibration=json.loads(calibration_path.read_text())
    if calibration['script_sha256']!=sha(Path(__file__).read_bytes()):raise ValueError('evaluation_script_changed')
    if (args.dir/'test-result.json').exists() or (args.dir/'test-consumption.json').exists():raise ValueError('test_already_consumed')
    # Reserve test exactly once after complete annotations have been verified.
    rows,consensus,path=scored(args.dir,'test')
    atomic_json(args.dir/'test-consumption.json',{'calibration_sha256':sha(calibration_path.read_bytes()),'annotations_sha256':sha(path.read_bytes())},exclusive=True)
    baseline=json.loads((args.dir/'model.json').read_text())['threshold']
    threshold=calibration['threshold']; m=metrics(rows,threshold)
    coverage=len(rows)/200
    enough=m['tp']+m['fn']>=50 and m['tn']+m['fp']>=50
    test_source_supported=groups_supported(rows,'source')
    test_strata_supported=groups_supported(rows,'stratum')
    passes=calibration['validation_constraints_feasible'] and calibration.get('validation_sources_supported',True) and calibration.get('validation_strata_supported',True) and test_source_supported and test_strata_supported and calibration['validation_coverage']>=.8 and coverage>=.8 and enough and (m['recall'] or 0)>=.9 and (m['precision'] or 0)>=.5
    report={'label_authority':manifest['label_authority'],'threshold':threshold,'baseline_threshold':baseline,
       'baseline':metrics(rows,baseline),'candidate':m,'annotation':consensus['summary'],'coverage':coverage,'minimum_class_counts_met':enough,
       'test_source_support':group_support(rows,'source'),'test_sources_supported':test_source_supported,
       'test_stratum_support':group_support(rows,'stratum'),'test_strata_supported':test_strata_supported,
       'status':'exploratory_targets_met' if passes else 'not_qualified','production_promotion':False,
       'by_language':{s:metrics([r for r in rows if r['language']==s],threshold) for s in sorted({r['language'] for r in rows})},
       'by_stratum':{s:metrics([r for r in rows if r['stratum']==s],threshold) for s in sorted({r['stratum'] for r in rows})},
       'limitations':manifest['limitations'],'hashes':{'manifest':sha((args.dir/'manifest.json').read_bytes()),'calibration':sha(calibration_path.read_bytes()),'consensus':sha(path.read_bytes())}}
    atomic_json(args.dir/'test-result.json',report,exclusive=True)
    candidate=json.loads((args.dir/'model.json').read_text());candidate['threshold']=threshold
    candidate['version']=candidate.get('version','')+'-autoeval-candidate'
    vm=calibration['validation_metrics']
    candidate['validation']={'documents':vm['n'],'precision':vm['precision'] or 0,'recall':vm['recall'] or 0,'f1':vm['f1'] or 0,'target_precision':.5,'minimum_recall':.9,'objective_met':calibration['validation_constraints_feasible'],'selection_rule':calibration['selection'],'label_authority':manifest['label_authority']}
    atomic_json(args.dir/'candidate-model.json',candidate,exclusive=True)
    print(json.dumps(report))

def main():
    p=argparse.ArgumentParser(description=__doc__)
    p.add_argument('action',choices=['freeze','calibrate','evaluate'])
    p.add_argument('--dir',type=Path,required=True)
    p.add_argument('--pool',type=Path,action='append',default=[])
    p.add_argument('--development',type=Path)
    p.add_argument('--binary',type=Path,default=Path('/tmp/bwt-evaldocuments'))
    args=p.parse_args()
    {'freeze':freeze,'calibrate':calibrate,'evaluate':evaluate}[args.action](args)
if __name__=='__main__':main()
