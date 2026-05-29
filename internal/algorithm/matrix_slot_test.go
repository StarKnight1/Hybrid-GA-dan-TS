package algorithm

import (
	"testing"

	"github.com/google/uuid"
)

func TestScheduleMatrixPlaceBlockUpdatesClassAndTeacher(t *testing.T) {
	classID := uuid.New()
	teacherID := uuid.New()
	block := matrixTestBlock(classID, &teacherID, 2)
	matrix := NewScheduleMatrix([]uuid.UUID{classID}, []uuid.UUID{teacherID}, []MatrixBlock{block}, nil)

	if err := matrix.PlaceBlock(block.ID, "monday", 1); err != nil {
		t.Fatalf("PlaceBlock returned error: %v", err)
	}

	for _, slotIndex := range []int{1, 2} {
		classCell, ok := matrix.ClassCell(classID, "monday", slotIndex)
		if !ok || classCell.State != CellOccupied || classCell.BlockID != block.ID {
			t.Fatalf("class cell %d = %+v, %v; want occupied by %s", slotIndex, classCell, ok, block.ID)
		}

		teacherCell, ok := matrix.TeacherCell(teacherID, "monday", slotIndex)
		if !ok || teacherCell.State != CellOccupied || teacherCell.BlockID != block.ID {
			t.Fatalf("teacher cell %d = %+v, %v; want occupied by %s", slotIndex, teacherCell, ok, block.ID)
		}
	}

	if err := matrix.ValidateIntegrity(); err != nil {
		t.Fatalf("ValidateIntegrity returned error: %v", err)
	}
}

func TestScheduleMatrixRejectsTeacherConflict(t *testing.T) {
	teacherID := uuid.New()
	classA := uuid.New()
	classB := uuid.New()
	blockA := matrixTestBlock(classA, &teacherID, 2)
	blockB := matrixTestBlock(classB, &teacherID, 1)
	matrix := NewScheduleMatrix([]uuid.UUID{classA, classB}, []uuid.UUID{teacherID}, []MatrixBlock{blockA, blockB}, nil)

	if err := matrix.PlaceBlock(blockA.ID, "tuesday", 3); err != nil {
		t.Fatalf("PlaceBlock blockA returned error: %v", err)
	}
	if err := matrix.PlaceBlock(blockB.ID, "tuesday", 4); err == nil {
		t.Fatal("PlaceBlock blockB succeeded; want teacher conflict")
	}
}

func TestScheduleMatrixMoveRollsBackOnRejectedPlacement(t *testing.T) {
	classID := uuid.New()
	teacherID := uuid.New()
	block := matrixTestBlock(classID, &teacherID, 2)
	matrix := NewScheduleMatrix([]uuid.UUID{classID}, []uuid.UUID{teacherID}, []MatrixBlock{block}, nil)

	if err := matrix.PlaceBlock(block.ID, "wednesday", 1); err != nil {
		t.Fatalf("PlaceBlock returned error: %v", err)
	}
	if err := matrix.MoveBlock(block.ID, "monday", 0); err == nil {
		t.Fatal("MoveBlock succeeded on blocked slot; want rejection")
	}

	placement, ok := matrix.Placement(block.ID)
	if !ok {
		t.Fatal("placement missing after rejected move")
	}
	if placement.Day != "wednesday" || placement.StartSlot != 1 {
		t.Fatalf("placement = %+v; want original wednesday slot 1", placement)
	}
	if err := matrix.ValidateIntegrity(); err != nil {
		t.Fatalf("ValidateIntegrity returned error after rollback: %v", err)
	}
}

func TestScheduleMatrixUsesSlotIndexNotSlicePosition(t *testing.T) {
	classID := uuid.New()
	block := matrixTestBlock(classID, nil, 1)
	daySlots := DaySlots{
		"monday": {
			{Index: 2, StartTime: "08:00", EndTime: "08:40", IsBlocked: false},
		},
	}
	matrix := NewScheduleMatrix([]uuid.UUID{classID}, nil, []MatrixBlock{block}, daySlots)

	if err := matrix.PlaceBlock(block.ID, "monday", 0); err == nil {
		t.Fatal("PlaceBlock succeeded at missing slot index 0; want blocked/missing slot rejection")
	}
	if err := matrix.PlaceBlock(block.ID, "monday", 2); err != nil {
		t.Fatalf("PlaceBlock at explicit slot index 2 returned error: %v", err)
	}
}

func matrixTestBlock(classID uuid.UUID, teacherID *uuid.UUID, duration int) MatrixBlock {
	return MatrixBlock{
		ID:        uuid.New(),
		TeacherID: teacherID,
		SubjectID: uuid.New(),
		ClassID:   classID,
		Duration:  duration,
	}
}
