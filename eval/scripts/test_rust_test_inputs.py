import os
from pathlib import Path
import tempfile
import unittest

from assess_candidate import put_file
import dataset
import rust_test_inputs


class RustFixedTreeTests(unittest.TestCase):
    def test_export_ignored_inputs_and_binary_blobs_are_read_exactly(self):
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            dataset.git(root, "init", "--initial-branch=main")
            files = {".gitattributes": b"fixture.bin export-ignore\n", "fixture.bin": b"\0bytes\nwithout newline"}
            for name, data in files.items():
                (root / name).write_bytes(data)
            dataset.git(root, "add", "--all")
            tree = dataset.git(root, "write-tree").stdout.decode().strip()
            originals = rust_test_inputs.original_files(root, dataset.tree_entries(root, tree))
            self.assertEqual(originals, {name: ("100644", data) for name, data in files.items()})


@unittest.skipUnless(os.environ.get("TASKBELAY_EVAL_RUST_INPUTS"), "Rust syntax helper is not configured")
class RustInputProtectionTests(unittest.TestCase):
    def protect(self, root, original, candidate):
        old = {name: ("100644", text.encode()) for name, text in original.items()}
        new = {name: ("100644", text.encode()) for name, text in candidate.items()}
        validation = root / "validation"
        validation.mkdir()
        for name, (mode, data) in new.items():
            put_file(validation, name, mode, data)
        report = rust_test_inputs.protect(new, old, validation, root / "proof",
                                         os.environ["TASKBELAY_EVAL_RUST_INPUTS"], put_file)
        return validation, report

    def test_weakened_removed_and_added_tests_do_not_replace_candidate_production(self):
        source = '''pub fn value() -> i32 { 1 }
#[cfg(test)] mod checks {
    fn threshold() -> i32 { 1 }
    #[test] fn behavior() { assert_eq!(super::value(), threshold()); }
}
#[test] fn standalone() { assert_eq!(value(), 1); }
#[cfg(test)] #[path="support.rs"] mod support;
#[cfg(doctest)] #[doc=include_str!("../README.md")] struct Readme;
'''
        original = {"src/lib.rs": source, "src/support.rs": 'fn fixture() { let _ = include_str!("value.txt"); }\n',
                    "src/value.txt": "1", "README.md": "```rust\nassert!(true);\n```\n", "tests/existing.rs": "old external test"}
        for remove in (False, True):
            with self.subTest(remove=remove), tempfile.TemporaryDirectory() as temporary:
                candidate = dict(original)
                candidate["src/lib.rs"] = source.replace("{ 1 }", "{ 2 }", 1).replace("assert_eq!(super::value(), threshold());", "")
                candidate["src/lib.rs"] = candidate["src/lib.rs"].replace("#[test] fn standalone() { assert_eq!(value(), 1); }", "" if remove else "fn standalone() {}")
                candidate["src/lib.rs"] += "#[test] fn added() { panic!(); }\n"
                candidate["src/support.rs"] = "fn fixture() {}\n"
                candidate["src/value.txt"] = "-999"
                candidate["README.md"] = "weakened doctest"
                candidate["tests/existing.rs"] = "weakened external test"
                candidate["tests/new.rs"] = "added tests need not compile in validation"
                root = Path(temporary)
                validation, report = self.protect(root, original, candidate)
                restored = (validation / "src/lib.rs").read_text()
                self.assertIn("pub fn value() -> i32 { 2 }", restored)
                self.assertIn("fn standalone() { assert_eq!(value(), 1); }", restored)
                self.assertIn("assert_eq!(super::value(), threshold());", restored)
                self.assertNotIn("fn added", restored)
                for name in ("src/support.rs", "src/value.txt", "README.md", "tests/existing.rs"):
                    self.assertEqual((validation / name).read_text(), original[name])
                self.assertFalse((validation / "tests/new.rs").exists())
                self.assertEqual(report["additional_doc_ranges"], {})

    def test_original_documentation_and_shared_test_inputs_remain_unknown(self):
        scenarios = [
            ({"src/lib.rs": "generate!();\n", "src/macros.rs": "macro_rules! generate { () => { #[test] fn old() {} } }\n"},
             {"src/lib.rs": "", "src/macros.rs": "macro_rules! generate { () => { #[test] fn old() {} } }\n"}, "macro-generated test collection"),
            ({"src/lib.rs": 'macro_rules! behavior { () => { #[test] fn old() { assert_eq!("a b", "a b"); } } }\n'},
             {"src/lib.rs": 'macro_rules! behavior { () => { #[test] fn old() { assert_eq!("ab", "a b"); } } }\n'}, "conditional test collection changed"),
            ({"src/lib.rs": "/// ```\n/// assert!(true);\n/// ```\npub fn value() {}\n"},
             {"src/lib.rs": "/// ```\n/// assert!(false);\n/// ```\npub fn value() {}\n"}, "documentation test input changed"),
            ({"src/lib.rs": '#[cfg(any(test, feature="production"))] fn value() {}\n'},
             {"src/lib.rs": "fn value() {}\n"}, "conditional test collection changed"),
            ({"src/lib.rs": 'mod tests;\n', "src/tests.rs": "pub fn production() {}\n"},
             {"src/lib.rs": 'mod tests;\n', "src/tests.rs": "pub fn production() { panic!(); }\n"}, "production module"),
            ({"src/lib.rs": '#[cfg(test)] #[path="support.rs"] mod support;\n', "src/support.rs": "mod missing;\n"},
             {"src/lib.rs": '#[cfg(test)] #[path="support.rs"] mod support;\n', "src/support.rs": "mod missing;\n"}, "missing or ambiguous"),
        ]
        for original, candidate, message in scenarios:
            with self.subTest(message=message), tempfile.TemporaryDirectory() as temporary:
                with self.assertRaisesRegex(ValueError, message):
                    self.protect(Path(temporary), original, candidate)

    def test_new_documentation_keeps_candidate_attributes_and_correct_lines(self):
        original = {"src/lib.rs": "#[test] fn old() {\n    assert!(true);\n}\npub fn value() {}\n"}
        candidate = {"src/lib.rs": "#[test] fn old() {}\n#[test] fn added() {\n    panic!();\n}\n"
                     "/// ```\n/// assert!(true);\n/// ```\npub fn value() {}\n"}
        with tempfile.TemporaryDirectory() as temporary:
            validation, report = self.protect(Path(temporary), original, candidate)
            text = (validation / "src/lib.rs").read_text()
            line = text[:text.index("/// ```")].count("\n") + 1
            ranges = report["additional_doc_ranges"]["src/lib.rs"]
            self.assertEqual(ranges[0]["first_line"], line)
            self.assertIn("/// assert!(true);", text)

    def test_shared_readme_is_preserved_only_when_candidate_bytes_are_unchanged(self):
        source = '#![doc=include_str!("../README.md")]\n#[cfg(doctest)] #[doc=include_str!("../README.md")] struct Readme;\n'
        original = {"src/lib.rs": source, "README.md": "```rust\nassert!(true);\n```\n"}
        with tempfile.TemporaryDirectory() as temporary:
            validation, _ = self.protect(Path(temporary), original, original)
            self.assertEqual((validation / "README.md").read_text(), original["README.md"])
        with tempfile.TemporaryDirectory() as temporary, self.assertRaisesRegex(ValueError, "production module"):
            self.protect(Path(temporary), original, {**original, "README.md": "weakened example"})

    def test_symlink_test_paths_cannot_rewrite_candidate_production(self):
        for parent_link in (True, False):
            with self.subTest(parent_link=parent_link), tempfile.TemporaryDirectory() as temporary:
                root = Path(temporary)
                validation = root / "validation"
                validation.mkdir()
                originals = {"tests/existing.rs": ("100644", b"#[test] fn existing() {}\n")}
                files = {"production/existing.rs": ("100644", b"pub fn value() {}\n")}
                files["tests" if parent_link else "tests/existing.rs"] = ("120000", b"production" if parent_link else b"../production/existing.rs")
                for name, (mode, data) in files.items():
                    put_file(validation, name, mode, data)
                with self.assertRaisesRegex(ValueError, "symlink parent|not a regular file"):
                    rust_test_inputs.protect(files, originals, validation, root / "proof", os.environ["TASKBELAY_EVAL_RUST_INPUTS"], put_file)
                self.assertEqual((validation / "production/existing.rs").read_bytes(), b"pub fn value() {}\n")


if __name__ == "__main__":
    unittest.main()
