package algorithm

import "github.com/google/uuid"

type MatrixBlock struct {
	ID        uuid.UUID
	TeacherID *uuid.UUID
	SubjectID uuid.UUID
	ClassID   uuid.UUID
	Duration  int
	GroupKey  *string // non-nil for SBP parallel groups; all blocks with the same key share one (Day, StartSlot)
}
