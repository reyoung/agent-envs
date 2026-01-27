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
    status: OJResultStatus

    @classmethod
    def from_json(cls, data: dict) -> "OJResult":
        details = data.get("details", {})
        details_status = details.get("status", "Fatal Error")
        error = data.get("error")
        if error is None:
            status = OJResultStatus.AC
        elif details_status in ("Fatal Error", "Disallowed Syscall", "Invalid", "Unknown"):
            status = OJResultStatus.SE
        elif details_status == "Time Limit Exceeded":
            status = OJResultStatus.TLE
        elif details_status == "Memory Limit Exceeded":
            status = OJResultStatus.MLE
        elif details_status == "Runtime Error":
            status = OJResultStatus.RE
        elif details_status in ("Normal", "Output Limit Exceeded"):
            status = OJResultStatus.WA
        else:
            raise RuntimeError(f"Unknown status in judge result: {details_status}")
        

        return cls(
            name=data["name"],
            error=data.get("error"),
            status=status,
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
            if result.status != OJResultStatus.AC:
                return result.status
        return OJResultStatus.AC
    
@dataclasses.dataclass(frozen=True)
class CompileSolution:
    compile_error: str | None = None
    binary: bytes | None = None
    type: typing.Literal["compile_cpp"] = "compile_cpp"

@dataclasses.dataclass(frozen=True)
class RunProgramSolution:
    exit_code: int
    stdout: bytes
    stderr: bytes
    memory: int | None = None
    time: float | None = None
    status: int | None = None
    type: typing.Literal["run_program"] = "run_program"

Solution = typing.Union[CppOJSolution, CompileSolution, RunProgramSolution]