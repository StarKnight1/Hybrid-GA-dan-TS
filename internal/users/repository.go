package users

import (
	"smp_mater_dei_be/internal/platform/config"
)

func GetUserByIdentifier(identifier string) (*User, error) {
	var user User
	err := config.DB.Where("login_identifier = ?", identifier).First(&user).Error
	return &user, err
}

func GetUserByID(id string) (*User, error) {
	var user User
	err := config.DB.Where("id = ?", id).First(&user).Error
	return &user, err
}
