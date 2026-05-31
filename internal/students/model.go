package students

import (
	"time"

	"gorm.io/gorm"
)

type Student struct {
	ID uint `gorm:"primaryKey;autoIncrement"`

	UserID uint `gorm:"not null;uniqueIndex"`

	FullName string `gorm:"not null"`
	Nickname string

	BirthPlace string
	BirthDate  time.Time
	Address    string
	Phone      string
	Religion   string
	Gender     string `gorm:"type:text;check:gender IN ('male','female')"`

	StudentNumber string `gorm:"uniqueIndex;not null"` // NIS
	ClassID       *uint  `gorm:"index"`               // link to classes table

	CreatedAt time.Time
	CreatedBy string
	UpdatedAt time.Time
	UpdatedBy string
	DeletedAt gorm.DeletedAt `gorm:"index"`
	DeletedBy *string
}
