package seeders

import (
	"fmt"
	"log"
	"strconv"
	"time"

	"smp_mater_dei_be/internal/classes"
	"smp_mater_dei_be/internal/platform/security"
	"smp_mater_dei_be/internal/students"
	"smp_mater_dei_be/internal/subjects"
	"smp_mater_dei_be/internal/teachers"
	teachingassignments "smp_mater_dei_be/internal/teaching_assignments"
	"smp_mater_dei_be/internal/users"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

func SeedTemp(db *gorm.DB) error {
	// Hapus permanen record soft-deleted agar unique constraint tidak menghalangi insert ulang.
	db.Unscoped().Where("deleted_at IS NOT NULL").Delete(&teachingassignments.TeachingAssignment{})
	db.Unscoped().Where("deleted_at IS NOT NULL").Delete(&subjects.Subject{})
	db.Unscoped().Where("deleted_at IS NOT NULL").Delete(&classes.Class{})
	db.Unscoped().Where("deleted_at IS NOT NULL").Delete(&teachers.Teacher{})

	err := db.Transaction(func(tx *gorm.DB) error {
		if err := seedTeachers(tx); err != nil {
			return err
		}
		if err := seedClasses(tx); err != nil {
			return err
		}
		if err := seedSubjects(tx); err != nil {
			return err
		}
		if err := seedDefaultUsers(tx); err != nil {
			return err
		}
		if err := SeedTeachingAssignments(tx); err != nil {
			return err
		}

		return nil
	})
	if err != nil {
		return fmt.Errorf("seed temp data: %w", err)
	}

	log.Println("[SEED] temp seeding completed")
	return nil
}

func seedDefaultUsers(db *gorm.DB) error {
	admin := users.User{}
	err := db.Where(&users.User{LoginIdentifier: "admin"}).
		Attrs(users.User{
			LoginIdentifier: "admin",
			PasswordHash:    security.HashPassword("admin"),
			Role:            "admin",
		}).
		FirstOrCreate(&admin).Error
	if err != nil {
		return fmt.Errorf("seed admin user: %w", err)
	}

	studentUser := users.User{}
	err = db.Where(&users.User{LoginIdentifier: "123456789"}).
		Attrs(users.User{
			PasswordHash:    security.HashPassword("password"),
			Role:            "student",
			LoginIdentifier: "123456789",
		}).
		FirstOrCreate(&studentUser).Error
	if err != nil {
		return fmt.Errorf("seed sample student user: %w", err)
	}

	studentBirthDate := time.Date(2004, 3, 22, 0, 0, 0, 0, time.UTC)
	studentProfile := students.Student{}
	err = db.Where("user_id = ?", studentUser.ID).
		Attrs(students.Student{
			UserID:        studentUser.ID,
			StudentNumber: "32220116",
			Gender:        "male",
		}).
		FirstOrCreate(&studentProfile).Error
	if err != nil {
		return fmt.Errorf("seed sample student profile: %w", err)
	}

	// Selalu update data profil agar perubahan nama/data seeder diterapkan ke record yang sudah ada.
	var class7A classes.Class
	if err := db.Where("name = ?", "7A").First(&class7A).Error; err != nil {
		return fmt.Errorf("class 7A not found for student seed: %w", err)
	}
	err = db.Model(&studentProfile).Updates(students.Student{
		FullName:   "Andrew Kristanto Mulyono",
		Nickname:   "Andrew",
		BirthPlace: "Pamulang, Tangerang Selatan",
		BirthDate:  studentBirthDate,
		Address:    "Jl. Anggur 5 Blok A 34 No. 6",
		Phone:      "085773838656",
		Religion:   "Katolik",
		Gender:     "male",
		ClassID:    &class7A.ID,
	}).Error
	if err != nil {
		return fmt.Errorf("update sample student profile: %w", err)
	}

	// Akun guru contoh, terhubung ke data guru nomor 1
	teacherUser := users.User{}
	err = db.Where(&users.User{LoginIdentifier: "teacher1"}).
		Attrs(users.User{
			PasswordHash:    security.HashPassword("password"),
			Role:            "teacher",
			LoginIdentifier: "teacher1",
		}).
		FirstOrCreate(&teacherUser).Error
	if err != nil {
		return fmt.Errorf("seed sample teacher user: %w", err)
	}

	// Hubungkan akun ke data guru
	if teacherUser.ID > 0 {
		db.Model(&teachers.Teacher{}).
			Where("teacher_number = ? AND (user_id IS NULL OR user_id = ?)", 1, teacherUser.ID).
			Update("user_id", teacherUser.ID)
	}

	log.Println("[SEED] default users seeded")
	return nil
}

func seedTeachers(db *gorm.DB) error {
	defaultBirthDate := time.Date(1980, 1, 1, 0, 0, 0, 0, time.UTC)

	data := []struct {
		Number int
		Name   string
		Gender string
	}{
		{1, "Margareta Kamsiati, S.Pd", "female"},
		{2, "Dra. Maria Renata Andajani", "female"},
		{3, "Drs. Antonius Sarjiyono", "male"},
		{4, "Drs. Albertus Dwi K.P.", "male"},
		{5, "Lusia Kriswarini, S. Pd.", "female"},
		{6, "Catarina Nur Retnowati, S.Pd", "female"},
		{7, "Petrus Sapto W, S. Pd.", "male"},
		{8, "Susanto, S.Kom", "male"},
		{9, "Juanda Gultom, S.Pd", "male"},
		{10, "Agnes Siwi S.Pd", "female"},
		{11, "Geta Kana Ginting", "male"},
		{12, "Oscar Adi Kuncoro, S.Pd", "male"},
		{13, "Agustinus Nanang Aris. K", "male"},
		{14, "Rini Hartawati, S.Pd", "female"},
		{15, "Henrikus Erda Putra, S.Pd", "male"},
		{16, "Naomi J. Gultom, S.Pd", "female"},
		{17, "Padmi Astuti", "female"},
		{18, "Conny Hendrayani", "female"},
		{19, "Y. Edlyn Araminta, S.Pd", "female"},
		{20, "Sylvia Alfonza Fono. S.Pd", "female"},
		{21, "Paulinus Ivan S.Pd", "male"},
		{22, "Gregorius Eduard Djati P.", "male"},
		{23, "Daniel Hamonangan-PP", "male"},
		{24, "Joshua Fouryan P. M. S.Pd", "male"},
		{25, "Tommy", "male"},
		{26, "Jacqualine Sheren Kippuw", "female"},
		{27, "Maria Marvi", "female"},
		{28, "Yanita Hendrina", "female"},
	}

	teachersToSeed := make([]teachers.Teacher, 0, len(data))
	for _, d := range data {
		teachersToSeed = append(teachersToSeed, teachers.Teacher{
			TeacherNumber: d.Number,
			FullName:      d.Name,
			Gender:        d.Gender,
			BirthPlace:    "Yogyakarta",
			BirthDate:     defaultBirthDate,
			Address:       "Jl. Mater Dei No. 1, Yogyakarta",
			Phone:         "08123456789",
			Religion:      "Katolik",
		})
	}

	err := db.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "teacher_number"}},
		DoNothing: true,
	}).Create(&teachersToSeed).Error
	if err != nil {
		return fmt.Errorf("seed teachers: %w", err)
	}

	log.Println("[SEED] teachers seeded")
	return nil
}

