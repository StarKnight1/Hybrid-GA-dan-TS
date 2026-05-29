package parents

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type Parent struct {
	ID uuid.UUID `gorm:"type:uuid;default:gen_random_uuid();primaryKey"`

	UserID uuid.UUID `gorm:"type:uuid;not null;uniqueIndex"`

	FullName string `gorm:"not null"`
	Phone    string
	Address  string

	CreatedAt time.Time
	CreatedBy string `default:"System"`
	UpdatedAt time.Time
	UpdatedBy string         `default:"System"`
	DeletedAt gorm.DeletedAt `gorm:"index"`
	DeletedBy *string
}
