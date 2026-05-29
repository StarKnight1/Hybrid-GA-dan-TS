package algorithm

type MatrixBlock struct {
	ID        uint
	TeacherID *uint
	SubjectID uint
	ClassID   uint
	Duration  int
	GroupKey  *string // non-nil for SBP parallel groups; all blocks with the same key share one (Day, StartSlot)
}
