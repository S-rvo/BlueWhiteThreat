import unittest
import numpy as np
from score_frozen_lexical import score
class Fake:
    def predict_proba(self,texts):return np.array([[.2,.8]]*len(texts))
class ScorerTest(unittest.TestCase):
    def setUp(self):self.c={'pipeline':Fake(),'source_thresholds':{'known':.8},'version':'test'}
    def test_known_source(self):self.assertTrue(score(self.c,[{'text':'sample','source':'known'}])[0]['relevant'])
    def test_unknown_source_abstains(self):self.assertIsNone(score(self.c,[{'text':'sample','source':'other'}])[0]['relevant'])
    def test_empty_batch(self):self.assertEqual(score(self.c,[]),[])
if __name__=='__main__':unittest.main()
