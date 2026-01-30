import base64
import dataclasses
import functools
import io
import json
import tarfile
import textwrap
from typing import Iterable

from .problems import Problem, CppOJ, CompileCPP, RunProgram


@dataclasses.dataclass
class BuildBinaryResult:
    binary: bytes
    args: list[str] | None = None
    capture_pattern: str | None = None


@functools.singledispatch
def build_binary(problem: Problem) -> BuildBinaryResult:  # pragma: no cover - dispatched implementations below
    raise NotImplementedError(f"Unsupported problem type: {problem}")


# -------------------- helpers --------------------

def _tar_gz_bytes(files: Iterable[tuple[str, bytes, int]]) -> bytes:
    """
    Build a gzip-compressed tar archive entirely in memory.

    :param files: iterable of (path, content, mode)
    :return: bytes of the tar.gz archive
    """
    buf = io.BytesIO()
    with tarfile.open(fileobj=buf, mode="w:gz") as tar:
        for name, content, mode in files:
            info = tarfile.TarInfo(name=name)
            info.size = len(content)
            info.mode = mode
            tar.addfile(info, io.BytesIO(content))
    return buf.getvalue()


def _self_extracting_script(payload_tar_gz: bytes, command: str) -> bytes:
    """
    Generate a bash self-extracting archive script similar to makeself.
    Supports a minimal CLI: --target <dir> to control extraction path.
    """
    payload_b64 = base64.b64encode(payload_tar_gz).decode("ascii")

    script = f"""#!/bin/bash
set -euo pipefail

PAYLOAD_CMD={json.dumps(command)}

usage() {{
  echo "Usage: $0 [--target DIR]"
  exit 1
}}

TARGET=""
while [[ $# -gt 0 ]]; do
  case "$1" in
    --target)
      [[ $# -ge 2 ]] || usage
      TARGET="$2"
      shift 2
      ;;
    *)
      break
      ;;
  esac
done

if [[ -z "$TARGET" ]]; then
  WORKDIR=$(mktemp -d)
  cleanup() {{ rm -rf "$WORKDIR"; }}
  trap cleanup EXIT
else
  WORKDIR="$TARGET"
  mkdir -p "$WORKDIR"
fi

ARCHIVE_LINE=$(awk '/^__ARCHIVE_BELOW__/ {{print NR + 1; exit 0;}}' "$0")
tail -n +"$ARCHIVE_LINE" "$0" | base64 -d | tar -xz -C "$WORKDIR"

cd "$WORKDIR"
bash "$PAYLOAD_CMD"
exit $?
__ARCHIVE_BELOW__
{payload_b64}
"""
    # Ensure trailing newline for POSIX shells
    return script.encode("utf-8")


def _cpp_build_sh(source_files: list[str]) -> bytes:
    return textwrap.dedent(
        f"""\
        #!/bin/bash
        set -ex
        if ! g++ -O2 -o solution {' '.join(shlex_quote(f) for f in source_files)} -std=c++17 2> compile_errors.txt; then
            echo "Compilation failed. See compile_errors.txt for details."
            exit 1
        else
            echo "Compilation succeeded. Removing compile_errors.txt"
            rm -f compile_errors.txt
        fi
        """
    ).encode("utf-8")


def _cpp_main_sh() -> bytes:
    return textwrap.dedent(
        """\
        #!/bin/bash
        set -e
        if ! ./build.sh; then
            echo "Build failed."
            cat compile_errors.txt
            exit 0
        fi
        ./judge.sh
        echo "=== Judge result ==="
        cat judge_result.jsonl
        """
    ).encode("utf-8")


def _cpp_judge_sh() -> bytes:
    return textwrap.dedent(
        """\
        #!/bin/bash
        set -ex
        ls -lha "$PWD/solution"
        /envlet/judgelet \
            --runprog-bin /envlet/runprog \
            --test-bin "$PWD/solution" \
            --tests-file input_spec.jsonl | tee judge_result.jsonl
        """
    ).encode("utf-8")


