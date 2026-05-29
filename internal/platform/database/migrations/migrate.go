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

// dropLegacyTables drops tables whose primary key changed from UUID to integer.
// PostgreSQL cannot cast uuid→bigint automatically, so we drop and recreate.
// The seeder repopulates all data on next startup.
func dropLegacyTables() {
	for _, t := range []string{"teaching_assignments", "subjects", "classes", "teachers"} {
		config.DB.Exec("DROP TABLE IF EXISTS " + t + " CASCADE")
	}
}

func Run() {
	dropLegacyTables()
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
