import asyncio
from agent_envs.proxy_client import ProxyClient
from agent_envs.executor import Executor
from agent_envs.problems import CompileCPP, RunProgram, FileContent
from agent_envs.solutions import CompileSolution, RunProgramSolution, Solution
from concurrent.futures import ThreadPoolExecutor
import json
import base64
import dataclasses
import typing
import math
import enum

@dataclasses.dataclass(frozen=True)
class SubTaskCase:
    test_case_name: str
    time_limit: float
    memory_limit: float

class TestCase(typing.TypedDict):
    input: str
    output: str

class Limit(typing.TypedDict):
    time_limit: float
    memory_limit: float

class RawSubTask(typing.TypedDict):
    name: str
    score: int
    test_case_names: list[str]
    limit: Limit

@dataclasses.dataclass(frozen=True)
class ParsedSubTask:
    name: str
    score: int
    cases: list[SubTaskCase]

class RunProgramWithoutBinary:
    def __init__(self,
                 stdin: bytes,
                 time_limit: float | None = None,
                 memory_limit_in_mb: int | None = None,
                 args: list[str] | None = None) -> None:
        self.stdin = stdin
        self.time_limit = time_limit
        self.memory_limit_in_mb = memory_limit_in_mb
        self.args = args
        self.type: typing.Literal["run_program"] = "run_program"
    
    def build(self, binary: bytes) -> RunProgram:
        return RunProgram(
            binary=binary,
            stdin=self.stdin,
            time_limit=self.time_limit,
            memory_limit_in_mb=self.memory_limit_in_mb,
            args=self.args,
        )

@dataclasses.dataclass(frozen=False)
class RunTaskCase:
    problem: RunProgramWithoutBinary
    stdout: str
    solution: RunProgramSolution | None = None

@dataclasses.dataclass(frozen=True)
class OJCheckTasks:
    test_cases: dict[SubTaskCase, RunTaskCase]
    sub_tasks: list[ParsedSubTask]

def _build_sub_task(built_test_cases: dict[SubTaskCase, RunTaskCase],
                    test_cases: dict[str, TestCase],
                    raw_sub_task: RawSubTask) -> ParsedSubTask:
    cases : list[SubTaskCase] = []
    for test_case_name in raw_sub_task["test_case_names"]:
        case_ = SubTaskCase(
            test_case_name=test_case_name,
            time_limit=raw_sub_task["limit"]["time_limit"],
            memory_limit=raw_sub_task["limit"]["memory_limit"],
        )
        if case_ not in built_test_cases:
            built_test_cases[case_] = RunTaskCase(
                problem=RunProgramWithoutBinary(
                    stdin=base64.b64encode(test_cases[test_case_name]["input"].encode("utf-8")),
                    time_limit=case_.time_limit,
                    memory_limit_in_mb=int(math.ceil( case_.memory_limit / 1024 / 1024)),
                    args=[],
                ),
                stdout=test_cases[test_case_name]["output"],
            )
        cases.append(case_)
    
    return ParsedSubTask(
        name=raw_sub_task["name"],
        score=raw_sub_task["score"],
        cases=cases,
    )



def _build_sub_task_case(test_cases: dict[str, TestCase], sub_tasks: list[RawSubTask]) -> OJCheckTasks:
    built_test_cases: dict[SubTaskCase, RunTaskCase] = {}

    parsed_sub_tasks = [_build_sub_task(built_test_cases, test_cases, sub_task) for sub_task in sub_tasks]

    return OJCheckTasks(
        test_cases=built_test_cases,
        sub_tasks=parsed_sub_tasks,
    )

class CommandType(enum.StrEnum):
    SOLUTION = "solution"
    CHECKER = "checker"
    MANAGER = "manager"


def _build_solution_compile_commands(files: dict[str, str]) -> CompileCPP:
    """
    构造Solution的CompileCPP编译命令
    """
    sources = [
        FileContent(
            filename="solution.cpp",
            content=files["solution.cpp"].encode("utf-8"),
        )
    ]
    if 'grader.cpp' in files:
        sources.append(
            FileContent(
                filename="grader.cpp",
                content=files["grader.cpp"].encode("utf-8"),
            )
        )
    else:
        raise RuntimeError("grader.cpp not found in grader files.")
    
    headers = [
        FileContent(
            filename=k,
            content=v.encode("utf-8"),
        ) for k, v in files.items() if k.endswith(".h")
    ]

    return CompileCPP(
        files=sources + headers,
    )
        
