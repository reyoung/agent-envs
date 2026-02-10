import sys
import json
import dataclasses

@dataclasses.dataclass(frozen=True)
class ProblemID:
    contest_id: int
    problem_index: str


def main():
    visited = set[ProblemID]()

    for line in sys.stdin:
        item = json.loads(line)
        total_cases = len(item['test_cases'])
        passed_count = item['passedTestCount']
        if total_cases != passed_count:
            continue

        problem_id = ProblemID(
            contest_id=item['contestId'],
            problem_index=item['index']
        )

        if problem_id in visited:
            continue
        visited.add(problem_id)

        json.dump({
            "code": item['code'],
            'contest_id': item['contestId'],
            'index': item['index'],
            'test_cases': item['test_cases'],
            'time_limit': item['time-limit'],
            'memory_limit': item['memory-limit'],
        }, sys.stdout)
        sys.stdout.write("\n")

if __name__ == "__main__":
    main()