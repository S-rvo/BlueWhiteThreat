#!/usr/bin/env python3
"""Experimental batch stdio bridge. Read JSONL, emit JSONL, diagnostics on stderr."""
import argparse,json,sys
from pathlib import Path
from score_frozen_lexical import load_candidate,score

def main():
    p=argparse.ArgumentParser();p.add_argument('--candidate',type=Path,required=True);a=p.parse_args()
    candidate=load_candidate(a.candidate)
    rows=[json.loads(l) for l in sys.stdin if l.strip()]
    for r in score(candidate,rows):print(json.dumps(r))
if __name__=='__main__':main()
