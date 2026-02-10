package main

import (
	"embed"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"

	"codeforces_bin/internal/cfbin"
)

//go:embed data/codeforces_problems.bin
var embeddedData embed.FS

func main() {
	contestID := flag.Uint("contest-id", 0, "Contest ID")
	index := flag.String("index", "", "Problem index (e.g. A, B1)")
	caseNumber := flag.Uint("case", 0, "0-based test case number")
	outDir := flag.String("out-dir", ".", "Output directory for input.txt/output.txt")
	inputPath := flag.String("input-path", "", "Optional override for input.txt path")
	outputPath := flag.String("output-path", "", "Optional override for output.txt path")
	listOnly := flag.Bool("list", false, "List all problems (contest_id index) and exit")
	flag.Parse()

	if *listOnly {
		if err := listProblems(); err != nil {
			log.Fatalf("list problems: %v", err)
		}
		return
	}

	if *contestID == 0 || *index == "" {
		log.Fatal("contest-id and index are required")
	}

	data, err := embeddedData.ReadFile("data/codeforces_problems.bin")
	if err != nil {
		log.Fatalf("read embedded data: %v", err)
	}

	reader, err := cfbin.NewReader(data)
	if err != nil {
		log.Fatalf("load data: %v", err)
	}

	input, output, total, err := reader.Get(uint32(*contestID), *index, uint32(*caseNumber))
	if err != nil {
		log.Fatalf("lookup failed: %v (total cases: %d)", err, total)
	}

	inp := *inputPath
	outp := *outputPath
	if inp == "" {
		inp = filepath.Join(*outDir, "input.txt")
	}
	if outp == "" {
		outp = filepath.Join(*outDir, "output.txt")
	}

	if err := writeFile(inp, input); err != nil {
		log.Fatalf("write input: %v", err)
	}
	if err := writeFile(outp, output); err != nil {
		log.Fatalf("write output: %v", err)
	}

	if total == 0 {
		fmt.Printf("wrote %s and %s (case %d, no total cases)\n", inp, outp, *caseNumber)
		return
	}
	fmt.Printf("wrote %s and %s (case %d/%d)\n", inp, outp, *caseNumber, total-1)
}

func listProblems() error {
	data, err := embeddedData.ReadFile("data/codeforces_problems.bin")
	if err != nil {
		return err
	}
	reader, err := cfbin.NewReader(data)
	if err != nil {
		return err
	}
	entries := reader.ListProblems()
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].ContestID == entries[j].ContestID {
			return entries[i].Index < entries[j].Index
		}
		return entries[i].ContestID < entries[j].ContestID
	})
	for _, entry := range entries {
		fmt.Printf("%d %s\n", entry.ContestID, entry.Index)
	}
	return nil
}

func writeFile(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}
