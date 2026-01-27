from agent_envs.proxy_client import ProxyClient
from agent_envs.executor import Executor
from agent_envs.problems import CompileCPP, FileContent, RunProgram
from agent_envs.solutions import CompileSolution

async def test_compile():
    cli = ProxyClient(url="http://localhost:8080/execute")

    exec = Executor(cli)
    exec.register_queue("compile_cpp", "gcc_jobs")
    exec.register_queue("run_program", "gcc_jobs")
    resp = await exec.execute(problem=CompileCPP(
            files=[
                FileContent(
                    filename="main.cpp",
                    content=b"""#include <iostream>
int main() {
    int a, b;
    std::cin >> a >> b;
    std::cout << (a + b) << std::endl;
    return 0;
}
""",
                )
            ],
        ))
    assert isinstance(resp, CompileSolution)
    assert resp.binary is not None
    assert resp.compile_error is None

    resp = await exec.execute(problem=RunProgram(
            binary=resp.binary,
            stdin=b"3 4\n",
        ))
    print(resp)