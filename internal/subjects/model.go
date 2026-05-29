package subjects

import (
	"time"

	"gorm.io/gorm"
)

type Subject struct {
	ID uint `gorm:"primaryKey;autoIncrement"`

	Name string

	CreatedAt time.Time
	CreatedBy string
	UpdatedAt time.Time
	UpdatedBy string
	DeletedAt gorm.DeletedAt `gorm:"index"`
	DeletedBy *string
}
