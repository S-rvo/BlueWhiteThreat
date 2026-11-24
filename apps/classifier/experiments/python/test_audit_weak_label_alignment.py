import unittest
from audit_weak_label_alignment import summarize

class WeakLabelAlignmentTest(unittest.TestCase):
    def test_abstentions_excluded_from_historical_agreement(self):
        report = summarize([
            {'source':'s','candidate':'weak_positive','historical_label':1},
            {'source':'s','candidate':'weak_negative','historical_label':1,'matched':{'ordinary_product_listing':True}},
            {'source':'s','candidate':'abstain','historical_label':0},
        ])
        total = report['total']
        self.assertEqual(total['eligible'], 2)
        self.assertEqual(total['abstain'], 1)
        self.assertEqual(total['historical_agreement'], 1)
        self.assertEqual(total['historical_disagreement'], 1)
        self.assertEqual(total['coverage'], 2/3)
        self.assertEqual(total['agreement_rate_on_nonabstentions'], .5)
        self.assertIn('not accuracy', report['interpretation'])
        self.assertEqual(report['historical_disagreement_matched_rules']['total'], {'ordinary_product_listing': 1})
    def test_source_strata_stay_separate(self):
        report = summarize([
            {'source':'a','candidate':'weak_positive','historical_label':1},
            {'source':'b','candidate':'weak_negative','historical_label':0},
        ])
        self.assertEqual(report['by_source']['a']['historical_agreement'], 1)
        self.assertEqual(report['by_source']['b']['historical_agreement'], 1)

if __name__ == '__main__': unittest.main()
