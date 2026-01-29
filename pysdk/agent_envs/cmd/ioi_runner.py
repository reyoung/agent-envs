import asyncio
from agent_envs.proxy_client import ProxyClient
from agent_envs.executor import Executor
from agent_envs.problems import CompileCPP, RunProgram, FileContent
from agent_envs.solutions import (
    CompileSolution,
    RunProgramSolution,
    RunProgramStatus,
    Solution,
)
from concurrent.futures import ThreadPoolExecutor
import json
import dataclasses
import typing
import math
import enum
import argparse
import sys

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
    score: float
    test_case_names: list[str]
    limit: Limit


@dataclasses.dataclass(frozen=True)
class ParsedSubTask:
    name: str
    score: float
    cases: list[SubTaskCase]


class RunProgramWithoutBinary:
    def __init__(
        self,
        input: str,
        output: str,
        time_limit: float | None = None,
        memory_limit_in_mb: int | None = None,
    ) -> None:
        self.time_limit = time_limit
        self.memory_limit_in_mb = memory_limit_in_mb
        self.input = input
        self.output = output
        self.type: typing.Literal["run_program"] = "run_program"
    
    def _shell(
            self, 
            files: dict[CommandType, bytes],
            entrypoint: str,
    ) -> RunProgram:
        file_list = [
            FileContent(filename=cmd_type.value, content=content) for cmd_type, content in files.items()
        ]
        file_list.append(
            FileContent(
                filename="input.txt",
                content=self.input.encode("utf-8"),
            )
        )
        file_list.append(
            FileContent(
                filename="output.txt",
                content=self.output.encode("utf-8"),
            )
        )
        file_list.append(
            FileContent(
                filename="main.sh",
                content=entrypoint.encode("utf-8"),
            )
        )
        return RunProgram(
            entrypoint="main.sh",
            files=file_list,
            time_limit=self.time_limit,
            memory_limit_in_mb=self.memory_limit_in_mb,
        )

    def build(self, files: dict[CommandType, bytes], problem: IOIProblem) -> RunProgram:
        if CommandType.CHECKER in files and CommandType.MANAGER not in files:
            return self._shell(files, """#!/bin/bash
set -e
cd $(dirname $0)                          
./solution < input.txt | ./checker input.txt /dev/stdin output.txt
""")    
        if CommandType.MANAGER in files and CommandType.CHECKER not in files:
            if problem.has_src_file("testlib.h"):
                return self._shell(files, """#!/bin/bash
set -e
cd $(dirname $0)
mkfifo ./solution_input.fifo
mkfifo ./solution_output.fifo
./solution ./solution_input.fifo ./solution_output.fifo &
SOLUTION_PID=$!
./manager ./solution_output.fifo ./solution_input.fifo < ./input.txt  &
MANAGER_PID=$!

trap "kill -9 $MANAGER_PID; kill -9 $SOLUTION_PID" SIGINT
trap "kill -9 $MANAGER_PID; kill -9 $SOLUTION_PID" SIGTERM

wait $MANAGER_PID
wait $SOLUTION_PID

rm ./solution_input.fifo
rm ./solution_output.fifo                                                         
""")


            raise NotImplementedError("Manager-only checking is not implemented.")
        
        raise NotImplementedError("Unsupported combination of solution/checker/manager binaries.")

    def _build_run_program_with_manager_and_testlib_h(
            self, files: dict[CommandType, bytes], problem: IOIProblem
    ) -> RunProgram:
        ...


@dataclasses.dataclass(frozen=False)
class RunTaskCase:
    problem: RunProgramWithoutBinary
    solution: RunProgramSolution | None = None


@dataclasses.dataclass(frozen=True)
class OJCheckTasks:
    test_cases: dict[SubTaskCase, RunTaskCase]
    sub_tasks: list[ParsedSubTask]


