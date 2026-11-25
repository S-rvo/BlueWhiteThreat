import unittest
from audit_oof_predictions import metrics, nested_calibration, select_recall_threshold, threshold_stability


class AuditOOFPredictionsTests(unittest.TestCase):
    def test_source_threshold_and_fallback(self):
        rows = [
            {"source": "a", "score": .6, "label": 1},
            {"source": "a", "score": .55, "label": 0},
            {"source": "b", "score": .45, "label": 0},
        ]
        result = metrics(rows, {"a": .6}, .4)
        self.assertEqual((result["tp"], result["fp"], result["fn"], result["tn"]), (1, 1, 0, 1))

    def test_nested_calibration_requires_and_uses_folds(self):
        self.assertFalse(nested_calibration([{"score": .5, "label": 1}])["available"])
        rows = []
        for fold in range(5):
            rows.extend([
                {"score": .8, "label": 1, "fold": fold},
                {"score": .2, "label": 0, "fold": fold},
            ])
        result = nested_calibration(rows)
        self.assertTrue(result["available"])
        self.assertEqual(result["folds"], 5)
        self.assertLess(result["brier"], .04)

    def test_threshold_stability_is_deterministic(self):
        rows = []
        for index in range(20):
            rows.append({"source": "darkweb_page", "group": f"p{index}", "score": .8, "label": 1})
            rows.append({"source": "darkweb_page", "group": f"n{index}", "score": .2, "label": 0})
        threshold, result = select_recall_threshold(rows)
        self.assertEqual(threshold, .8)
        self.assertEqual(result["precision"], 1)
        self.assertEqual(threshold_stability(rows, repeats=20), threshold_stability(rows, repeats=20))


if __name__ == "__main__":
    unittest.main()
