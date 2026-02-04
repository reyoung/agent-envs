package main

import (
	"bufio"
	"bytes"
	"database/sql"
	"encoding/json"
	"flag"
	"io"
	"log"
	"math"
	"os"
	"reflect"
	"strconv"
	"strings"

	_ "github.com/mattn/go-sqlite3"
)

var (
	flagFilename     = flag.String("filename", "data.json", "Path to the IOI JSON file")
	flagSqliteOutput = flag.String("sqlite-output", "ioi_data.db", "Path to the output SQLite database file")
	flagBatchSize    = flag.Int("batch-size", 500, "Number of records to insert per transaction")
)

type Limit struct {
	TimeLimit   float32 `json:"time_limit"`
	MemoryLimit float32 `json:"memory_limit"`
}

type SubTask struct {
	Limit         Limit    `json:"limit"`
	Name          string   `json:"name"`
	Score         IntValue `json:"score"`
	TestCaseNames []string `json:"test_case_names"`
}

type Metadata struct {
	ID             string              `json:"id"`
	Year           IntValue            `json:"year"`
	SampleSolution string              `json:"sample_solution"`
	SubTasks       []SubTask           `json:"sub_tasks,omitempty"`
	GraderFiles    map[string]string   `json:"grader_files,omitempty"`
	TestCases      map[string]TestCase `json:"test_cases,omitempty"`
}

type TestCase struct {
	Input  string `json:"input"`
	Output string `json:"output"`
}

type IOIRecord struct {
	Metadata       Metadata            `json:"metadata"`
	ID             string              `json:"id"`
	Year           IntValue            `json:"year"`
	SampleSolution string              `json:"sample_solution"`
	SubTasks       []SubTask           `json:"sub_tasks"`
	GraderFiles    map[string]string   `json:"grader_files"`
	TestCases      map[string]TestCase `json:"test_cases"`
}

type IntValue int

func (v *IntValue) UnmarshalJSON(data []byte) error {
	data = bytes.TrimSpace(data)
	if len(data) == 0 || bytes.Equal(data, []byte("null")) {
		*v = 0
		return nil
	}

	var number float64
	if err := json.Unmarshal(data, &number); err == nil {
		rounded := math.Round(number)
		if math.Abs(number-rounded) > 1e-9 {
			return &json.UnmarshalTypeError{Value: string(data), Type: reflect.TypeOf(IntValue(0))}
		}
		*v = IntValue(int64(rounded))
		return nil
	}

	var text string
	if err := json.Unmarshal(data, &text); err != nil {
		return err
	}
	if text == "" {
		*v = 0
		return nil
	}
	parsed, err := strconv.ParseFloat(text, 64)
	if err != nil {
		return err
	}
	rounded := math.Round(parsed)
	if math.Abs(parsed-rounded) > 1e-9 {
		return &json.UnmarshalTypeError{Value: text, Type: reflect.TypeOf(IntValue(0))}
	}
	*v = IntValue(int64(rounded))
	return nil
}

type batchWriter struct {
	tx                   *sql.Tx
	stmtProblem          *sql.Stmt
	stmtTestCase         *sql.Stmt
	stmtSubTask          *sql.Stmt
	stmtSubTaskTestCases *sql.Stmt
	count                int
}

func panicIf(err error) {
	if err != nil {
		log.Panicf("error: %v", err)
	}
}

