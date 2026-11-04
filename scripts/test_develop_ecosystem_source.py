"""Develop source preflight rejects stale, malformed and mis-pinned consumers."""
import importlib.util
from pathlib import Path
import unittest

spec = importlib.util.spec_from_file_location(
    "preflight", Path(__file__).with_name("develop-ecosystem-source.py")
)
mod = importlib.util.module_from_spec(spec)
spec.loader.exec_module(mod)


class PreflightTests(unittest.TestCase):
    def setUp(self):
        self.core, self.demo, self.console, self.connector = (
            "a" * 40, "b" * 40, "c" * 40, "d" * 40
        )
        self.console_pin = {
            "schema": "baseharbor.ecosystem-candidate/v1",
            "release": "0.4.27", "core": self.core, "demo": self.demo
        }
        self.connector_pin = {
            **self.console_pin, "console": self.console
        }

    def verify(self, **changes):
        args = dict(core=self.core, demo=self.demo, console=self.console,
                    connector=self.connector, demo_core=self.core,
                    console_candidate=self.console_pin,
                    connector_candidate=self.connector_pin)
        args.update(changes)
        return mod.validate(**args)

    def test_compatible_is_only_source_preflight_not_runtime_success(self):
        result = self.verify()
        self.assertEqual("source-compatible", result["result"])
        self.assertIs(result["runtime_qualified"], False)
        self.assertEqual(set(result["sources"]), {"core", "demo", "console", "connector"})

    def test_stale_demo_core_is_rejected(self):
        with self.assertRaises(ValueError):
            self.verify(demo_core="e" * 40)

    def test_stale_console_pin_is_rejected(self):
        with self.assertRaises(ValueError):
            self.verify(console_candidate={**self.console_pin, "demo": "e" * 40})

    def test_stale_connector_pin_is_rejected(self):
        with self.assertRaises(ValueError):
            self.verify(connector_candidate={**self.connector_pin, "console": "e" * 40})

    def test_release_labels_must_match_but_need_not_be_released(self):
        with self.assertRaises(ValueError):
            self.verify(connector_candidate={**self.connector_pin, "release": "0.4.26"})

    def test_missing_extra_or_non_sha_rejected(self):
        for variant in (
            {"console_candidate": None},
            {"connector_candidate": {**self.connector_pin, "extra": True}},
            {"core": "develop"},
            {"demo": ""},
            {"console_candidate": {**self.console_pin, "schema": "unknown"}},
        ):
            with self.subTest(variant=variant), self.assertRaises(ValueError):
                self.verify(**variant)


if __name__ == "__main__":
    unittest.main()
