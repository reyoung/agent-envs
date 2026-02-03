from agent_envs.proxy_client import create_proxy_client
from agent_envs.executor import Executor
from agent_envs.problems import CppOJ, FileContent, OJTestCase
from agent_envs.solutions import OJResultStatus


async def test_proxy_client_initialization():
    cli = create_proxy_client(url="grpc://localhost:8080")

    exec = Executor(cli)
    exec.register_queue("cpp_oj", "gcc_jobs")
    resp = await exec.execute(
        problem=CppOJ(
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
            test_cases=[
                OJTestCase(
                    name="test1",
                    input=b"1 2\n",
                    output=b"3\n",
                ),
                OJTestCase(
                    name="test2",
                    input=b"10 20\n",
                    output=b"30\n",
                ),
            ],
        )
    )

    assert resp.status == OJResultStatus.AC, f"Expected AC but got {resp.results}"
