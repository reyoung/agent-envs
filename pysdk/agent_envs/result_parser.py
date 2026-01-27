import json
from .proxy_client import ExecResult
from .problems import Problem, CppOJ, CompileCPP, RunProgram
from .solutions import Solution, CppOJSolution, OJResult, CompileSolution, RunProgramSolution
import functools
import pickle


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
    pickle.dump((run_program, exec_result), open("run_program_debug.pkl", "wb"))
    raise NotImplementedError("RunProgram result parsing not implemented yet.")