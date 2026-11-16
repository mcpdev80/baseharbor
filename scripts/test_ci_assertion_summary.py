import importlib.util
import pathlib
import unittest

spec = importlib.util.spec_from_file_location("assertions", pathlib.Path(__file__).with_name("ci-assertion-summary.py"))
module = importlib.util.module_from_spec(spec)
spec.loader.exec_module(module)


class AssertionSummaryTests(unittest.TestCase):
    def test_failure_survives_cleanup_and_redacts_credentials(self):
        result = module.summarize('Error: preflight failed password="secret phrase" token=secret-token postgres://alice:pass@db/a\n[acceptance] cleanup: FAILED cleanup\n[runtime-reset] FAILED teardown')
        text = str(result)
        for secret in ["secret phrase", "secret-token", "alice:pass", "teardown", "FAILED cleanup"]:
            self.assertNotIn(secret, text)
        self.assertEqual("preflight", result["classification"])
        self.assertIn("preflight failed", text)

    def test_output_is_bounded_and_signature_ignores_secret_values(self):
        result = module.summarize("\n".join(["Error: tls certificate " + "a" * 3000] * 1000))
        self.assertLessEqual(len(result["assertions"]), 8)
        self.assertLessEqual(max(map(len, result["assertions"])), 300)
        self.assertEqual(module.summarize("Error: tls token=first")["failure_key"], module.summarize("Error: tls token=second")["failure_key"])


if __name__ == "__main__":
    unittest.main()
