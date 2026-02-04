from agent_envs.proxy_client import create_proxy_client
from agent_envs.executor import Executor
from agent_envs.problems import CompileIOIBinary, FileContent
from agent_envs.solutions import CompileIOISolution
import json
import os

async def test_compile():
    cli = create_proxy_client(url="grpc://localhost:8080")
    exec = Executor(cli)
    exec.register_queue("compile_ioi_binary", "gcc_jobs")
    with open(os.path.join(os.path.dirname(__file__), "cmd", "test_ioi_runner.json"), "r") as f:
        json_payload = json.load(f)
    
    problem_id: str = json_payload["metadata"]["id"]
    year: int = json_payload["metadata"]["year"]
    sample_solution: str = json_payload["metadata"]["sample_solution"]

    resp = await exec.execute(
        problem=CompileIOIBinary(
            solution=sample_solution,
            problem_id=problem_id,
            year=year,
        )
    )
    assert isinstance(resp, CompileIOISolution)
