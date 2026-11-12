import importlib.util
from pathlib import Path
import unittest

spec = importlib.util.spec_from_file_location("proof", Path(__file__).with_name("ci-onboarding-native-proof.py"))
proof = importlib.util.module_from_spec(spec)
spec.loader.exec_module(proof)

TEST = "TestWorkstationOnboardingRemoteNativeInCI"


class NativeProofTests(unittest.TestCase):
    def events(self):
        return [{"Test": t, "Action": "pass"} for t in [TEST] + [TEST + "/" + name for name in proof.REQUIRED]]

    def test_complete_native_proof_passes(self):
        self.assertEqual(6, proof.verify(self.events(), TEST))

    def test_go_test_zero_matching_does_not_pass(self):
        with self.assertRaisesRegex(ValueError, "missing"):
            proof.verify([{"Action": "pass", "Package": "github.com/mcpdev80/baseharbor/cmd/baha"}], TEST)

    def test_skipped_cell_does_not_pass(self):
        events = self.events()
        events[2]["Action"] = "skip"
        with self.assertRaisesRegex(ValueError, "missing"):
            proof.verify(events, TEST)

    def test_failed_package_does_not_pass(self):
        events = self.events() + [{"Action": "fail", "Package": "github.com/mcpdev80/baseharbor/cmd/baha"}]
        with self.assertRaisesRegex(ValueError, "Go package failed"):
            proof.verify(events, TEST)


if __name__ == "__main__":
    unittest.main()
