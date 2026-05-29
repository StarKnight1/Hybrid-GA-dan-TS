package classes

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type Class struct {
	ID uuid.UUID `gorm:"type:uuid;default:gen_random_uuid();primaryKey"`

	Grade int    `gorm:"not null"`             // 7, 8, 9
	Code  string `gorm:"not null"`             // A, B, C
	Name  string `gorm:"uniqueIndex;not null"` // "7A", "8B"

	IsActive bool `gorm:"default:true"`

	HomeroomTeacherID *uuid.UUID `gorm:"type:uuid;index;uniqueIndex:uniq_homeroom_teacher_year"`

	CreatedAt time.Time
	CreatedBy string
	UpdatedAt time.Time
	UpdatedBy string
	DeletedAt gorm.DeletedAt
	DeletedBy *string
}
