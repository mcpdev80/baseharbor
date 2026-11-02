import importlib.util
from pathlib import Path
import unittest

spec = importlib.util.spec_from_file_location("broker_mtls_rejection", Path(__file__).with_name("ci-broker-mtls-rejection.py"))
module = importlib.util.module_from_spec(spec)
spec.loader.exec_module(module)


IDENTITY_REJECTION = "runtime client certificate identity does not match this application broker"


class MTLSRejectionTest(unittest.TestCase):
    def test_client_cert_alert_from_verified_target(self):
        self.assertTrue(module.rejected("172.17.0.2", 56, "172.17.0.2|0|000", "curl: (56) TLS alert, bad certificate", IDENTITY_REJECTION))
        self.assertTrue(module.rejected("172.17.0.2", 35, "172.17.0.2|0|000", "curl: (35) bad certificate, TLS alert", IDENTITY_REJECTION))
        self.assertTrue(module.rejected("172.17.0.2", 52, "172.17.0.2|0|000", "TLS alert, bad certificate", IDENTITY_REJECTION))

    def test_unrelated_failures_do_not_pass(self):
        for code, meta, error in [
            (7, "172.17.0.2|0|000", "connection refused"),
            (60, "172.17.0.2|20|000", "SSL certificate problem"),
            (56, "172.17.0.3|0|000", "tlsv1 alert bad certificate"),
            (56, "172.17.0.2|0|401", "tlsv1 alert bad certificate"),
            (0, "172.17.0.2|0|000", "tlsv1 alert bad certificate"),
            (256, "172.17.0.2|0|000", "tlsv1 alert bad certificate"),
            (56, "", "tlsv1 alert bad certificate"),
            (56, "172.17.0.2|0|000", "connection reset by peer"),
            (56, "172.17.0.2|0|000", "tlsv1 alert unknown ca"),
            (56, "172.17.0.2|0|000", "bad certificate\nTLS alert"),
        ]:
            with self.subTest(code=code, metadata=meta, error=error):
                self.assertFalse(module.rejected("172.17.0.2", code, meta, error, IDENTITY_REJECTION))

    def test_rejects_missing_server_identity_evidence(self):
        self.assertFalse(module.rejected("172.17.0.2", 56, "172.17.0.2|0|000",
                                         "tlsv1 alert bad certificate", "unrelated server error"))


if __name__ == "__main__":
    unittest.main()