func seedClasses(db *gorm.DB) error {
	grades := []int{7, 8, 9}
	codes := []string{"A", "B", "C", "D", "E", "F"}
	inactiveClassNames := []string{"7F", "9F"}
	classesToSeed := make([]classes.Class, 0, len(grades)*len(codes))

	for _, grade := range grades {
		for _, code := range codes {
			classesToSeed = append(classesToSeed, classes.Class{
				Grade: grade,
				Code:  code,
				Name:  fmt.Sprintf("%d%s", grade, code),
			})
		}
	}

	err := db.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "name"}},
		DoNothing: true,
	}).Create(&classesToSeed).Error
	if err != nil {
		return fmt.Errorf("seed classes: %w", err)
	}

	err = db.Model(&classes.Class{}).
		Where("name IN ?", inactiveClassNames).
		Update("is_active", false).Error
	if err != nil {
		return fmt.Errorf("set inactive classes: %w", err)
	}

	log.Println("[SEED] classes seeded")
	return nil
}

func seedSubjects(db *gorm.DB) error {
	subjectNames := []string{
		"Pancasila", "IPA", "IPS", "BK", "Matematika", "Informatika", "PJOK", "Agama", "Bahasa Indonesia", "Bahasa Inggris", "Seni Budaya",
	}

	var existingNames []string
	err := db.Model(&subjects.Subject{}).
		Where("name IN ?", subjectNames).
		Pluck("name", &existingNames).Error
	if err != nil {
		return fmt.Errorf("load existing subjects: %w", err)
	}

	existingSet := make(map[string]struct{}, len(existingNames))
	for _, name := range existingNames {
		existingSet[name] = struct{}{}
	}

	subjectsToSeed := make([]subjects.Subject, 0, len(subjectNames)-len(existingNames))
	for _, name := range subjectNames {
		if _, found := existingSet[name]; found {
			continue
		}
		subjectsToSeed = append(subjectsToSeed, subjects.Subject{Name: name})
	}

	if len(subjectsToSeed) == 0 {
		log.Println("[SEED] subjects seeded")
		return nil
	}

	if err := db.Create(&subjectsToSeed).Error; err != nil {
		return fmt.Errorf("seed subjects: %w", err)
	}

	log.Println("[SEED] subjects seeded")
	return nil
}

