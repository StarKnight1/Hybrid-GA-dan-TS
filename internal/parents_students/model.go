package parents_students

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type ParentStudent struct {
	ID        uuid.UUID `gorm:"type:uuid;default:gen_random_uuid();primaryKey"`
	ParentID  uuid.UUID
	StudentID uuid.UUID
	Relation  string // father, mother, guardian

	CreatedAt time.Time
	CreatedBy string
	UpdatedAt time.Time
	UpdatedBy string
	DeletedAt gorm.DeletedAt `gorm:"index"`
	DeletedBy *string
}
