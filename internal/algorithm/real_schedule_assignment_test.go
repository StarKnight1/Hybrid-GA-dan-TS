package algorithm

import (
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"testing"
)

func TestSeederAssignmentJPTotalsMatchRealScheduleRows(t *testing.T) {
	seederAssignments := parseSeederAssignmentTotals(t)
	realScheduleTotals := realScheduleRowTotals()

	for key, assignmentJP := range seederAssignments {
		if realJP := realScheduleTotals[key]; realJP != assignmentJP {
			t.Fatalf("assignment %s has %d JP in seeder, %d rows in real schedule", key, assignmentJP, realJP)
		}
	}
	for key, realJP := range realScheduleTotals {
		if assignmentJP := seederAssignments[key]; assignmentJP != realJP {
			t.Fatalf("real schedule %s has %d rows, %d JP in seeder", key, realJP, assignmentJP)
		}
	}
}

func parseSeederAssignmentTotals(t *testing.T) map[string]int {
	t.Helper()

	path := filepath.Join("..", "platform", "database", "seeders", "temp_seeder.go")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read temp seeder: %v", err)
	}

	start := regexp.MustCompile(`assignments := \[\]assignment\{`).FindIndex(raw)
	if start == nil {
		t.Fatal("assignment slice not found in temp seeder")
	}
	end := regexp.MustCompile(`\n\t\}\n\n\tvar existingAssignments`).FindIndex(raw[start[1]:])
	if end == nil {
		t.Fatal("assignment slice end not found in temp seeder")
	}

	body := raw[start[1] : start[1]+end[0]]
	rowRE := regexp.MustCompile(`\{"([^"]*)",\s*"([^"]+)",\s*"([^"]+)",\s*(\d+),\s*"([^"]*)"\}`)

	totals := make(map[string]int)
	for _, match := range rowRE.FindAllSubmatch(body, -1) {
		teacherNumber := string(match[1])
		className := string(match[3])
		if teacherNumber == "" {
			teacherNumber = realScheduleSBPTeacherNumber
		}

		jp, err := strconv.Atoi(string(match[4]))
		if err != nil {
			t.Fatalf("parse JP %q: %v", match[4], err)
		}

		totals[teacherNumber+"|"+className] += jp
	}

	if len(totals) == 0 {
		t.Fatal("no assignment rows parsed from temp seeder")
	}
	return totals
}

func realScheduleRowTotals() map[string]int {
	totals := make(map[string]int)
	for _, entry := range RealSchedule {
		totals[entry.TeacherNumber+"|"+entry.ClassName]++
	}
	return totals
}
