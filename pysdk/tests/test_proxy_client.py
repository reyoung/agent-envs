import pytest
from agent_envs.proxy_client import ProxyClient, ExecRequest, ExecResult
async def test_proxy_client_initialization():
    cli = ProxyClient(url="http://localhost:8080/execute")
    try:
        resp = await cli.execute(ExecRequest(
            queue_name="gcc_jobs",
            binary=b"""#!/bin/bash
set -e
echo "OK"            
"""
        ))
    finally:
        await cli.close()
    print(resp)
    assert resp.exit_code == 0
    assert resp.stdout.strip() == b"OK"
    assert resp.stderr.strip() == b""
    