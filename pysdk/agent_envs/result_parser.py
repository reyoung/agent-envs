import json
from .proxy_client import ExecResult
from .problems import Problem, CppOJ, CompileCPP, RunProgram, CompileIOIBinary
from .solutions import (
    Solution,
    CppOJSolution,
    OJResult,
    CompileSolution,
    RunProgramSolution,
    RunProgramStatus,
    CompileIOISolution,
)
import functools


@functools.singledispatch
async def parse_result(problem: Problem, exec_result: ExecResult) -> Solution:
    raise NotImplementedError(f"Unsupported problem type: {problem.type}")


def _ensure_succeed(exec_result: ExecResult):
    if exec_result.exit_code != 0:
        raise RuntimeError(
            f"Execution failed with exit code {exec_result.exit_code}.\n"
            f"Stdout: {exec_result.stdout.decode('utf-8')}\n"
            f"Stderr: {exec_result.stderr.decode('utf-8')}\n"
        )


@parse_result.register
async def _(problem: CppOJ, exec_result: ExecResult) -> Solution:
    _ensure_succeed(exec_result)
    files = exec_result.file_dict()
    compile_error_file = files.get("workspace/compile_errors.txt")
    judge_result_file = files.get("workspace/judge_result.jsonl")

    if compile_error_file is not None:
        compile_errors = compile_error_file.decode("utf-8")
        return CppOJSolution(results=[], compile_error=compile_errors)

    if judge_result_file is None:
        raise RuntimeError("Judge result file not found in execution result.")

    content = judge_result_file.decode("utf-8")
    results = []
    for line in content.split("\n"):
        line = line.strip()
        if not line:
            continue
        js_line = json.loads(line)
        results.append(OJResult.from_json(js_line))

    return CppOJSolution(results=results)


@parse_result.register
async def _(problem: CompileCPP, exec_result: ExecResult) -> Solution:
    _ensure_succeed(exec_result)
    files = exec_result.file_dict()
    compile_error_file = files.get("workspace/compile_errors.txt")
    result_binary_file = files.get("workspace/solution")
    if compile_error_file is not None:
        compile_error = compile_error_file.decode("utf-8")
    else:
        compile_error = None

    return CompileSolution(binary=result_binary_file, compile_error=compile_error)


@parse_result.register
async def _(run_program: RunProgram, exec_result: ExecResult) -> Solution:
    files = exec_result.file_dict()
    runprog_result = files.get("workspace/runprog.result")
    stdout = files.get("workspace/program.stdout", b"")
    stderr = files.get("workspace/program.stderr", b"")

    if runprog_result is None:
        # program failed to launch or runprog crashed before writing result
        return RunProgramSolution(
            exit_code=exec_result.exit_code,
            stdout=stdout,
            stderr=stderr,
            memory=None,
            time=None,
            status=RunProgramStatus.UNKNOWN,
        )

    status, time_ms, memory_kb, exit_code = _parse_runprog_result(runprog_result)
    return RunProgramSolution(
        exit_code=exit_code,
        stdout=stdout,
        stderr=stderr,
        memory=memory_kb,
        time=time_ms / 1000,
        status=status,
    )


def _parse_runprog_result(content: bytes) -> tuple[RunProgramStatus, int, int, int]:
    try:
        parts = content.decode("utf-8").strip().split()
    except UnicodeDecodeError as e:
        raise RuntimeError("runprog.result is not valid utf-8") from e

    if len(parts) != 4:
        raise RuntimeError(f"runprog.result malformed: expected 4 space-separated integers, got {len(parts)}")

    try:
        status_code, time_ms, memory_kb, exit_code = map(int, parts)
    except ValueError as e:
        raise RuntimeError("runprog.result contains non-integer values") from e

    status = _status_from_code(status_code)
    return status, time_ms, memory_kb, exit_code


def _status_from_code(code: int) -> RunProgramStatus:
    mapping = {
        0: RunProgramStatus.NORMAL,
        1: RunProgramStatus.INVALID,
        2: RunProgramStatus.RUNTIME_ERROR,
        3: RunProgramStatus.MEMORY_LIMIT_EXCEEDED,
        4: RunProgramStatus.TIME_LIMIT_EXCEEDED,
        5: RunProgramStatus.OUTPUT_LIMIT_EXCEEDED,
        6: RunProgramStatus.DISALLOWED_SYSCALL,
        7: RunProgramStatus.FATAL_ERROR,
    }
    return mapping.get(code, RunProgramStatus.UNKNOWN)

@parse_result.register
async def _(compile_ioi_binary: CompileIOIBinary, exec_result: ExecResult) -> Solution:
    _ensure_succeed(exec_result)
    files = {k.removeprefix("workspace/"): v for k, v in exec_result.file_dict().items()}
    return CompileIOISolution(binaries=files)