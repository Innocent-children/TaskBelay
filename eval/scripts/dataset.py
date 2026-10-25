"""Prepare a PR task from its exact base tree without importing answer history."""

import argparse
import hashlib
import json
import os
from pathlib import Path, PurePosixPath
import re
import subprocess


def git(repository, *arguments, data=None, check=True):
    # Evaluation repositories must not inherit filters, hooks, signing or index paths.
    environment = {key: value for key, value in os.environ.items() if not key.startswith("GIT_")}
    environment.update(GIT_CONFIG_NOSYSTEM="1", GIT_CONFIG_GLOBAL=os.devnull)
    return subprocess.run(["git", "-C", str(repository), *arguments], input=data,
                          stdout=subprocess.PIPE, stderr=subprocess.PIPE,
                          env=environment, check=check)


def digest(path):
    return hashlib.sha256(path.read_bytes()).hexdigest()


def read_task(directory):
    directory = Path(directory).resolve(strict=True)
    task = json.loads((directory / "task.json").read_text(encoding="utf-8"))
    required = {"request.md", "reference.patch", "implementation.patch", "hidden.patch"}
    if not required.issubset(task["sha256"]):
        raise ValueError("required task artifact hashes are missing")
    for key in ("base_commit", "base_tree", "merge_commit"):
        if not re.fullmatch(r"[a-f0-9]{40}|[a-f0-9]{64}", task[key]):
            raise ValueError("invalid object identity: " + key)
    for name, expected in task["sha256"].items():
        path = directory / name
        if path.resolve().parent != directory or not path.is_file() or digest(path) != expected:
            raise ValueError("task artifact changed: " + name)
    return task


def tree_entries(repository, tree):
    entries = []
    for entry in git(repository, "ls-tree", "-r", "-z", tree).stdout.split(b"\0"):
        if not entry:
            continue
        info, raw_path = entry.split(b"\t", 1)
        mode, kind, object_id = info.decode("ascii").split()
        path = PurePosixPath(raw_path.decode("utf-8"))
        if path.is_absolute() or any(part in {".", ".."} or part.lower() == ".git" for part in path.parts):
            raise ValueError("unsafe tree path: " + str(path))
        if kind != "blob" or mode not in {"100644", "100755", "120000"}:
            raise ValueError("unsupported tree entry: " + str(path))
        entries.append((path, mode, object_id))
    return entries


def materialize_tree(repository, tree, workspace):
    """Copy one fixed tree and its blobs into a new repository, without history."""
    repository = Path(repository).resolve(strict=True)
    workspace = Path(workspace).resolve()
    if workspace.exists():
        raise ValueError("workspace must be new")
    if workspace.is_relative_to(repository) or repository.is_relative_to(workspace):
        raise ValueError("source repository must be outside the workspace")
    if not re.fullmatch(r"[a-f0-9]{40}|[a-f0-9]{64}", tree) or git(repository, "cat-file", "-t", tree).stdout != b"tree\n":
        raise ValueError("a fixed tree object is required")
    entries = tree_entries(repository, tree)
    for path, mode, object_id in entries:
        if mode == "120000":
            target = git(repository, "cat-file", "blob", object_id).stdout.decode("utf-8")
            if Path(target).is_absolute() or not (workspace / path.parent / target).resolve().is_relative_to(workspace):
                raise ValueError("symlink leaves evaluation workspace: " + str(path))

    # Pack only the tree and blobs. No original commits, remotes or future objects
    # enter the evaluated repository, including those reachable in the source clone.
    objects = git(repository, "rev-list", "--objects", tree).stdout
    object_ids = b"\n".join(line.split(b" ", 1)[0] for line in objects.splitlines()) + b"\n"
    pack = git(repository, "pack-objects", "--stdout", data=object_ids).stdout
    workspace.mkdir(parents=True)
    git(workspace, "init", "--initial-branch=main")
    git(workspace, "config", "core.autocrlf", "false")
    git(workspace, "config", "core.filemode", "true")
    git(workspace, "unpack-objects", "-q", data=pack)
    git(workspace, "read-tree", tree)
    git(workspace, "checkout-index", "--all")
    commit = git(workspace, "-c", "user.name=TaskBelay Evaluation",
                 "-c", "user.email=taskbelay@example.invalid", "-c", "commit.gpgsign=false",
                 "commit-tree", tree, data=b"Evaluation baseline\n").stdout.decode().strip()
    git(workspace, "update-ref", "refs/heads/main", commit)
    if git(workspace, "status", "--porcelain").stdout:
        raise ValueError("materialized workspace differs from its recorded Git tree")
    return {"workspace": str(workspace), "tree": tree, "evaluation_commit": commit, "files": len(entries)}


def prepare(task_directory, repository, workspace):
    task_directory = Path(task_directory).resolve(strict=True)
    repository = Path(repository).resolve(strict=True)
    workspace = Path(workspace).resolve()
    if workspace.is_relative_to(task_directory) or task_directory.is_relative_to(workspace):
        raise ValueError("reference material must be outside the workspace")
    task = read_task(task_directory)
    base_tree = git(repository, "rev-parse", task["base_commit"] + "^{tree}").stdout.decode().strip()
    if base_tree != task["base_tree"]:
        raise ValueError("source repository does not match the recorded base tree")
    parent = git(repository, "rev-parse", task["merge_commit"] + "^1").stdout.decode().strip()
    if parent != task["base_commit"]:
        raise ValueError("base is not the merge commit's first parent")
    receipt = materialize_tree(repository, base_tree, workspace)
    if git(workspace, "cat-file", "-e", task["merge_commit"], check=False).returncode == 0:
        raise ValueError("reference history leaked into the evaluation workspace")
    return {"task_id": task["id"], "workspace": str(workspace), "base_tree": base_tree,
            "evaluation_commit": receipt["evaluation_commit"], "files": receipt["files"], "hidden_tests_added": False}


def apply_material(task_directory, workspace, name):
    if name not in {"implementation.patch", "hidden.patch"}:
        raise ValueError("unknown evaluation material")
    read_task(task_directory)
    path = Path(task_directory).resolve() / name
    if not path.read_bytes():
        raise ValueError("evaluation patch is empty: " + name)
    git(workspace, "apply", "--check", str(path))
    git(workspace, "apply", str(path))


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("task_directory", type=Path)
    parser.add_argument("source_repository", type=Path)
    parser.add_argument("new_workspace", type=Path)
    arguments = parser.parse_args()
    result = prepare(arguments.task_directory, arguments.source_repository, arguments.new_workspace)
    print(json.dumps(result, ensure_ascii=False, indent=2))


if __name__ == "__main__":
    main()
