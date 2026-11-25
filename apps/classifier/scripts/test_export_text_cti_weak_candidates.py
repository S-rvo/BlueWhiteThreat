import unittest
from export_text_cti_weak_candidates import export_record

class ExportWeakCandidateTest(unittest.TestCase):
    def setUp(self):
        self.document = {'id':'x','text':'A vulnerability allows an attacker to read files.','label':0,
                         'source':'github-advisories','source_type':'advisory','group':'g'}
        import hashlib
        self.weak = {'id':'x','text_sha256':hashlib.sha256(self.document['text'].encode()).hexdigest(),
                     'candidate':'weak_positive','rule':'explicit_vulnerability_and_impact',
                     'matched':{'vulnerability_mechanism':True,'impact_mechanism':True}}
    def test_keeps_candidate_separate_from_historical_and_blocks_training(self):
        row = export_record(self.document,self.weak)
        self.assertEqual(row['target'],'text_cti_evidence')
        self.assertEqual(row['candidate_label'],1)
        self.assertEqual(row['historical_label_separate'],0)
        self.assertFalse(row['training_eligible'])
        self.assertIn('not features',row['feature_policy'])
    def test_abstention_exports_null_label(self):
        weak = dict(self.weak,candidate='abstain',rule='uncertain',matched={})
        row = export_record(self.document,weak)
        self.assertIsNone(row['candidate_label'])
        self.assertEqual(row['candidate_status'],'abstain')
    def test_rejects_changed_text(self):
        weak = dict(self.weak,text_sha256='0'*64)
        with self.assertRaisesRegex(ValueError,'text_hash_mismatch'):
            export_record(self.document,weak)

if __name__ == '__main__': unittest.main()
