package algorithm

import (
	"testing"

	"github.com/google/uuid"
)

func TestBuildScheduleMatrixFromRealEntriesGroupsContiguousRows(t *testing.T) {
	teacherID := uuid.New()
	classID := uuid.New()
	subjectID := uuid.New()

	result, err := BuildScheduleMatrixFromRealEntries(
		[]RealScheduleEntry{
			{TeacherNumber: "26", ClassName: "7A", Day: "monday", TimeStart: "07:50"},
			{TeacherNumber: "26", ClassName: "7A", Day: "monday", TimeStart: "08:30"},
		},
		RealScheduleMatrixOptions{
			TeacherNumberToID: map[string]uuid.UUID{"26": teacherID},
			ClassNameToID:     map[string]uuid.UUID{"7A": classID},
			SubjectByTeacherNumber: map[string]uuid.UUID{
				"26": subjectID,
			},
		},
	)
	if err != nil {
		t.Fatalf("BuildScheduleMatrixFromRealEntries returned error: %v", err)
	}
	if len(result.Blocks) != 1 {
		t.Fatalf("len(blocks) = %d; want 1", len(result.Blocks))
	}
	if result.Blocks[0].Duration != 2 {
		t.Fatalf("block duration = %d; want 2", result.Blocks[0].Duration)
	}

	slotA, _ := MatrixSlotIndexFromTimeStart("monday", "07:50", nil)
	slotB, _ := MatrixSlotIndexFromTimeStart("monday", "08:30", nil)
	cellA, ok := result.Matrix.ClassCell(classID, "monday", slotA)
	if !ok || cellA.State != CellOccupied {
		t.Fatalf("first class cell = %+v, %v; want occupied", cellA, ok)
	}
	cellB, ok := result.Matrix.ClassCell(classID, "monday", slotB)
	if !ok || cellB.State != CellOccupied {
		t.Fatalf("second class cell = %+v, %v; want occupied", cellB, ok)
	}
	if cellA.BlockID != cellB.BlockID {
		t.Fatalf("contiguous real rows were assigned different block IDs: %s and %s", cellA.BlockID, cellB.BlockID)
	}
}

func TestBuildRealScheduleMatrixFullManualSchedule(t *testing.T) {
	opts := fakeRealScheduleMatrixOptions(RealSchedule)

	result, err := BuildRealScheduleMatrix(opts)
	if err != nil {
		t.Fatalf("BuildRealScheduleMatrix returned error: %v", err)
	}
	if len(result.Blocks) == 0 {
		t.Fatal("BuildRealScheduleMatrix returned no blocks")
	}
	if len(result.Placements) != len(result.Blocks) {
		t.Fatalf("placements = %d; want same as blocks %d", len(result.Placements), len(result.Blocks))
	}
	if err := result.Matrix.ValidateIntegrity(); err != nil {
		t.Fatalf("ValidateIntegrity returned error: %v", err)
	}
}

func fakeRealScheduleMatrixOptions(entries []RealScheduleEntry) RealScheduleMatrixOptions {
	teacherNumberToID := make(map[string]uuid.UUID)
	classNameToID := make(map[string]uuid.UUID)
	subjectByTeacherNumber := make(map[string]uuid.UUID)

	for _, entry := range entries {
		if _, ok := classNameToID[entry.ClassName]; !ok {
			classNameToID[entry.ClassName] = uuid.New()
		}
		if entry.TeacherNumber == realScheduleSBPTeacherNumber {
			continue
		}
		if _, ok := teacherNumberToID[entry.TeacherNumber]; !ok {
			teacherNumberToID[entry.TeacherNumber] = uuid.New()
		}
		if _, ok := subjectByTeacherNumber[entry.TeacherNumber]; !ok {
			subjectByTeacherNumber[entry.TeacherNumber] = uuid.New()
		}
	}

	return RealScheduleMatrixOptions{
		TeacherNumberToID:      teacherNumberToID,
		ClassNameToID:          classNameToID,
		SubjectByTeacherNumber: subjectByTeacherNumber,
		SBPSubjectID:           uuid.New(),
	}
}
