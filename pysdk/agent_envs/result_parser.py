import json
from .proxy_client import ExecResult
from .problems import Problem, CppOJ, CompileCPP
from .solutions import Solution, CppOJSolution, OJResult, CompileSolution
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
    compile_error_file = None
    judge_result_file = None
    for f in exec_result.files:
        if f.filename.endswith("judge_result.jsonl"):
            judge_result_file = f
        elif f.filename.endswith("compile_errors.txt"):
            compile_error_file = f
    
    if compile_error_file is not None:
        compile_errors = compile_error_file.content.decode("utf-8")
        return CppOJSolution(results=[], compile_error=compile_errors)

    if judge_result_file is None:
        raise RuntimeError("Judge result file not found in execution result.")
    
    content = judge_result_file.content.decode("utf-8")
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
    compile_error_file = None
    result_binary_file = None
    for f in exec_result.files:
        if f.filename.endswith("compile_errors.txt"):
            compile_error_file = f.content.decode("utf-8")

        if f.filename.endswith("solution"):
            result_binary_file = f.content

    return CompileSolution(binary=result_binary_file, compile_error=compile_error_file)