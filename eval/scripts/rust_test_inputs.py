"""Protect existing Rust tests without replacing candidate production files."""

import base64
from collections import defaultdict
import json
from pathlib import Path, PurePosixPath
import re
import subprocess

import dataset


def original_files(repository, entries):
    identifiers = sorted({blob for _, _, blob in entries})
    raw = dataset.git(repository, "cat-file", "--batch", data="".join(blob + "\n" for blob in identifiers).encode()).stdout
    blobs, position = {}, 0
    for identifier in identifiers:
        end = raw.index(b"\n", position)
        blob, kind, length = raw[position:end].decode("ascii").split()
        size = int(length)
        position = end + 1
        if blob != identifier or kind != "blob" or size < 0 or raw[position+size:position+size+1] != b"\n":
            raise ValueError("fixed Git blob batch is incomplete or out of order")
        blobs[blob] = raw[position:position+size]
        position += size + 1
    if position != len(raw):
        raise ValueError("unexpected trailing Git blob data")
    return {str(name): (mode, blobs[blob]) for name, mode, blob in entries}


def test_file(name):
    path = PurePosixPath(name)
    return (any(part in {"test", "tests"} for part in path.parts[:-1])
            or path.name in {"test.rs", "tests.rs"} or path.name.endswith("_test.rs"))


def configuration_file(name):
    path = PurePosixPath(name)
    return path.name in {"Cargo.toml", "Cargo.lock", "rust-toolchain", "rust-toolchain.toml", "build.rs"} or ".cargo" in path.parts


def doc_text(item):
    return "\n".join(part["text"].replace("\r\n", "\n") for part in item["docs"])


def may_test(text):
    return bool("```" in text or "~~~" in text or "#[doc" in text or "#![doc" in text
                or re.search(r"(?m)^(?:(?:///|//!) ?(?:    |\t)|\s*\*? {4})", text))


def index_items(description):
    result = defaultdict(list)
    for item in description["items"]:
        result[item["id"]].append(item)
    return result


def only(items, key):
    matches = items.get(key, [])
    if len(matches) != 1:
        raise ValueError("Rust syntax owner is missing or ambiguous: " + key)
    return matches[0]


def top_tests(description):
    result = []
    for item in sorted((item for item in description["items"] if item["test"]), key=lambda item: (item["start"], -item["end"])):
        if not any(previous["start"] <= item["start"] and item["end"] <= previous["end"] for previous in result):
            result.append(item)
    return result


def contained(item, tests):
    return any(test["start"] <= item["start"] and item["end"] <= test["end"] for test in tests)


def restore_source(base, candidate, before, after):
    for description in (before, after):
        if description.get("error"):
            raise ValueError(description["path"] + ": " + description["error"])
    original, current = index_items(before), index_items(after)
    old_tests, new_tests = top_tests(before), top_tests(after)
    edits, additional_docs = [], []
    matched = set()
    for test in old_tests:
        only(original, test["id"])
        if test["id"] in current:
            target = only(current, test["id"])
            edits.append((target["start"], target["end"], base[test["start"]:test["end"]]))
            matched.add(target["id"])
            continue
        parent = only(current, test["parent"])
        neighbors = [item for item in before["items"] if item["parent"] == test["parent"]
                     and item["id"] != test["id"] and not contained(item, old_tests)]
        following = sorted((item for item in neighbors if item["start"] >= test["end"] and item["id"] in current), key=lambda item: item["start"])
        preceding = sorted((item for item in neighbors if item["end"] <= test["start"] and item["id"] in current), key=lambda item: -item["end"])
        if following:
            position = only(current, following[0]["id"])["start"]
        elif preceding:
            position = only(current, preceding[0]["id"])["end"]
        elif parent["insert"] >= 0:
            position = parent["insert"]
        else:
            raise ValueError("Rust test has no preserved insertion scope: " + test["id"])
        edits.append((position, position, b"\n" + base[test["start"]:test["end"]] + b"\n"))
    for test in new_tests:
        if test["id"] in matched:
            continue
        if test["id"] in original and not only(original, test["id"])["test"]:
            raise ValueError("candidate changes a production declaration into a test: " + test["id"])
        edits.append((test["start"], test["end"], b""))
    for key in original.keys() | current.keys():
        old, new = original.get(key, []), current.get(key, [])
        relevant_old = [item for item in old if not contained(item, old_tests)]
        relevant_new = [item for item in new if not contained(item, new_tests) and item["id"] not in matched]
        controls_old = [item["test_control"] for item in relevant_old if item["test_control"]]
        controls_new = [item["test_control"] for item in relevant_new if item["test_control"]]
        if controls_old != controls_new:
            raise ValueError("Rust conditional test collection changed: " + key)
        guarded = [item for item in relevant_old if may_test(doc_text(item))]
        if guarded:
            previous, present = only(original, key), only(current, key)
            if doc_text(previous) != doc_text(present):
                raise ValueError("existing Rust documentation test input changed: " + key)
        elif relevant_new:
            for item in relevant_new:
                if may_test(doc_text(item)):
                    additional_docs.extend({"first_line": part["line"], "last_line": part["last_line"]} for part in item["docs"])
    edits.sort(key=lambda edit: (edit[0], -edit[1]))
    merged, position = bytearray(), 0
    applied, effective = [], []
    for start, end, replacement in edits:
        if start < position:
            if end <= position:
                continue
            raise ValueError("overlapping Rust test restoration ranges")
        if not 0 <= start <= end <= len(candidate):
            raise ValueError("Rust test restoration range leaves source")
        merged.extend(candidate[position:start]); merged.extend(replacement)
        applied.append({"start": start, "end": end, "replacement_bytes": len(replacement)})
        effective.append((start, end, replacement))
        position = end
    merged.extend(candidate[position:])
    # Native doctest line identities refer to the validation source after edits.
    for doc in additional_docs:
        for key in ("first_line", "last_line"):
            original_line = doc[key]
            shift = 0
            for start, end, replacement in effective:
                if candidate[:end].count(b"\n") + 1 <= original_line:
                    shift += replacement.count(b"\n") - candidate[start:end].count(b"\n")
            doc[key] += shift
    return bytes(merged), applied, additional_docs


