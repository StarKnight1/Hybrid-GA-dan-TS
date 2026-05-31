package teachingassignments

import (
	"time"

	"gorm.io/gorm"
)

type TeachingAssignment struct {
	ID uint `gorm:"primaryKey;autoIncrement"`

	// TeacherID nullable — entri SBP tidak memiliki guru yang ditugaskan
	TeacherID *uint `gorm:"index"`

	SubjectID uint `gorm:"not null;index"`
	ClassID   uint `gorm:"not null;index"`

	JP int `gorm:"not null"` // jam pelajaran per minggu

	// GroupKey diisi untuk grup paralel SBP (mis. "SBP-7-ABC"), nil untuk mapel lain
	GroupKey *string `gorm:"index"`

	CreatedAt time.Time
	CreatedBy string         `gorm:"default:'SYSTEM'"`
	UpdatedAt time.Time
	UpdatedBy string         `gorm:"default:'SYSTEM'"`
	DeletedAt gorm.DeletedAt `gorm:"index"`
	DeletedBy *string
}
