import functools
from .problems import Problem, CppOJ, CompileCPP, RunProgram
import asyncio
import contextlib
import tempfile
import shutil
import aiofiles
import os
import json
import base64
import dataclasses
import shlex

@dataclasses.dataclass
class BuildBinaryResult:
    binary: bytes
    args: list[str] | None = None
    capture_pattern: str | None = None


@functools.singledispatch
async def build_binary(problem: Problem) -> BuildBinaryResult:
    raise NotImplementedError(f"Unsupported problem type: {problem.type}")

@contextlib.contextmanager
def _temp_dir():
    dirpath = tempfile.mkdtemp()
    try:
        yield dirpath
    finally:
        shutil.rmtree(dirpath)


@build_binary.register
async def _(cpp_oj: CppOJ) -> BuildBinaryResult:
    with _temp_dir() as dirpath:
        source_files = []

        for file in cpp_oj.files:
            async with aiofiles.open(f"{dirpath}/{file.filename}", "wb") as f:
                await f.write(file.content)
                if file.filename.endswith(".cpp") or file.filename.endswith(".c"):
                    source_files.append(file.filename)
        
        async with aiofiles.open(f"{dirpath}/build.sh", "w") as f:
            await f.write(
                """#!/bin/bash
set -ex
if ! g++ -O2 -o solution {} -std=c++17 2> compile_errors.txt; then
    echo "Compilation failed. See compile_errors.txt for details."
    exit 1
else
    echo "Compilation succeeded. Removing compile_errors.txt"
    rm -f compile_errors.txt
fi
""".format(" ".join(shlex.quote(f) for f in source_files)))
        
        os.chmod(f"{dirpath}/build.sh", 0o755)


        async with aiofiles.open(f"{dirpath}/input_spec.jsonl", "w") as f:
            for test_case in cpp_oj.test_cases:
                spec: dict[str, str | int] = {
                    "name": test_case.name,
                    "input": base64.b64encode(test_case.input).decode("utf-8"),
                    "output": base64.b64encode(test_case.output).decode("utf-8"),
                }

                if test_case.time_limit is not None:
                    spec["time_limit"] = f"{test_case.time_limit:.4f}s"

                if test_case.memory_limit_in_mb is not None:
                    spec["memory_limit_in_mb"] = test_case.memory_limit_in_mb

                await f.write(json.dumps(spec))
                await f.write("\n")
        
        async with aiofiles.open(f"{dirpath}/judge.sh", "w") as f:
            await f.write(
                """#!/bin/bash
set -ex
ls -lha $PWD/solution
/envlet/judgelet \
    --runprog-bin /envlet/runprog \
    --test-bin $PWD/solution \
    --tests-file input_spec.jsonl | tee judge_result.jsonl
""")
        os.chmod(f"{dirpath}/judge.sh", 0o755)

        async with aiofiles.open(f"{dirpath}/main.sh", "w") as f:
            await f.write(
                """#!/bin/bash
set -e
if ! ./build.sh; then
    echo "Build failed."
    cat compile_errors.txt
    exit 0
fi
./judge.sh
echo "=== Judge result ==="
cat judge_result.jsonl
""")
        
        os.chmod(f"{dirpath}/main.sh", 0o755)


        output_file = f"{dirpath}/bin"
        
        proc = await asyncio.create_subprocess_exec(
            "makeself",
            "--gzip",
            dirpath,
            output_file,
            "CPP OJ Solution",
            "./main.sh",
            stdout=asyncio.subprocess.PIPE,
            stderr=asyncio.subprocess.PIPE,
        )
        _, stderr = await proc.communicate()
        if proc.returncode != 0:
            stderr_output = stderr.decode() if stderr else ""
            raise RuntimeError(f"Failed to create self-extracting archive: {stderr_output}")
        async with aiofiles.open(output_file, "rb") as f:
            binary_content = await f.read()
        return BuildBinaryResult(binary=binary_content,
                                 capture_pattern=r"^workspace/(judge_result\.jsonl|compile_errors\.txt)$",
                                 args=[
                                     "--target",
                                     "workspace"
                                 ])

