import unittest
from annotation_v4 import POLICY, consensus, request_for, segments, validate


class StructuredAnnotationV4Test(unittest.TestCase):
    def test_schema_requires_class_and_evidence(self):
        request, parts = request_for({'text': 'Full server dump offered for sale.'}, 'qwen3:8b')
        props = request['format']['properties']
        self.assertEqual(props['label'], {'type': 'integer', 'enum': [0, 1]})
        self.assertEqual(props['evidence_ids']['minItems'], 1)
        with self.assertRaises(ValueError):
            validate({'label': 1, 'reason': 'dump', 'evidence_ids': []}, parts)

    def test_policy_keeps_compromised_data_rule(self):
        self.assertIn('full server dump for sale', POLICY)
        self.assertIn('Always select at least one evidence segment', POLICY)

    def test_disagreement_is_uncertainty(self):
        docs=[{'id':'x','text':'sample'}]
        votes={('x','a'):{'status':'decided','label':1},('x','b'):{'status':'decided','label':0}}
        item=consensus(docs,votes,['a','b'])['documents'][0]
        self.assertEqual(item['status'],'disagreement')
        self.assertIsNone(item['label'])


if __name__ == '__main__': unittest.main()