func initDB(db *sql.DB) {
	_, err := db.Exec(`
	CREATE TABLE IF NOT EXISTS problems (
		id BIGINT PRIMARY KEY, 
		year INT NOT NULL, 
		name VARCHAR(128) NOT NULL
	);
	CREATE UNIQUE INDEX IF NOT EXISTS unique_problem_year_name ON problems (year, name);

	CREATE TABLE IF NOT EXISTS test_cases (
		id BIGINT PRIMARY KEY,
		problem_id BIGINT NOT NULL,
		name VARCHAR(128) NOT NULL,
		input TEXT NOT NULL,
		output TEXT NOT NULL,
		FOREIGN KEY (problem_id) REFERENCES problems(id) ON DELETE CASCADE
	);
	CREATE UNIQUE INDEX IF NOT EXISTS unique_test_case_problem_name ON test_cases (problem_id, name);

	CREATE TABLE IF NOT EXISTS sub_tasks (
		id BIGINT PRIMARY KEY,
		problem_id BIGINT NOT NULL,
		name VARCHAR(128) NOT NULL,
		score INT NOT NULL,
		time_limit float32 NOT NULL,
		memory_limit float32 NOT NULL,
		FOREIGN KEY (problem_id) REFERENCES problems(id) ON DELETE CASCADE
	);
	CREATE UNIQUE INDEX IF NOT EXISTS unique_sub_task_problem_name ON sub_tasks (problem_id, name);

	CREATE TABLE IF NOT EXISTS sub_task_test_cases (
	    id BIGINT PRIMARY KEY,
		sub_task_id BIGINT NOT NULL,
		test_case_id BIGINT NOT NULL,
		FOREIGN KEY (sub_task_id) REFERENCES sub_tasks(id) ON DELETE CASCADE,
		FOREIGN KEY (test_case_id) REFERENCES test_cases(id) ON DELETE CASCADE
	);
	`)
	panicIf(err)

}

func newBatchWriter(db *sql.DB) (*batchWriter, error) {
	tx, err := db.Begin()
	if err != nil {
		return nil, err
	}

	stmtProblem, err := tx.Prepare(`INSERT INTO problems (id, year, name) VALUES (?, ?, ?)`)
	if err != nil {
		_ = tx.Rollback()
		return nil, err
	}

	stmtTestCase, err := tx.Prepare(`INSERT INTO test_cases (id, problem_id, name, input, output) VALUES (?, ?, ?, ?, ?)`)
	if err != nil {
		_ = stmtProblem.Close()
		_ = tx.Rollback()
		return nil, err
	}

	stmtSubTask, err := tx.Prepare(`INSERT INTO sub_tasks (id, problem_id, name, score, time_limit, memory_limit) VALUES (?, ?, ?, ?, ?, ?)`)
	if err != nil {
		_ = stmtTestCase.Close()
		_ = stmtProblem.Close()
		_ = tx.Rollback()
		return nil, err
	}

	stmtSubTaskTestCases, err := tx.Prepare(`INSERT INTO sub_task_test_cases (sub_task_id, test_case_id) VALUES (?, ?)`)
	if err != nil {
		_ = stmtSubTask.Close()
		_ = stmtTestCase.Close()
		_ = stmtProblem.Close()
		_ = tx.Rollback()
		return nil, err
	}

	return &batchWriter{
		tx:                   tx,
		stmtProblem:          stmtProblem,
		stmtTestCase:         stmtTestCase,
		stmtSubTask:          stmtSubTask,
		stmtSubTaskTestCases: stmtSubTaskTestCases,
	}, nil
}

func (bw *batchWriter) commit() error {
	if err := bw.closeStatements(); err != nil {
		_ = bw.tx.Rollback()
		return err
	}
	return bw.tx.Commit()
}

func (bw *batchWriter) rollback() error {
	_ = bw.closeStatements()
	return bw.tx.Rollback()
}

func (bw *batchWriter) closeStatements() error {
	var closeErr error
	if err := bw.stmtSubTaskTestCases.Close(); err != nil && closeErr == nil {
		closeErr = err
	}
	if err := bw.stmtSubTask.Close(); err != nil && closeErr == nil {
		closeErr = err
	}
	if err := bw.stmtTestCase.Close(); err != nil && closeErr == nil {
		closeErr = err
	}
	if err := bw.stmtProblem.Close(); err != nil && closeErr == nil {
		closeErr = err
	}
	return closeErr
}

