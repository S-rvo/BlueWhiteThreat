#!/usr/bin/env python3
"""Resume a frozen campaign through blind annotation, calibration and held-out test.
Requires the frozen documents and two local Ollama model artifacts. No production
model is replaced. A consumed test is never silently reopened.
"""
import argparse
import json
from pathlib import Path
import subprocess
import sys

from annotate_autoeval import sha
from automatic_campaign import verify


def run(directory):
    directory=directory.resolve()
    verify(directory)
    scripts=Path(__file__).resolve().parent
    calibration_path=directory/'calibration.json'
    result_path=directory/'test-result.json'
    if result_path.exists():
        result=json.loads(result_path.read_text())
        if result['hashes']['calibration']!=sha(calibration_path.read_bytes()):
            raise ValueError('completed_calibration_changed')
        if result['hashes']['consensus']!=sha((directory/'test-annotations/consensus.json').read_bytes()):
            raise ValueError('completed_test_annotations_changed')
        print(json.dumps(result))
        return
    if (directory/'test-consumption.json').exists():
        raise ValueError('test_consumed_without_completed_report; preserve artifacts, start a new independent campaign')
    for split,action in [('validation','calibrate'),('test','evaluate')]:
        subprocess.run([sys.executable,'-B',str(scripts/'annotation_v2.py'),'--input',str(directory/(split+'.jsonl')),
            '--out',str(directory/(split+'-annotations'))],check=True)
        if action=='calibrate' and calibration_path.exists():
            calibration=json.loads(calibration_path.read_text())
            annotation=directory/'validation-annotations'
            if calibration['annotation_sha256']!=sha((annotation/'consensus.json').read_bytes()) or calibration['annotation_plan_sha256']!=sha((annotation/'plan.json').read_bytes()):
                raise ValueError('frozen_validation_annotations_changed')
            continue
        subprocess.run([sys.executable,'-B',str(scripts/'automatic_campaign.py'),action,'--dir',str(directory)],check=True)

if __name__=='__main__':
    parser=argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--dir',type=Path,required=True)
    run(parser.parse_args().dir)
