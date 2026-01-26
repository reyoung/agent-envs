from agent_envs.binary_builder import build_binary
from agent_envs.problems import CppOJ, FileContent, OJTestCase
async def test_cpp_binary_builder():

    bin = await build_binary(
        CppOJ(
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
    assert len(bin.binary) > 0