import json
from .proxy_client import ExecResult
from .problems import Problem, CppOJ
from .solutions import Solution, CppOJSolution, OJResult
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
    judge_result_file = None
    for f in exec_result.files:
        if f.filename.endswith("judge_result.jsonl"):
            judge_result_file = f
            break
    if judge_result_file is None:
        raise RuntimeError("Judge result file not found in execution result.")
    
    results = []
    for line in judge_result_file.content.decode("utf-8").split():
        js_line = json.loads(line)
        results.append(OJResult.from_json(js_line))
    
    return CppOJSolution(results=results)