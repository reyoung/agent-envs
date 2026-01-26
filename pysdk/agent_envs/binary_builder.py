import functools
from .problems import Problem, CppOJ
import asyncio
import contextlib
import tempfile
import shutil
import aiofiles
import os
import json
import base64
import dataclasses

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
g++ -O2 -o solution {} -std=c++17
""".format(" ".join(source_files)))
        
        os.chmod(f"{dirpath}/build.sh", 0o755)


        async with aiofiles.open(f"{dirpath}/input_spec.jsonl", "w") as f:
            for test_case in cpp_oj.test_cases:
                await f.write(json.dumps({
                    "name": test_case.name,
                    "input": base64.b64encode(test_case.input).decode("utf-8"),
                    "output": base64.b64encode(test_case.output).decode("utf-8"),
                }))
                await f.write("\n")
        
        async with aiofiles.open(f"{dirpath}/judge.sh", "w") as f:
            await f.write(
                """#!/bin/bash
set -ex
/envlet/judgelet --test-bin ./solution --tests-file input_spec.jsonl > judge_result.jsonl
""")
        os.chmod(f"{dirpath}/judge.sh", 0o755)

        async with aiofiles.open(f"{dirpath}/main.sh", "w") as f:
            await f.write(
                """#!/bin/bash
set -e
./build.sh
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
                                 capture_pattern=r"^workspace/judge_result\.jsonl$",
                                 args=[
                                     "--target",
                                     "workspace"
                                 ])
