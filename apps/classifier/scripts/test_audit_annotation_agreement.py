import unittest

from audit_annotation_agreement import audit


class AgreementAuditTest(unittest.TestCase):
    def test_majority_does_not_count_as_unanimity(self):
        models = ["a", "b", "c"]
        votes = {"doc": {"a": {"status": "decided", "label": 1},
                         "b": {"status": "decided", "label": 1},
                         "c": {"status": "decided", "label": 0}}}
        result = audit(votes, models)
        self.assertEqual(result["counts"]["majority_only"], 1)
        self.assertEqual(result["counts"]["unanimous"], 0)
        self.assertEqual(result["pairwise"]["a__b"]["agreement"], 1.0)
        self.assertEqual(result["pairwise"]["a__c"]["agreement"], 0.0)

    def test_partial_vote_is_reported_separately(self):
        result = audit({"doc": {"a": {"status": "decided", "label": 0},
                                 "b": {"status": "abstention", "label": None}}}, ["a", "b", "c"])
        self.assertEqual(result["counts"]["insufficient_votes"], 1)
        self.assertIsNone(result["pairwise"]["a__b"]["agreement"])

    def test_criterion_rates_and_agreement_are_reported(self):
        votes = {"doc": {m: {"status": "decided", "label": label,
                             "criteria": {"incident": flag}}
                         for m, label, flag in [("a", 1, True), ("b", 0, False), ("c", 0, False)]}}
        result = audit(votes, ["a", "b", "c"])
        self.assertEqual(result["criteria"]["incident"]["positive_counts"], {"a": 1, "b": 0, "c": 0})
        self.assertEqual(result["criteria"]["incident"]["pairwise"]["b__c"]["agreement"], 1.0)
        self.assertEqual(result["criteria"]["incident"]["pairwise"]["a__b"]["agreement"], 0.0)


if __name__ == "__main__":
    unittest.main()
