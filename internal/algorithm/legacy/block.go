package algorithm

import (
	teachingassignments "smp_mater_dei_be/internal/teaching_assignments"

	"github.com/google/uuid"
)

// ConstraintType defines special placement rules for a block
type ConstraintType string

const (
	ConstraintNone           ConstraintType = ""
	ConstraintFinishBefore11 ConstraintType = "FINISH_BEFORE_11" // PJOK 2JP practical block
)

// Block represents a single schedulable unit derived from a teaching assignment.
// The GA will place each block into a concrete day + timeslot.
type Block struct {
	ID           uuid.UUID
	AssignmentID uuid.UUID

	TeacherID *uuid.UUID
	SubjectID uuid.UUID
	ClassID   uuid.UUID

	JP int // number of periods this block occupies (1, 2, or 3)

	// Constraint on placement
	Constraint ConstraintType

	// Blocks sharing the same SiblingGroupID must NOT be placed on the same day.
	// Applies to split assignments (JP 4, 5, 6).
	// PJOK uses a dedicated 2JP practical + 1JP theory split without this day lock.
	SiblingGroupID *uuid.UUID

	// Seni Budaya parallel scheduling:
	// Blocks sharing the same ParallelGroupKey must be placed on the same day AND timeslot.
	// Mirrors GroupKey from the teaching assignment (e.g. "SBP-7-ABC").
	ParallelGroupKey *string

	// Which stream this block belongs to within a parallel group (e.g. "G1", "G2", "G3").
	// Nil for non-Seni-Budaya blocks.
	SplitGroupKey *string
}

// GenerateBlocks converts teaching assignments into schedulable blocks.
// pjokSubjectID is the UUID of the "PJOK" subject, used to apply its special split rule.
//
// Splitting rules:
//
//	JP 1 → [1]
//	JP 2 → [2]
//	JP 3 → [3]       (PJOK exception: [2 (FINISH_BEFORE_11), 1])
//	JP 4 → [2, 2]    different days
//	JP 5 → [3, 2]    different days (seed matching can consume as 3+2 or 2+3)
//	JP 6 → [3, 3]    different days
func GenerateBlocks(assignments []teachingassignments.TeachingAssignment, pjokSubjectID uuid.UUID) []Block {
	var blocks []Block

	for _, a := range assignments {
		isPJOK := a.SubjectID == pjokSubjectID

		switch {
		case isPJOK:
			// PJOK (3 JP) → 2JP practical (must finish before 11:00) + 1JP theory.
			// Same-day placement is allowed.
			blocks = append(blocks,
				Block{
					ID:           uuid.New(),
					AssignmentID: a.ID,
					TeacherID:    a.TeacherID,
					SubjectID:    a.SubjectID,
					ClassID:      a.ClassID,
					JP:           2,
					Constraint:   ConstraintFinishBefore11,
				},
				Block{
					ID:           uuid.New(),
					AssignmentID: a.ID,
					TeacherID:    a.TeacherID,
					SubjectID:    a.SubjectID,
					ClassID:      a.ClassID,
					JP:           1,
					Constraint:   ConstraintNone,
				},
			)

		case a.JP <= 3:
			// 1, 2, 3 JP → single straight block, no splitting
			blocks = append(blocks, Block{
				ID:               uuid.New(),
				AssignmentID:     a.ID,
				TeacherID:        a.TeacherID,
				SubjectID:        a.SubjectID,
				ClassID:          a.ClassID,
				JP:               a.JP,
				Constraint:       ConstraintNone,
				ParallelGroupKey: a.GroupKey,
			})

		case a.JP == 4:
			// 4 JP → 2 + 2, different days
			siblingID := uuid.New()
			for i := 0; i < 2; i++ {
				blocks = append(blocks, Block{
					ID:             uuid.New(),
					AssignmentID:   a.ID,
					TeacherID:      a.TeacherID,
					SubjectID:      a.SubjectID,
					ClassID:        a.ClassID,
					JP:             2,
					Constraint:     ConstraintNone,
					SiblingGroupID: &siblingID,
				})
			}

		case a.JP == 5:
			// 5 JP → 3 + 2, different days.
			// Seed matching supports flexible consumption order (3+2 or 2+3).
			siblingID := uuid.New()
			blocks = append(blocks,
				Block{
					ID:             uuid.New(),
					AssignmentID:   a.ID,
					TeacherID:      a.TeacherID,
					SubjectID:      a.SubjectID,
					ClassID:        a.ClassID,
					JP:             3,
					Constraint:     ConstraintNone,
					SiblingGroupID: &siblingID,
				},
				Block{
					ID:             uuid.New(),
					AssignmentID:   a.ID,
					TeacherID:      a.TeacherID,
					SubjectID:      a.SubjectID,
					ClassID:        a.ClassID,
					JP:             2,
					Constraint:     ConstraintNone,
					SiblingGroupID: &siblingID,
				},
			)

		case a.JP == 6:
			// 6 JP → 3 + 3, different days
			siblingID := uuid.New()
			for i := 0; i < 2; i++ {
				blocks = append(blocks, Block{
					ID:             uuid.New(),
					AssignmentID:   a.ID,
					TeacherID:      a.TeacherID,
					SubjectID:      a.SubjectID,
					ClassID:        a.ClassID,
					JP:             3,
					Constraint:     ConstraintNone,
					SiblingGroupID: &siblingID,
				})
			}
		}
	}

	return blocks
}
