from agent_envs.proxy_client import create_proxy_client
from agent_envs.executor import Executor
from agent_envs.problems import RunProgram, FileContent, Bind
from agent_envs.solutions import RunProgramSolution
import textwrap

async def test_proxy_client_initialization():
    cli = create_proxy_client(url="grpc://localhost:8080")
    exec = Executor(cli)
    exec.register_queue("run_program", "gcc_jobs")
    run_program = RunProgram(
        entrypoint="main.sh", files=[
    FileContent(filename="main.py", content=b"print('Hello, World!')"),
    FileContent(filename="main.sh", content=textwrap.dedent(f"""\
        #!/bin/bash
        set -xe
        cd "$(dirname "$0")"                                                    
        . /envs/py-3.13-env/bin/activate
        python -u main.py
""").encode('utf-8')
    )],
    extra_ro_binds=[
        Bind(source="/envlet/envs/py-3.13-env", target="/envs/py-3.13-env")
    ]
    )
    res = await exec.execute(run_program)
    assert isinstance(res, RunProgramSolution)
    assert res.stdout == b"Hello, World!\n"