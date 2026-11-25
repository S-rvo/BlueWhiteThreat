import unittest
from annotation_v2 import segments, validate, consensus

class StructuredAnnotationTest(unittest.TestCase):
    def test_segments_preserve_every_unicode_character(self):
        text = 'Écran, 😀: ' + ('installation réseau et paramètres\n' * 80)
        parts = segments(text)
        self.assertEqual(''.join(p['text'] for p in parts), text)
        for p in parts:
            self.assertEqual(text[p['start']:p['end']], p['text'])

    def test_reconstructs_exact_evidence_without_model_quoting(self):
        text = 'Ordinary display troubleshooting.'
        answer = dict(label=0, reason='The subject is display settings.', evidence_ids=[0])
        vote = validate(answer, segments(text))
        self.assertEqual(vote['evidence'][0]['text'], text)
        for ids in [[99], [True], [], [0,0]]:
            with self.assertRaises(ValueError):
                validate({**answer, 'evidence_ids':ids}, segments(text))

    def test_disagreement_and_missing_votes_are_not_negatives(self):
        docs=[{'id':'a','text':'ordinary settings'}]
        votes={('a','one'):{'status':'decided','label':1},('a','two'):{'status':'decided','label':0}}
        r=consensus(docs,votes,['one','two'])
        self.assertEqual(r['documents'][0]['status'],'disagreement')
        self.assertIsNone(r['documents'][0]['label'])
        r=consensus(docs,{},['one','two'])
        self.assertEqual(r['documents'][0]['status'],'pending')

if __name__=='__main__':unittest.main()
