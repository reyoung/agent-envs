from agent_envs.result_parser import parse_result
from agent_envs.problems import RunProgram
from agent_envs.proxy_client import ExecResult, FileContent
from agent_envs.solutions import RunProgramSolution

async def test_run_program_result_parsing():
    exec_result = ExecResult(exit_code=0, 
                             stdout=b'Creating directory workspace\nVerifying archive integrity... MD5 checksums are OK. All good.\nUncompressing Run Program\n', 
                             stderr=b'     0% \x08\x08\x08\x08\x08\x08\x08   35% \x08\x08\x08\x08\x08\x08\x08   70% \x08\x08\x08\x08\x08\x08\x08 100%       0% \x08\x08\x08\x08\x08\x08\x08   35% \x08\x08\x08\x08\x08\x08\x08   70% \x08\x08\x08\x08\x08\x08\x08 100%  ', 
                             files=[FileContent(filename='workspace/program.stderr', content=b''), 
                                    FileContent(filename='workspace/program.stdout', content=b'7\n'), 
                                    FileContent(filename='workspace/runprog.result', content=b'0 1 532 0\n')])
    r = await parse_result(RunProgram(binary=b'\x7fELF...\x00', stdin=b'3 4\n'), exec_result)
    assert isinstance(r, RunProgramSolution)
    assert r.exit_code == 0
    assert r.stdout == b'7\n'
    assert r.stderr == b''
    assert r.time == 0.001
    assert r.memory == 532
    assert r.status == "SUCCESS"