func SeedTeachingAssignments(db *gorm.DB) error {
	teacherMap := make(map[string]uint)
	subjectMap := make(map[string]uint)
	classMap := make(map[string]uint)

	var allTeachers []teachers.Teacher
	var allSubjects []subjects.Subject
	var allClasses []classes.Class

	if err := db.Find(&allTeachers).Error; err != nil {
		return fmt.Errorf("load teachers for assignment seed: %w", err)
	}
	if err := db.Find(&allSubjects).Error; err != nil {
		return fmt.Errorf("load subjects for assignment seed: %w", err)
	}
	if err := db.Find(&allClasses).Error; err != nil {
		return fmt.Errorf("load classes for assignment seed: %w", err)
	}

	for _, t := range allTeachers {
		teacherMap[strconv.Itoa(t.TeacherNumber)] = t.ID
	}
	for _, s := range allSubjects {
		subjectMap[s.Name] = s.ID
	}
	for _, c := range allClasses {
		classMap[c.Name] = c.ID
	}

	type assignment struct {
		TeacherNumber string
		SubjectName   string
		ClassName     string
		JP            int
		GroupKey      string
	}

	assignments := []assignment{

		// Guru 1 – Pancasila
		{"1", "Pancasila", "7D", 3, ""},
		{"1", "Pancasila", "8A", 3, ""},
		{"1", "Pancasila", "8B", 3, ""},
		{"1", "Pancasila", "8C", 3, ""},
		{"1", "Pancasila", "8D", 3, ""},
		{"1", "Pancasila", "8E", 3, ""},
		{"1", "Pancasila", "8F", 3, ""},

		// Guru 2 – IPA
		{"2", "IPA", "8A", 3, ""},
		{"2", "IPA", "8B", 3, ""},
		{"2", "IPA", "8C", 3, ""},
		{"2", "IPA", "8D", 3, ""},
		{"2", "IPA", "8E", 3, ""},
		{"2", "IPA", "8F", 3, ""},

		// Guru 3 – IPS
		{"3", "IPS", "7A", 4, ""},
		{"3", "IPS", "7B", 4, ""},
		{"3", "IPS", "7C", 4, ""},
		{"3", "IPS", "7D", 4, ""},
		{"3", "IPS", "7E", 4, ""},
		{"3", "IPS", "8D", 4, ""},
		{"3", "IPS", "8E", 4, ""},
		{"3", "IPS", "8F", 4, ""},

		// Guru 4 – IPA
		{"4", "IPA", "8A", 3, ""},
		{"4", "IPA", "8B", 3, ""},
		{"4", "IPA", "8C", 3, ""},
		{"4", "IPA", "8D", 3, ""},
		{"4", "IPA", "8E", 3, ""},
		{"4", "IPA", "8F", 3, ""},

		// Guru 5 – BK
		{"5", "BK", "7A", 1, ""},
		{"5", "BK", "7B", 1, ""},
		{"5", "BK", "7C", 1, ""},
		{"5", "BK", "7D", 1, ""},
		{"5", "BK", "7E", 1, ""},
		{"5", "BK", "8A", 1, ""},
		{"5", "BK", "8B", 1, ""},
		{"5", "BK", "8C", 1, ""},

		// Guru 6 – Matematika
		{"6", "Matematika", "9A", 5, ""},
		{"6", "Matematika", "9B", 5, ""},
		{"6", "Matematika", "9C", 5, ""},
		{"6", "Matematika", "9D", 5, ""},
		{"6", "Matematika", "9E", 5, ""},

		// Guru 7 – BK
		{"7", "BK", "8D", 1, ""},
		{"7", "BK", "8E", 1, ""},
		{"7", "BK", "8F", 1, ""},
		{"7", "BK", "9A", 1, ""},
		{"7", "BK", "9B", 1, ""},
		{"7", "BK", "9C", 1, ""},
		{"7", "BK", "9D", 1, ""},
		{"7", "BK", "9E", 1, ""},

		// Guru 8 – Informatika
		{"8", "Informatika", "7A", 3, ""},
		{"8", "Informatika", "7B", 3, ""},
		{"8", "Informatika", "7C", 3, ""},
		{"8", "Informatika", "9A", 3, ""},
		{"8", "Informatika", "9B", 3, ""},
		{"8", "Informatika", "9C", 3, ""},
		{"8", "Informatika", "9D", 3, ""},
		{"8", "Informatika", "9E", 3, ""},

		// Guru 9 – PJOK
		{"9", "PJOK", "8D", 3, ""},
		{"9", "PJOK", "8E", 3, ""},
		{"9", "PJOK", "8F", 3, ""},
		{"9", "PJOK", "9A", 3, ""},
		{"9", "PJOK", "9B", 3, ""},
		{"9", "PJOK", "9C", 3, ""},
		{"9", "PJOK", "9D", 3, ""},
		{"9", "PJOK", "9E", 3, ""},

		// Guru 10 – IPA
		{"10", "IPA", "9A", 5, ""},
		{"10", "IPA", "9B", 5, ""},
		{"10", "IPA", "9C", 5, ""},
		{"10", "IPA", "9D", 5, ""},
		{"10", "IPA", "9E", 5, ""},

		// Guru 11 – Agama
		{"11", "Agama", "8A", 3, ""},
		{"11", "Agama", "8B", 3, ""},
		{"11", "Agama", "8C", 3, ""},
		{"11", "Agama", "9A", 3, ""},
		{"11", "Agama", "9B", 3, ""},
		{"11", "Agama", "9C", 3, ""},
		{"11", "Agama", "9D", 3, ""},
		{"11", "Agama", "9E", 3, ""},

		// Guru 12 – Matematika
		{"12", "Matematika", "7A", 5, ""},
		{"12", "Matematika", "7B", 5, ""},
		{"12", "Matematika", "7C", 5, ""},
		{"12", "Matematika", "7D", 5, ""},
		{"12", "Matematika", "7E", 5, ""},

		// Guru 13 – Agama
		{"13", "Agama", "7A", 3, ""},
		{"13", "Agama", "7B", 3, ""},
		{"13", "Agama", "7C", 3, ""},
		{"13", "Agama", "7D", 3, ""},
		{"13", "Agama", "7E", 3, ""},
		{"13", "Agama", "8D", 3, ""},
		{"13", "Agama", "8E", 3, ""},
		{"13", "Agama", "8F", 3, ""},

		// Guru 14 – Bahasa Indonesia
		{"14", "Bahasa Indonesia", "9A", 6, ""},
		{"14", "Bahasa Indonesia", "9B", 6, ""},
		{"14", "Bahasa Indonesia", "9C", 6, ""},
		{"14", "Bahasa Indonesia", "9D", 6, ""},
		{"14", "Bahasa Indonesia", "9E", 6, ""},

		// Guru 15 – Bahasa Inggris
		{"15", "Bahasa Inggris", "9A", 4, ""},
		{"15", "Bahasa Inggris", "9B", 4, ""},
		{"15", "Bahasa Inggris", "9C", 4, ""},
		{"15", "Bahasa Inggris", "9D", 4, ""},
		{"15", "Bahasa Inggris", "9E", 4, ""},

		// Guru 16 – IPS
		{"16", "IPS", "8A", 4, ""},
		{"16", "IPS", "8B", 4, ""},
		{"16", "IPS", "8C", 4, ""},
		{"16", "IPS", "9A", 4, ""},
		{"16", "IPS", "9B", 4, ""},
		{"16", "IPS", "9C", 4, ""},
		{"16", "IPS", "9D", 4, ""},
		{"16", "IPS", "9E", 4, ""},

		// Guru 19 – Bahasa Inggris
		{"19", "Bahasa Inggris", "7A", 4, ""},
		{"19", "Bahasa Inggris", "7B", 4, ""},
		{"19", "Bahasa Inggris", "7C", 4, ""},
		{"19", "Bahasa Inggris", "7D", 4, ""},
		{"19", "Bahasa Inggris", "7E", 4, ""},

		// Guru 20 – Bahasa Inggris
		{"20", "Bahasa Inggris", "8A", 4, ""},
		{"20", "Bahasa Inggris", "8B", 4, ""},
		{"20", "Bahasa Inggris", "8C", 4, ""},
		{"20", "Bahasa Inggris", "8D", 4, ""},
		{"20", "Bahasa Inggris", "8E", 4, ""},
		{"20", "Bahasa Inggris", "8F", 4, ""},

		// Guru 21 – Bahasa Indonesia
		{"21", "Bahasa Indonesia", "8A", 5, ""},
		{"21", "Bahasa Indonesia", "8B", 5, ""},
		{"21", "Bahasa Indonesia", "8C", 5, ""},
		{"21", "Bahasa Indonesia", "8D", 5, ""},
		{"21", "Bahasa Indonesia", "8E", 5, ""},
		{"21", "Bahasa Indonesia", "8F", 5, ""},

		// Guru 22 – Informatika
		{"22", "Informatika", "7D", 3, ""},
		{"22", "Informatika", "7E", 3, ""},
		{"22", "Informatika", "8A", 3, ""},
		{"22", "Informatika", "8B", 3, ""},
		{"22", "Informatika", "8C", 3, ""},
		{"22", "Informatika", "8D", 3, ""},
		{"22", "Informatika", "8E", 3, ""},
		{"22", "Informatika", "8F", 3, ""},

		// Guru 23 – Pancasila
		{"23", "Pancasila", "7A", 3, ""},
		{"23", "Pancasila", "7B", 3, ""},
		{"23", "Pancasila", "7C", 3, ""},
		{"23", "Pancasila", "7E", 3, ""},
		{"23", "Pancasila", "9A", 3, ""},
		{"23", "Pancasila", "9B", 3, ""},
		{"23", "Pancasila", "9C", 3, ""},
		{"23", "Pancasila", "9D", 3, ""},
		{"23", "Pancasila", "9E", 3, ""},

		// Guru 24 – Bahasa Indonesia
		{"24", "Bahasa Indonesia", "7A", 6, ""},
		{"24", "Bahasa Indonesia", "7B", 6, ""},
		{"24", "Bahasa Indonesia", "7C", 6, ""},
		{"24", "Bahasa Indonesia", "7D", 6, ""},
		{"24", "Bahasa Indonesia", "7E", 6, ""},

		// Guru 26 – IPA
		{"26", "IPA", "7A", 5, ""},
		{"26", "IPA", "7B", 5, ""},
		{"26", "IPA", "7C", 5, ""},
		{"26", "IPA", "7D", 5, ""},
		{"26", "IPA", "7E", 5, ""},

		// Guru 27 – Matematika
		{"27", "Matematika", "8A", 5, ""},
		{"27", "Matematika", "8B", 5, ""},
		{"27", "Matematika", "8C", 5, ""},
		{"27", "Matematika", "8D", 5, ""},
		{"27", "Matematika", "8E", 5, ""},
		{"27", "Matematika", "8F", 5, ""},

		// Guru 28 – PJOK
		{"28", "PJOK", "7A", 3, ""},
		{"28", "PJOK", "7B", 3, ""},
		{"28", "PJOK", "7C", 3, ""},
		{"28", "PJOK", "7D", 3, ""},
		{"28", "PJOK", "7E", 3, ""},
		{"28", "PJOK", "8A", 3, ""},
		{"28", "PJOK", "8B", 3, ""},
		{"28", "PJOK", "8C", 3, ""},
	}
	// Penugasan SBP dibuat otomatis, tidak perlu diisi manual.

	var existingAssignments []teachingassignments.TeachingAssignment
	if err := db.Find(&existingAssignments).Error; err != nil {
		return fmt.Errorf("load existing assignments: %w", err)
	}

	existingSet := make(map[string]struct{}, len(existingAssignments))
	for _, ea := range existingAssignments {
		existingSet[buildAssignmentKey(ea.TeacherID, ea.SubjectID, ea.ClassID, ea.GroupKey)] = struct{}{}
	}

	assignmentsToCreate := make([]teachingassignments.TeachingAssignment, 0, len(assignments))
	for _, a := range assignments {
		subjectName := normalizeSubjectName(a.SubjectName)

		subjectID, ok := subjectMap[subjectName]
		if !ok {
			return fmt.Errorf("subject not found for assignment: %s", subjectName)
		}

		classID, ok := classMap[a.ClassName]
		if !ok {
			return fmt.Errorf("class not found for assignment: %s", a.ClassName)
		}

		var teacherID *uint
		if a.TeacherNumber != "" {
			id, ok := teacherMap[a.TeacherNumber]
			if !ok {
				return fmt.Errorf("teacher not found for assignment: %s", a.TeacherNumber)
			}
			teacherID = &id
		}

		var groupKey *string
		if a.GroupKey != "" {
			gk := a.GroupKey
			groupKey = &gk
		}

		key := buildAssignmentKey(teacherID, subjectID, classID, groupKey)
		if _, exists := existingSet[key]; exists {
			continue
		}

		assignmentsToCreate = append(assignmentsToCreate, teachingassignments.TeachingAssignment{
			TeacherID: teacherID,
			SubjectID: subjectID,
			ClassID:   classID,
			JP:        a.JP,
			GroupKey:  groupKey,
			CreatedBy: "SYSTEM",
			UpdatedBy: "SYSTEM",
		})
		existingSet[key] = struct{}{}
	}

	if len(assignmentsToCreate) > 0 {
		if err := db.CreateInBatches(assignmentsToCreate, 200).Error; err != nil {
			return fmt.Errorf("seed teaching assignments: %w", err)
		}
	}

	// Buat penugasan SBP otomatis per tingkat (maks 3 kelas/grup).
	if err := seedSBPAssignments(db, existingSet, subjectMap, classMap); err != nil {
		return fmt.Errorf("seed SBP assignments: %w", err)
	}

	log.Println("[SEED] teaching assignments seeded")
	return nil
}

