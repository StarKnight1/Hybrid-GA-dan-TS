package admin

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"smp_mater_dei_be/internal/classes"
	"smp_mater_dei_be/internal/platform/config"
	"smp_mater_dei_be/internal/subjects"
	"smp_mater_dei_be/internal/teachers"
	teachingassignments "smp_mater_dei_be/internal/teaching_assignments"
	"smp_mater_dei_be/internal/transport/http/response"

	"github.com/gin-gonic/gin"
	"github.com/xuri/excelize/v2"
	"gorm.io/gorm/clause"
)

// DownloadTemplateHandler serves a pre-filled Excel template for data input.
func DownloadTemplateHandler(c *gin.Context) {
	f := buildTemplate()

	buf, err := f.WriteToBuffer()
	if err != nil {
		response.Fail(c, http.StatusInternalServerError, "failed to generate template", err.Error())
		return
	}

	c.Header("Content-Disposition", "attachment; filename=template_data_jadwal.xlsx")
	c.Header("Content-Type", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet")
	c.Data(http.StatusOK, "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet", buf.Bytes())
}

func buildTemplate() *excelize.File {
	f := excelize.NewFile()

	// ── Sheet: Petunjuk ──────────────────────────────────────────────────────
	instrSheet := "Petunjuk"
	f.SetSheetName("Sheet1", instrSheet)
	instrs := []string{
		"PETUNJUK PENGISIAN DATA JADWAL",
		"",
		"1. Sheet 'Guru'   : Isi data guru (nomor, nama, jenis kelamin).",
		"2. Sheet 'Kelas'  : Isi nama kelas dan status aktif.",
		"3. Sheet 'Penugasan' : Isi penugasan mengajar per kelas.",
		"",
		"Catatan:",
		"- Kolom 'No' diisi otomatis, tidak perlu diisi.",
		"- Jenis Kelamin: L (Laki-laki) atau P (Perempuan).",
		"- Aktif (Kelas): Ya atau Tidak.",
		"- Nomor Guru pada Penugasan harus sesuai dengan sheet Guru.",
		"- Untuk mata pelajaran SBP (Seni Budaya Paralel), kosongkan Nomor Guru",
		"  dan isi Group Key (contoh: SBP-7-ABC).",
		"- Group Key diisi hanya untuk kelas yang dijadwalkan bersamaan (paralel).",
	}
	for i, line := range instrs {
		f.SetCellValue(instrSheet, cellName(1, i+1), line)
	}

	// ── Sheet: Guru ───────────────────────────────────────────────────────────
	guruSheet := "Guru"
	f.NewSheet(guruSheet)
	guruHeaders := []string{"No", "Nomor Guru", "Nama Guru", "Jenis Kelamin (L/P)"}
	for col, h := range guruHeaders {
		f.SetCellValue(guruSheet, cellName(col+1, 1), h)
	}
	// sample rows
	samples := [][]interface{}{
		{1, 1, "Margareta Kamsiati, S.Pd", "P"},
		{2, 2, "Drs. Antonius Sarjiyono", "L"},
	}
	for i, row := range samples {
		for col, val := range row {
			f.SetCellValue(guruSheet, cellName(col+1, i+2), val)
		}
	}

	// ── Sheet: Kelas ──────────────────────────────────────────────────────────
	kelasSheet := "Kelas"
	f.NewSheet(kelasSheet)
	kelasHeaders := []string{"No", "Nama Kelas", "Aktif (Ya/Tidak)"}
	for col, h := range kelasHeaders {
		f.SetCellValue(kelasSheet, cellName(col+1, 1), h)
	}
	kelasData := [][]interface{}{
		{1, "7A", "Ya"}, {2, "7B", "Ya"}, {3, "7C", "Ya"},
		{4, "7D", "Ya"}, {5, "7E", "Ya"}, {6, "7F", "Tidak"},
		{7, "8A", "Ya"}, {8, "8B", "Ya"}, {9, "8C", "Ya"},
		{10, "8D", "Ya"}, {11, "8E", "Ya"}, {12, "8F", "Ya"},
		{13, "9A", "Ya"}, {14, "9B", "Ya"}, {15, "9C", "Ya"},
		{16, "9D", "Ya"}, {17, "9E", "Ya"}, {18, "9F", "Tidak"},
	}
	for i, row := range kelasData {
		for col, val := range row {
			f.SetCellValue(kelasSheet, cellName(col+1, i+2), val)
		}
	}

	// ── Sheet: Penugasan ──────────────────────────────────────────────────────
	tugasSheet := "Penugasan"
	f.NewSheet(tugasSheet)
	tugasHeaders := []string{"No", "Nomor Guru", "Mata Pelajaran", "Nama Kelas", "JP Per Minggu", "Group Key (Opsional)"}
	for col, h := range tugasHeaders {
		f.SetCellValue(tugasSheet, cellName(col+1, 1), h)
	}
	tugasData := [][]interface{}{
		{1, 1, "Pancasila", "7D", 3, ""},
		{2, 1, "Pancasila", "8A", 3, ""},
		{3, 2, "IPA", "8A", 3, ""},
		{4, "", "Seni Budaya", "7A", 3, "SBP-7-ABC"},
		{5, "", "Seni Budaya", "7B", 3, "SBP-7-ABC"},
		{6, "", "Seni Budaya", "7C", 3, "SBP-7-ABC"},
	}
	for i, row := range tugasData {
		for col, val := range row {
			f.SetCellValue(tugasSheet, cellName(col+1, i+2), val)
		}
	}

	idx, _ := f.GetSheetIndex(instrSheet)
	f.SetActiveSheet(idx)
	return f
}

// UploadDataHandler parses the uploaded Excel file and replaces teaching data in the DB.
func UploadDataHandler(c *gin.Context) {
	file, err := c.FormFile("file")
	if err != nil {
		response.Fail(c, http.StatusBadRequest, "file required", err.Error())
		return
	}

	src, err := file.Open()
	if err != nil {
		response.Fail(c, http.StatusInternalServerError, "cannot open file", err.Error())
		return
	}
	defer src.Close()

	f, err := excelize.OpenReader(src)
	if err != nil {
		response.Fail(c, http.StatusBadRequest, "invalid excel file", err.Error())
		return
	}

	// ── Parse Guru ───────────────────────────────────────────────────────────
	guruRows, err := f.GetRows("Guru")
	if err != nil || len(guruRows) < 2 {
		response.Fail(c, http.StatusBadRequest, "sheet 'Guru' tidak ditemukan atau kosong", nil)
		return
	}

	defaultBirth := time.Date(1980, 1, 1, 0, 0, 0, 0, time.UTC)
	teacherList := make([]teachers.Teacher, 0)
	for _, row := range guruRows[1:] {
		if len(row) < 4 || strings.TrimSpace(row[1]) == "" {
			continue
		}
		num, err := strconv.Atoi(strings.TrimSpace(row[1]))
		if err != nil {
			continue
		}
		gender := "male"
		if strings.ToUpper(strings.TrimSpace(row[3])) == "P" {
			gender = "female"
		}
		teacherList = append(teacherList, teachers.Teacher{
			TeacherNumber: num,
			FullName:      strings.TrimSpace(row[2]),
			Gender:        gender,
			BirthPlace:    "Yogyakarta",
			BirthDate:     defaultBirth,
			Address:       "-",
			Phone:         "-",
			Religion:      "-",
		})
	}

	// ── Parse Kelas ───────────────────────────────────────────────────────────
	kelasRows, err := f.GetRows("Kelas")
	if err != nil || len(kelasRows) < 2 {
		response.Fail(c, http.StatusBadRequest, "sheet 'Kelas' tidak ditemukan atau kosong", nil)
		return
	}

	type classRow struct {
		name   string
		active bool
	}
	classList := make([]classRow, 0)
	for _, row := range kelasRows[1:] {
		if len(row) < 2 || strings.TrimSpace(row[1]) == "" {
			continue
		}
		active := true
		if len(row) >= 3 && strings.ToUpper(strings.TrimSpace(row[2])) == "TIDAK" {
			active = false
		}
		classList = append(classList, classRow{strings.TrimSpace(row[1]), active})
	}

	// ── Parse Penugasan ───────────────────────────────────────────────────────
	tugasRows, err := f.GetRows("Penugasan")
	if err != nil || len(tugasRows) < 2 {
		response.Fail(c, http.StatusBadRequest, "sheet 'Penugasan' tidak ditemukan atau kosong", nil)
		return
	}

	type assignRow struct {
		teacherNum string
		subject    string
		class      string
		jp         int
		groupKey   string
	}
	assignList := make([]assignRow, 0)
	subjectSet := make(map[string]struct{})
	for _, row := range tugasRows[1:] {
		if len(row) < 5 || strings.TrimSpace(row[2]) == "" || strings.TrimSpace(row[3]) == "" {
			continue
		}
		jp, _ := strconv.Atoi(strings.TrimSpace(row[4]))
		if jp <= 0 {
			continue
		}
		groupKey := ""
		if len(row) >= 6 {
			groupKey = strings.TrimSpace(row[5])
		}
		subjectName := strings.TrimSpace(row[2])
		subjectSet[subjectName] = struct{}{}
		assignList = append(assignList, assignRow{
			teacherNum: strings.TrimSpace(row[1]),
			subject:    subjectName,
			class:      strings.TrimSpace(row[3]),
			jp:         jp,
			groupKey:   groupKey,
		})
	}

	// ── Persist to DB ─────────────────────────────────────────────────────────
	db := config.DB

	// Upsert teachers
	if len(teacherList) > 0 {
		if err := db.Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "teacher_number"}},
			DoUpdates: clause.AssignmentColumns([]string{"full_name", "gender", "updated_at"}),
		}).Create(&teacherList).Error; err != nil {
			response.Fail(c, http.StatusInternalServerError, "failed to upsert teachers", err.Error())
			return
		}
	}

	// Upsert classes
	for _, cr := range classList {
		name := cr.name
		grade := 0
		code := ""
		if len(name) >= 2 {
			g, err := strconv.Atoi(string(name[0]))
			if err == nil {
				grade = g
				code = name[1:]
			}
		}
		cl := classes.Class{Grade: grade, Code: code, Name: name}
		res := db.Where("name = ?", name).FirstOrCreate(&cl)
		if res.Error != nil {
			response.Fail(c, http.StatusInternalServerError, fmt.Sprintf("failed to upsert class %s", name), res.Error.Error())
			return
		}
		db.Model(&cl).Update("is_active", cr.active)
	}

	// Upsert subjects
	for name := range subjectSet {
		sub := subjects.Subject{Name: name}
		if err := db.Where("name = ?", name).FirstOrCreate(&sub).Error; err != nil {
			response.Fail(c, http.StatusInternalServerError, fmt.Sprintf("failed to upsert subject %s", name), err.Error())
			return
		}
	}

	// Build lookup maps
	teacherMap := make(map[string]uint)
	var allTeachers []teachers.Teacher
	db.Find(&allTeachers)
	for _, t := range allTeachers {
		teacherMap[strconv.Itoa(t.TeacherNumber)] = t.ID
	}

	subjectMap := make(map[string]uint)
	var allSubjects []subjects.Subject
	db.Find(&allSubjects)
	for _, s := range allSubjects {
		subjectMap[s.Name] = s.ID
	}

	classMap := make(map[string]uint)
	var allClasses []classes.Class
	db.Find(&allClasses)
	for _, cl := range allClasses {
		classMap[cl.Name] = cl.ID
	}

	// Clear and re-insert teaching assignments
	if err := db.Where("1 = 1").Delete(&teachingassignments.TeachingAssignment{}).Error; err != nil {
		response.Fail(c, http.StatusInternalServerError, "failed to clear teaching assignments", err.Error())
		return
	}

	newAssignments := make([]teachingassignments.TeachingAssignment, 0, len(assignList))
	for _, a := range assignList {
		subjectID, ok := subjectMap[a.subject]
		if !ok {
			continue
		}
		classID, ok := classMap[a.class]
		if !ok {
			continue
		}

		var teacherID *uint
		if a.teacherNum != "" {
			if id, ok := teacherMap[a.teacherNum]; ok {
				teacherID = &id
			}
		}

		var groupKey *string
		if a.groupKey != "" {
			gk := a.groupKey
			groupKey = &gk
		}

		newAssignments = append(newAssignments, teachingassignments.TeachingAssignment{
			TeacherID: teacherID,
			SubjectID: subjectID,
			ClassID:   classID,
			JP:        a.jp,
			GroupKey:  groupKey,
			CreatedBy: "UPLOAD",
			UpdatedBy: "UPLOAD",
		})
	}

	if len(newAssignments) > 0 {
		if err := db.CreateInBatches(newAssignments, 200).Error; err != nil {
			response.Fail(c, http.StatusInternalServerError, "failed to insert teaching assignments", err.Error())
			return
		}
	}

	response.OK(c, gin.H{
		"teachers":    len(teacherList),
		"classes":     len(classList),
		"subjects":    len(subjectSet),
		"assignments": len(newAssignments),
	}, "data uploaded successfully")
}

// DataStatusHandler returns counts of current DB data.
func DataStatusHandler(c *gin.Context) {
	db := config.DB

	var teacherCount, classCount, subjectCount, assignCount int64
	db.Model(&teachers.Teacher{}).Count(&teacherCount)
	db.Model(&classes.Class{}).Where("is_active = true").Count(&classCount)
	db.Model(&subjects.Subject{}).Count(&subjectCount)
	db.Model(&teachingassignments.TeachingAssignment{}).Count(&assignCount)

	response.OK(c, gin.H{
		"teachers":           teacherCount,
		"activeClasses":      classCount,
		"subjects":           subjectCount,
		"teachingAssignments": assignCount,
	}, "ok")
}

func cellName(col, row int) string {
	name, _ := excelize.CoordinatesToCellName(col, row)
	return name
}
