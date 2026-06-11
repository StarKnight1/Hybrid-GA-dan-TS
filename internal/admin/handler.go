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

// DownloadTemplateHandler mengirim template Excel yang sudah terisi contoh data.
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

	// Gaya umum
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
		Alignment: &excelize.Alignment{Vertical: "center", WrapText: true},
	})
	altStyle, _ := f.NewStyle(&excelize.Style{
		Fill:      excelize.Fill{Type: "pattern", Color: []string{"EEF4FF"}, Pattern: 1},
		Border:    thinBorder,
		Alignment: &excelize.Alignment{Vertical: "center", WrapText: true},
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
	sbpYaStyle, _ := f.NewStyle(&excelize.Style{
		Font:      &excelize.Font{Bold: true, Color: "1A6B1A", Size: 10},
		Fill:      excelize.Fill{Type: "pattern", Color: []string{"E8F5E9"}, Pattern: 1},
		Border:    thinBorder,
		Alignment: &excelize.Alignment{Horizontal: "center", Vertical: "center"},
	})
	sbpTidakStyle, _ := f.NewStyle(&excelize.Style{
		Font:      &excelize.Font{Color: "888888", Size: 10},
		Fill:      excelize.Fill{Type: "pattern", Color: []string{"F5F5F5"}, Pattern: 1},
		Border:    thinBorder,
		Alignment: &excelize.Alignment{Horizontal: "center", Vertical: "center"},
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

	// Sheet: Petunjuk
	instrSheet := "Petunjuk"
	f.SetSheetName("Sheet1", instrSheet)
	f.SetColWidth(instrSheet, "A", "A", 80)

	f.SetCellValue(instrSheet, "A1", "PETUNJUK PENGISIAN DATA JADWAL — SMP Mater Dei")
	f.SetCellStyle(instrSheet, "A1", "A1", noteStyle)
	f.SetRowHeight(instrSheet, 1, 28)

	lines := []struct {
		row  int
		text string
		sect bool
		warn bool
	}{
		{3, "LANGKAH PENGISIAN:", true, false},
		{4, "1. Sheet \"Guru\"      : Isi data setiap guru (nomor unik, nama lengkap, jenis kelamin).", false, false},
		{5, "2. Sheet \"Kelas\"     : Isi nama kelas, status aktif, dan kolom \"Ada SBP\".", false, false},
		{6, "3. Sheet \"Penugasan\": Isi penugasan mengajar. Satu baris = satu guru, satu mata pelajaran, semua kelas yang diajar (pisahkan koma).", false, false},
		{7, "4. Upload file melalui tombol \"Upload Data Excel\" di halaman Dashboard.", false, false},
		{9, "ATURAN PENTING:", true, false},
		{10, "• Kolom \"No\" boleh kosong — diisi otomatis.", false, false},
		{11, "• Jenis Kelamin: L (Laki-laki) atau P (Perempuan).", false, false},
		{12, "• Aktif (Kelas): Ya atau Tidak. Kelas Tidak tidak akan dijadwalkan.", false, false},
		{13, "• Ada SBP (Kelas): Ya = kelas ikut SBP, Tidak = kelas tidak mendapat SBP.", false, false},
		{14, "• Nomor Guru pada Penugasan HARUS sesuai nomor di sheet Guru.", false, false},
		{15, "• JP Per Minggu = jumlah JP per minggu untuk SETIAP kelas yang diajar.", false, false},
		{16, "• Kolom \"Kelas-kelas\" diisi nama kelas dipisah koma. Contoh: 7A,7B,7C,8A,8B", false, false},
		{18, "SBP (Seni Budaya Paralel) — OTOMATIS:", true, false},
		{19, "• SBP TIDAK perlu diisi di sheet Penugasan. Sistem menangani SBP secara otomatis.", false, false},
		{20, "• Kelas yang kolom \"Ada SBP\" = Ya akan otomatis mendapat 3 JP SBP per minggu.", false, false},
		{21, "• Kelas dikelompokkan per tingkat (max 3 kelas/grup) dan dijadwalkan paralel (GroupKey otomatis).", false, false},
		{22, "• Contoh: 7A, 7B, 7C dalam satu grup → jadwal SBP ketiga kelas di slot waktu yang sama.", false, false},
		{24, "⚠  Jangan mengubah nama sheet atau menghapus baris header!", false, true},
	}
	for _, l := range lines {
		f.SetCellValue(instrSheet, cellName(1, l.row), l.text)
		switch {
		case l.warn:
			f.SetCellStyle(instrSheet, cellName(1, l.row), cellName(1, l.row), warnStyle)
		case l.sect:
			f.SetCellStyle(instrSheet, cellName(1, l.row), cellName(1, l.row), hdrStyle)
			f.SetRowHeight(instrSheet, l.row, 22)
		default:
			f.SetCellStyle(instrSheet, cellName(1, l.row), cellName(1, l.row), bodyStyle)
		}
	}

	// Sheet: Guru
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
	guruSamples := [][]interface{}{
		{1, 1, "Margareta Kamsiati, S.Pd", "P"},
		{2, 2, "Drs. Antonius Sarjiyono", "L"},
		{3, 3, "Agustinus Tukiman, S.Pd", "L"},
	}
	for i, row := range guruSamples {
		for col, val := range row {
			f.SetCellValue(guruSheet, cellName(col+1, i+2), val)
		}
	}
	applyTable(guruSheet, "D1", len(guruSamples), 4)

	// Sheet: Kelas
	kelasSheet := "Kelas"
	f.NewSheet(kelasSheet)
	f.SetColWidth(kelasSheet, "A", "A", 6)
	f.SetColWidth(kelasSheet, "B", "B", 16)
	f.SetColWidth(kelasSheet, "C", "C", 18)
	f.SetColWidth(kelasSheet, "D", "D", 20)

	kelasHeaders := []string{"No", "Nama Kelas", "Aktif (Ya/Tidak)", "Ada SBP (Ya/Tidak)"}
	for col, h := range kelasHeaders {
		f.SetCellValue(kelasSheet, cellName(col+1, 1), h)
	}
	kelasData := [][]interface{}{
		{1, "7A", "Ya", "Ya"}, {2, "7B", "Ya", "Ya"}, {3, "7C", "Ya", "Ya"},
		{4, "7D", "Ya", "Ya"}, {5, "7E", "Ya", "Ya"}, {6, "7F", "Tidak", "Tidak"},
		{7, "8A", "Ya", "Ya"}, {8, "8B", "Ya", "Ya"}, {9, "8C", "Ya", "Ya"},
		{10, "8D", "Ya", "Ya"}, {11, "8E", "Ya", "Ya"}, {12, "8F", "Ya", "Ya"},
		{13, "9A", "Ya", "Ya"}, {14, "9B", "Ya", "Ya"}, {15, "9C", "Ya", "Ya"},
		{16, "9D", "Ya", "Ya"}, {17, "9E", "Ya", "Ya"}, {18, "9F", "Tidak", "Tidak"},
	}
	for i, row := range kelasData {
		r := i + 2
		for col, val := range row {
			f.SetCellValue(kelasSheet, cellName(col+1, r), val)
		}
		last := cellName(4, r)
		if i%2 == 0 {
			f.SetCellStyle(kelasSheet, cellName(1, r), last, altStyle)
		} else {
			f.SetCellStyle(kelasSheet, cellName(1, r), last, dataStyle)
		}
		// Warna kolom SBP sesuai nilai
		sbpVal := fmt.Sprintf("%v", row[3])
		if strings.ToUpper(sbpVal) == "YA" {
			f.SetCellStyle(kelasSheet, cellName(4, r), cellName(4, r), sbpYaStyle)
		} else {
			f.SetCellStyle(kelasSheet, cellName(4, r), cellName(4, r), sbpTidakStyle)
		}
		f.SetRowHeight(kelasSheet, r, 20)
	}
	f.SetCellStyle(kelasSheet, "A1", "D1", hdrStyle)
	f.SetRowHeight(kelasSheet, 1, 28)

	// Sheet: Penugasan
	tugasSheet := "Penugasan"
	f.NewSheet(tugasSheet)
	f.SetColWidth(tugasSheet, "A", "A", 5)
	f.SetColWidth(tugasSheet, "B", "B", 13)
	f.SetColWidth(tugasSheet, "C", "C", 28)
	f.SetColWidth(tugasSheet, "D", "D", 46)
	f.SetColWidth(tugasSheet, "E", "E", 14)
	f.SetColWidth(tugasSheet, "F", "F", 22)

	tugasHeaders := []string{
		"No", "Nomor Guru", "Mata Pelajaran",
		"Kelas-kelas (pisah koma)", "JP Per Minggu", "Group Key (Opsional)",
	}
	for col, h := range tugasHeaders {
		f.SetCellValue(tugasSheet, cellName(col+1, 1), h)
	}
	applyTable(tugasSheet, "F1", 0, 6)

	tugasRows := [][]interface{}{
		{1, 1, "Pancasila", "7D,8A,8B,8C,8D,8E,8F", 3, ""},
		{2, 2, "IPA", "8A,8B,8C,8D,8E,8F", 3, ""},
		{3, 3, "IPS", "7A,7B,7C,7D,7E,8D,8E,8F", 4, ""},
		{4, 4, "Matematika", "7A,7B,7C,7D,7E", 5, ""},
	}
	for i, row := range tugasRows {
		r := i + 2
		for col, val := range row {
			f.SetCellValue(tugasSheet, cellName(col+1, r), val)
		}
		last := cellName(6, r)
		if i%2 == 0 {
			f.SetCellStyle(tugasSheet, cellName(1, r), last, altStyle)
		} else {
			f.SetCellStyle(tugasSheet, cellName(1, r), last, dataStyle)
		}
		f.SetRowHeight(tugasSheet, r, 22)
	}

	idx, _ := f.GetSheetIndex(instrSheet)
	f.SetActiveSheet(idx)
	return f
}

// UploadDataHandler memproses file Excel yang diupload dan memperbarui data penugasan di database.
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

	// Baca data guru
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

	// Baca data kelas
	kelasRows, err := f.GetRows("Kelas")
	if err != nil || len(kelasRows) < 2 {
		response.Fail(c, http.StatusBadRequest, "sheet 'Kelas' tidak ditemukan atau kosong", nil)
		return
	}

	type classRow struct {
		name   string
		active bool
		hasSBP bool
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
		hasSBP := false
		if len(row) >= 4 && strings.ToUpper(strings.TrimSpace(row[3])) == "YA" {
			hasSBP = true
		}
		classList = append(classList, classRow{strings.TrimSpace(row[1]), active, hasSBP})
	}

	// Baca data penugasan
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
		teacherNum := strings.TrimSpace(row[1])

		// Kolom kelas bisa diisi beberapa nama dipisah koma
		for _, rawClass := range strings.Split(row[3], ",") {
			className := strings.TrimSpace(rawClass)
			if className == "" {
				continue
			}
			assignList = append(assignList, assignRow{
				teacherNum: teacherNum,
				subject:    subjectName,
				class:      className,
				jp:         jp,
				groupKey:   groupKey,
			})
		}
	}

	// Simpan ke database
	db := config.DB

	if len(teacherList) > 0 {
		if err := db.Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "teacher_number"}},
			DoUpdates: clause.AssignmentColumns([]string{"full_name", "gender", "updated_at"}),
		}).Create(&teacherList).Error; err != nil {
			response.Fail(c, http.StatusInternalServerError, "failed to upsert teachers", err.Error())
			return
		}
	}

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

	// Pastikan mata pelajaran Seni Budaya ada jika ada kelas SBP
	for _, cr := range classList {
		if cr.active && cr.hasSBP {
			subjectSet["Seni Budaya"] = struct{}{}
			break
		}
	}

	for name := range subjectSet {
		sub := subjects.Subject{Name: name}
		if err := db.Where("name = ?", name).FirstOrCreate(&sub).Error; err != nil {
			response.Fail(c, http.StatusInternalServerError, fmt.Sprintf("failed to upsert subject %s", name), err.Error())
			return
		}
	}

	// Peta ID untuk lookup
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
	db.Order("grade, code").Find(&allClasses)
	for _, cl := range allClasses {
		classMap[cl.Name] = cl.ID
	}

	// Hapus dan isi ulang penugasan mengajar
	if err := db.Where("1 = 1").Delete(&teachingassignments.TeachingAssignment{}).Error; err != nil {
		response.Fail(c, http.StatusInternalServerError, "failed to clear teaching assignments", err.Error())
		return
	}

	newAssignments := make([]teachingassignments.TeachingAssignment, 0, len(assignList))

	// Penugasan reguler dari sheet Penugasan
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

	// Penugasan SBP otomatis: kelas dikelompokkan per tingkat, maks 3/grup
	sbpSubjectID, hasSBPSubject := subjectMap["Seni Budaya"]
	if hasSBPSubject {
		classListMap := make(map[string]classRow, len(classList))
		for _, cr := range classList {
			classListMap[cr.name] = cr
		}

		type sbpCls struct {
			id    uint
			grade int
			code  string
		}
		var sbpClasses []sbpCls
		for _, cls := range allClasses {
			if cr, found := classListMap[cls.Name]; found && cr.active && cr.hasSBP {
				sbpClasses = append(sbpClasses, sbpCls{cls.ID, cls.Grade, cls.Code})
			}
		}

		gradeGroup := make(map[int][]sbpCls)
		for _, sc := range sbpClasses {
			gradeGroup[sc.grade] = append(gradeGroup[sc.grade], sc)
		}
		for _, grade := range []int{7, 8, 9} {
			gradeClasses := gradeGroup[grade]
			for i := 0; i < len(gradeClasses); i += 3 {
				end := i + 3
				if end > len(gradeClasses) {
					end = len(gradeClasses)
				}
				group := gradeClasses[i:end]
				codes := ""
				for _, sc := range group {
					codes += sc.code
				}
				gk := fmt.Sprintf("SBP-%d-%s", grade, codes)
				for _, sc := range group {
					id := sc.id
					newAssignments = append(newAssignments, teachingassignments.TeachingAssignment{
						TeacherID: nil,
						SubjectID: sbpSubjectID,
						ClassID:   id,
						JP:        3,
						GroupKey:  &gk,
						CreatedBy: "UPLOAD",
						UpdatedBy: "UPLOAD",
					})
				}
			}
		}
	}

	if len(newAssignments) > 0 {
		if err := db.CreateInBatches(newAssignments, 200).Error; err != nil {
			response.Fail(c, http.StatusInternalServerError, "failed to insert teaching assignments", err.Error())
			return
		}
	}

	// Hitung jumlah penugasan SBP untuk respons
	sbpCount := 0
	for _, a := range newAssignments {
		if a.TeacherID == nil && a.GroupKey != nil {
			sbpCount++
		}
	}

	response.OK(c, gin.H{
		"teachers":       len(teacherList),
		"classes":        len(classList),
		"subjects":       len(subjectSet),
		"assignments":    len(newAssignments) - sbpCount,
		"sbpAssignments": sbpCount,
	}, "data uploaded successfully")
}

