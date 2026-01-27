import dataclasses
import typing
import enum

class OJResultStatus(enum.StrEnum):
    AC = "Accepted"
    WA = "Wrong Answer"
    TLE = "Time Limit Exceeded"
    MLE = "Memory Limit Exceeded"
    RE = "Runtime Error"
    CE = "Compilation Error"
    SE = "System Error"

@dataclasses.dataclass(frozen=True)
class OJResult:
    name: str
    error: str | None
    type: OJResultStatus

    @classmethod
    def from_json(cls, data: dict) -> "OJResult":
        details = data.get("details", {})
        details_status = details.get("status", "Fatal Error")
        error = data.get("error")
        if error is None:
            type = OJResultStatus.AC
        elif details_status in ("Fatal Error", "Disallowed Syscall", "Invalid", "Unknown"):
            type = OJResultStatus.SE
        elif details_status == "Time Limit Exceeded":
            type = OJResultStatus.TLE
        elif details_status == "Memory Limit Exceeded":
            type = OJResultStatus.MLE
        elif details_status == "Runtime Error":
            type = OJResultStatus.RE
        elif details_status == "Normal":
            type = OJResultStatus.WA
        else:
            raise RuntimeError(f"Unknown status in judge result: {details_status}")
        

        return cls(
            name=data["name"],
            error=data.get("error"),
            type=type,
        )


@dataclasses.dataclass(frozen=True)
class CppOJSolution:
    results: list[OJResult]
    compile_error: str | None = None
    type: typing.Literal["cpp_oj"] = "cpp_oj"

    @property
    def status(self) -> OJResultStatus:
        if self.compile_error is not None:
            return OJResultStatus.CE
        for result in self.results:
            if result.type != OJResultStatus.AC:
                return result.type
        return OJResultStatus.AC

Solution = typing.Union[CppOJSolution]