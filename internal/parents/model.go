package parents

import (
	"time"

	"gorm.io/gorm"
)

type Parent struct {
	ID uint `gorm:"primaryKey;autoIncrement"`

	UserID uint `gorm:"not null;uniqueIndex"`

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
