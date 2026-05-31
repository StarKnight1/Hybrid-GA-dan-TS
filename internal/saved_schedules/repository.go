package savedschedules

import (
	"smp_mater_dei_be/internal/platform/config"

	"gorm.io/gorm"
)

func Create(s *SavedSchedule) error {
	return config.DB.Create(s).Error
}

func List() ([]SavedScheduleListItem, error) {
	var items []SavedScheduleListItem
	err := config.DB.Model(&SavedSchedule{}).
		Select("id, title, created_at, created_by, is_active").
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

// Activate mengaktifkan jadwal ini dan menonaktifkan semua jadwal lain.
func Activate(id uint) error {
	return config.DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&SavedSchedule{}).Where("id > ?", 0).Update("is_active", false).Error; err != nil {
			return err
		}
		return tx.Model(&SavedSchedule{}).Where("id = ?", id).Update("is_active", true).Error
	})
}

// GetActive mengambil jadwal yang sedang aktif/diterbitkan.
func GetActive() (*SavedSchedule, error) {
	var s SavedSchedule
	err := config.DB.Where("is_active = ?", true).First(&s).Error
	return &s, err
}
