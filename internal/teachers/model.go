package teachers

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type Teacher struct {
	ID uuid.UUID `gorm:"type:uuid;default:gen_random_uuid();primaryKey"`

	UserID uuid.UUID `gorm:"type:uuid;not null;uniqueIndex"`

	TeacherNumber int `gorm:"uniqueIndex;not null"`

	FullName   string `gorm:"not null"`
	BirthPlace string
	BirthDate  time.Time

	Address string
	Phone   string

	Gender   string `gorm:"type:text;check:gender IN ('male','female')"`
	Religion string

	CreatedAt time.Time
	CreatedBy string
	UpdatedAt time.Time
	UpdatedBy string
	DeletedAt gorm.DeletedAt `gorm:"index"`
	DeletedBy *string
}
