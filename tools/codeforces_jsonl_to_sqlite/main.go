package main

import (
	"bufio"
	"bytes"
	"database/sql"
	"encoding/json"
	"flag"
	"fmt"
	"hash/fnv"
	"io"
	"log"
	"os"

	_ "github.com/mattn/go-sqlite3"
)

var (
	flagInputs      arrayFlags
	flagSqlitePath  = flag.String("sqlite-output", "codeforces.db", "Path to the output SQLite database file")
	flagBatchSize   = flag.Int("batch-size", 1000, "Number of records to insert per transaction")
	flagDropAndInit = flag.Bool("drop", false, "Drop existing table before import")
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
	ContestID   int        `json:"contestId"`
	Index       string     `json:"index"`
	TimeLimit   TextValue  `json:"time-limit"`
	MemoryLimit TextValue  `json:"memory-limit"`
	ProblemDesc TextValue  `json:"problem-description"`
	InputSpec   TextValue  `json:"input-specification"`
	Title       TextValue  `json:"title"`
	DemoInput   TextValue  `json:"demo-input"`
	DemoOutput  TextValue  `json:"demo-output"`
	TestCases   []testCase `json:"test_cases"`
}

type testCase struct {
	Input  string `json:"input"`
	Output string `json:"output"`
}

type TextValue string

func (t *TextValue) UnmarshalJSON(data []byte) error {
	data = bytes.TrimSpace(data)
	if len(data) == 0 || bytes.Equal(data, []byte("null")) {
		*t = ""
		return nil
	}
	var s string
	if err := json.Unmarshal(data, &s); err == nil {
		*t = TextValue(s)
		return nil
	}
	*t = TextValue(string(data))
	return nil
}

func main() {
	flag.Var(&flagInputs, "input", "Path to codeforces JSONL file (can be provided multiple times)")
	flag.Parse()

	if len(flagInputs) == 0 {
		log.Fatal("no input files provided; use -input datasets/codeforces.train.jsonl (and/or test)")
	}

	db, err := sql.Open("sqlite3", *flagSqlitePath)
	if err != nil {
		log.Fatalf("open sqlite: %v", err)
	}
	defer db.Close()

	if err := initDB(db, *flagDropAndInit); err != nil {
		log.Fatalf("init db: %v", err)
	}

	writer, err := newBatchWriter(db, *flagBatchSize)
	if err != nil {
		log.Fatalf("new batch writer: %v", err)
	}

	total := 0
	for _, path := range flagInputs {
		n, err := importFile(path, writer)
		if err != nil {
			log.Fatalf("import %s: %v", path, err)
		}
		total += n
	}

	if err := writer.Flush(); err != nil {
		log.Fatalf("flush: %v", err)
	}

	log.Printf("imported %d records into %s", total, *flagSqlitePath)
}

func initDB(db *sql.DB, drop bool) error {
	const problemsTable = "codeforces_problems"
	const testCasesTable = "codeforces_test_cases"
	if drop {
		if _, err := db.Exec(`DROP TABLE IF EXISTS "codeforces_problems";`); err != nil {
			return err
		}
		if _, err := db.Exec(`DROP TABLE IF EXISTS "codeforces_test_cases";`); err != nil {
			return err
		}
	}
	_, err := db.Exec(fmt.Sprintf(`
	CREATE TABLE IF NOT EXISTS "%s" (
		id BIGINT PRIMARY KEY,
		contest_id INTEGER NOT NULL,
		problem_index TEXT NOT NULL,
		time_limit TEXT,
		memory_limit TEXT,
		problem_description TEXT,
		input_specification TEXT,
		title TEXT,
		demo_input TEXT,
		demo_output TEXT
	);
	CREATE UNIQUE INDEX IF NOT EXISTS codeforces_problem_contest_idx
		ON "%s" (contest_id, problem_index);

	CREATE TABLE IF NOT EXISTS "%s" (
		problem_id BIGINT NOT NULL,
		case_offset INTEGER NOT NULL,
		input TEXT,
		output TEXT,
		PRIMARY KEY (problem_id, case_offset),
		FOREIGN KEY (problem_id) REFERENCES "%s"(id) ON DELETE CASCADE
	);
	`, problemsTable, problemsTable, testCasesTable, problemsTable))
	return err
}

type batchWriter struct {
	db        *sql.DB
	batchSize int

	tx              *sql.Tx
	stmtUpsert      *sql.Stmt
	stmtDeleteCases *sql.Stmt
	stmtInsertCase  *sql.Stmt
	cnt             int
}

