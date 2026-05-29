package classes

import (
	"time"

	"gorm.io/gorm"
)

type Class struct {
	ID uint `gorm:"primaryKey;autoIncrement"`

	Grade int    `gorm:"not null"`             // 7, 8, 9
	Code  string `gorm:"not null"`             // A, B, C
	Name  string `gorm:"uniqueIndex;not null"` // "7A", "8B"

	IsActive bool `gorm:"default:true"`

	HomeroomTeacherID *uint `gorm:"index;uniqueIndex:uniq_homeroom_teacher_year"`

	CreatedAt time.Time
	CreatedBy string
	UpdatedAt time.Time
	UpdatedBy string
	DeletedAt gorm.DeletedAt
	DeletedBy *string
}
