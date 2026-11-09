#!/usr/bin/env python3
"""Build pinned HEAD archives through a local-only Go module proxy, never an App run."""
import argparse
import difflib
import hashlib
import json
import os
from pathlib import Path, PurePosixPath
import re
import shutil
import signal
import subprocess
import tarfile
import tempfile
import time
import uuid

FIXTURE = "v0.0.0-repro.20261102"
MODULES = {"monitor": "gitee.com/charsen/moo-monitor-go", "scaffold": "gitee.com/charsen/go-moo-scaffold"}
HELPER = '''package main
import("fmt";"os";"golang.org/x/mod/module";"golang.org/x/mod/zip")
func main(){if len(os.Args)!=5{os.Exit(2)};f,e:=os.OpenFile(os.Args[4],os.O_CREATE|os.O_EXCL|os.O_WRONLY,0600);if e!=nil{fmt.Fprintln(os.Stderr,e);os.Exit(1)};e=zip.CreateFromDir(f,module.Version{Path:os.Args[1],Version:os.Args[2]},os.Args[3]);c:=f.Close();if e!=nil||c!=nil{fmt.Fprintln(os.Stderr,e,c);os.Exit(1)}}
'''

class ReproductionError(Exception):
    pass


def file_digest(path):
    digest = hashlib.sha256()
    with path.open("rb") as source:
        for chunk in iter(lambda: source.read(1024 * 1024), b""):
            digest.update(chunk)
    return digest.hexdigest()


def arguments():
    parser = argparse.ArgumentParser(description=__doc__)
    for name in ("monitor", "scaffold", "host"):
        parser.add_argument("--" + name, required=True, type=Path)
        parser.add_argument("--" + name + "-sha", required=True)
    parser.add_argument("--go", required=True, type=Path)
    parser.add_argument("--cache-download", required=True, type=Path)
    parser.add_argument("--report", required=True, type=Path)
    return parser.parse_args()


def cleanup(root, marker):
    if json.loads((root / "owner.json").read_text()) != marker:
        raise ReproductionError("owned root marker changed; retained")
    allowed = {"owner.json", "tmp", "logs", "proxy", "modcache", "buildcache", "monitor.tar", "monitor-source", "scaffold.tar", "scaffold-source", "host.tar", "host-source", "modulezip-helper", "modulezip", "host", "monitor-mcp"}
    if any(path.name not in allowed for path in root.iterdir()):
        raise ReproductionError("unknown fixture entry; retained")
    # Go extracts read-only module-cache directories. Make only this owned tree
    # removable, without following links or touching the original module cache.
    for directory, _, _ in os.walk(root, followlinks=False):
        Path(directory).chmod(0o700)
    for path in root.iterdir():
        if path.name == "owner.json":
            continue
        if path.is_symlink() or not path.is_dir():
            path.unlink()
        else:
            shutil.rmtree(path)
    (root / "owner.json").unlink()
    root.rmdir()


