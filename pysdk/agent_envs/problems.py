from __future__ import annotations

import dataclasses
import typing
import base64


@dataclasses.dataclass(frozen=True)
class FileContent:
    filename: str
    content: bytes

    @classmethod
    def from_json(cls, data: dict) -> typing.Self:
        return cls(
            filename=data["filename"],
            content=base64.b64decode(data["content"]),
        )


@dataclasses.dataclass(frozen=True)
class OJTestCase:
    name: str
    input: bytes
    output: bytes
    time_limit: float | None = None
    memory_limit_in_mb: int | None = None

    @classmethod
    def from_json(cls, data: dict) -> typing.Self:
        return cls(
            name=data["name"],
            input=base64.b64decode(data["input"]),
            output=base64.b64decode(data["output"]),
            time_limit=data.get("time_limit"),
            memory_limit_in_mb=data.get("memory_limit_in_mb"),
        )


@dataclasses.dataclass(frozen=True)
class CppOJ:
    files: list[FileContent]
    test_cases: list[OJTestCase]
    type: typing.Literal["cpp_oj"] = "cpp_oj"

    @classmethod
    def from_json(cls, data: dict) -> typing.Self:
        return cls(
            files=[FileContent.from_json(f) for f in data.get("files", [])],
            test_cases=[OJTestCase.from_json(tc) for tc in data.get("test_cases", [])],
        )


@dataclasses.dataclass(frozen=True)
class CompileCPP:
    files: list[FileContent]
    type: typing.Literal["compile_cpp"] = "compile_cpp"

    @classmethod
    def from_json(cls, data: dict) -> typing.Self:
        return cls(
            files=[FileContent.from_json(f) for f in data.get("files", [])],
        )


@dataclasses.dataclass(frozen=True)
class RunProgram:
    entrypoint: str
    files: list[FileContent]
    time_limit: float | None = None
    memory_limit_in_mb: int | None = None
    args: list[str] | None = None
    type: typing.Literal["run_program"] = "run_program"

    @classmethod
    def from_json(cls, data: dict) -> typing.Self:
        return cls(
            entrypoint=data["entrypoint"],
            files=[FileContent.from_json(f) for f in data.get("files", [])],
            time_limit=data.get("time_limit"),
            memory_limit_in_mb=data.get("memory_limit_in_mb"),
            args=data.get("args"),
        )


Problem = typing.Union[CppOJ, CompileCPP, RunProgram]


def problem_from_json(data: dict) -> Problem:
    problem_type = data.get("type")
    if problem_type == "cpp_oj":
        return CppOJ.from_json(data)
    elif problem_type == "compile_cpp":
        return CompileCPP.from_json(data)
    elif problem_type == "run_program":
        return RunProgram.from_json(data)
    else:
        raise NotImplementedError(f"Unsupported problem type: {problem_type}")
