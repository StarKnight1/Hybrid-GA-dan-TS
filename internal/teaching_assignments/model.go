package teachingassignments

import (
	"time"

	"gorm.io/gorm"
)

type TeachingAssignment struct {
	ID uint `gorm:"primaryKey;autoIncrement"`

	// TeacherID is nullable — SBP entries have no assigned teacher
	TeacherID *uint `gorm:"index"`

	SubjectID uint `gorm:"not null;index"`
	ClassID   uint `gorm:"not null;index"`

	JP int `gorm:"not null"` // jam pelajaran per week

	// GroupKey is set for SBP parallel groups (e.g. "SBP-7-ABC")
	// Nil for all other subjects
	GroupKey *string `gorm:"index"`

	CreatedAt time.Time
	CreatedBy string         `gorm:"default:'SYSTEM'"`
	UpdatedAt time.Time
	UpdatedBy string         `gorm:"default:'SYSTEM'"`
	DeletedAt gorm.DeletedAt `gorm:"index"`
	DeletedBy *string
}
