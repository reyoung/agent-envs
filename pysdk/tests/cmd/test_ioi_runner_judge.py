from agent_envs.cmd.ioi_runner import IOIJudger, UnknownResult
import os


async def _do_test(js_name: str):
    with open(os.path.join(os.path.dirname(__file__), js_name), "r") as f:
        json_payload = f.read()

    judger = IOIJudger(concurrency=1000, endpoint="http://127.0.0.1:8080/batch_execute", num_threads=1)
    try:
        res = await judger.judge(json_payload)
        assert isinstance(res.reason,dict)
        for k, v in res.reason.items():
            if v.get_score() == 0.0:
                print(k, v)
            # if not isinstance(v, UnknownResult):
            #     continue
            # print(v.solution.stdout)
            # print(v.solution.stderr)
        assert abs(res.score - 100) < 1e-6, f"Expected score 100 but got {res.score}"
    finally:
        await judger.close()

async def test_ioi_judger():
    await _do_test("test_ioi_runner.json")

async def test_ioi_judger_2():
    await _do_test("test_ioi_runner_2.json")

async def test_ioi_judger_3():
    await _do_test("test_ioi_runner_3.json")

async def test_ioi_judger_4():
    await _do_test("test_ioi_runner_4.json")