def protect(files, originals, validation, directory, helper, put_file):
    helper = Path(helper).resolve(strict=True)
    directory.mkdir()
    records = []
    for side, values in (("base", originals), ("candidate", files)):
        for name, (mode, data) in sorted(values.items()):
            if name.endswith(".rs") and not test_file(name):
                if mode not in {"100644", "100755"}:
                    raise ValueError("Rust source must be a regular file: " + name)
                records.append({"side": side, "path": name, "data": base64.b64encode(data).decode("ascii")})
    payload = json.dumps(records).encode()
    (directory / "input.json").write_bytes(payload)
    digest = dataset.digest(helper)
    try:
        process = subprocess.run([str(helper)], input=payload, capture_output=True, timeout=120)
    except subprocess.TimeoutExpired as error:
        (directory / "stdout.json").write_bytes(error.stdout or b"")
        (directory / "stderr.log").write_bytes(error.stderr or b"")
        raise ValueError("Rust input helper timed out") from error
    (directory / "stdout.json").write_bytes(process.stdout)
    (directory / "stderr.log").write_bytes(process.stderr)
    if process.returncode != 0 or dataset.digest(helper) != digest:
        raise ValueError("Rust input helper failed or changed during collection")
    descriptions = {(row["side"], row["path"]): row for row in json.loads(process.stdout)}
    if len(descriptions) != len(records) or set(descriptions) != {(row["side"], row["path"]) for row in records}:
        raise ValueError("Rust input helper returned an incomplete source inventory")
    original_tests = {name for name in originals if test_file(name)}
    candidate_tests = {name for name in files if test_file(name)}
    ordinary_refs, protected_refs, guarded_refs = set(), set(), set()
    resolved = []
    for (side, name), description in descriptions.items():
        if description.get("error"):
            raise ValueError(name + ": " + description["error"])
        values = originals if side == "base" else files
        for reference in description["references"]:
            choices = [choice for choice in reference["choices"] if choice in values]
            resolved.append((side, name, choices, reference))
    changed = True
    while changed:
        changed = False
        for side, name, choices, reference in resolved:
            tests = original_tests if side == "base" else candidate_tests
            if len(choices) != 1:
                if reference["test"] or reference["doc"] or name in tests:
                    raise ValueError("Rust test or documentation input is missing or ambiguous: " + name)
                continue
            selected = choices[0]
            if (reference["test"] or name in tests) and selected not in tests:
                tests.add(selected)
                changed = True
    protected_refs.update(original_tests | candidate_tests)
    for side, name, choices, reference in resolved:
        if len(choices) != 1:
            continue
        selected = choices[0]
        tests = original_tests if side == "base" else candidate_tests
        if reference["test"] or name in tests:
            protected_refs.add(selected)
        elif reference["doc"]:
            guarded_refs.add(selected)
        else:
            ordinary_refs.add(selected)
    shared = protected_refs & (ordinary_refs | guarded_refs)
    if any(originals.get(name) != files.get(name) for name in shared):
        raise ValueError("Rust test input is also used by a production module")
    for name in guarded_refs:
        if originals.get(name) != files.get(name):
            raise ValueError("included production documentation changed: " + name)
    for name in original_tests | candidate_tests:
        for values in (originals, files):
            if name in values and values[name][0] not in {"100644", "100755"}:
                raise ValueError("Rust test input is not a regular file: " + name)
        parent = (validation / name).parent
        while parent != validation:
            if parent.is_symlink():
                raise ValueError("Rust test input has a symlink parent: " + name)
            parent = parent.parent
    for name in candidate_tests:
        path = validation / name
        if path.exists() or path.is_symlink():
            path.unlink()
    for name in original_tests:
        mode, data = originals[name]
        put_file(validation, name, mode, data)
    changes, docs = {}, {}
    opaque_test_macros = [description["path"] for description in descriptions.values()
                          if any("/macro_definition:" in item["id"] and item["test_control"]
                                 and not contained(item, top_tests(description)) for item in description["items"])]
    changed_production = []
    names = {name for _, name in descriptions} - original_tests - candidate_tests
    empty = {"items": [{"id": "$", "parent": "", "start": 0, "end": 0, "insert": 0, "test": False, "docs": [], "test_control": []}]}
    for name in sorted(names):
        before = descriptions.get(("base", name), {**empty, "path": name})
        after = descriptions.get(("candidate", name), {**empty, "path": name})
        old = originals.get(name, ("100644", b""))[1]
        mode, new = files.get(name, ("100644", b""))
        merged, edits, additional = restore_source(old, new, before, after)
        if merged != old:
            changed_production.append(name)
        if edits:
            put_file(validation, name, mode, merged)
            changes[name] = edits
        if additional:
            docs[name] = additional
    if opaque_test_macros and changed_production:
        raise ValueError("Rust macro-generated test collection cannot be proven unchanged: " + ", ".join(sorted(set(opaque_test_macros))))
    result = {"helper_sha256": digest, "input_sha256": dataset.digest(directory / "input.json"),
              "output_sha256": dataset.digest(directory / "stdout.json"), "original_test_files": sorted(original_tests),
              "candidate_test_files": sorted(candidate_tests), "source_restorations": changes, "additional_doc_ranges": docs}
    (directory / "result.json").write_text(json.dumps(result, indent=2) + "\n")
    return result