def _build_checker_compile_commands(files: dict[str, str]) -> typing.Generator[tuple[CommandType, CompileCPP], None, None]:
    headers = [
        FileContent(
            filename=k,
            content=v.encode("utf-8"),
        ) for k, v in files.items() if k.endswith(".h")
    ]
    if 'checker.cpp' in files:
        yield (CommandType.CHECKER, CompileCPP(
            files=[
                FileContent(
                    filename="checker.cpp",
                    content=files["checker.cpp"].encode("utf-8"),
                )
            ] + headers,
        ))
    
    if 'manager.cpp' in files:
        yield (CommandType.MANAGER, CompileCPP(
            files=[
                FileContent(
                    filename="manager.cpp",
                    content=files["manager.cpp"].encode("utf-8"),
                )
            ] + headers,
        ))



class IOIProblem:
    def __init__(self, json_payload: str, solution: str | None = None) -> None:
        js = json.loads(json_payload)
        sources: dict[str, str] = js["metadata"]["grader_files"]
        if solution is None:
            solution = js["metadata"]["sample_solution"]
            assert solution is not None, "Solution code must be provided either in the JSON payload or as an argument."
        
        sources["solution.cpp"] = solution

        # self._solution_compile_command = _build_solution_compile_commands(sources)
        self._compile_commands = dict(_build_checker_compile_commands(sources))
        self._compile_commands[CommandType.SOLUTION] = _build_solution_compile_commands(sources)

        test_cases = js["metadata"]["test_cases"]
        sub_tasks = js["metadata"]["sub_tasks"]
        self._check_taks = _build_sub_task_case(test_cases, sub_tasks)

    def compile_command(self) -> dict[CommandType, CompileCPP]:
        return self._compile_commands



    def test_cases(self, binary: bytes) -> typing.Generator[tuple[SubTaskCase, RunProgram], None, None]:
        for case_, run_task_case in self._check_taks.test_cases.items():
            program = run_task_case.problem.build(binary)
            yield case_, program
    
    def score(self, solutions: typing.Generator[tuple[SubTaskCase, RunProgramSolution], None, None]) -> int:
        pass


@dataclasses.dataclass
class IOIJudgeResult:
    score: float
    reasons: list[str] = dataclasses.field(default_factory=list)

T = typing.TypeVar("T")

class CompileError(RuntimeError):
    pass


class IOIJudger:
    def __init__(self, 
                 concurrency: int,
                 endpoint: str,
                 num_threads: int) -> None:
        self._concurrency = asyncio.Semaphore(concurrency)
        self._proxy_client = ProxyClient(endpoint)
        self._executor = Executor(self._proxy_client)
        self._executor.register_queue("compile_cpp", "gcc_jobs")
        self._executor.register_queue("run_program", "gcc_jobs")
        self._thread_pool = ThreadPoolExecutor(max_workers=num_threads)

    async def judge(self, line: str, solution: str | None = None) -> IOIJudgeResult:
        loop = asyncio.get_event_loop()
        problem = await loop.run_in_executor(self._thread_pool, IOIProblem, line, solution)

        binaries = await self._compile_problem(problem)
        
        raise NotImplementedError()
    
    async def _compile_problem(self, problem: IOIProblem) -> dict[str, bytes]:
        commands = problem.compile_command()
        compile_result_tasks: dict[str, asyncio.Task[Solution]] = {
            k: await self._create_task(
                self._executor.execute(problem=cmd)
            ) for k, cmd in commands.items()
        }
        result: dict[str, bytes] = {}
        for k, task in compile_result_tasks.items():
            task = await task
            if not isinstance(task, CompileSolution):
                raise RuntimeError(f"Expected CompileSolution, got {type(task)} for {k}.")
            
            if task.compile_error is not None:
                raise CompileError(f"Compilation error in {k}:\n{task.compile_error}")
            
            assert task.binary is not None
            result[k] = task.binary

        return result

    async def _create_task(self, coro: asyncio._CoroutineLike[T]) -> asyncio.Task[T]:
        loop = asyncio.get_event_loop()
        await self._concurrency.acquire()
        task = loop.create_task(coro)
        task.add_done_callback(lambda t: self._concurrency.release())
        return task


    
    async def close(self):
        await self._proxy_client.close()
        self._thread_pool.shutdown(wait=True)

def main():
    pass

if __name__ == "__main__":
    main()