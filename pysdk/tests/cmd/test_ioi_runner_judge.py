from agent_envs.cmd.ioi_runner import IOIJudger
import os


async def test_ioi_judger():
    with open(os.path.join(os.path.dirname(__file__), "test_ioi_runner.json"), "r") as f:
        json_payload = f.read()

    judger = IOIJudger(concurrency=1000, endpoint="http://127.0.0.1:8080/execute", num_threads=1)
    try:
        res = await judger.judge(json_payload)
        assert abs(res.score - 100) < 1e-6, f"Expected score 100 but got {res.score}"
    finally:
        await judger.close()

async def test_ioi_judger_2():
    with open(os.path.join(os.path.dirname(__file__), "test_ioi_runner_2.json"), "r") as f:
        json_payload = f.read()

    judger = IOIJudger(concurrency=1000, endpoint="http://127.0.0.1:8080/execute", num_threads=1)
    try:
        res = await judger.judge(json_payload)
        assert isinstance(res.reason, dict)
        assert abs(res.score - 100) < 1e-6, f"Expected score 100 but got {res.score}"
    finally:
        await judger.close()