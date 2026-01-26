import dataclasses
import typing

@dataclasses.dataclass(frozen=True)
class OJResult:
    name: str
    error: str | None

    @classmethod
    def from_json(cls, data: dict) -> "OJResult":
        return cls(
            name=data["name"],
            error=data.get("error"),
        )


@dataclasses.dataclass(frozen=True)
class CppOJSolution:
    results: list[OJResult]
    type: typing.Literal["cpp_oj"] = "cpp_oj"

Solution = typing.Union[CppOJSolution]