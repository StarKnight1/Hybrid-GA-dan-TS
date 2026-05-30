package savedschedules

import (
	"time"

	"gorm.io/gorm"
)

type SavedSchedule struct {
	ID uint `gorm:"primaryKey;autoIncrement"`

	Title   string `gorm:"not null"`
	Entries string `gorm:"column:entries_json;type:text;not null"` // JSON array of ScheduleEntry
	Meta    string `gorm:"column:meta_json;type:text"`             // JSON ScheduleMeta

	CreatedAt time.Time
	CreatedBy string
	UpdatedAt time.Time
	UpdatedBy string
	DeletedAt gorm.DeletedAt `gorm:"index"`
	DeletedBy *string
}

type SavedScheduleListItem struct {
	ID        uint      `json:"id"`
	Title     string    `json:"title"`
	CreatedAt time.Time `json:"createdAt"`
	CreatedBy string    `json:"createdBy"`
}
