import json,tempfile,unittest
from pathlib import Path
from unittest.mock import patch
from annotate_autoeval import sha
from evaluate_candidate import batch,freeze

class CandidateEvaluationTest(unittest.TestCase):
    def test_only_verified_consensus_enters_metrics(self):
        with tempfile.TemporaryDirectory() as td:
            p=Path(td);docs=[{'id':'a','text':'topic a'},{'id':'b','text':'topic b'}]
            (p/'docs.jsonl').write_text(''.join(json.dumps(x)+'\n' for x in docs))
            votes=[dict(id=d['id'],text_sha256=sha(d['text'].encode()),status='consensus' if d['id']=='a' else 'disagreement',label=1 if d['id']=='a' else None) for d in docs]
            report={'documents':votes,'summary':{'counts':{'pending':0}}}
            (p/'consensus.json').write_text(json.dumps(report))
            original,agreed,summary=batch(p/'docs.jsonl',p/'consensus.json')
            self.assertEqual(len(original),2)
            self.assertEqual([x['id'] for x in agreed],['a'])
            votes[0]['text_sha256']='changed'
            (p/'consensus.json').write_text(json.dumps(report))
            with self.assertRaisesRegex(ValueError,'annotation_text_changed'):batch(p/'docs.jsonl',p/'consensus.json')
    def test_additional_comparison_cannot_be_planned_after_unblinding(self):
        with tempfile.TemporaryDirectory() as td:
            p=Path(td);(p/'campaign').mkdir();(p/'retraining').mkdir()
            (p/'campaign/test-result.json').write_text('{}')
            with patch('evaluate_candidate.BASE',p):
                with self.assertRaisesRegex(ValueError,'original_test_already_unblinded'):freeze()

if __name__=='__main__':unittest.main()
