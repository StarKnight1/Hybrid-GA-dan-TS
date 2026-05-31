package algorithm

// MatrixBlock merepresentasikan satu unit pengajaran yang dapat dijadwalkan, berasal dari satu penugasan mengajar.
// Blok yang termasuk grup paralel yang sama (mis. SBP) berbagi GroupKey dan harus
// selalu menempati (Day, StartSlot) yang sama dalam jadwal.
type MatrixBlock struct {
	ID        uint
	TeacherID *uint
	SubjectID uint
	ClassID   uint
	Duration  int
	GroupKey  *string
}

// NewMatrixBlock membuat MatrixBlock dengan atribut yang diberikan.
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
