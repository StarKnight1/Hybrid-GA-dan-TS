package migrations

import (
	"smp_mater_dei_be/internal/classes"
	"smp_mater_dei_be/internal/parents"
	"smp_mater_dei_be/internal/parents_students"
	"smp_mater_dei_be/internal/platform/config"
	"smp_mater_dei_be/internal/students"
	"smp_mater_dei_be/internal/subjects"
	"smp_mater_dei_be/internal/teachers"
	teachingassignments "smp_mater_dei_be/internal/teaching_assignments"
	"smp_mater_dei_be/internal/users"
)

func Run() {
	config.DB.AutoMigrate(
		&classes.Class{},
		&parents.Parent{},
		&parents_students.ParentStudent{},
		&students.Student{},
		&subjects.Subject{},
		&teachers.Teacher{},
		&users.User{},
		&teachingassignments.TeachingAssignment{},
	)
}
