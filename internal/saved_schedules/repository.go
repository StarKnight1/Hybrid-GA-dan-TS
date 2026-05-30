package savedschedules

import (
	"smp_mater_dei_be/internal/platform/config"
)

func Create(s *SavedSchedule) error {
	return config.DB.Create(s).Error
}

func List() ([]SavedScheduleListItem, error) {
	var items []SavedScheduleListItem
	err := config.DB.Model(&SavedSchedule{}).
		Select("id, title, created_at, created_by").
		Order("created_at DESC").
		Scan(&items).Error
	return items, err
}

func GetByID(id uint) (*SavedSchedule, error) {
	var s SavedSchedule
	err := config.DB.First(&s, id).Error
	return &s, err
}

func Delete(id uint) error {
	return config.DB.Delete(&SavedSchedule{}, id).Error
}
