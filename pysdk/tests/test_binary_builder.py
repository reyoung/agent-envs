from agent_envs.binary_builder import build_binary
from agent_envs.problems import CppOJ, FileContent, OJTestCase, RunProgram
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


async def test_run_program_binary_builder_basic():
    """Test basic RunProgram binary builder with minimal parameters."""
    # Create a simple echo program binary
    simple_binary = b"#!/bin/bash\necho 'Hello from test'\n"
    
    bin_result = await build_binary(
        RunProgram(
            binary=simple_binary,
            stdin=b"test input\n",
        )
    )
    
    # Verify the binary was created
    assert len(bin_result.binary) > 0
    # Verify the capture pattern is set correctly
    assert bin_result.capture_pattern == r"^workspace/(runprog\.result|program\.stdout|program\.stderr)$"
    # Verify the args are set correctly
    assert bin_result.args == ["--target", "workspace"]


async def test_run_program_binary_builder_with_time_limit():
    """Test RunProgram binary builder with custom time_limit."""
    simple_binary = b"#!/bin/bash\necho 'Test'\n"
    
    bin_result = await build_binary(
        RunProgram(
            binary=simple_binary,
            stdin=b"",
            time_limit=2.5,
        )
    )
    
    assert len(bin_result.binary) > 0
    assert bin_result.capture_pattern == r"^workspace/(runprog\.result|program\.stdout|program\.stderr)$"
    assert bin_result.args == ["--target", "workspace"]


async def test_run_program_binary_builder_with_memory_limit():
    """Test RunProgram binary builder with custom memory_limit_in_mb."""
    simple_binary = b"#!/bin/bash\necho 'Test'\n"
    
    bin_result = await build_binary(
        RunProgram(
            binary=simple_binary,
            stdin=b"",
            memory_limit_in_mb=512,
        )
    )
    
    assert len(bin_result.binary) > 0
    assert bin_result.capture_pattern == r"^workspace/(runprog\.result|program\.stdout|program\.stderr)$"
    assert bin_result.args == ["--target", "workspace"]


async def test_run_program_binary_builder_with_args():
    """Test RunProgram binary builder with custom args."""
    simple_binary = b"#!/bin/bash\necho \"Args: $@\"\n"
    
    bin_result = await build_binary(
        RunProgram(
            binary=simple_binary,
            stdin=b"",
            args=["--flag", "value", "arg with spaces"],
        )
    )
    
    assert len(bin_result.binary) > 0
    assert bin_result.capture_pattern == r"^workspace/(runprog\.result|program\.stdout|program\.stderr)$"
    assert bin_result.args == ["--target", "workspace"]


async def test_run_program_binary_builder_with_all_parameters():
    """Test RunProgram binary builder with all parameters set."""
    simple_binary = b"#!/bin/bash\ncat\n"  # Simple cat program
    
    bin_result = await build_binary(
        RunProgram(
            binary=simple_binary,
            stdin=b"input data\n",
            time_limit=3.0,
            memory_limit_in_mb=1024,
            args=["--verbose", "test"],
        )
    )
    
    assert len(bin_result.binary) > 0
    assert bin_result.capture_pattern == r"^workspace/(runprog\.result|program\.stdout|program\.stderr)$"
    assert bin_result.args == ["--target", "workspace"]