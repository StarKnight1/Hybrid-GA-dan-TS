package classes

import "smp_mater_dei_be/internal/platform/config"

func GetByID(id uint) (*Class, error) {
	var c Class
	err := config.DB.First(&c, id).Error
	return &c, err
}
