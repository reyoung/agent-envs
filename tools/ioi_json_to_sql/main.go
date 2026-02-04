package main

import (
	"database/sql"
	"flag"
	"log"

	_ "github.com/mattn/go-sqlite3"
)

var (
	flagFilename     = flag.String("filename", "data.json", "Path to the IOI JSON file")
	flagSqliteOutput = flag.String("sqlite-output", "ioi_data.db", "Path to the output SQLite database file")
)

type Limit struct {
	TimeLimit   int `json:"time_limit"`
	MemoryLimit int `json:"memory_limit"`
}

type SubTask struct {
	Limit         Limit    `json:"limit"`
	Name          string   `json:"name"`
	Score         int      `json:"score"`
	TestCaseNames []string `json:"test_case_names"`
}

type Metadata struct {
	ID             string `json:"id"`
	Year           int    `json:"year"`
	SampleSolution string `json:"sample_solution"`
}

type TestCase struct {
	Input  string `json:"input"`
	Output string `json:"output"`
}

type IOIRecord struct {
	Metadata    Metadata            `json:"metadata"`
	SubTasks    []SubTask           `json:"sub_tasks"`
	GraderFiles map[string]string   `json:"grader_files"`
	TestCases   map[string]TestCase `json:"test_cases"`
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
		time_limit INT NOT NULL,
		memory_limit INT NOT NULL,
		FOREIGN KEY (problem_id) REFERENCES problems(id) ON DELETE CASCADE
	);
	CREATE UNIQUE INDEX IF NOT EXISTS unique_sub_task_problem_name ON sub_tasks (problem_id, name);

	CREATE TABLE IF NOT EXISTS sub_task_test_cases (
		sub_task_id BIGINT NOT NULL,
		test_case_id BIGINT NOT NULL,
		PRIMARY KEY (sub_task_id, test_case_id),
		FOREIGN KEY (sub_task_id) REFERENCES sub_tasks(id) ON DELETE CASCADE,
		FOREIGN KEY (test_case_id) REFERENCES test_cases(id) ON DELETE CASCADE
	);
	`)
	panicIf(err)

}

func main() {
	flag.Parse()
	db, err := sql.Open("sqlite3", *flagSqliteOutput)
	panicIf(err)
	defer db.Close()
	initDB(db)
}
