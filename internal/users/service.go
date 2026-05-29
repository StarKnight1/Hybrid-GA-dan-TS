package users

import (
	"errors"
	"smp_mater_dei_be/internal/parents"
	"smp_mater_dei_be/internal/platform/logging"
	"smp_mater_dei_be/internal/platform/security"
	"smp_mater_dei_be/internal/students"
	"smp_mater_dei_be/internal/teachers"
	"smp_mater_dei_be/internal/users/dto"

	"go.uber.org/zap"
)

var ErrInvalidCredentials = errors.New("invalid credentials")

func Login(identifier, password string) (string, error) {
	user, err := GetUserByIdentifier(identifier)
	logging.Logger.Info("login_attempt", zap.Any("user", user), zap.Error(err))
	if err != nil {
		return "", err
	}

	if !security.CheckPassword(user.PasswordHash, password) {
		logging.Logger.Info("login_failed", zap.String("identifier", identifier))
		return "", ErrInvalidCredentials
	}

	return GenerateToken(user.ID)
}

func GetProfile(userID string) (any, error) {
	user, err := GetUserByID(userID)
	if err != nil {
		return nil, err
	}

	switch user.Role {
	case "student":
		student, err := students.GetStudentByUserID(userID)
		if err != nil {
			return nil, err
		}

		studentResponse := dto.ProfileResponse{
			Username: student.FullName,
		}

		return studentResponse, nil

	case "parent":
		// Handle parent role
		parent, err := parents.GetParentByUserID(userID)
		if err != nil {
			return nil, err
		}

		parentResponse := dto.ProfileResponse{
			Username: parent.FullName,
		}

		return parentResponse, nil

	case "teacher":
		// Handle teacher role
		teacher, err := teachers.GetTeacherByUserID(userID)
		if err != nil {
			return nil, err
		}

		teacherResponse := dto.ProfileResponse{
			Username: teacher.FullName,
		}

		return teacherResponse, nil

	default:
		logging.Logger.Warn("unknown_role", zap.String("userID", userID), zap.String("role", user.Role))
		return nil, errors.New("unknown role")
	}
}
