import unittest
from annotation_v5 import POLICY, consensus, request_for, validate

CRITERIA = {
    'incident_or_intrusion': False,
    'stolen_or_leaked_data': False,
    'compromised_access': False,
    'malicious_campaign_or_service': False,
    'concrete_vulnerability_or_exploit': False,
    'threat_tied_defense': False,
}

class StructuredAnnotationV5Test(unittest.TestCase):
    def answer(self, **updates):
        result = {**CRITERIA, 'reason': 'supported decision', 'evidence_ids': [0]}
        result.update(updates)
        return result

    def test_class_is_derived_from_atomic_criteria(self):
        _, parts = request_for({'text': 'A concrete command injection vulnerability.'}, 'qwen3:8b')
        positive = validate(self.answer(concrete_vulnerability_or_exploit=True), parts)
        negative = validate(self.answer(), parts)
        self.assertEqual(positive['label'], 1)
        self.assertEqual(negative['label'], 0)

    def test_schema_has_no_model_selected_label(self):
        request, _ = request_for({'text': 'sample'}, 'qwen3:8b')
        self.assertNotIn('label', request['format']['properties'])
        self.assertIn('concrete_vulnerability_or_exploit', request['format']['required'])

    def test_invalid_or_missing_criterion_is_rejected(self):
        _, parts = request_for({'text': 'sample'}, 'qwen3:8b')
        bad = self.answer()
        bad['incident_or_intrusion'] = 1
        with self.assertRaises(ValueError): validate(bad, parts)
        missing = self.answer(); del missing['compromised_access']
        with self.assertRaises(ValueError): validate(missing, parts)

    def test_policy_explicitly_covers_vulnerability(self):
        self.assertIn('concrete vulnerability', POLICY)

if __name__ == '__main__': unittest.main()
