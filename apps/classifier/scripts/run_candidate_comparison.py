#!/usr/bin/env python3
"""Resume the already frozen candidate comparison without selecting on test data."""
import argparse,json,subprocess,sys,time
from pathlib import Path
from evaluate_candidate import BASE
from annotate_autoeval import sha

def main():
    p=argparse.ArgumentParser(description=__doc__);p.add_argument('--wait-original',action='store_true');args=p.parse_args()
    r=BASE/'retraining';scripts=Path(__file__).resolve().parent
    plan=json.loads((r/'fixed-test-models.json').read_text())
    for m in plan['models'].values():
        if sha((r/m['file']).read_bytes())!=m['sha256']:raise ValueError('frozen_model_changed')
    if (r/'comparison-result.json').exists():
        result=json.loads((r/'comparison-result.json').read_text())
        if result['plan_sha256']!=sha((r/'fixed-test-models.json').read_bytes()):raise ValueError('completed_comparison_changed')
        print(json.dumps(result));return
    deadline=time.monotonic()+3600
    while not (BASE/'campaign/test-result.json').exists():
        if not args.wait_original:raise ValueError('complete the original frozen campaign first')
        if time.monotonic()>=deadline:raise TimeoutError('original campaign not complete; preserve and resume')
        time.sleep(5)
    if (r/'comparison-consumption.json').exists():raise ValueError('interrupted_test_comparison; preserve receipt')
    subprocess.run([sys.executable,'-B',str(scripts/'annotation_v2.py'),'--input',str(r/'test.jsonl'),'--out',str(r/'test-annotations')],check=True)
    subprocess.run([sys.executable,'-B',str(scripts/'evaluate_candidate.py'),'evaluate'],check=True)
if __name__=='__main__':main()