func normalizeRecord(record IOIRecord) (Metadata, []SubTask, map[string]TestCase) {
	meta := record.Metadata
	if meta.ID == "" && record.ID != "" {
		meta.ID = record.ID
		meta.Year = record.Year
		meta.SampleSolution = record.SampleSolution
	}

	subTasks := record.SubTasks
	if len(subTasks) == 0 && len(meta.SubTasks) > 0 {
		subTasks = meta.SubTasks
	}

	testCases := record.TestCases
	if len(testCases) == 0 && len(meta.TestCases) > 0 {
		testCases = meta.TestCases
	}

	return meta, subTasks, testCases
}

func main() {
	flag.Parse()
	db, err := sql.Open("sqlite3", *flagSqliteOutput)
	panicIf(err)
	initDB(db)

	_, err = db.Exec(`PRAGMA foreign_keys = ON;`)
	panicIf(err)

	inputFile, err := os.Open(*flagFilename)
	panicIf(err)
	defer inputFile.Close()

	if *flagBatchSize <= 0 {
		log.Panicf("error: batch-size must be positive")
	}

	batch, err := newBatchWriter(db)
	panicIf(err)
	defer func() {
		_ = batch.rollback()
	}()

	reader := bufio.NewReader(inputFile)
	lineNumber := 0
	var problemID int64 = 1
	var testCaseID int64 = 1
	var subTaskID int64 = 1

	for {
		line, readErr := reader.ReadBytes('\n')
		if len(line) == 0 && readErr == io.EOF {
			break
		}

		lineNumber++
		line = bytes.TrimSpace(line)
		if len(line) == 0 {
			if readErr == io.EOF {
				break
			}
			continue
		}

		var record IOIRecord
		if err := json.Unmarshal(line, &record); err != nil {
			log.Panicf("error: line %d: %v", lineNumber, err)
		}

		meta, subTasks, testCases := normalizeRecord(record)
		if meta.ID == "" || meta.Year == 0 {
			log.Panicf("error: line %d: missing metadata id or year", lineNumber)
		}

		currentProblemID := problemID
		_, err := batch.stmtProblem.Exec(currentProblemID, meta.Year, meta.ID)
		panicIf(err)
		problemID++

		testCaseIDs := make(map[string]int64, len(testCases))
		for name, testCase := range testCases {
			currentTestCaseID := testCaseID
			_, err := batch.stmtTestCase.Exec(currentTestCaseID, currentProblemID, name, testCase.Input, testCase.Output)
			panicIf(err)
			testCaseIDs[name] = currentTestCaseID
			testCaseID++
		}

		for _, subTask := range subTasks {
			currentSubTaskID := subTaskID
			_, err := batch.stmtSubTask.Exec(
				currentSubTaskID,
				currentProblemID,
				subTask.Name,
				subTask.Score,
				float64(subTask.Limit.TimeLimit),
				float64(subTask.Limit.MemoryLimit),
			)
			panicIf(err)

			for _, testCaseName := range subTask.TestCaseNames {
				testID, ok := testCaseIDs[testCaseName]
				if !ok {
					log.Panicf("error: line %d: subtask %q references missing test case %q", lineNumber, subTask.Name, testCaseName)
				}
				_, err := batch.stmtSubTaskTestCases.Exec(currentSubTaskID, testID)
				panicIf(err)
			}
			subTaskID++
		}

		batch.count++
		if batch.count >= *flagBatchSize {
			panicIf(batch.commit())
			batch, err = newBatchWriter(db)
			panicIf(err)
		}

		if readErr == io.EOF {
			break
		}
	}

	panicIf(batch.commit())
	vacuumPath := *flagSqliteOutput + ".vacuum"
	_ = os.Remove(vacuumPath)
	vacuumSQL := "VACUUM INTO '" + strings.ReplaceAll(vacuumPath, "'", "''") + "';"
	_, err = db.Exec(vacuumSQL)
	panicIf(err)
	panicIf(db.Close())
	_ = os.Remove(*flagSqliteOutput)
	panicIf(os.Rename(vacuumPath, *flagSqliteOutput))
}
