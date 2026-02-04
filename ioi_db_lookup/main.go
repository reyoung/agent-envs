package main

import (
	"database/sql"
	"errors"
	"flag"
	"fmt"
	"log"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"

	_ "github.com/glebarez/go-sqlite"
)

func main() {
	log.SetFlags(0)
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}

	switch os.Args[1] {
	case "grader-files":
		if err := runGraderFiles(os.Args[2:]); err != nil {
			log.Fatal(err)
		}
	case "test-case":
		if err := runTestCase(os.Args[2:]); err != nil {
			log.Fatal(err)
		}
	case "-h", "--help", "help":
		usage()
	default:
		usage()
		log.Fatalf("unknown subcommand: %s", os.Args[1])
	}
}

func usage() {
	fmt.Fprint(os.Stderr, `Usage:
  ioi_db_lookup grader-files -db <path> (-problem-id <id> | -problem <name>) [-year <year>]
  ioi_db_lookup test-case   -db <path> (-problem-id <id> | -problem <name>) [-year <year>] -test-case-id <name>
`)
}

func runGraderFiles(args []string) error {
	fs := flag.NewFlagSet("grader-files", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)

	var dbPath string
	var problemName string
	var problemID int64
	var year int
	fs.StringVar(&dbPath, "db", "", "Path to sqlite database")
	fs.StringVar(&dbPath, "sqlite", "", "Path to sqlite database")
	fs.StringVar(&problemName, "problem", "", "Problem name")
	fs.StringVar(&problemName, "problem-name", "", "Problem name")
	fs.Int64Var(&problemID, "problem-id", 0, "Problem id")
	fs.IntVar(&year, "year", 0, "Problem year")

	if err := fs.Parse(args); err != nil {
		return err
	}
	if dbPath == "" {
		return errors.New("missing -db")
	}
	if problemID == 0 && problemName == "" {
		return errors.New("missing -problem-id or -problem")
	}

	db, err := openReadonly(dbPath)
	if err != nil {
		return err
	}
	defer db.Close()

	resolvedID, err := resolveProblemID(db, problemID, problemName, year)
	if err != nil {
		return err
	}

	files, err := fetchGraderFiles(db, resolvedID)
	if err != nil {
		return err
	}
	if len(files) == 0 {
		return fmt.Errorf("no grader files found for problem %d", resolvedID)
	}

	names := make([]string, 0, len(files))
	for name := range files {
		names = append(names, name)
	}
	sort.Strings(names)

	for _, name := range names {
		safeName, err := sanitizeFilename(name)
		if err != nil {
			return err
		}
		if err := os.WriteFile(safeName, files[name], 0o644); err != nil {
			return err
		}
	}

	return nil
}

func runTestCase(args []string) error {
	fs := flag.NewFlagSet("test-case", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)

	var dbPath string
	var problemName string
	var problemID int64
	var testCaseName string
	var year int
	fs.StringVar(&dbPath, "db", "", "Path to sqlite database")
	fs.StringVar(&dbPath, "sqlite", "", "Path to sqlite database")
	fs.StringVar(&problemName, "problem", "", "Problem name")
	fs.StringVar(&problemName, "problem-name", "", "Problem name")
	fs.Int64Var(&problemID, "problem-id", 0, "Problem id")
	fs.StringVar(&testCaseName, "test-case-id", "", "Test case name")
	fs.StringVar(&testCaseName, "test-case", "", "Test case name")
	fs.StringVar(&testCaseName, "case-id", "", "Test case name")
	fs.IntVar(&year, "year", 0, "Problem year")

	if err := fs.Parse(args); err != nil {
		return err
	}
	if dbPath == "" {
		return errors.New("missing -db")
	}
	if testCaseName == "" {
		return errors.New("missing -test-case-id")
	}
	if problemID == 0 && problemName == "" {
		return errors.New("missing -problem-id or -problem")
	}

	db, err := openReadonly(dbPath)
	if err != nil {
		return err
	}
	defer db.Close()

	resolvedID, err := resolveProblemID(db, problemID, problemName, year)
	if err != nil {
		return err
	}

	input, output, err := fetchTestCase(db, resolvedID, testCaseName)
	if err != nil {
		return err
	}

	if err := os.WriteFile("input.txt", input, 0o644); err != nil {
		return err
	}
	if err := os.WriteFile("output.txt", output, 0o644); err != nil {
		return err
	}

	return nil
}

func openReadonly(path string) (*sql.DB, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	info, err := os.Stat(abs)
	if err != nil {
		return nil, err
	}
	if info.IsDir() {
		return nil, fmt.Errorf("db path is a directory: %s", abs)
	}

	u := url.URL{Scheme: "file", Path: abs}
	q := u.Query()
	q.Set("mode", "ro")
	u.RawQuery = q.Encode()
	return sql.Open("sqlite", u.String())
}