def _build_sub_task(
    built_test_cases: dict[SubTaskCase, RunTaskCase],
    test_cases: dict[str, TestCase],
    raw_sub_task: RawSubTask,
) -> ParsedSubTask:
    cases: list[SubTaskCase] = []
    for test_case_name in raw_sub_task["test_case_names"]:
        case_ = SubTaskCase(
            test_case_name=test_case_name,
            time_limit=raw_sub_task["limit"]["time_limit"],
            memory_limit=raw_sub_task["limit"]["memory_limit"],
        )
        if case_ not in built_test_cases:
            built_test_cases[case_] = RunTaskCase(
                problem=RunProgramWithoutBinary(
                    input=test_cases[test_case_name]["input"],
                    output=test_cases[test_case_name]["output"],
                    time_limit=case_.time_limit,
                    memory_limit_in_mb=int(math.ceil(case_.memory_limit / 1024 / 1024)),
                ),
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
    if "grader.cpp" in files:
        sources.append(
            FileContent(
                filename="grader.cpp",
                content=files["grader.cpp"].encode("utf-8"),
            )
        )
        
    elif "stub.cpp" in files:
        sources.append(
            FileContent(
                filename="stub.cpp",
                content=files["stub.cpp"].encode("utf-8"),
            )
        )

    else:
        raise RuntimeError("grader.cpp not found in grader files. file names {}".format(files.keys()))

    headers = [
        FileContent(
            filename=k,
            content=v.encode("utf-8"),
        )
        for k, v in files.items()
        if k.endswith(".h")
    ]

    return CompileCPP(
        files=sources + headers,
    )


def _build_checker_compile_commands(
    files: dict[str, str],
) -> typing.Generator[tuple[CommandType, CompileCPP], None, None]:
    headers = [
        FileContent(
            filename=k,
            content=v.encode("utf-8"),
        )
        for k, v in files.items()
        if k.endswith(".h")
    ]
    if "checker.cpp" in files:
        yield (
            CommandType.CHECKER,
            CompileCPP(
                files=[
                    FileContent(
                        filename="checker.cpp",
                        content=files["checker.cpp"].encode("utf-8"),
                    )
                ]
                + headers,
            ),
        )

    if "manager.cpp" in files:
        yield (
            CommandType.MANAGER,
            CompileCPP(
                files=[
                    FileContent(
                        filename="manager.cpp",
                        content=files["manager.cpp"].encode("utf-8"),
                    )
                ]
                + headers,
            ),
        )


class ScoreDetail(typing.NamedTuple):
    task_score: float
    test_score: float
    n_cases: int

    @property
    def score(self) -> float:
        return (self.test_score / self.n_cases) * self.task_score


class IOIProblem:
    def __init__(self, json_payload: str, solution: str | None = None) -> None:
        js = json.loads(json_payload)
        sources: dict[str, str] = js["metadata"]["grader_files"]
        self._id = js["metadata"]["id"]
        self._year = js["metadata"]["year"]
        if solution is None:
            solution = js["metadata"]["sample_solution"]
            assert solution is not None, "Solution code must be provided either in the JSON payload or as an argument."

        sources["solution.cpp"] = solution

        self._compile_commands = dict(_build_checker_compile_commands(sources))
        self._compile_commands[CommandType.SOLUTION] = _build_solution_compile_commands(sources)

        test_cases = js["metadata"]["test_cases"]
        sub_tasks = js["metadata"]["sub_tasks"]
        self._check_tasks = _build_sub_task_case(test_cases, sub_tasks)
        self._source_file_names = set(sources.keys())
    
    def has_src_file(self, filename: str) -> bool:
        return filename in self._source_file_names

    @property
    def problem_id(self) -> str:
        return self._id

    @property
    def year(self) -> int:
        return self._year

    def compile_command(self) -> dict[CommandType, CompileCPP]:
        return self._compile_commands

    def check_tasks(self) -> OJCheckTasks:
        return self._check_tasks

    def score(self, results: dict[SubTaskCase, JudgeResult]) -> dict[str, ScoreDetail]:
        return {
            sub_task.name: ScoreDetail(
                sub_task.score,
                sum(results[c].get_score() for c in sub_task.cases),
                len(sub_task.cases),
            )
            for sub_task in self._check_tasks.sub_tasks
        }


class CompileError(RuntimeError):
    pass


@dataclasses.dataclass(frozen=True)
class SystemError:
    stdout: str
    stderr: str
    exit_code: int

    def get_score(self) -> float:
        return 0.0


class TimeLimitExceeded:
    def get_score(self) -> float:
        return 0.0


class MemoryLimitExceeded:
    def get_score(self) -> float:
        return 0.0


@dataclasses.dataclass(frozen=True)
class JudgedScore:
    score: float
    detail: str
    time: float
    memory_in_mb: float

    def get_score(self) -> float:
        return self.score


@dataclasses.dataclass(frozen=True)
class UnknownResult:
    solution: RunProgramSolution

    def get_score(self) -> float:
        return 0.0


JudgeResult = TimeLimitExceeded | SystemError | MemoryLimitExceeded | JudgedScore | UnknownResult

Reason = dict[SubTaskCase, JudgeResult] | str


@dataclasses.dataclass
class IOIJudgeResult:
    scores: dict[str, ScoreDetail]
    reason: Reason

    @property
    def score(self) -> float:
        return sum(score_detail.score for score_detail in self.scores.values())


def _parse_result(result: RunProgramSolution) -> JudgeResult:
    if result.status is None:
        return SystemError(
            stdout=result.stdout.decode("utf-8"),
            stderr=result.stderr.decode("utf-8"),
            exit_code=result.exit_code,
        )

    if result.status == RunProgramStatus.TIME_LIMIT_EXCEEDED:
        return TimeLimitExceeded()

    if result.status == RunProgramStatus.MEMORY_LIMIT_EXCEEDED:
        return MemoryLimitExceeded()

    if result.status == RunProgramStatus.NORMAL:
        score = float(result.stdout.decode("utf-8").strip())
        detail = result.stderr.decode("utf-8").strip()
        return JudgedScore(
            score=score,
            detail=detail,
            time=result.time if result.time is not None else 0.0,
            memory_in_mb=float(result.memory) / 1024 if result.memory is not None else 0.0,
        )

    return UnknownResult(solution=result)


T = typing.TypeVar("T")


class IOIJudger:
    def __init__(self, concurrency: int, endpoint: str, num_threads: int) -> None:
        self._concurrency = asyncio.Semaphore(concurrency)
        self._proxy_client = ProxyClient(endpoint)
        self._executor = Executor(self._proxy_client)
        self._executor.register_queue("compile_cpp", "gcc_jobs")
        self._executor.register_queue("run_program", "gcc_jobs")
        self._thread_pool = ThreadPoolExecutor(max_workers=num_threads)

    async def judge(self, line: str, solution: str | None = None) -> IOIJudgeResult:
        loop = asyncio.get_running_loop()
        problem = await loop.run_in_executor(self._thread_pool, IOIProblem, line, solution)

        try:
            binaries = await self._compile_problem(problem)
        except CompileError as e:
            return IOIJudgeResult(
                scores={},
                reason=str(e),
            )
        check_results = await self._judge(problem, binaries)

        scores = await loop.run_in_executor(self._thread_pool, problem.score, check_results)
        return IOIJudgeResult(
            scores=scores,
            reason=check_results,
        )

    async def _judge(self, problem: IOIProblem, binaries: dict[CommandType, bytes]) -> dict[SubTaskCase, JudgeResult]:
        check_tasks = problem.check_tasks()
        check_task_futures: dict[SubTaskCase, asyncio.Task[Solution]] = {}
        for case_, run_task_case in check_tasks.test_cases.items():
            run_program = run_task_case.problem.build(binaries, problem)
            solution_task = await self._create_task(self._executor.execute(problem=run_program))
            check_task_futures[case_] = solution_task

        check_results: dict[SubTaskCase, RunProgramSolution] = {}
        for case_, task in check_task_futures.items():
            result = await task
            if not isinstance(result, RunProgramSolution):
                raise RuntimeError(f"Expected RunProgramSolution, got {type(result)}.")
            check_results[case_] = result

        loop = asyncio.get_running_loop()
        return await loop.run_in_executor(
            self._thread_pool,
            self._parse_judge_results,
            check_results,
        )

    def _parse_judge_results(
        self, check_results: dict[SubTaskCase, RunProgramSolution]
    ) -> dict[SubTaskCase, JudgeResult]:
        return {k: _parse_result(v) for k, v in check_results.items()}

    async def _compile_problem(self, problem: IOIProblem) -> dict[CommandType, bytes]:
        commands = problem.compile_command()
        compile_result_tasks: dict[CommandType, asyncio.Task[Solution]] = {
            k: await self._create_task(self._executor.execute(problem=cmd)) for k, cmd in commands.items()
        }
        result: dict[CommandType, bytes] = {}
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
        loop = asyncio.get_running_loop()
        await self._concurrency.acquire()
        task = loop.create_task(coro)
        task.add_done_callback(lambda t: self._concurrency.release())
        return task

    async def close(self):
        await self._proxy_client.close()
        self._thread_pool.shutdown(wait=True)


async def amain():
    parser = argparse.ArgumentParser(description="Judge IOI-style problems from a JSONL file.")
    parser.add_argument(
        "--jsonl",
        "-f",
        required=True,
        help="Path to JSONL file containing judge payloads. Use '-' to read from stdin.",
    )
    parser.add_argument(
        "--endpoint",
        "-e",
        default="http://127.0.0.1:8080/execute",
        help="Proxy execute endpoint.",
    )
    parser.add_argument(
        "--concurrency",
        "-c",
        type=int,
        default=4,
        help="Max concurrent compile/run tasks.",
    )
    parser.add_argument(
        "--threads",
        "-t",
        type=int,
        default=None,
        help="Thread pool size for CPU-bound work (default: same as --concurrency).",
    )
    parser.add_argument(
        "--solution-file",
        "-s",
        default=None,
        help="Optional path to a solution file overriding the sample solution in payloads.",
    )

    args = parser.parse_args()

    num_threads = args.threads if args.threads is not None else args.concurrency

    solution_code: str | None = None
    if args.solution_file is not None:
        with open(args.solution_file, "r", encoding="utf-8") as f:
            solution_code = f.read()

    if args.jsonl == "-":
        lines_iter = sys.stdin
        need_close = False
    else:
        lines_iter = open(args.jsonl, "r", encoding="utf-8")
        need_close = True

    judger = IOIJudger(concurrency=args.concurrency, endpoint=args.endpoint, num_threads=num_threads)

    try:
        for line_no, raw_line in enumerate(lines_iter, start=1):
            line = raw_line.strip()
            if not line:
                continue
            try:
                result = await judger.judge(line, solution=solution_code)
            except Exception as e:
                print(f"Line {line_no}: judge failed with error: {e}", file=sys.stderr)
                print(line, file=sys.stderr)
                return 1

            if abs(result.score - 100.0) > 1e-6:
                print(
                    f"Line {line_no}: expected score 100, got {result.score}. Reason: {result.reason}",
                    file=sys.stderr,
                )
                print(line, file=sys.stderr)
                return 1
    finally:
        if need_close:
            lines_iter.close()
        await judger.close()

    return 0


def main():
    sys.exit(asyncio.run(amain()))


if __name__ == "__main__":
    main()
