package parents

import "smp_mater_dei_be/internal/platform/config"

func GetParentByUserID(userID string) (*Parent, error) {
	var parent Parent
	err := config.DB.Where("user_id = ?", userID).First(&parent).Error
	return &parent, err
}