def _input_spec_jsonl(test_cases) -> bytes:
    lines: list[str] = []
    for test_case in test_cases:
        spec: dict[str, str | int] = {
            "name": test_case.name,
            "input": base64.b64encode(test_case.input).decode("utf-8"),
            "output": base64.b64encode(test_case.output).decode("utf-8"),
        }
        if test_case.time_limit is not None:
            spec["time_limit"] = f"{test_case.time_limit:.4f}s"
        if test_case.memory_limit_in_mb is not None:
            spec["memory_limit_in_mb"] = test_case.memory_limit_in_mb
        lines.append(json.dumps(spec))
    return ("\n".join(lines) + ("\n" if lines else "")).encode("utf-8")


def shlex_quote(s: str) -> str:
    """Minimal shlex.quote replacement (kept local to avoid extra imports)."""
    if not s:
        return "''"
    if all(c.isalnum() or c in "._-/" for c in s):
        return s
    return "'" + s.replace("'", "'\"'\"'") + "'"


# -------------------- dispatch implementations --------------------


@build_binary.register
def _(cpp_oj: CppOJ) -> BuildBinaryResult:
    source_files: list[str] = []
    files: list[tuple[str, bytes, int]] = []

    for file in cpp_oj.files:
        files.append((file.filename, file.content, 0o644))
        if file.filename.endswith(".cpp") or file.filename.endswith(".c"):
            source_files.append(file.filename)

    files.extend(
        [
            ("build.sh", _cpp_build_sh(source_files), 0o755),
            ("judge.sh", _cpp_judge_sh(), 0o755),
            ("main.sh", _cpp_main_sh(), 0o755),
            ("input_spec.jsonl", _input_spec_jsonl(cpp_oj.test_cases), 0o644),
        ]
    )

    payload = _tar_gz_bytes(files)
    script = _self_extracting_script(payload, "./main.sh")

    return BuildBinaryResult(
        binary=script,
        capture_pattern=r"^workspace/(judge_result\.jsonl|compile_errors\.txt)$",
        args=["--target", "workspace"],
    )


@build_binary.register
def _(compile_cpp: CompileCPP) -> BuildBinaryResult:
    source_files: list[str] = []
    files: list[tuple[str, bytes, int]] = []

    for file in compile_cpp.files:
        files.append((file.filename, file.content, 0o644))
        if file.filename.endswith(".cpp") or file.filename.endswith(".c"):
            source_files.append(file.filename)

    files.append(("build.sh", _cpp_build_sh(source_files), 0o755))

    payload = _tar_gz_bytes(files)
    script = _self_extracting_script(payload, "./build.sh")

    return BuildBinaryResult(
        binary=script,
        capture_pattern=r"^workspace/(compile_errors\.txt|solution)$",
        args=["--target", "workspace"],
    )


@build_binary.register
def _(run_program: RunProgram) -> BuildBinaryResult:
    files: list[tuple[str, bytes, int]] = []

    for file in run_program.files:
        # Preserve executable bit for all provided files to mirror previous behavior
        files.append((file.filename, file.content, 0o755))

    run_script = textwrap.dedent(
        f"""\
        #!/bin/bash
        set -e
        cd "$(dirname "$0")"
        ls -lha .
        echo "Running program: {shlex_quote(run_program.entrypoint)}"
        /envlet/runprog \\
            -tl {run_program.time_limit if run_program.time_limit is not None else 1:.4f}s \\
            -ml {run_program.memory_limit_in_mb if run_program.memory_limit_in_mb is not None else 256} \\
            -res runprog.result \\
            -runner container \\
            -unsafe \\
            -cgroup \\
            -bind-pwd \\
            "$PWD/{shlex_quote(run_program.entrypoint)}" 1> program.stdout 2> program.stderr
        """
    ).encode("utf-8")

    files.append(("run", run_script, 0o755))

    payload = _tar_gz_bytes(files)
    script = _self_extracting_script(payload, "./run")

    return BuildBinaryResult(
        binary=script,
        capture_pattern=r"^workspace/(runprog\.result|program\.stdout|program\.stderr)$",
        args=["--target", "workspace"],
    )
