import subprocess
from agent_envs.binary_builder import build_binary, _self_extracting_script, _tar_gz_bytes
from agent_envs.problems import CppOJ, CompileCPP, FileContent, OJTestCase


def test_cpp_binary_builder():
    bin = build_binary(
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


def test_compile_cpp_binary_builder():
    bin = build_binary(
        CompileCPP(
            files=[
                FileContent(
                    filename="solution.cpp",
                    content=b"""#include <iostream>
int main() {
    std::cout << "Hello, World!" << std::endl;
    return 0;
}
""",
                )
            ],
        )
    )
    assert len(bin.binary) > 0
    assert bin.capture_pattern == r"^workspace/(compile_errors\.txt|solution)$"
    assert bin.args == ["--target", "workspace"]


def test_self_extracting_script_runs_and_extracts(tmp_path):
    files = [
        ("main.sh", b"#!/bin/bash\nset -e\necho ok > output.txt\n", 0o755),
        ("data.txt", b"payload-data", 0o644),
    ]
    payload = _tar_gz_bytes(files)
    script = _self_extracting_script(payload, "./main.sh")

    script_path = tmp_path / "bundle.sh"
    script_path.write_bytes(script)
    script_path.chmod(0o755)

    target_dir = tmp_path / "out"
    target_dir.mkdir()

    subprocess.run([script_path.as_posix(), "--target", target_dir.as_posix()], check=True)

    assert (target_dir / "data.txt").read_bytes() == b"payload-data"
    assert (target_dir / "output.txt").read_text().strip() == "ok"
