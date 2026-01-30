from .problems import Problem
from .solutions import Solution
from .proxy_client import ProxyClient, ExecRequest
from .binary_builder import build_binary
from .result_parser import parse_result
from concurrent.futures import ThreadPoolExecutor
import asyncio

__all__ = ["Executor"]


class Executor:
    def __init__(self, proxy_client: ProxyClient,
                 build_binary_pool: ThreadPoolExecutor | None = None) -> None:
        self._queue_names: dict[str, str] = {}
        self._proxy_client = proxy_client
        self._build_binary_pool = build_binary_pool

    def register_queue(self, problem_type: str, queue_name: str):
        self._queue_names[problem_type] = queue_name

    async def _build_binary(self, problem: Problem):
        if self._build_binary_pool is not None:
            loop = asyncio.get_running_loop()
            return await loop.run_in_executor(
                self._build_binary_pool, build_binary, problem
            )
        else:
            return build_binary(problem)

    async def execute(self, problem: Problem) -> Solution:
        binary = await self._build_binary(problem)
        queue_name = self._queue_names[problem.type]
        resp = await self._proxy_client.execute(
            ExecRequest(
                queue_name=queue_name,
                binary=binary.binary,
                capture_pattern=binary.capture_pattern,
                args=binary.args,
            )
        )
        return await parse_result(problem, resp)
