import tempfile
import unittest
from pathlib import Path
from unittest.mock import patch

import check_docs as docs


class DocumentationValidatorTests(unittest.TestCase):
    def test_heading_anchors(self):
        self.assertEqual(docs.anchor_for_heading("Playwright"), "playwright")
        self.assertEqual(docs.anchor_for_heading("Shared properties"), "shared-properties")
        self.assertEqual(docs.anchor_for_heading("Café & More"), "café--more")

    def test_duplicate_and_explicit_anchors(self):
        with tempfile.TemporaryDirectory() as directory:
            path = Path(directory) / "test.md"
            path.write_text(
                "# Overview\n"
                "## Repeated\n"
                "## Repeated\n"
                "<a id=\"custom-anchor\"></a>\n"
                "```html\n"
                "<a id=\"fake-anchor\"></a>\n"
                "```\n",
                encoding="utf-8",
            )

            actual = docs.anchors(path)
            self.assertTrue(
                {"overview", "repeated", "repeated-1", "custom-anchor"}
                <= actual
            )
            self.assertNotIn("fake-anchor", actual)

    def test_html_link_parser(self):
        parser = docs.LinkParser()
        parser.feed(
            "<a href=\"docs/playwright.md#verification\">Guide</a>"
            "<a name=\"legacy-anchor\"></a>"
        )
        self.assertEqual(
            parser.links,
            ["docs/playwright.md#verification"],
        )
        self.assertIn("legacy-anchor", parser.ids)


    def test_local_html_links_and_broken_targets(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            page = root / "page.md"
            target = root / "target.md"

            target.write_text(
                "# Valid Heading\n"
                "## Repeated\n"
                "## Repeated\n"
                "<a id=\"explicit-anchor\"></a>\n",
                encoding="utf-8",
            )

            page.write_text(
                "<a href=\"target.md#valid-heading\">Valid</a>\n"
                "<a href=\"target.md#repeated-1\">Duplicate</a>\n"
                "<a href=\"target.md#explicit-anchor\">Explicit</a>\n"
                "[Encoded](target.md#valid%2Dheading)\n"
                "```html\n"
                "<a href=\"missing-example.md\">Example</a>\n"
                "```\n",
                encoding="utf-8",
            )

            with patch.object(docs, "markdown_files", return_value=[page]):
                with patch.object(docs, "ROOT", root):
                    errors = []
                    docs.validate_local_links(errors)
                    self.assertEqual(errors, [])

                    page.write_text(
                        "<a href=\"missing-page.md\">Broken</a>\n"
                        "<a href=\"target.md#missing-anchor\">Broken anchor</a>\n",
                        encoding="utf-8",
                    )

                    errors = []
                    docs.validate_local_links(errors)
                    self.assertEqual(len(errors), 2)
                    self.assertTrue(any("missing-page.md" in e for e in errors))
                    self.assertTrue(any("missing-anchor" in e for e in errors))


if __name__ == "__main__":
    unittest.main()