func seedSBPAssignments(db *gorm.DB, existingSet map[string]struct{}, subjectMap map[string]uint, classMap map[string]uint) error {
	sbpSubjectID, ok := subjectMap["Seni Budaya"]
	if !ok {
		return nil // mata pelajaran belum ada, lewati
	}

	var activeClasses []classes.Class
	if err := db.Where("is_active = true").Order("grade, code").Find(&activeClasses).Error; err != nil {
		return err
	}

	gradeGroup := make(map[int][]classes.Class)
	for _, cls := range activeClasses {
		gradeGroup[cls.Grade] = append(gradeGroup[cls.Grade], cls)
	}

	var sbpToCreate []teachingassignments.TeachingAssignment
	for _, grade := range []int{7, 8, 9} {
		gradeClasses := gradeGroup[grade]
		for i := 0; i < len(gradeClasses); i += 3 {
			group := gradeClasses[i:min(i+3, len(gradeClasses))]
			codes := ""
			for _, cls := range group {
				codes += cls.Code
			}
			gk := fmt.Sprintf("SBP-%d-%s", grade, codes)
			for _, cls := range group {
				key := buildAssignmentKey(nil, sbpSubjectID, cls.ID, &gk)
				if _, exists := existingSet[key]; exists {
					continue
				}
				existingSet[key] = struct{}{}
				gkCopy := gk
				sbpToCreate = append(sbpToCreate, teachingassignments.TeachingAssignment{
					TeacherID: nil,
					SubjectID: sbpSubjectID,
					ClassID:   cls.ID,
					JP:        3,
					GroupKey:  &gkCopy,
					CreatedBy: "SYSTEM",
					UpdatedBy: "SYSTEM",
				})
			}
		}
	}
	_ = classMap

	if len(sbpToCreate) == 0 {
		return nil
	}
	return db.CreateInBatches(sbpToCreate, 200).Error
}

func buildAssignmentKey(teacherID *uint, subjectID uint, classID uint, groupKey *string) string {
	teacherPart := "nil"
	if teacherID != nil {
		teacherPart = strconv.FormatUint(uint64(*teacherID), 10)
	}

	groupPart := ""
	if groupKey != nil {
		groupPart = *groupKey
	}

	return teacherPart + "|" + strconv.FormatUint(uint64(subjectID), 10) + "|" + strconv.FormatUint(uint64(classID), 10) + "|" + groupPart
}

func normalizeSubjectName(name string) string {
	if name == "SBP" {
		return "Seni Budaya"
	}

	return name
}
