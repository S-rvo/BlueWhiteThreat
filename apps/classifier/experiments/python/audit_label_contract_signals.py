#!/usr/bin/env python3
"""Aggregate diagnostic: historical labels vs literal signals and feed provenance."""
import json,pathlib,re,hashlib,collections
base=pathlib.Path('apps/classifier/data');src=base/'iteration-2026-10-23-representation-comparison/development.jsonl';rows=[json.loads(x) for x in src.read_text().splitlines()];patterns={'vulnerability_or_advisory_marker':r'\bvulnerab\w*|CVE-20|GHSA-|\bexploit\b|\bsecurity flaw\b','concrete_impact_or_mechanism':r'\ballows?\b|\bcould\b|\battacker\b|\bdenial.of.service\b|\bunauthori[sz]ed\b|\baccess control\b','ransomware_or_compromise_marker':r'\bransomware\b|\bransom\b|\bextort\w*|\bexfiltrat\w*|\bstolen\b|\bbreach\b|\bvictim\b'};out={}
for source in sorted({r['source'] for r in rows}):
 out[source]={}
 for label in (0,1):
  subset=[r for r in rows if r['source']==source and r['label']==label];feature_counts={k:sum(bool(re.search(p,r['text'],re.I)) for r in subset) for k,p in patterns.items()};any_signal=sum(any(re.search(p,r['text'],re.I) for p in patterns.values()) for r in subset);out[source][str(label)]={'documents':len(subset),'literal_signal_counts':feature_counts,'any_signal_count':any_signal,'no_signal_count':len(subset)-any_signal}
report={'input_sha256':hashlib.sha256(src.read_bytes()).hexdigest(),'patterns':patterns,'strata':out,'semantics':'Regex matches are diagnostic features, not labels. Missing a match is not evidence of irrelevance. Source membership is provenance/weak supervision, not fact verification.','no_labels_changed':True,'training_run':False,'reserve_read':False};dest=base/'iteration-2026-10-25-source-label-audit/label-contract-signal-audit.json';dest.write_text(json.dumps(report,indent=2)+'\n');print(json.dumps(out,indent=2))
