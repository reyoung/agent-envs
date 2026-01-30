import pytest
from agent_envs.proxy_client import create_proxy_client, ExecRequest


async def test_proxy_client_initialization():
    cli = create_proxy_client(url="http://localhost:8080/batch_execute")
    try:
        resp = await cli.execute(
            ExecRequest(
                queue_name="gcc_jobs",
                binary=b"""#!/bin/bash
set -e
echo "OK"            
""",
            )
        )
    finally:
        await cli.close()
    assert resp.exit_code == 0
    assert resp.stdout.strip() == b"OK"
    assert resp.stderr.strip() == b""
