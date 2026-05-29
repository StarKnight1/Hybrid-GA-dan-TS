package teachers

import "smp_mater_dei_be/internal/platform/config"

func GetTeacherByUserID(userID string) (*Teacher, error) {
	var teacher Teacher
	err := config.DB.Where("user_id = ?", userID).First(&teacher).Error
	return &teacher, err
}
