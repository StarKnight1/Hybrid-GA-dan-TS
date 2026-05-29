package students

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type Student struct {
	ID uuid.UUID `gorm:"type:uuid;default:gen_random_uuid();primaryKey"`

	UserID uuid.UUID `gorm:"type:uuid;not null;uniqueIndex"`

	FullName string `gorm:"not null"`
	Nickname string

	BirthPlace string
	BirthDate  time.Time
	Address    string
	Phone      string
	Religion   string
	Gender     string `gorm:"type:text;check:gender IN ('male','female')"`

	StudentNumber string `gorm:"uniqueIndex;not null"` // NIS

	CreatedAt time.Time
	CreatedBy string
	UpdatedAt time.Time
	UpdatedBy string
	DeletedAt gorm.DeletedAt `gorm:"index"`
	DeletedBy *string
}
