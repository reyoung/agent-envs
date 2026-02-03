from agent_envs.proxy_client import create_proxy_client
from agent_envs.executor import Executor
from agent_envs.problems import CompileCPP, FileContent
from agent_envs.solutions import CompileSolution


async def test_compile():
    cli = create_proxy_client(url="grpc://localhost:8080")

    exec = Executor(cli)
    exec.register_queue("compile_cpp", "gcc_jobs")
    resp = await exec.execute(
        problem=CompileCPP(
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
        )
    )
    assert isinstance(resp, CompileSolution)
    assert resp.binary is not None
    assert resp.compile_error is None
