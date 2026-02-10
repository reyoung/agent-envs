# Codeforces Binary Tool

This tool converts `datasets/codeforces_problems.jsonl` into a compact binary file and builds a lookup binary that can emit `input.txt`/`output.txt` by `(contest_id, index, test_case)`.

## Build

```bash
cd tools/codeforces_bin
make build
```

This generates:
- `tools/codeforces_bin/cmd/cf_case/data/codeforces_problems.bin` (binary data, gitignored)
- `tools/codeforces_bin/bin/cf_case` (lookup binary, gitignored)

## Usage

```bash
./bin/cf_case -contest-id 1234 -index A -case 1 -out-dir .
```

By default the test case number is 0-based.

Optional flags:
- `-input-path /path/to/input.txt`
- `-output-path /path/to/output.txt`

## Docker

```bash
tools/codeforces_bin/build_image_and_push
```
