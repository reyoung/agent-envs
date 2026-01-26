import json
import httpx
import dataclasses
import base64

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
            res["args"] = self.args # type: ignore
            
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


class ProxyClient:
    def __init__(self, url: str):
        self._url = url
        self._http_client = httpx.AsyncClient(
            timeout=httpx.Timeout(60.0, read=1800.0)
        )
    

    async def execute(self, request: ExecRequest) -> ExecResult:
        resp = await self._http_client.post(url=self._url, json=request.as_json())
        try:
            resp.raise_for_status()
            req = await resp.aread()
            req_json = json.loads(req)
            if "error" in req_json:
                raise RuntimeError(f"Proxy server error: {req_json['error']}")
            return ExecResult.from_json(req_json["result"])
        finally:
            await resp.aclose()
    
    async def close(self):
        await self._http_client.aclose()