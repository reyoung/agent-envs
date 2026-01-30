import asyncio
import json
import httpx
import dataclasses
import base64
import typing
import aioautobatch

@dataclasses.dataclass(frozen=True)
class ExecRequest:
    queue_name: str
    binary: bytes
    capture_pattern: str | None = None
    args: list[str] | None = None

    def as_json(self) -> dict:
        res = {
            "queue_name": self.queue_name,
            "binary": base64.b64encode(self.binary).decode("utf-8"),
        }
        if self.capture_pattern is not None:
            res["capture_pattern"] = self.capture_pattern
        if self.args is not None:
            res["args"] = self.args  # type: ignore

        return res


@dataclasses.dataclass
class FileContent:
    filename: str
    content: bytes

    @classmethod
    def from_json(cls, data: dict) -> "FileContent":
        return cls(
            filename=data["filename"],
            content=base64.b64decode(data["content"]) if "content" in data else b"",
        )


@dataclasses.dataclass(frozen=True)
class ExecResult:
    exit_code: int
    stdout: bytes
    stderr: bytes
    files: list[FileContent]

    @classmethod
    def from_json(cls, data: dict) -> "ExecResult":
        if "files" not in data:
            files = []
        else:
            files = [FileContent.from_json(f) for f in data["files"]]
        return cls(
            exit_code=data["exit_code"],
            stdout=base64.b64decode(data["stdout"]) if "stdout" in data else b"",
            stderr=base64.b64decode(data["stderr"]) if "stderr" in data else b"",
            files=files,
        )

    def file_dict(self) -> dict[str, bytes]:
        return {f.filename: f.content for f in self.files}
    

class ProxyClient(typing.Protocol):
    def execute(self, request: ExecRequest) -> typing.Awaitable[ExecResult]:
        ...

    async def close(self):
        ...

def _create_http_client() -> httpx.AsyncClient:
    return httpx.AsyncClient(timeout=httpx.Timeout(60.0, read=1800.0), limits=httpx.Limits(
        max_keepalive_connections=100, max_connections=65535
    ))

class _SingleProxyClient(ProxyClient):
    def __init__(self, url: str):
        self._url = url
        self._http_client = _create_http_client()

    async def execute(self, request: ExecRequest) -> ExecResult:
        resp = await self._http_client.post(url=self._url, json=request.as_json())
        resp.raise_for_status()
        resp_json = resp.json()
        if "error" in resp_json:
            raise RuntimeError(f"Proxy server error: {resp_json['error']}")
        return ExecResult.from_json(resp_json["result"])

    async def close(self):
        await self._http_client.aclose()


_auto_batch_kwargs = {
    "start_delay": 0,
    "max_delay": 1.0,
    "batch_size": 200,
    "max_concurrent_batches": None,  # No limit on concurrent batches
}


class _BatchProxyClient(ProxyClient):
    def __init__(self, url: str) -> None:
        super().__init__()
        self._url = url
        self._http_client = _create_http_client()
        
        self._execute = aioautobatch.autobatch(
            self._batch_execute,
            **_auto_batch_kwargs
        )
    
    async def execute(self, request: ExecRequest) -> ExecResult:
        val = await self._execute(request)
        err_or_result = await val
        if isinstance(err_or_result, Exception):
            raise err_or_result
        return err_or_result
    
    async def _batch_execute(self, args_list: list[tuple[ExecRequest]]) -> list[ExecResult | Exception]:
            async def content():
                for (request,) in args_list:
                    yield json.dumps(request.as_json()).encode("utf-8") + b"\n"
                    
            res = []

            async with self._http_client.stream("POST", url=self._url, headers={"Content-Type": "application/x-ndjson"},
                                                content=content()) as response:
                response.raise_for_status()
                async for line in response.aiter_lines():
                    if not line:
                        continue
                    resp_json = json.loads(line.strip())
                    if "error" in resp_json:
                        res.append(RuntimeError(f"Proxy server error: {resp_json['error']}"))
                    res.append(ExecResult.from_json(resp_json["result"]))
            if len(res) != len(args_list):
                raise RuntimeError(f"Batch execute returned {len(res)} results, expected {len(args_list)}")
            return res

    async def close(self):
        await self._http_client.aclose()

class _UnorderedBatchProxyClient(ProxyClient):
    def __init__(self, url: str) -> None:
        super().__init__()
        self._url = url
        self._http_client = _create_http_client()
        
        self._execute = aioautobatch.autobatch(
            self._batch_execute,
            **_auto_batch_kwargs
        )
    
    async def execute(self, request: ExecRequest) -> ExecResult:
        val = await self._execute(request)
        err_or_result = await val
        if isinstance(err_or_result, Exception):
            raise err_or_result
        return err_or_result
    
    async def _batch_execute(self, args_list: list[tuple[ExecRequest]], futures: list[asyncio.Future[ExecResult]]):
        async def content():
            for req_id, (request,) in enumerate(args_list):
                req_js = request.as_json()
                req_js["id"] = str(req_id)
                str_buf = json.dumps(req_js)
                assert "\n" not in str_buf
                yield str_buf.encode("utf-8") + b"\n"
        
        async with self._http_client.stream("POST", url=self._url, headers={"Content-Type": "application/x-ndjson"},
                                            content=content()) as response:
            response.raise_for_status()
            internal_errors = []
            async for line in response.aiter_lines():
                if not line:
                    continue
                resp_json = json.loads(line.strip())
                req_id_str: str = resp_json["id"]
                if req_id_str == "":
                    internal_errors.append(resp_json.get("error", "Unknown error"))
                    internal_errors.append(f"line {line}")
                    continue
                try:
                    req_id = int(req_id_str)
                except ValueError:
                    internal_errors.append(f"Invalid request ID: {req_id_str}, line {line}")
                    continue

                fut = futures[req_id]

                if "error" in resp_json:
                    fut.set_exception(RuntimeError(f"Proxy server error: {resp_json['error']}"))
                    continue
                fut.set_result(ExecResult.from_json(resp_json["result"]))
            
            if internal_errors:
                raise RuntimeError(f"Internal errors occurred during batch execution: {internal_errors}")

    async def close(self):
        await self._http_client.aclose()


def create_proxy_client(url: str) -> ProxyClient:
    if url.endswith("unordered_batch_execute"):
        return _UnorderedBatchProxyClient(url)
    if url.endswith("batch_execute"):
        return _BatchProxyClient(url)
    if url.endswith("execute"):
        return _SingleProxyClient(url)
    else:
        raise ValueError(f"Invalid proxy URL: {url}")