func newBatchWriter(db *sql.DB, batchSize int) (*batchWriter, error) {
	if batchSize <= 0 {
		batchSize = 1000
	}
	bw := &batchWriter{
		db:        db,
		batchSize: batchSize,
	}
	if err := bw.reset(); err != nil {
		return nil, err
	}
	return bw, nil
}

func (b *batchWriter) reset() error {
	const problemsTable = "codeforces_problems"
	const testCasesTable = "codeforces_test_cases"
	if b.tx != nil {
		_ = b.tx.Rollback()
	}
	tx, err := b.db.Begin()
	if err != nil {
		return err
	}
	stmtUpsert, err := tx.Prepare(fmt.Sprintf(`
	INSERT INTO "%s"
	(id, contest_id, problem_index, time_limit, memory_limit, problem_description, input_specification, title, demo_input, demo_output)
	VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	ON CONFLICT (contest_id, problem_index) DO UPDATE SET
		time_limit = excluded.time_limit,
		memory_limit = excluded.memory_limit,
		problem_description = excluded.problem_description,
		input_specification = excluded.input_specification,
		title = excluded.title,
		demo_input = excluded.demo_input,
		demo_output = excluded.demo_output
	`, problemsTable))
	if err != nil {
		_ = tx.Rollback()
		return err
	}
	stmtDeleteCases, err := tx.Prepare(fmt.Sprintf(`
	DELETE FROM "%s" WHERE problem_id = ?
	`, testCasesTable))
	if err != nil {
		_ = stmtUpsert.Close()
		_ = tx.Rollback()
		return err
	}
	stmtInsertCase, err := tx.Prepare(fmt.Sprintf(`
	INSERT OR REPLACE INTO "%s"
	(problem_id, case_offset, input, output)
	VALUES (?, ?, ?, ?)
	`, testCasesTable))
	if err != nil {
		_ = stmtDeleteCases.Close()
		_ = stmtUpsert.Close()
		_ = tx.Rollback()
		return err
	}
	b.tx = tx
	b.stmtUpsert = stmtUpsert
	b.stmtDeleteCases = stmtDeleteCases
	b.stmtInsertCase = stmtInsertCase
	b.cnt = 0
	return nil
}

func (b *batchWriter) Insert(r record) error {
	if b.tx == nil || b.stmtUpsert == nil || b.stmtInsertCase == nil {
		if err := b.reset(); err != nil {
			return err
		}
	}
	problemID := problemIDFor(r.ContestID, r.Index)
	if _, err := b.stmtUpsert.Exec(
		problemID,
		r.ContestID,
		r.Index,
		nullIfEmpty(string(r.TimeLimit)),
		nullIfEmpty(string(r.MemoryLimit)),
		nullIfEmpty(string(r.ProblemDesc)),
		nullIfEmpty(string(r.InputSpec)),
		nullIfEmpty(string(r.Title)),
		nullIfEmpty(string(r.DemoInput)),
		nullIfEmpty(string(r.DemoOutput)),
	); err != nil {
		return err
	}
	if _, err := b.stmtDeleteCases.Exec(problemID); err != nil {
		return err
	}
	for i, tc := range r.TestCases {
		if _, err := b.stmtInsertCase.Exec(
			problemID,
			i,
			nullIfEmpty(tc.Input),
			nullIfEmpty(tc.Output),
		); err != nil {
			return err
		}
	}
	b.cnt++
	if b.cnt >= b.batchSize {
		return b.Flush()
	}
	return nil
}

func (b *batchWriter) Flush() error {
	if b.tx == nil {
		return nil
	}
	if err := b.stmtInsertCase.Close(); err != nil {
		_ = b.tx.Rollback()
		return err
	}
	if err := b.stmtDeleteCases.Close(); err != nil {
		_ = b.tx.Rollback()
		return err
	}
	if err := b.stmtUpsert.Close(); err != nil {
		_ = b.tx.Rollback()
		return err
	}
	if err := b.tx.Commit(); err != nil {
		_ = b.tx.Rollback()
		return err
	}
	b.tx = nil
	b.stmtUpsert = nil
	b.stmtDeleteCases = nil
	b.stmtInsertCase = nil
	b.cnt = 0
	return b.reset()
}

func importFile(path string, writer *batchWriter) (int, error) {
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
				continue
			}
			if err := writer.Insert(r); err != nil {
				return count, fmt.Errorf("line %d insert: %w", lineNum, err)
			}
			count++
		}
		if err == io.EOF {
			break
		}
	}
	return count, nil
}

func nullIfEmpty(v string) interface{} {
	if v == "" {
		return nil
	}
	return v
}

func problemIDFor(contestID int, index string) int64 {
	h := fnv.New64a()
	_, _ = h.Write([]byte(fmt.Sprintf("%d:%s", contestID, index)))
	return int64(h.Sum64())
}