def main():
    args = arguments()
    if args.report.exists() or not args.report.parent.is_dir():
        raise ReproductionError("report must be a new file in an existing directory")
    for name in ("monitor", "scaffold", "host"):
        if not re.fullmatch(r"[a-f0-9]{40}", getattr(args, name + "_sha")):
            raise ReproductionError("explicit full source SHA required")
    cache = args.cache_download.resolve(strict=True)
    for suffix in ("mod", "zip", "info"):
        item = cache / "golang.org/x/mod/@v" / ("v0.37.0." + suffix)
        if not item.is_file() or item.is_symlink():
            raise ReproductionError("locked modulezip helper is not fully cached")
    root = Path(tempfile.mkdtemp(prefix="moo-release-fixture-")).resolve()
    root.chmod(0o700)
    marker = {"owned": True, "purpose": "local-go-module-fixture", "root": str(root), "nonce": uuid.uuid4().hex}
    (root / "owner.json").write_text(json.dumps(marker))
    (root / "owner.json").chmod(0o600)
    scratch = root / "tmp"
    scratch.mkdir(mode=0o700)
    logs = root / "logs"
    logs.mkdir()
    proxy = root / "proxy"
    proxy.mkdir()
    environment = {key: os.environ[key] for key in ("PATH", "HOME", "TMPDIR") if key in os.environ}
    environment.update({"GOENV": "off", "GOWORK": "off", "GOTOOLCHAIN": "local", "GOPRIVATE": "gitee.com/charsen/*", "GONOPROXY": "none", "GONOSUMDB": "*", "GOSUMDB": "off", "GOVCS": "*:off", "GOPROXY": proxy.as_uri() + "," + cache.as_uri(), "GOMODCACHE": str(root / "modcache"), "GOCACHE": str(root / "buildcache"), "GIT_TERMINAL_PROMPT": "0", "GIT_CONFIG_NOSYSTEM": "1", "GIT_CONFIG_GLOBAL": os.devnull, "GOMAXPROCS": "4", "TMPDIR": str(scratch), "GOTMPDIR": str(scratch)})
    report = {"kind": "local-go-module-fixture-not-a-release", "fixture_version": FIXTURE, "status": "incomplete", "source_commits": {}, "archive_sha256": {}, "commands": [], "production_binaries_executed": False, "network_proxy_sources": ["owned-file-proxy", "existing-file-cache-only"], "remote_discovery_or_tags_verified": False}
    succeeded = False
    host = None
    before = sum_before = None
    requires_only = None

    def run(command, cwd, label):
        report["commands"].append(label)
        print("stage:", label, flush=True)
        with (logs / (label + ".log")).open("wb") as output:
            process = subprocess.Popen([str(value) for value in command], cwd=cwd, env=environment, stdin=subprocess.DEVNULL, stdout=output, stderr=subprocess.STDOUT, start_new_session=True)
            try:
                code = process.wait(timeout=600)
            except BaseException:
                # This session/process group was created by this call, never a shared service.
                try:
                    os.killpg(process.pid, signal.SIGTERM)
                except ProcessLookupError:
                    pass
                deadline = time.monotonic() + 2
                while time.monotonic() < deadline:
                    try:
                        os.killpg(process.pid, 0)
                    except ProcessLookupError:
                        break
                    time.sleep(0.05)
                else:
                    try:
                        os.killpg(process.pid, signal.SIGKILL)
                    except ProcessLookupError:
                        pass
                process.wait(timeout=5)
                deadline = time.monotonic() + 2
                while time.monotonic() < deadline:
                    try:
                        os.killpg(process.pid, 0)
                    except ProcessLookupError:
                        break
                    time.sleep(0.05)
                else:
                    raise ReproductionError("owned command group cleanup incomplete")
                raise
        if code:
            raise ReproductionError(label + " failed; inspect the owned log")
        return (logs / (label + ".log")).read_text()

    try:
        report["toolchain"] = json.loads(run([args.go, "env", "-json", "GOVERSION", "GOOS", "GOARCH", "CGO_ENABLED", "CC"], root, "toolchain"))
        sources = {}
        for name in ("monitor", "scaffold", "host"):
            repository = getattr(args, name).resolve(strict=True)
            sha = getattr(args, name + "_sha")
            actual = run(["git", "rev-parse", sha + "^{commit}"], repository, name + "-commit").strip()
            if actual != sha:
                raise ReproductionError("source revision mismatch")
            report["source_commits"][name] = sha
            archive = root / (name + ".tar")
            # git archive reads the commit tree, not the index or untracked/ignored files.
            report["commands"].append("archive-" + name)
            print("stage: archive-" + name, flush=True)
            with archive.open("wb") as output:
                completed = subprocess.run(["git", "archive", "--format=tar", sha], cwd=repository, env=environment, stdin=subprocess.DEVNULL, stdout=output, stderr=subprocess.PIPE, timeout=60)
            if completed.returncode:
                raise ReproductionError("source archive failed")
            destination = root / (name + "-source")
            destination.mkdir()
            report["archive_sha256"][name] = file_digest(archive)
            with tarfile.open(archive) as source:
                if source.pax_headers.get("comment") != sha:
                    raise ReproductionError("archive commit header mismatch")
                members = source.getmembers()
                if len(members) > 50000 or sum(member.size for member in members) > 128 * 1024 * 1024:
                    raise ReproductionError("source archive exceeds fixture limits")
                for member in members:
                    path = PurePosixPath(member.name)
                    if path.is_absolute() or ".." in path.parts or not (member.isdir() or member.isfile()):
                        raise ReproductionError("unsafe source archive member")
                    if path.name == ".env" or (path.name.startswith(".env.") and path.name != ".env.example"):
                        raise ReproductionError("source archive contains an environment file")
                    target = destination.joinpath(*path.parts)
                    if member.isdir():
                        target.mkdir(parents=True, exist_ok=True)
                    else:
                        target.parent.mkdir(parents=True, exist_ok=True)
                        with source.extractfile(member) as incoming, target.open("xb") as outgoing:
                            shutil.copyfileobj(incoming, outgoing)
                        if target.stat().st_size != member.size:
                            raise ReproductionError("incomplete source archive member")
                        target.chmod(member.mode & 0o777)
            sources[name] = destination
        helper = root / "modulezip-helper"
        helper.mkdir()
        (helper / "go.mod").write_text("module fixture.invalid/modulezip\n\ngo 1.26.0\n\nrequire golang.org/x/mod v0.37.0\n")
        (helper / "main.go").write_text(HELPER)
        helper_binary = root / "modulezip"
        run([args.go, "build", "-mod=mod", "-o", helper_binary, "."], helper, "build-cached-modulezip-helper")
        for name, module in MODULES.items():
            source = sources[name]
            expected = re.search(r"^module\s+(\S+)", (source / "go.mod").read_text(), re.M)
            if expected is None or expected.group(1) != module:
                raise ReproductionError("module identity mismatch")
            versions = proxy / module / "@v"
            versions.mkdir(parents=True)
            shutil.copyfile(source / "go.mod", versions / (FIXTURE + ".mod"))
            (versions / "list").write_text(FIXTURE + "\n")
            (versions / (FIXTURE + ".info")).write_text(json.dumps({"Version": FIXTURE, "Time": "2026-11-02T00:00:00Z"}))
            run([helper_binary, module, FIXTURE, source, versions / (FIXTURE + ".zip")], root, "zip-" + name)
        host = sources["host"] / "engine"
        before = (host / "go.mod").read_text()
        sum_before = (host / "go.sum").read_text() if (host / "go.sum").exists() else ""
        for module in MODULES.values():
            run([args.go, "mod", "edit", "-require=" + module + "@" + FIXTURE], host, "require-" + module.rsplit("/", 1)[-1])
        requires_only = (host / "go.mod").read_text()
        run([args.go, "list", "-mod=mod", "-deps", ".", "./cmd/monitor-mcp"], host, "load-production-graph")
        report["private_modules"] = []
        for name, module in MODULES.items():
            selected = json.loads(run([args.go, "list", "-mod=readonly", "-m", "-json", module], host, "selected-" + name))
            directory = Path(selected.get("Dir", "")).resolve()
            if selected.get("Version") != FIXTURE or selected.get("Replace") is not None or not directory.is_relative_to(root / "modcache"):
                raise ReproductionError("private module did not resolve from owned proxy cache")
            report["private_modules"].append({"path": module, "version": selected["Version"], "sum": selected.get("Sum"), "go_mod_sum": selected.get("GoModSum"), "replace": False, "owned_module_cache": True})
        report["artifacts"] = []
        for name, package in (("host", "."), ("monitor-mcp", "./cmd/monitor-mcp")):
            binary = root / name
            run([args.go, "build", "-mod=readonly", "-o", binary, package], host, "build-" + name)
            if not binary.is_file() or binary.stat().st_size == 0:
                raise ReproductionError("build artifact missing")
            report["artifacts"].append({"name": name, "bytes": binary.stat().st_size, "sha256": file_digest(binary), "executed": False})
        report["status"] = "passed"
        succeeded = True
    except BaseException as failure:
        report["status"] = "failed"
        report["failure"] = str(failure) if isinstance(failure, ReproductionError) else type(failure).__name__
    finally:
        if host is not None and before is not None:
            final_mod = (host / "go.mod").read_text()
            final_sum = (host / "go.sum").read_text() if (host / "go.sum").exists() else ""
            report["temporary_host_manifest_diff"] = "".join(difflib.unified_diff(before.splitlines(keepends=True), final_mod.splitlines(keepends=True), fromfile="HEAD/engine/go.mod", tofile="fixture/engine/go.mod"))
            report["temporary_host_go_sum_diff"] = "".join(difflib.unified_diff(sum_before.splitlines(keepends=True), final_sum.splitlines(keepends=True), fromfile="HEAD/engine/go.sum", tofile="fixture/engine/go.sum"))
            if requires_only is not None:
                report["explicit_fixture_require_diff"] = "".join(difflib.unified_diff(before.splitlines(keepends=True), requires_only.splitlines(keepends=True), fromfile="HEAD/engine/go.mod", tofile="fixture-requires/engine/go.mod"))
                report["dependency_graph_manifest_diff"] = "".join(difflib.unified_diff(requires_only.splitlines(keepends=True), final_mod.splitlines(keepends=True), fromfile="fixture-requires/engine/go.mod", tofile="resolved-fixture/engine/go.mod"))
        if succeeded:
            try:
                cleanup(root, marker)
                report["owned_root_removed"] = True
            except Exception:
                report["status"] = "failed"
                report["failure"] = "owned fixture cleanup failed; retained"
                succeeded = False
        if not succeeded:
            print("failed fixture retained for this task's diagnosis:", root)
        descriptor = os.open(args.report, os.O_WRONLY | os.O_CREAT | os.O_EXCL, 0o600)
        with os.fdopen(descriptor, "w") as output:
            json.dump(report, output, indent=2)
            output.write("\n")
    print("local fixture status:", report["status"], "; not a release or remote resolution proof")
    return 0 if succeeded else 1

if __name__ == "__main__":
    raise SystemExit(main())
