import tempfile
import unittest
from pathlib import Path

import prepare_test_prod_config as config


class TestProductionConfigPreparation(unittest.TestCase):
    def prepare_config(self, source_text, proxied=False, overlay_text=None):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            source = root / "source.yml"
            destination = root / "generated.yml"
            source.write_text(source_text, encoding="utf-8")

            overlay = None
            if overlay_text is not None:
                overlay = root / "overlay.yml"
                overlay.write_text(overlay_text, encoding="utf-8")

            config.prepare(
                source=source,
                destination=destination,
                overlay=overlay,
                frontend_diagnostics=True,
                https=False,
                proxied=proxied,
                resource_proxy_origins=[],
            )
            return destination.read_text(encoding="utf-8")

    def test_proxy_awareness_enabled(self):
        result = self.prepare_config(
            "server:\n  frontend-diagnostics: false\nauth:\n  users: []\n",
            proxied=True,
        )
        self.assertIn("  proxied: true\n", result)
        self.assertIn("  frontend-diagnostics: true\n", result)
        self.assertEqual(result.count("server:\n"), 1)

    def test_default_does_not_enable_proxy(self):
        result = self.prepare_config(
            "server:\n  frontend-diagnostics: false\n",
        )
        self.assertNotIn("  proxied: true\n", result)
        self.assertEqual(result.count("server:\n"), 1)

    def test_existing_proxy_setting_is_updated(self):
        result = self.prepare_config(
            "server:\n  proxied: false\n",
            proxied=True,
        )
        self.assertEqual(result.count("  proxied:"), 1)
        self.assertIn("  proxied: true\n", result)

    def test_duplicate_server_sections_rejected(self):
        with self.assertRaisesRegex(ValueError, "exactly one top-level server"):
            self.prepare_config("server:\nserver:\n", proxied=True)

    def test_overlay_preserves_single_server_mapping(self):
        result = self.prepare_config(
            "server:\n  frontend-diagnostics: false\n",
            proxied=True,
            overlay_text="auth:\n  guest:\n    enabled: true\n",
        )
        self.assertEqual(result.count("server:\n"), 1)
        self.assertIn("  proxied: true\n", result)
        self.assertIn("  guest:\n", result)


if __name__ == "__main__":
    unittest.main()