// ClearDataHandler menghapus permanen semua data yang diupload dari Excel (guru, kelas, mata pelajaran, penugasan).
func ClearDataHandler(c *gin.Context) {
	db := config.DB

	if err := db.Unscoped().Where("1 = 1").Delete(&teachingassignments.TeachingAssignment{}).Error; err != nil {
		response.Fail(c, http.StatusInternalServerError, "gagal menghapus penugasan", err.Error())
		return
	}
	if err := db.Unscoped().Where("1 = 1").Delete(&subjects.Subject{}).Error; err != nil {
		response.Fail(c, http.StatusInternalServerError, "gagal menghapus mata pelajaran", err.Error())
		return
	}
	if err := db.Unscoped().Where("1 = 1").Delete(&classes.Class{}).Error; err != nil {
		response.Fail(c, http.StatusInternalServerError, "gagal menghapus kelas", err.Error())
		return
	}
	if err := db.Unscoped().Where("1 = 1").Delete(&teachers.Teacher{}).Error; err != nil {
		response.Fail(c, http.StatusInternalServerError, "gagal menghapus guru", err.Error())
		return
	}

	response.OK(c, nil, "semua data berhasil dihapus")
}

// DataStatusHandler mengembalikan jumlah data saat ini di database.
func DataStatusHandler(c *gin.Context) {
	db := config.DB

	var teacherCount, classCount, subjectCount, assignCount int64
	db.Model(&teachers.Teacher{}).Count(&teacherCount)
	db.Model(&classes.Class{}).Where("is_active = true").Count(&classCount)
	db.Model(&subjects.Subject{}).Count(&subjectCount)
	db.Model(&teachingassignments.TeachingAssignment{}).Count(&assignCount)

	response.OK(c, gin.H{
		"teachers":            teacherCount,
		"activeClasses":       classCount,
		"subjects":            subjectCount,
		"teachingAssignments": assignCount,
	}, "ok")
}

func cellName(col, row int) string {
	name, _ := excelize.CoordinatesToCellName(col, row)
	return name
}
