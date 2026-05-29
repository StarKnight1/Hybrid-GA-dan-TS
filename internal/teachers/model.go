package teachers

import (
	"time"

	"gorm.io/gorm"
)

type Teacher struct {
	ID uint `gorm:"primaryKey;autoIncrement"`

	UserID *uint `gorm:"uniqueIndex"`

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
