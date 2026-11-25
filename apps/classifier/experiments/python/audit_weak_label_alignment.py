#!/usr/bin/env python3
"""Compare weak-label proposals with historical labels; historical labels are not gold."""
import argparse, collections, json, pathlib
BASE = pathlib.Path('apps/classifier/data/iteration-2026-10-25-source-label-audit')
INPUT = BASE / 'text-evidence-weak-labels-v6.jsonl'
OUTPUT = BASE / 'text-evidence-weak-label-alignment-v6.json'

def summarize(rows):
    by_source = collections.defaultdict(lambda: collections.Counter())
    disagreement_rules = collections.defaultdict(collections.Counter)
    total_disagreement_rules = collections.Counter()
    total = collections.Counter()
    for row in rows:
        source = row['source']
        candidate = row['candidate']
        group = by_source[source]
        group['total'] += 1
        total['total'] += 1
        if candidate == 'abstain':
            group['abstain'] += 1
            total['abstain'] += 1
            continue
        prediction = int(candidate == 'weak_positive')
        agrees = prediction == int(row['historical_label'])
        group['eligible'] += 1
        total['eligible'] += 1
        group['historical_agreement'] += int(agrees)
        total['historical_agreement'] += int(agrees)
        group['historical_disagreement'] += int(not agrees)
        total['historical_disagreement'] += int(not agrees)
        if not agrees:
            for rule in row.get('matched', {}):
                disagreement_rules[source][rule] += 1
                total_disagreement_rules[rule] += 1
        group[candidate] += 1
        total[candidate] += 1
    def finalize(counter):
        result = dict(counter)
        eligible = result.get('eligible', 0)
        result['coverage'] = eligible / result['total'] if result.get('total') else 0.0
        result['agreement_rate_on_nonabstentions'] = result.get('historical_agreement', 0) / eligible if eligible else None
        return result
    return {'total': finalize(total), 'by_source': {key: finalize(value) for key, value in sorted(by_source.items())},
            'historical_disagreement_matched_rules': {'total': dict(total_disagreement_rules),
                'by_source': {key: dict(value) for key, value in sorted(disagreement_rules.items())}},
            'interpretation': 'Agreement is only against historical noisy labels; it is not accuracy or validation against ground truth.',
            'used_for_training': False}

def main():
    parser = argparse.ArgumentParser()
    parser.add_argument('--input', type=pathlib.Path, default=INPUT)
    parser.add_argument('--output', type=pathlib.Path, default=OUTPUT)
    args = parser.parse_args()
    rows = [json.loads(line) for line in args.input.read_text().splitlines() if line.strip()]
    report = summarize(rows)
    args.output.write_text(json.dumps(report, indent=2) + '\n')
    print(json.dumps(report, indent=2))

if __name__ == '__main__':
    main()
