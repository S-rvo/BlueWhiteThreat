#!/usr/bin/env python3
"""Predeclare three fixed models, then compare on two blind held-out batches.
No model, threshold or training selection happens after test results are read.
"""
import argparse,json,shutil,subprocess,sys
from pathlib import Path
from annotate_autoeval import atomic_json,sha
from automatic_campaign import ROOT,lines,metrics
BASE=ROOT/'data/automatic-v2-2026-10-19'

def freeze():
    r=BASE/'retraining'
    if (r/'fixed-test-models.json').exists():raise ValueError('comparison_already_frozen')
    if (BASE/'campaign/test-result.json').exists():raise ValueError('original_test_already_unblinded')
    frozen=json.loads((r/'candidate-freeze.json').read_text())
    if frozen['model_sha256']!=sha((r/'candidate-oof.json').read_bytes()):raise ValueError('candidate_changed')
    raw=(BASE/'campaign/model.json').read_bytes()
    (r/'legacy-model.json').write_bytes(raw)
    calibrated=json.loads(raw)
    calibrated['threshold']=json.loads((BASE/'campaign/calibration.json').read_text())['threshold']
    calibrated['version']+='-validation-threshold'
    atomic_json(r/'legacy-calibrated-model.json',calibrated,exclusive=True)
    models={name:{'file':path,'sha256':sha((r/path).read_bytes()),'threshold':json.loads((r/path).read_text())['threshold']} for name,path in [('deployed','legacy-model.json'),('threshold_only','legacy-calibrated-model.json'),('retrained','candidate-oof.json')]}
    plan={'models':models,'primary_candidate':'retrained','selection':'already frozen using OOF development data; no test-based selection',
      'original_test_sha256':sha((BASE/'campaign/test.jsonl').read_bytes()),'challenge_test_sha256':sha((r/'test.jsonl').read_bytes()),
      'label_authority':'agreement of two local AI models, not human truth','protocol_sha256':sha((r/'crossfit-plan.json').read_bytes()),
      'evaluation_script_sha256':sha(Path(__file__).read_bytes()),'production_promotion':False}
    shutil.copyfile(__file__,r/'evaluate_candidate.snapshot.py')
    atomic_json(r/'fixed-test-models.json',plan,exclusive=True)
    print(json.dumps(plan))

def batch(batch_path,consensus_path):
    docs=lines(batch_path);consensus=json.loads(consensus_path.read_text())
    if consensus['summary']['counts']['pending']:raise ValueError('annotations_incomplete')
    byid={v['id']:v for v in consensus['documents']}
    if set(byid)!={d['id'] for d in docs}:raise ValueError('annotation_ids_changed')
    agreed=[]
    for d in docs:
        v=byid[d['id']]
        if v['text_sha256']!=sha(d['text'].encode()):raise ValueError('annotation_text_changed')
        if v['status']=='consensus':
            if type(v['label']) is not int or v['label'] not in (0,1):raise ValueError('invalid_consensus_label')
            agreed.append({**d,'label':v['label']})
    return docs,agreed,consensus['summary']

def evaluate():
    r=BASE/'retraining';plan=json.loads((r/'fixed-test-models.json').read_text())
    if plan['evaluation_script_sha256']!=sha(Path(__file__).read_bytes()):raise ValueError('frozen_evaluation_changed')
    for name,m in plan['models'].items():
        if sha((r/m['file']).read_bytes())!=m['sha256']:raise ValueError('fixed_model_changed: '+name)
    original=BASE/'campaign/test.jsonl';challenge=r/'test.jsonl'
    if sha(original.read_bytes())!=plan['original_test_sha256'] or sha(challenge.read_bytes())!=plan['challenge_test_sha256']:raise ValueError('fixed_test_changed')
    batches={name:batch(path,consensus) for name,path,consensus in [('original',original,BASE/'campaign/test-annotations/consensus.json'),('challenge',challenge,r/'test-annotations/consensus.json')]}
    receipt=r/'comparison-consumption.json'
    if receipt.exists():raise ValueError('comparison_already_consumed')
    atomic_json(receipt,{'plan_sha256':sha((r/'fixed-test-models.json').read_bytes()),'test_annotations':{name:sha(path.read_bytes()) for name,path in [('original',BASE/'campaign/test-annotations/consensus.json'),('challenge',r/'test-annotations/consensus.json')]}},exclusive=True)
    result={'label_authority':plan['label_authority'],'models':plan['models'],'tests':{},'production_promotion':False,'selection_on_test':False}
    for name,(docs,agreed,summary) in batches.items():
        path=original if name=='original' else challenge
        details={'annotation':summary,'coverage':len(agreed)/len(docs),'source_distribution':'public question sample' if name=='original' else 'balanced advisory/general challenge, not production prevalence','models':{}}
        for model_name,m in plan['models'].items():
            output=subprocess.check_output([str((BASE/'campaign/evaldocuments').resolve()),'--mode','scores','--input',str(path),'--model',str(r/m['file'])],text=True)
            scores={v['id']:v['score'] for v in (json.loads(x) for x in output.split('\n') if x)}
            rows=[{**d,'score':scores[d['id']]} for d in agreed]
            details['models'][model_name]={'overall':metrics(rows,m['threshold']),'by_stratum':{s:metrics([d for d in rows if d['stratum']==s],m['threshold']) for s in sorted({d['stratum'] for d in rows})}}
        result['tests'][name]=details
    orig=result['tests']['original'];challenge_result=result['tests']['challenge']
    a=orig['models']['retrained']['overall'];b=challenge_result['models']['retrained']['overall']
    specificity=b['tn']/(b['tn']+b['fp']) if b['tn']+b['fp'] else 0
    oof=json.loads((r/'candidate-oof.json.selection.json').read_text())['selected']
    gates={'OOF_constraints':oof['constraints_feasible'],'original_coverage':orig['coverage']>=.8,'original_class_support':a['tp']+a['fn']>=15,
      'original_precision':(a['precision'] or 0)>=.5,'original_recall':(a['recall'] or 0)>=.9,'original_f1_improved':(a['f1'] or 0)>(orig['models']['deployed']['overall']['f1'] or 0),
      'challenge_coverage':challenge_result['coverage']>=.8,'challenge_class_support':min(b['tp']+b['fn'],b['tn']+b['fp'])>=50,
      'challenge_precision':(b['precision'] or 0)>=.5,'challenge_recall':(b['recall'] or 0)>=.9,'challenge_specificity':specificity>=.95}
    result['gates']=gates;result['status']='exploratory_targets_met' if all(gates.values()) else 'not_qualified'
    result['challenge_specificity']=specificity
    result['limitations']=['AI reference can share errors.','Few original positives and French documents give wide uncertainty.','Balanced challenge precision does not estimate production precision.','OOF development is not independent of model selection.','Public advisories/questions do not establish Tor forum performance.']
    result['plan_sha256']=sha((r/'fixed-test-models.json').read_bytes())
    atomic_json(r/'comparison-result.json',result,exclusive=True)
    print(json.dumps(result))

def main():
    p=argparse.ArgumentParser(description=__doc__);p.add_argument('action',choices=['freeze','evaluate']);args=p.parse_args()
    {'freeze':freeze,'evaluate':evaluate}[args.action]()
if __name__=='__main__':main()
