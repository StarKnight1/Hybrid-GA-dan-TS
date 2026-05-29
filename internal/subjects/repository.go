package subjects

import "smp_mater_dei_be/internal/platform/config"

func GetAllSubjects() ([]Subject, error) {
	var list []Subject
	err := config.DB.Order("id").Find(&list).Error
	return list, err
}

func CreateSubject(name string) (*Subject, error) {
	s := Subject{Name: name}
	if err := config.DB.Create(&s).Error; err != nil {
		return nil, err
	}
	return &s, nil
}

func UpdateSubject(id uint, name string) (*Subject, error) {
	var s Subject
	if err := config.DB.First(&s, id).Error; err != nil {
		return nil, err
	}
	s.Name = name
	if err := config.DB.Save(&s).Error; err != nil {
		return nil, err
	}
	return &s, nil
}

func DeleteSubject(id uint) error {
	return config.DB.Delete(&Subject{}, id).Error
}
