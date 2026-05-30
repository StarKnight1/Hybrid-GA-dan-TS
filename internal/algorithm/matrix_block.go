package algorithm

// MatrixBlock represents a single schedulable teaching unit derived from one teaching assignment.
// Blocks belonging to the same parallel group (e.g. SBP) share a GroupKey and must
// always occupy the same (Day, StartSlot) in the timetable.
type MatrixBlock struct {
	ID        uint
	TeacherID *uint
	SubjectID uint
	ClassID   uint
	Duration  int
	GroupKey  *string
}

// NewMatrixBlock constructs a MatrixBlock with the given attributes.
func NewMatrixBlock(id uint, teacherID *uint, subjectID, classID uint, duration int, groupKey *string) MatrixBlock {
	return MatrixBlock{
		ID:        id,
		TeacherID: teacherID,
		SubjectID: subjectID,
		ClassID:   classID,
		Duration:  duration,
		GroupKey:  groupKey,
	}
}
