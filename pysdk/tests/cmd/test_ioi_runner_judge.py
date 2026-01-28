from agent_envs.cmd.ioi_runner import IOIProblem, IOIJudger
import os


async def test_ioi_judger():
    with open(
        os.path.join(os.path.dirname(__file__), "test_ioi_runner.json"), "r"
    ) as f:
        json_payload = f.read()

    judger = IOIJudger(
        concurrency=100, endpoint="http://127.0.0.1:8080/execute", num_threads=1
    )
    try:
        res = await judger.judge(json_payload)
        print(res.score)
    finally:
        await judger.close()
