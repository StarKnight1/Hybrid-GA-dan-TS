package users

import (
	"errors"
	"smp_mater_dei_be/internal/classes"
	"smp_mater_dei_be/internal/parents"
	"smp_mater_dei_be/internal/platform/logging"
	"smp_mater_dei_be/internal/platform/security"
	"smp_mater_dei_be/internal/students"
	"smp_mater_dei_be/internal/teachers"
	"smp_mater_dei_be/internal/users/dto"

	"go.uber.org/zap"
)

var ErrInvalidCredentials = errors.New("invalid credentials")

func Login(identifier, password string) (string, string, error) {
	user, err := GetUserByIdentifier(identifier)
	logging.Logger.Info("login_attempt", zap.Any("user", user), zap.Error(err))
	if err != nil {
		return "", "", err
	}

	if !security.CheckPassword(user.PasswordHash, password) {
		logging.Logger.Info("login_failed", zap.String("identifier", identifier))
		return "", "", ErrInvalidCredentials
	}

	token, err := GenerateToken(user.ID, user.Role)
	return token, user.Role, err
}

func GetProfile(userID string) (any, error) {
	user, err := GetUserByID(userID)
	if err != nil {
		return nil, err
	}

	switch user.Role {
	case "admin":
		return dto.ProfileResponse{Username: "Administrator", Role: user.Role}, nil

	case "student":
		student, err := students.GetStudentByUserID(userID)
		if err != nil {
			return dto.ProfileResponse{Username: user.LoginIdentifier, Role: user.Role}, nil
		}
		className := ""
		if student.ClassID != nil {
			if cls, err := classes.GetByID(*student.ClassID); err == nil {
				className = cls.Name
			}
		}
		return dto.ProfileResponse{
			Username:  student.FullName,
			Role:      user.Role,
			ClassName: className,
			ClassID:   student.ClassID,
		}, nil

	case "parent":
		parent, err := parents.GetParentByUserID(userID)
		if err != nil {
			return dto.ProfileResponse{Username: user.LoginIdentifier, Role: user.Role}, nil
		}
		return dto.ProfileResponse{Username: parent.FullName, Role: user.Role}, nil

	case "teacher":
		teacher, err := teachers.GetTeacherByUserID(userID)
		if err != nil {
			return dto.ProfileResponse{Username: user.LoginIdentifier, Role: user.Role}, nil
		}
		return dto.ProfileResponse{
			Username:    teacher.FullName,
			Role:        user.Role,
			TeacherName: teacher.FullName,
			TeacherID:   &teacher.ID,
		}, nil

	default:
		logging.Logger.Warn("unknown_role", zap.String("userID", userID), zap.String("role", user.Role))
		return dto.ProfileResponse{Username: user.LoginIdentifier, Role: user.Role}, nil
	}
}
