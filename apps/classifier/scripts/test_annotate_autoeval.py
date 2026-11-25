import unittest
from annotate_autoeval import validate_answer


class AnnotationContractTest(unittest.TestCase):
    def test_exact_unicode_evidence_spans(self):
        text = 'Écran : problème de réglage ordinaire.'
        vote = validate_answer({'label': 0, 'reason': 'Ordinary display settings.',
                                'evidence_quote': 'problème de réglage', 'full_text_read': True}, text)
        self.assertEqual(text[vote['evidence_start']:vote['evidence_end']], 'problème de réglage')
        self.assertEqual(vote['status'], 'decided')

    def test_invalid_evidence_and_boolean_labels_fail(self):
        for change in [{'evidence_quote': 'invented quotation'}, {'full_text_read': False}, {'label': True}]:
            answer = {'label': 1, 'reason': 'Concrete threat.', 'evidence_quote': 'ransomware incident', 'full_text_read': True}
            answer.update(change)
            with self.assertRaises(ValueError):
                validate_answer(answer, 'A ransomware incident affected this system.')

    def test_abstention_is_not_a_negative(self):
        vote = validate_answer({'label': None, 'reason': 'Insufficient context.',
                                'evidence_quote': '', 'full_text_read': True}, 'Ambiguous text.')
        self.assertIsNone(vote['label'])
        self.assertEqual(vote['status'], 'abstained')


if __name__ == '__main__':
    unittest.main()
