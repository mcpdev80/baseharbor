"""Validate the isolated broker gateway endpoint without weakening TLS."""
import importlib.util
import subprocess
import sys
import tempfile
import unittest
from pathlib import Path


spec = importlib.util.spec_from_file_location(
    "broker_docs_url", Path(__file__).with_name("ci-broker-docs-url.py"))
helper = importlib.util.module_from_spec(spec)
spec.loader.exec_module(helper)


class BrokerDocsURLTests(unittest.TestCase):
    def test_exact_gateway_with_default_and_explicit_port(self):
        for url in ("https://broker-demo.baha.localhost/swagger/",
                    "https://broker-demo.baha.localhost:18443/swagger/"):
            self.assertEqual(url, helper.extract_url("[INFO] Swagger/OpenAPI: " + url))

    def test_rejects_unexpected_or_ambiguous_endpoint(self):
        invalid = ("http://broker-demo.baha.localhost/swagger/",
                   "https://broker-demo.baha.localhost.evil/swagger/",
                   "https://user@broker-demo.baha.localhost/swagger/",
                   "https://broker-demo.baha.localhost/swagger/?token=x",
                   "https://broker-demo.baha.localhost/other/",
                   "https://broker-demo.baha.localhost:70000/swagger/")
        for url in invalid:
            with self.subTest(url=url), self.assertRaises(ValueError):
                helper.extract_url("Swagger/OpenAPI: " + url)
        with self.assertRaises(ValueError):
            helper.extract_url("Swagger/OpenAPI: https://broker-demo.baha.localhost/swagger/\n"
                               "Swagger/OpenAPI: https://broker-demo.baha.localhost/swagger/")
        with self.assertRaises(ValueError):
            helper.extract_url("no docs URL")

    def test_invalid_port_diagnostic_does_not_echo_untrusted_url(self):
        with tempfile.TemporaryDirectory() as tmp:
            output = Path(tmp) / "broker.out"
            output.write_text("Swagger/OpenAPI: https://broker-demo.baha.localhost:sensitive-marker/swagger/", encoding="utf-8")
            result = subprocess.run([sys.executable, spec.origin, str(output)],
                                    text=True, capture_output=True)
            self.assertNotEqual(0, result.returncode)
            self.assertNotIn("sensitive-marker", result.stderr)
            self.assertEqual("broker docs URL: invalid or inaccessible broker output\n", result.stderr)


if __name__ == "__main__":
    unittest.main()
