package algorithm

import (
	"fmt"
	"strconv"

	teachingassignments "smp_mater_dei_be/internal/teaching_assignments"

	"github.com/google/uuid"
)

func GenerateMatrixBlocks(assignments []teachingassignments.TeachingAssignment, pjokSubjectID uint) ([]MatrixBlock, error) {
	// Grouped blocks (SBP) are placed first so DecodeChromosome gives them
	// priority before other subjects claim their class grid slots.
	grouped := make([]MatrixBlock, 0, len(assignments))
	ungrouped := make([]MatrixBlock, 0, len(assignments)*2)

	for _, assignment := range assignments {
		durations, err := SplitAssignmentJP(assignment.JP, assignment.SubjectID == pjokSubjectID)
		if err != nil {
			return nil, fmt.Errorf("split assignment %d: %w", assignment.ID, err)
		}

		for partIndex, duration := range durations {
			b := MatrixBlock{
				ID:        matrixBlockIDForAssignment(assignment, partIndex, duration),
				TeacherID: assignment.TeacherID,
				SubjectID: assignment.SubjectID,
				ClassID:   assignment.ClassID,
				Duration:  duration,
				GroupKey:  assignment.GroupKey,
			}
			if assignment.GroupKey != nil {
				grouped = append(grouped, b)
			} else {
				ungrouped = append(ungrouped, b)
			}
		}
	}

	return append(grouped, ungrouped...), nil
}

func SplitAssignmentJP(jp int, isPJOK bool) ([]int, error) {
	if isPJOK {
		if jp != 3 {
			return nil, fmt.Errorf("PJOK assignment must be 3 JP, got %d", jp)
		}
		return []int{2, 1}, nil
	}

	switch jp {
	case 1:
		return []int{1}, nil
	case 2:
		return []int{2}, nil
	case 3:
		return []int{3}, nil
	case 4:
		return []int{2, 2}, nil
	case 5:
		return []int{3, 2}, nil
	case 6:
		return []int{3, 3}, nil
	default:
		return nil, fmt.Errorf("unsupported JP total %d", jp)
	}
}

// GroupIndex maps a GroupKey to the indices of all blocks in that group
// within a given blocks slice. All blocks in a group must share the same
// (Day, StartSlot) gene — they are scheduled in parallel.
type GroupIndex map[string][]int

// BuildGroupIndex derives group membership from the blocks slice.
func BuildGroupIndex(blocks []MatrixBlock) GroupIndex {
	idx := make(GroupIndex)
	for i, b := range blocks {
		if b.GroupKey == nil {
			continue
		}
		idx[*b.GroupKey] = append(idx[*b.GroupKey], i)
	}
	return idx
}

func matrixBlockIDForAssignment(assignment teachingassignments.TeachingAssignment, partIndex int, duration int) uuid.UUID {
	teacherID := "nil"
	if assignment.TeacherID != nil {
		teacherID = strconv.FormatUint(uint64(*assignment.TeacherID), 10)
	}

	name := fmt.Sprintf(
		"matrix-block|%d|%s|%d|%d|%d|%d|%d",
		assignment.ID,
		teacherID,
		assignment.SubjectID,
		assignment.ClassID,
		assignment.JP,
		partIndex,
		duration,
	)
	return uuid.NewSHA1(uuid.NameSpaceOID, []byte(name))
}
