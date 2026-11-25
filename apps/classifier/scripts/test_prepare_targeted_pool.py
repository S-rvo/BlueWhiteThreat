import unittest
from prepare_targeted_pool import STRONG, AMBIGUOUS, SECONDARY
class TargetedPoolTest(unittest.TestCase):
    def test_strong_signal(self): self.assertTrue(STRONG.search('Initial access broker offers corporate foothold'))
    def test_ambiguous_requires_context(self):
        self.assertTrue(AMBIGUOUS.search('database maintenance'))
        self.assertFalse(SECONDARY.search('database maintenance'))
        self.assertTrue(SECONDARY.search('database from a breached corporate victim'))
    def test_ordinary_shop_is_not_selected(self):
        text='Buy gift cards with crypto and fast delivery'
        self.assertFalse(STRONG.search(text)); self.assertFalse(AMBIGUOUS.search(text) and SECONDARY.search(text))
if __name__=='__main__': unittest.main()
