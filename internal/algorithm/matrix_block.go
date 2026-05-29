package algorithm

import "github.com/google/uuid"

type MatrixBlock struct {
	ID        uuid.UUID // deterministic SHA1, internal only — not stored in DB
	TeacherID *uint
	SubjectID uint
	ClassID   uint
	Duration  int
	GroupKey  *string // non-nil for SBP parallel groups; all blocks with the same key share one (Day, StartSlot)
}
