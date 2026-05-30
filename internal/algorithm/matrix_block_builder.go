package algorithm

import (
	"fmt"

	teachingassignments "smp_mater_dei_be/internal/teaching_assignments"
)

// GenerateMatrixBlocks converts teaching assignments into schedulable MatrixBlocks.
// Parallel group blocks (SBP) are placed first in the returned slice so that
// DecodeChromosome can prioritize them before individual class slots fill up.
func GenerateMatrixBlocks(assignments []teachingassignments.TeachingAssignment, pjokSubjectID uint) ([]MatrixBlock, error) {
	parallelList := make([]MatrixBlock, 0, len(assignments))
	singleList := make([]MatrixBlock, 0, len(assignments)*2)

	nextID := uint(0)
	for _, assign := range assignments {
		durationList, err := SplitAssignmentJP(assign.JP, assign.SubjectID == pjokSubjectID)
		if err != nil {
			return nil, fmt.Errorf("split assignment %d: %w", assign.ID, err)
		}
		for _, dur := range durationList {
			nextID++
			blk := MatrixBlock{
				ID:        nextID,
				TeacherID: assign.TeacherID,
				SubjectID: assign.SubjectID,
				ClassID:   assign.ClassID,
				Duration:  dur,
				GroupKey:  assign.GroupKey,
			}
			switch assign.GroupKey != nil {
			case true:
				parallelList = append(parallelList, blk)
			default:
				singleList = append(singleList, blk)
			}
		}
	}

	return append(parallelList, singleList...), nil
}

// SplitAssignmentJP breaks a weekly JP total into concrete block durations.
// PJOK must always be 3 JP and is split into [2, 1] (practical + theory).
// All other subjects follow the standard decomposition table.
func SplitAssignmentJP(jp int, isPJOK bool) ([]int, error) {
	if isPJOK {
		if jp != 3 {
			return nil, fmt.Errorf("PJOK assignment must be 3 JP, got %d", jp)
		}
		return []int{2, 1}, nil
	}

	table := map[int][]int{
		1: {1},
		2: {2},
		3: {3},
		4: {2, 2},
		5: {3, 2},
		6: {3, 3},
	}
	result, ok := table[jp]
	if !ok {
		return nil, fmt.Errorf("unsupported JP total %d", jp)
	}
	return result, nil
}

// GroupMap maps a GroupKey to the indices of all member blocks within a given block slice.
// Every block sharing the same GroupKey must be scheduled at the same (Day, StartSlot).
type GroupIndex map[string][]int

// IndexGroups builds a GroupIndex from a block slice by scanning each block's GroupKey.
func BuildGroupIndex(blocks []MatrixBlock) GroupIndex {
	peta := make(GroupIndex)
	for posisi, unit := range blocks {
		if unit.GroupKey == nil {
			continue
		}
		peta[*unit.GroupKey] = append(peta[*unit.GroupKey], posisi)
	}
	return peta
}
