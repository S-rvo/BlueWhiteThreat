import unittest
import numpy as np
from compare_grouped_representations import metrics, select

class ComparisonTest(unittest.TestCase):
    def test_perfect_scores(self):
        m=metrics(np.array([0,1]),np.array([.1,.9]),.5)
        self.assertEqual((m['tp'],m['tn'],m['fp'],m['fn']),(1,1,0,0))
    def test_selected_threshold_protects_recall(self):
        y=np.array([1]*10+[0]*10);s=np.linspace(0,1,20)
        self.assertGreaterEqual(select(y,s)['recall'],.9)
    def test_no_positive_predictions(self):
        self.assertEqual(metrics(np.array([0,1]),np.array([.1,.2]),.9)['precision'],0)

if __name__=='__main__':unittest.main()
