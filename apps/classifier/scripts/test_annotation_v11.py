import unittest
from annotation_v11 import POLICY, consensus, request_for, validate

CRITERIA = ('incident_or_intrusion', 'stolen_or_leaked_data', 'compromised_access',
            'malicious_campaign_or_service', 'concrete_vulnerability_or_exploit', 'threat_tied_defense')

class EvidenceGroundedAnnotationV11Test(unittest.TestCase):
    def answer(self, positive=None, evidence=None):
        positive = positive or {}
        evidence = evidence or {}
        return {
            'criteria': {name: bool(positive.get(name, False)) for name in CRITERIA},
            'evidence_by_criterion': {name: evidence.get(name, []) for name in CRITERIA},
            'reason': 'criterion-level evidence checked',
        }

    def test_label_is_derived_from_atomic_criteria(self):
        _, parts = request_for({'text': 'A concrete command injection vulnerability.'}, 'qwen3:8b')
        good = self.answer({'concrete_vulnerability_or_exploit': True}, {'concrete_vulnerability_or_exploit': [0]})
        self.assertEqual(validate(good, parts)['label'], 1)
        self.assertEqual(validate(self.answer(), parts)['label'], 0)

    def test_true_criterion_requires_its_own_citation(self):
        _, parts = request_for({'text': 'A breach exposed records.'}, 'qwen3:8b')
        bad = self.answer({'stolen_or_leaked_data': True})
        with self.assertRaisesRegex(ValueError, 'criterion_missing_evidence'):
            validate(bad, parts)

    def test_false_criterion_cannot_carry_positive_evidence(self):
        _, parts = request_for({'text': 'A breach exposed records.'}, 'qwen3:8b')
        bad = self.answer(evidence={'stolen_or_leaked_data': [0]})
        with self.assertRaisesRegex(ValueError, 'evidence_for_false_criterion'):
            validate(bad, parts)

    def test_invalid_segment_reference_is_rejected(self):
        _, parts = request_for({'text': 'A concrete command injection vulnerability.'}, 'qwen3:8b')
        bad = self.answer({'concrete_vulnerability_or_exploit': True}, {'concrete_vulnerability_or_exploit': [1]})
        with self.assertRaisesRegex(ValueError, 'invalid_evidence_id'):
            validate(bad, parts)

    def test_schema_requires_evidence_mapping_and_has_no_free_label(self):
        request, _ = request_for({'text': 'sample'}, 'qwen3:8b')
        self.assertEqual(set(request['format']['required']), {'criteria', 'evidence_by_criterion', 'reason'})
        self.assertNotIn('label', request['format']['properties'])
        self.assertIn('Evidence-grounding v11', POLICY)

    def test_consensus_authority_is_correctly_versioned(self):
        r = consensus([{'id':'d','text':'x'}], {('d',m):{'status':'decided','label':0} for m in ('a','b','c')}, ('a','b','c'))
        self.assertIn('policy v11', r['summary']['label_authority'])

    def test_majority_coverage_is_not_reported_as_unanimity(self):
        models = ('a', 'b', 'c')
        votes = {('d', 'a'): {'status': 'decided', 'label': 1},
                 ('d', 'b'): {'status': 'decided', 'label': 1},
                 ('d', 'c'): {'status': 'decided', 'label': 0}}
        r = consensus([{'id': 'd', 'text': 'x'}], votes, models)
        self.assertEqual(r['documents'][0]['status'], 'consensus')
        self.assertEqual(r['documents'][0]['vote_agreement'], 'majority_only')
        self.assertEqual(r['summary']['vote_agreement_counts']['unanimous'], 0)
        self.assertEqual(r['summary']['pairwise_agreement']['a__b']['agreement'], 1.0)
        self.assertEqual(r['summary']['pairwise_agreement']['a__c']['agreement'], 0.0)
        self.assertIsNone(r['documents'][0]['strict_label'])

    def test_strict_label_exists_only_for_unanimous_ballot(self):
        models = ('a', 'b', 'c')
        votes = {('d', m): {'status': 'decided', 'label': 0} for m in models}
        r = consensus([{'id': 'd', 'text': 'x'}], votes, models)
        self.assertEqual(r['documents'][0]['strict_label'], 0)
        self.assertEqual(r['summary']['strict_label_count'], 1)

if __name__ == '__main__':
    unittest.main()
