package parents_students

import (
	"time"

	"gorm.io/gorm"
)

type ParentStudent struct {
	ID        uint `gorm:"primaryKey;autoIncrement"`
	ParentID  uint
	StudentID uint
	Relation  string // father, mother, guardian

	CreatedAt time.Time
	CreatedBy string
	UpdatedAt time.Time
	UpdatedBy string
	DeletedAt gorm.DeletedAt `gorm:"index"`
	DeletedBy *string
}
