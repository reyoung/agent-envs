package main

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"

	"codeforces_bin/internal/cfbin"
)

type arrayFlags []string

func (a *arrayFlags) String() string {
	return fmt.Sprint([]string(*a))
}

func (a *arrayFlags) Set(value string) error {
	*a = append(*a, value)
	return nil
}

type record struct {
	ContestID int        `json:"contest_id"`
	Index     string     `json:"index"`
	TestCases []testCase `json:"test_cases"`
}

type testCase struct {
	Input  string `json:"input"`
	Output string `json:"output"`
}

var (
	flagInputs arrayFlags
	flagOutput = flag.String("output", "cmd/cf_case/data/codeforces_problems.bin", "Path to output binary")
)

func main() {
	flag.Var(&flagInputs, "input", "Path to codeforces JSONL file (can be provided multiple times)")
	flag.Parse()

	if len(flagInputs) == 0 {
		log.Fatal("no input files provided; use -input datasets/codeforces_problems.jsonl")
	}

	if err := buildBinary(flagInputs, *flagOutput); err != nil {
		log.Fatalf("build: %v", err)
	}
}

func buildBinary(inputs []string, output string) error {
	if err := os.MkdirAll(filepath.Dir(output), 0o755); err != nil {
		return err
	}
	out, err := os.Create(output)
	if err != nil {
		return err
	}
	defer out.Close()

	if _, err := out.Write(make([]byte, cfbin.HeaderSize)); err != nil {
		return err
	}

	dataWriter := bufio.NewWriter(out)
	dataPos := uint64(0)

	indexTemp, err := os.CreateTemp("", "cf_index_*.bin")
	if err != nil {
		return err
	}
	defer func() {
		_ = indexTemp.Close()
		_ = os.Remove(indexTemp.Name())
	}()
	indexWriter := bufio.NewWriter(indexTemp)

	var totalProblems int
	for _, path := range inputs {
		n, err := importFile(path, dataWriter, &dataPos, indexWriter)
		if err != nil {
			return fmt.Errorf("import %s: %w", path, err)
		}
		totalProblems += n
	}

	if err := dataWriter.Flush(); err != nil {
		return err
	}
	if err := indexWriter.Flush(); err != nil {
		return err
	}

	if _, err := out.Seek(0, io.SeekEnd); err != nil {
		return err
	}
	indexSize, err := indexTemp.Seek(0, io.SeekEnd)
	if err != nil {
		return err
	}
	if _, err := indexTemp.Seek(0, io.SeekStart); err != nil {
		return err
	}
	if _, err := io.Copy(out, indexTemp); err != nil {
		return err
	}

	header := cfbin.NewHeader(dataPos, uint64(indexSize))
	if _, err := out.Seek(0, io.SeekStart); err != nil {
		return err
	}
	if err := cfbin.WriteHeader(out, header); err != nil {
		return err
	}

	log.Printf("wrote %d problems to %s", totalProblems, output)
	return nil
}

func importFile(path string, dataWriter io.Writer, dataPos *uint64, indexWriter io.Writer) (int, error) {
	f, err := os.Open(path)
	if err != nil {
		return 0, err
	}
	defer f.Close()

	reader := bufio.NewReader(f)
	count := 0
	lineNum := 0
	for {
		line, err := reader.ReadBytes('\n')
		if err != nil && err != io.EOF {
			return count, err
		}
		if len(bytes.TrimSpace(line)) > 0 {
			lineNum++
			var r record
			if err := json.Unmarshal(line, &r); err != nil {
				return count, fmt.Errorf("line %d: %w", lineNum, err)
			}
			if r.ContestID == 0 || r.Index == "" {
				goto nextLine
			}
			if err := writeProblem(r, dataWriter, dataPos, indexWriter); err != nil {
				return count, fmt.Errorf("line %d write: %w", lineNum, err)
			}
			count++
		}
	nextLine:
		if err == io.EOF {
			break
		}
	}
	return count, nil
}

func writeProblem(r record, dataWriter io.Writer, dataPos *uint64, indexWriter io.Writer) error {
	contestID := uint32(r.ContestID)
	indexBytes := []byte(r.Index)
	if len(indexBytes) > 255 {
		return fmt.Errorf("index too long: %s", r.Index)
	}
	caseCount := uint32(len(r.TestCases))

	if err := binary.Write(indexWriter, binary.LittleEndian, contestID); err != nil {
		return err
	}
	if err := binary.Write(indexWriter, binary.LittleEndian, uint8(len(indexBytes))); err != nil {
		return err
	}
	if _, err := indexWriter.Write(indexBytes); err != nil {
		return err
	}
	if err := binary.Write(indexWriter, binary.LittleEndian, caseCount); err != nil {
		return err
	}

	for _, tc := range r.TestCases {
		inputBytes := []byte(tc.Input)
		inputOffset := *dataPos
		if _, err := dataWriter.Write(inputBytes); err != nil {
			return err
		}
		*dataPos += uint64(len(inputBytes))

		outputBytes := []byte(tc.Output)
		outputOffset := *dataPos
		if _, err := dataWriter.Write(outputBytes); err != nil {
			return err
		}
		*dataPos += uint64(len(outputBytes))

		if err := binary.Write(indexWriter, binary.LittleEndian, inputOffset); err != nil {
			return err
		}
		if err := binary.Write(indexWriter, binary.LittleEndian, uint32(len(inputBytes))); err != nil {
			return err
		}
		if err := binary.Write(indexWriter, binary.LittleEndian, outputOffset); err != nil {
			return err
		}
		if err := binary.Write(indexWriter, binary.LittleEndian, uint32(len(outputBytes))); err != nil {
			return err
		}
	}
	return nil
}
