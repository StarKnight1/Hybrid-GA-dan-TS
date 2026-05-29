package users

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type User struct {
	ID uuid.UUID `gorm:"type:uuid;default:gen_random_uuid();primaryKey"`

	LoginIdentifier string `gorm:"uniqueIndex;not null"`
	PasswordHash    string `gorm:"not null"`
	Role            string `gorm:"type:text;not null"`

	IsActive    bool `gorm:"default:true"`
	LastLoginAt *time.Time

	CreatedAt time.Time
	CreatedBy string
	UpdatedAt time.Time
	UpdatedBy string
	DeletedAt gorm.DeletedAt `gorm:"index"`
	DeletedBy *string
}
