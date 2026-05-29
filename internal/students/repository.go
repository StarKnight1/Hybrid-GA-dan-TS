package students

import (
	"smp_mater_dei_be/internal/platform/config"
)

func GetStudentByUserID(userID string) (*Student, error) {
	var student Student
	err := config.DB.Where("user_id = ?", userID).First(&student).Error
	return &student, err
}