@build_binary.register
async def _(compile_cpp: CompileCPP) -> BuildBinaryResult:
    with _temp_dir() as dirpath:
        source_files = []

        for file in compile_cpp.files:
            async with aiofiles.open(f"{dirpath}/{file.filename}", "wb") as f:
                await f.write(file.content)
                if file.filename.endswith(".cpp") or file.filename.endswith(".c"):
                    source_files.append(file.filename)
        
        async with aiofiles.open(f"{dirpath}/build.sh", "w") as f:
            await f.write(
                """#!/bin/bash
set -ex
if ! g++ -O2 -o solution {} -std=c++17 2> compile_errors.txt; then
    echo "Compilation failed. See compile_errors.txt for details."
else
    echo "Compilation succeeded. Removing compile_errors.txt"
    rm -f compile_errors.txt
fi
""".format(" ".join(shlex.quote(f) for f in source_files)))
        
        os.chmod(f"{dirpath}/build.sh", 0o755)
        output_file = f"{dirpath}/bin"
        proc = await asyncio.create_subprocess_exec(
            "makeself",
            "--gzip",
            dirpath,
            output_file,
            "CPP Compile Solution",
            "./build.sh",
            stdout=asyncio.subprocess.PIPE,
            stderr=asyncio.subprocess.PIPE,
        )
        _, stderr = await proc.communicate()
        if proc.returncode != 0:
            stderr_output = stderr.decode() if stderr else ""
            raise RuntimeError(f"Failed to create self-extracting archive: {stderr_output}")
        
        async with aiofiles.open(output_file, "rb") as f:
            binary_content = await f.read()
    
    return BuildBinaryResult(binary=binary_content,
                            capture_pattern=r"^workspace/(compile_errors\.txt|solution)$",
                            args=[
                                "--target",
                                "workspace"
                            ])

@build_binary.register
async def _(run_program: RunProgram) -> BuildBinaryResult:
    with _temp_dir() as dirpath:
        async with aiofiles.open(f"{dirpath}/bin", "wb") as f:
            await f.write(run_program.binary)
        
        os.chmod(f"{dirpath}/bin", 0o755)

        async with aiofiles.open(f"{dirpath}/main.sh", "w") as f:
            await f.write(
                f"""#!/bin/bash
set -e
cd $(dirname $0)
cat > input_b64 <<EOF
{base64.b64encode(run_program.stdin).decode("utf-8")}
EOF

base64 -d < input_b64 | /envlet/runprog \
    -tl {run_program.time_limit if run_program.time_limit is not None else 1:.4f}s \
    -ml {run_program.memory_limit_in_mb if run_program.memory_limit_in_mb is not None else 256}\
    -res runprog.result \
    -runner container \
    -unsafe \
    -cgroup \
    -bind-pwd \
    $PWD/bin { " ".join(shlex.quote(arg) for arg in run_program.args) if run_program.args else "" } > program.stdout 2> program.stderr
""")
        os.chmod(f"{dirpath}/main.sh", 0o755)

        output_file = f"{dirpath}/run_binary"
        proc = await asyncio.create_subprocess_exec(
            "makeself",
            "--gzip",
            dirpath,
            output_file,
            "Run Program",
            "./main.sh",
            stdout=asyncio.subprocess.PIPE,
            stderr=asyncio.subprocess.PIPE,
        )
        _, stderr = await proc.communicate()
        if proc.returncode != 0:
            stderr_output = stderr.decode() if stderr else ""
            raise RuntimeError(f"Failed to create self-extracting archive: {stderr_output}")
        
        async with aiofiles.open(output_file, "rb") as f:
            binary_content = await f.read()

    return BuildBinaryResult(binary=binary_content,
                            capture_pattern=r"^workspace/(runprog\.result|program\.stdout|program\.stderr)$",
                            args=[
                                "--target",
                                "workspace"
                            ])