func resolveProblemID(db *sql.DB, problemID int64, problemName string, year int) (int64, error) {
	if problemID != 0 {
		var id int64
		err := db.QueryRow(`SELECT id FROM problems WHERE id = ?`, problemID).Scan(&id)
		if errors.Is(err, sql.ErrNoRows) {
			return 0, fmt.Errorf("problem id %d not found", problemID)
		}
		if err != nil {
			return 0, err
		}
		return id, nil
	}

	if year > 0 {
		var id int64
		err := db.QueryRow(`SELECT id FROM problems WHERE name = ? AND year = ?`, problemName, year).Scan(&id)
		if errors.Is(err, sql.ErrNoRows) {
			return 0, fmt.Errorf("problem %q in year %d not found", problemName, year)
		}
		if err != nil {
			return 0, err
		}
		return id, nil
	}

	rows, err := db.Query(`SELECT id, year FROM problems WHERE name = ? ORDER BY year`, problemName)
	if err != nil {
		return 0, err
	}
	defer rows.Close()

	var id int64
	var foundYears []int
	for rows.Next() {
		var rowID int64
		var rowYear int
		if err := rows.Scan(&rowID, &rowYear); err != nil {
			return 0, err
		}
		if id == 0 {
			id = rowID
		}
		foundYears = append(foundYears, rowYear)
	}
	if err := rows.Err(); err != nil {
		return 0, err
	}
	if id == 0 {
		return 0, fmt.Errorf("problem %q not found", problemName)
	}
	if len(foundYears) > 1 {
		return 0, fmt.Errorf("problem %q exists in multiple years %v; pass -year", problemName, foundYears)
	}
	return id, nil
}

func fetchTestCase(db *sql.DB, problemID int64, testCaseName string) ([]byte, []byte, error) {
	var input []byte
	var output []byte
	err := db.QueryRow(
		`SELECT input, output FROM test_cases WHERE name = ? AND problem_id = ?`,
		testCaseName,
		problemID,
	).Scan(&input, &output)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil, fmt.Errorf("test case %q for problem %d not found", testCaseName, problemID)
	}
	if err != nil {
		return nil, nil, err
	}
	return input, output, nil
}

func fetchGraderFiles(db *sql.DB, problemID int64) (map[string][]byte, error) {
	tableInfo, err := graderFilesColumns(db)
	if err != nil {
		return nil, err
	}
	if !tableInfo.exists {
		return nil, errors.New("grader_files table not found; regenerate sqlite with grader files enabled")
	}

	query := fmt.Sprintf(
		`SELECT %s, %s FROM grader_files WHERE problem_id = ? ORDER BY %s`,
		tableInfo.nameColumn,
		tableInfo.contentColumn,
		tableInfo.nameColumn,
	)
	rows, err := db.Query(query, problemID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	files := make(map[string][]byte)
	for rows.Next() {
		var name string
		var content []byte
		if err := rows.Scan(&name, &content); err != nil {
			return nil, err
		}
		files[name] = content
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return files, nil
}

type graderFilesInfo struct {
	exists        bool
	nameColumn    string
	contentColumn string
}

func graderFilesColumns(db *sql.DB) (graderFilesInfo, error) {
	rows, err := db.Query(`PRAGMA table_info(grader_files)`)
	if err != nil {
		return graderFilesInfo{}, err
	}
	defer rows.Close()

	var columns []string
	for rows.Next() {
		var cid int
		var name string
		var colType string
		var notNull int
		var dflt sql.NullString
		var pk int
		if err := rows.Scan(&cid, &name, &colType, &notNull, &dflt, &pk); err != nil {
			return graderFilesInfo{}, err
		}
		columns = append(columns, strings.ToLower(name))
	}
	if err := rows.Err(); err != nil {
		return graderFilesInfo{}, err
	}
	if len(columns) == 0 {
		return graderFilesInfo{exists: false}, nil
	}

	info := graderFilesInfo{exists: true}
	nameCandidates := []string{"name", "filename", "file_name"}
	contentCandidates := []string{"content", "data", "body", "source"}

	for _, candidate := range nameCandidates {
		if hasColumn(columns, candidate) {
			info.nameColumn = candidate
			break
		}
	}
	for _, candidate := range contentCandidates {
		if hasColumn(columns, candidate) {
			info.contentColumn = candidate
			break
		}
	}

	if info.nameColumn == "" {
		info.nameColumn = "name"
	}
	if info.contentColumn == "" {
		info.contentColumn = "content"
	}

	return info, nil
}

func hasColumn(columns []string, name string) bool {
	for _, column := range columns {
		if column == name {
			return true
		}
	}
	return false
}

func sanitizeFilename(name string) (string, error) {
	if name == "" {
		return "", errors.New("grader file name is empty")
	}
	if strings.ContainsAny(name, `/\`) || name != filepath.Base(name) {
		return "", fmt.Errorf("grader file name contains path separators: %q", name)
	}
	clean := filepath.Clean(name)
	if clean == "." || clean == string(filepath.Separator) {
		return "", fmt.Errorf("invalid grader file name: %q", name)
	}
	return name, nil
}
