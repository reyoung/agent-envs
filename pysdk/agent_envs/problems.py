from __future__ import annotations

import dataclasses
import typing


@dataclasses.dataclass(frozen=True)
class FileContent:
    filename: str
    content: bytes


@dataclasses.dataclass(frozen=True)
class OJTestCase:
    name: str
    input: bytes
    output: bytes


@dataclasses.dataclass(frozen=True)
class CppOJ:
    files: list[FileContent]
    test_cases: list[OJTestCase]
    type: typing.Literal["cpp_oj"] = "cpp_oj"


Problem = typing.Union[CppOJ]
