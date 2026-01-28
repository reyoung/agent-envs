from agent_envs.cmd.ioi_runner import IOIProblem
import os


def test_ioi_parser():
    with open(
        os.path.join(os.path.dirname(__file__), "test_ioi_runner.json"), "r"
    ) as f:
        json_payload = f.read()

    problem = IOIProblem(json_payload)
    cmd = problem.compile_command()
    assert len(cmd) > 0

    cases = list(problem.test_cases(b""))
    assert len(cases) > 0
