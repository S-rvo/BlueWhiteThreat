#!/usr/bin/env python3
"""Export v10 text-CTI weak labels with their uncertainty and provenance kept explicit."""
import collections
import hashlib
import json
from pathlib import Path

BASE = Path('apps/classifier/data/iteration-2026-10-25-source-label-audit')
DEVELOPMENT = Path('apps/classifier/data/iteration-2026-10-23-representation-comparison/development.jsonl')
WEAK = BASE / 'text-evidence-weak-labels-v10.jsonl'
OUTPUT = BASE / 'text-cti-evidence-candidates-v10.jsonl'
SUMMARY = BASE / 'text-cti-evidence-candidates-v10-summary.json'
METHOD = 'deterministic_regex_weak_v10'


def export_record(document, weak):
    if document['id'] != weak['id']:
        raise ValueError('id_mismatch')
    text = document['text']
    if hashlib.sha256(text.encode()).hexdigest() != weak['text_sha256']:
        raise ValueError('text_hash_mismatch')
    candidate = weak['candidate']
    label = {'weak_positive': 1, 'weak_negative': 0}.get(candidate)
    return {
        'schema_version': 1,
        'id': document['id'],
        'text': text,
        'text_sha256': weak['text_sha256'],
        'target': 'text_cti_evidence',
        'candidate_label': label,
        'candidate_status': candidate,
        'candidate_method': METHOD,
        'candidate_rule': weak['rule'],
        'matched_cues': weak['matched'],
        'training_eligible': False,
        'historical_label_separate': int(document['label']),
        'provenance': {
            'source': document.get('source'),
            'source_type': document.get('source_type'),
            'group': document.get('group'),
            'source_actor': document.get('source_actor'),
            'source_date': document.get('source_date'),
            'collected_at': document.get('collected_at'),
        },
        'feature_policy': 'text only; provenance and historical_label_separate are metadata, not features',
    }


def main():
    documents = [json.loads(x) for x in DEVELOPMENT.read_text().splitlines() if x.strip()]
    weak_rows = [json.loads(x) for x in WEAK.read_text().splitlines() if x.strip()]
    if len(documents) != len(weak_rows) or len({d['id'] for d in documents}) != len(documents):
        raise ValueError('input_cardinality_or_id_conflict')
    by_id = {d['id']: d for d in documents}
    rows = [export_record(by_id[w['id']], w) for w in weak_rows]
    counts = collections.Counter(row['candidate_status'] for row in rows)
    with OUTPUT.open('x') as out:
        for row in rows:
            out.write(json.dumps(row, ensure_ascii=False) + '\n')
    OUTPUT.chmod(0o600)
    report = {
        'schema_version': 1,
        'input_development_sha256': hashlib.sha256(DEVELOPMENT.read_bytes()).hexdigest(),
        'input_weak_labels_sha256': hashlib.sha256(WEAK.read_bytes()).hexdigest(),
        'output_sha256': hashlib.sha256(OUTPUT.read_bytes()).hexdigest(),
        'documents': len(rows),
        'candidate_counts': dict(counts),
        'target': 'text_cti_evidence',
        'candidate_method': METHOD,
        'all_training_eligible': False,
        'historical_labels_modified': False,
        'production_model_modified': False,
        'warning': 'Weak regex candidates only; abstentions remain null and all rows are blocked from training pending independent validation.',
    }
    with SUMMARY.open('x') as out:
        out.write(json.dumps(report, indent=2) + '\n')
    SUMMARY.chmod(0o600)
    print(json.dumps(report, indent=2))


if __name__ == '__main__':
    main()
