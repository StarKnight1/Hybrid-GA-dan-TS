package teachingassignments

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type TeachingAssignment struct {
	ID uuid.UUID `gorm:"type:uuid;default:gen_random_uuid();primaryKey"`

	// TeacherID is nullable — SBP entries have no assigned teacher
	TeacherID *uuid.UUID `gorm:"type:uuid;index"`

	SubjectID uuid.UUID `gorm:"type:uuid;not null;index"`
	ClassID   uuid.UUID `gorm:"type:uuid;not null;index"`

	JP int `gorm:"not null"` // jam pelajaran per week

	// GroupKey is set for SBP parallel groups (e.g. "SBP-7-ABC")
	// Nil for all other subjects
	GroupKey *string `gorm:"index"`

	CreatedAt time.Time
	CreatedBy string `gorm:"default:'SYSTEM'"`
	UpdatedAt time.Time
	UpdatedBy string         `gorm:"default:'SYSTEM'"`
	DeletedAt gorm.DeletedAt `gorm:"index"`
	DeletedBy *string
}
