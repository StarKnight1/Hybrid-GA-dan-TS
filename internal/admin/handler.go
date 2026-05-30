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

	// ── Common styles ─────────────────────────────────────────────────────────
	thinBorder := []excelize.Border{
		{Type: "left", Color: "CCCCCC", Style: 1},
		{Type: "right", Color: "CCCCCC", Style: 1},
		{Type: "top", Color: "CCCCCC", Style: 1},
		{Type: "bottom", Color: "CCCCCC", Style: 1},
	}
	hdrStyle, _ := f.NewStyle(&excelize.Style{
		Font: &excelize.Font{Bold: true, Color: "FFFFFF", Size: 11},
		Fill: excelize.Fill{Type: "pattern", Color: []string{"1F538D"}, Pattern: 1},
		Alignment: &excelize.Alignment{Horizontal: "center", Vertical: "center", WrapText: true},
		Border: []excelize.Border{
			{Type: "left", Color: "1F538D", Style: 1},
			{Type: "right", Color: "1F538D", Style: 1},
			{Type: "top", Color: "1F538D", Style: 1},
			{Type: "bottom", Color: "1F538D", Style: 1},
		},
	})
	dataStyle, _ := f.NewStyle(&excelize.Style{
		Border:    thinBorder,
		Alignment: &excelize.Alignment{Vertical: "center"},
	})
	altStyle, _ := f.NewStyle(&excelize.Style{
		Fill:      excelize.Fill{Type: "pattern", Color: []string{"EEF4FF"}, Pattern: 1},
		Border:    thinBorder,
		Alignment: &excelize.Alignment{Vertical: "center"},
	})
	noteStyle, _ := f.NewStyle(&excelize.Style{
		Font: &excelize.Font{Bold: true, Color: "1F538D", Size: 14},
	})
	bodyStyle, _ := f.NewStyle(&excelize.Style{
		Font:      &excelize.Font{Size: 11},
		Alignment: &excelize.Alignment{WrapText: true},
	})
	warnStyle, _ := f.NewStyle(&excelize.Style{
		Font: &excelize.Font{Italic: true, Color: "FF8C00", Size: 10},
	})

	applyTable := func(sheet string, hdrEnd string, dataRows int, cols int) {
		f.SetCellStyle(sheet, "A1", hdrEnd, hdrStyle)
		f.SetRowHeight(sheet, 1, 28)
		for r := 2; r <= dataRows+1; r++ {
			last, _ := excelize.CoordinatesToCellName(cols, r)
			if r%2 == 0 {
				f.SetCellStyle(sheet, cellName(1, r), last, altStyle)
			} else {
				f.SetCellStyle(sheet, cellName(1, r), last, dataStyle)
			}
			f.SetRowHeight(sheet, r, 20)
		}
	}

	// ── Sheet: Petunjuk ──────────────────────────────────────────────────────
	instrSheet := "Petunjuk"
	f.SetSheetName("Sheet1", instrSheet)
	f.SetColWidth(instrSheet, "A", "A", 70)

	f.SetCellValue(instrSheet, "A1", "PETUNJUK PENGISIAN DATA JADWAL — SMP Mater Dei")
	f.SetCellStyle(instrSheet, "A1", "A1", noteStyle)
	f.SetRowHeight(instrSheet, 1, 28)

	lines := []struct {
		row  int
		text string
		warn bool
	}{
		{3, "LANGKAH PENGISIAN:", false},
		{4, "1. Sheet \"Guru\"      : Isi data setiap guru (nomor unik, nama lengkap, jenis kelamin).", false},
		{5, "2. Sheet \"Kelas\"     : Isi nama kelas dan status aktif (Ya/Tidak).", false},
		{6, "3. Sheet \"Penugasan\": Isi penugasan mengajar — satu baris = satu guru × satu kelas × satu mata pelajaran.", false},
		{7, "4. Upload file ini melalui tombol \"Upload Data Excel\" di halaman Dashboard.", false},
		{9, "ATURAN PENTING:", false},
		{10, "• Kolom \"No\" bisa dibiarkan kosong — sistem mengisi otomatis.", false},
		{11, "• Jenis Kelamin diisi: L (Laki-laki) atau P (Perempuan).", false},
		{12, "• Aktif (Kelas) diisi: Ya atau Tidak. Kelas \"Tidak\" tidak akan dijadwalkan.", false},
		{13, "• Nomor Guru pada sheet Penugasan HARUS sesuai dengan sheet Guru.", false},
		{14, "• JP Per Minggu = jumlah jam pelajaran per minggu (misal: 3).", false},
		{16, "MATA PELAJARAN PARALEL (SBP / Seni Budaya Paralel):", false},
		{17, "• Kosongkan kolom Nomor Guru untuk kelas SBP.", false},
		{18, "• Isi Group Key yang sama untuk semua kelas yang dijadwalkan paralel.", false},
		{19, "• Contoh Group Key: \"SBP-7-ABC\" untuk kelas 7A, 7B, 7C.", false},
		{20, "⚠  Jangan mengubah nama sheet atau menghapus baris header!", true},
	}
	for _, l := range lines {
		f.SetCellValue(instrSheet, cellName(1, l.row), l.text)
		if l.warn {
			f.SetCellStyle(instrSheet, cellName(1, l.row), cellName(1, l.row), warnStyle)
		} else if l.row == 3 || l.row == 9 || l.row == 16 {
			f.SetCellStyle(instrSheet, cellName(1, l.row), cellName(1, l.row), hdrStyle)
			f.SetRowHeight(instrSheet, l.row, 22)
		} else {
			f.SetCellStyle(instrSheet, cellName(1, l.row), cellName(1, l.row), bodyStyle)
		}
	}
	_ = bodyStyle

	// ── Sheet: Guru ───────────────────────────────────────────────────────────
	guruSheet := "Guru"
	f.NewSheet(guruSheet)
	f.SetColWidth(guruSheet, "A", "A", 6)
	f.SetColWidth(guruSheet, "B", "B", 14)
	f.SetColWidth(guruSheet, "C", "C", 38)
	f.SetColWidth(guruSheet, "D", "D", 20)

	guruHeaders := []string{"No", "Nomor Guru", "Nama Guru", "Jenis Kelamin (L/P)"}
	for col, h := range guruHeaders {
		f.SetCellValue(guruSheet, cellName(col+1, 1), h)
	}
	samples := [][]interface{}{
		{1, 1, "Margareta Kamsiati, S.Pd", "P"},
		{2, 2, "Drs. Antonius Sarjiyono", "L"},
		{3, 3, "Agustinus Tukiman, S.Pd", "L"},
	}
	for i, row := range samples {
		for col, val := range row {
			f.SetCellValue(guruSheet, cellName(col+1, i+2), val)
		}
	}
	applyTable(guruSheet, "D1", len(samples), 4)

	// ── Sheet: Kelas ──────────────────────────────────────────────────────────
	kelasSheet := "Kelas"
	f.NewSheet(kelasSheet)
	f.SetColWidth(kelasSheet, "A", "A", 6)
	f.SetColWidth(kelasSheet, "B", "B", 18)
	f.SetColWidth(kelasSheet, "C", "C", 18)

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
	applyTable(kelasSheet, "C1", len(kelasData), 3)

	// ── Sheet: Penugasan ──────────────────────────────────────────────────────
	tugasSheet := "Penugasan"
	f.NewSheet(tugasSheet)
	f.SetColWidth(tugasSheet, "A", "A", 6)
	f.SetColWidth(tugasSheet, "B", "B", 14)
	f.SetColWidth(tugasSheet, "C", "C", 28)
	f.SetColWidth(tugasSheet, "D", "D", 14)
	f.SetColWidth(tugasSheet, "E", "E", 16)
	f.SetColWidth(tugasSheet, "F", "F", 26)

	tugasHeaders := []string{"No", "Nomor Guru", "Mata Pelajaran", "Nama Kelas", "JP Per Minggu", "Group Key (Opsional)"}
	for col, h := range tugasHeaders {
		f.SetCellValue(tugasSheet, cellName(col+1, 1), h)
	}
	tugasData := [][]interface{}{
		{1, 1, "Pancasila", "7D", 3, ""},
		{2, 1, "Pancasila", "8A", 3, ""},
		{3, 2, "IPA", "8A", 3, ""},
		{4, 3, "Matematika", "7A", 4, ""},
		{5, "", "Seni Budaya", "7A", 3, "SBP-7-ABC"},
		{6, "", "Seni Budaya", "7B", 3, "SBP-7-ABC"},
		{7, "", "Seni Budaya", "7C", 3, "SBP-7-ABC"},
	}
	for i, row := range tugasData {
		for col, val := range row {
			f.SetCellValue(tugasSheet, cellName(col+1, i+2), val)
		}
	}
	applyTable(tugasSheet, "F1", len(tugasData), 6)

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
