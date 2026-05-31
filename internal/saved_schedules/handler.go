package savedschedules

import (
	"encoding/json"
	"fmt"
	"net/http"
	"smp_mater_dei_be/internal/schedule"
	"smp_mater_dei_be/internal/transport/http/response"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/xuri/excelize/v2"
)

type SaveRequest struct {
	Title   string          `json:"title" binding:"required"`
	Entries json.RawMessage `json:"entries" binding:"required"`
	Meta    json.RawMessage `json:"meta"`
}

func SaveHandler(c *gin.Context) {
	var req SaveRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, http.StatusBadRequest, "invalid request", err.Error())
		return
	}

	userID, _ := c.Get("userID")

	s := &SavedSchedule{
		Title:     req.Title,
		Entries:   string(req.Entries),
		Meta:      string(req.Meta),
		CreatedBy: fmt.Sprintf("%v", userID),
		UpdatedBy: fmt.Sprintf("%v", userID),
	}

	if err := Create(s); err != nil {
		response.Fail(c, http.StatusInternalServerError, "failed to save schedule", err.Error())
		return
	}

	response.OK(c, gin.H{"id": s.ID}, "schedule saved")
}

func ListHandler(c *gin.Context) {
	items, err := List()
	if err != nil {
		response.Fail(c, http.StatusInternalServerError, "failed to list schedules", err.Error())
		return
	}
	response.OK(c, items, "ok")
}

func GetHandler(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		response.Fail(c, http.StatusBadRequest, "invalid id", nil)
		return
	}

	s, err := GetByID(uint(id))
	if err != nil {
		response.Fail(c, http.StatusNotFound, "schedule not found", err.Error())
		return
	}

	var entries []schedule.ScheduleEntry
	if err := json.Unmarshal([]byte(s.Entries), &entries); err != nil {
		response.Fail(c, http.StatusInternalServerError, "failed to parse entries", err.Error())
		return
	}

	var meta schedule.ScheduleMeta
	_ = json.Unmarshal([]byte(s.Meta), &meta)

	response.OK(c, gin.H{
		"id":        s.ID,
		"title":     s.Title,
		"entries":   entries,
		"meta":      meta,
		"createdAt": s.CreatedAt,
		"createdBy": s.CreatedBy,
	}, "ok")
}

func DeleteHandler(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		response.Fail(c, http.StatusBadRequest, "invalid id", nil)
		return
	}

	if err := Delete(uint(id)); err != nil {
		response.Fail(c, http.StatusInternalServerError, "failed to delete schedule", err.Error())
		return
	}

	response.OK(c, nil, "deleted")
}

func DeployHandler(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		response.Fail(c, http.StatusBadRequest, "invalid id", nil)
		return
	}

	if _, err := GetByID(uint(id)); err != nil {
		response.Fail(c, http.StatusNotFound, "schedule not found", err.Error())
		return
	}

	if err := Activate(uint(id)); err != nil {
		response.Fail(c, http.StatusInternalServerError, "failed to deploy schedule", err.Error())
		return
	}

	response.OK(c, nil, "schedule deployed")
}

func GetActiveHandler(c *gin.Context) {
	s, err := GetActive()
	if err != nil {
		response.Fail(c, http.StatusNotFound, "no active schedule", err.Error())
		return
	}

	var entries []schedule.ScheduleEntry
	if err := json.Unmarshal([]byte(s.Entries), &entries); err != nil {
		response.Fail(c, http.StatusInternalServerError, "failed to parse entries", err.Error())
		return
	}

	var meta schedule.ScheduleMeta
	_ = json.Unmarshal([]byte(s.Meta), &meta)

	response.OK(c, gin.H{
		"id":        s.ID,
		"title":     s.Title,
		"entries":   entries,
		"meta":      meta,
		"createdAt": s.CreatedAt,
		"createdBy": s.CreatedBy,
	}, "ok")
}

func ExportHandler(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		response.Fail(c, http.StatusBadRequest, "invalid id", nil)
		return
	}

	s, err := GetByID(uint(id))
	if err != nil {
		response.Fail(c, http.StatusNotFound, "schedule not found", err.Error())
		return
	}

	var entries []schedule.ScheduleEntry
	if err := json.Unmarshal([]byte(s.Entries), &entries); err != nil {
		response.Fail(c, http.StatusInternalServerError, "failed to parse entries", err.Error())
		return
	}

	f, err := buildScheduleExcel(s.Title, entries)
	if err != nil {
		response.Fail(c, http.StatusInternalServerError, "failed to generate excel", err.Error())
		return
	}

	filename := strings.ReplaceAll(s.Title, " ", "_") + ".xlsx"
	c.Header("Content-Disposition", "attachment; filename="+filename)
	c.Header("Content-Type", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet")

	buf, _ := f.WriteToBuffer()
	c.Data(http.StatusOK, "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet", buf.Bytes())
}

var days = []string{"monday", "tuesday", "wednesday", "thursday", "friday"}
var dayLabels = map[string]string{
	"monday":    "Senin",
	"tuesday":   "Selasa",
	"wednesday": "Rabu",
	"thursday":  "Kamis",
	"friday":    "Jumat",
}

func buildScheduleExcel(title string, entries []schedule.ScheduleEntry) (*excelize.File, error) {
	f := excelize.NewFile()
	now := time.Now()

	// Definisi gaya
	thinBorder := []excelize.Border{
		{Type: "left", Color: "BBBBBB", Style: 1},
		{Type: "right", Color: "BBBBBB", Style: 1},
		{Type: "top", Color: "BBBBBB", Style: 1},
		{Type: "bottom", Color: "BBBBBB", Style: 1},
	}
	blueBorder := []excelize.Border{
		{Type: "left", Color: "1F538D", Style: 1},
		{Type: "right", Color: "1F538D", Style: 1},
		{Type: "top", Color: "1F538D", Style: 1},
		{Type: "bottom", Color: "1F538D", Style: 1},
	}

	// Gaya cover
	sBanner, _ := f.NewStyle(&excelize.Style{
		Font:      &excelize.Font{Bold: true, Color: "FFFFFF", Size: 16},
		Fill:      excelize.Fill{Type: "pattern", Color: []string{"1F538D"}, Pattern: 1},
		Alignment: &excelize.Alignment{Horizontal: "center", Vertical: "center"},
		Border:    blueBorder,
	})
	sSubtitle, _ := f.NewStyle(&excelize.Style{
		Font:      &excelize.Font{Bold: true, Color: "2E75B6", Size: 13},
		Alignment: &excelize.Alignment{Horizontal: "center", Vertical: "center"},
	})
	sTitleSub, _ := f.NewStyle(&excelize.Style{
		Font:      &excelize.Font{Italic: true, Color: "1F538D", Size: 12},
		Alignment: &excelize.Alignment{Horizontal: "center", Vertical: "center"},
	})
	sInfoLabel, _ := f.NewStyle(&excelize.Style{
		Font:      &excelize.Font{Bold: true, Color: "1F538D", Size: 11},
		Alignment: &excelize.Alignment{Horizontal: "right", Vertical: "center"},
	})
	sInfoVal, _ := f.NewStyle(&excelize.Style{
		Font:      &excelize.Font{Color: "333333", Size: 11},
		Alignment: &excelize.Alignment{Horizontal: "left", Vertical: "center"},
	})
	sStatHdr, _ := f.NewStyle(&excelize.Style{
		Font: &excelize.Font{Bold: true, Color: "FFFFFF", Size: 11},
		Fill: excelize.Fill{Type: "pattern", Color: []string{"F4A800"}, Pattern: 1},
		Alignment: &excelize.Alignment{Horizontal: "center", Vertical: "center"},
		Border: []excelize.Border{
			{Type: "left", Color: "F4A800", Style: 1},
			{Type: "right", Color: "F4A800", Style: 1},
			{Type: "top", Color: "F4A800", Style: 1},
			{Type: "bottom", Color: "F4A800", Style: 1},
		},
	})
	sStatVal, _ := f.NewStyle(&excelize.Style{
		Font: &excelize.Font{Bold: true, Color: "1F538D", Size: 20},
		Fill: excelize.Fill{Type: "pattern", Color: []string{"EEF4FF"}, Pattern: 1},
		Alignment: &excelize.Alignment{Horizontal: "center", Vertical: "center"},
		Border: []excelize.Border{
			{Type: "left", Color: "2E75B6", Style: 1},
			{Type: "right", Color: "2E75B6", Style: 1},
			{Type: "top", Color: "2E75B6", Style: 1},
			{Type: "bottom", Color: "2E75B6", Style: 1},
		},
	})

	// Gaya tabel
	sTableHdr, _ := f.NewStyle(&excelize.Style{
		Font:      &excelize.Font{Bold: true, Color: "FFFFFF", Size: 11},
		Fill:      excelize.Fill{Type: "pattern", Color: []string{"1F538D"}, Pattern: 1},
		Alignment: &excelize.Alignment{Horizontal: "center", Vertical: "center", WrapText: true},
		Border:    thinBorder,
	})
	sDayHdr, _ := f.NewStyle(&excelize.Style{
		Font:      &excelize.Font{Bold: true, Color: "FFFFFF", Size: 11},
		Fill:      excelize.Fill{Type: "pattern", Color: []string{"2E75B6"}, Pattern: 1},
		Alignment: &excelize.Alignment{Horizontal: "center", Vertical: "center"},
		Border:    thinBorder,
	})
	sTimeHdr, _ := f.NewStyle(&excelize.Style{
		Font:      &excelize.Font{Bold: true, Color: "FFFFFF", Size: 11},
		Fill:      excelize.Fill{Type: "pattern", Color: []string{"1F538D"}, Pattern: 1},
		Alignment: &excelize.Alignment{Horizontal: "center", Vertical: "center"},
		Border:    thinBorder,
	})
	sTimeData, _ := f.NewStyle(&excelize.Style{
		Font:      &excelize.Font{Bold: true, Color: "1F538D", Size: 9},
		Fill:      excelize.Fill{Type: "pattern", Color: []string{"DDEEFF"}, Pattern: 1},
		Alignment: &excelize.Alignment{Horizontal: "center", Vertical: "center"},
		Border:    thinBorder,
	})
	sDataNorm, _ := f.NewStyle(&excelize.Style{
		Font:      &excelize.Font{Size: 10},
		Alignment: &excelize.Alignment{Horizontal: "center", Vertical: "center", WrapText: true},
		Border:    thinBorder,
	})
	sDataAlt, _ := f.NewStyle(&excelize.Style{
		Font:      &excelize.Font{Size: 10},
		Fill:      excelize.Fill{Type: "pattern", Color: []string{"EEF4FF"}, Pattern: 1},
		Alignment: &excelize.Alignment{Horizontal: "center", Vertical: "center", WrapText: true},
		Border:    thinBorder,
	})

	// Kumpulkan data kelas dan guru
	classMap := make(map[string][]schedule.ScheduleEntry)
	teacherSet := make(map[string]struct{})
	for _, e := range entries {
		classMap[e.ClassName] = append(classMap[e.ClassName], e)
		if e.TeacherName != "" {
			teacherSet[e.TeacherName] = struct{}{}
		}
	}
	classNames := make([]string, 0, len(classMap))
	for cn := range classMap {
		classNames = append(classNames, cn)
	}
	sortStrings(classNames)

	// Sheet: Ringkasan
	cover := "Ringkasan"
	f.SetSheetName("Sheet1", cover)
	f.SetColWidth(cover, "A", "A", 24)
	f.SetColWidth(cover, "B", "F", 16)

	f.MergeCell(cover, "A1", "F1")
	f.SetCellValue(cover, "A1", "JADWAL PELAJARAN")
	f.SetCellStyle(cover, "A1", "F1", sBanner)
	f.SetRowHeight(cover, 1, 52)

	f.MergeCell(cover, "A2", "F2")
	f.SetCellValue(cover, "A2", "SMP Mater Dei Yogyakarta")
	f.SetCellStyle(cover, "A2", "F2", sSubtitle)
	f.SetRowHeight(cover, 2, 30)

	f.MergeCell(cover, "A3", "F3")
	f.SetCellValue(cover, "A3", title)
	f.SetCellStyle(cover, "A3", "F3", sTitleSub)
	f.SetRowHeight(cover, 3, 26)

	f.SetRowHeight(cover, 4, 14)

	// Baris metadata
	infoRows := [][]string{
		{"Dibuat pada:", now.Format("02 January 2006, 15:04 WIB")},
		{"Total JP terjadwal:", fmt.Sprintf("%d JP", len(entries))},
		{"Jumlah kelas:", fmt.Sprintf("%d kelas", len(classNames))},
		{"Guru mengajar:", fmt.Sprintf("%d guru aktif", len(teacherSet))},
	}
	for i, ir := range infoRows {
		r := i + 5
		f.MergeCell(cover, cellName(2, r), cellName(6, r))
		f.SetCellValue(cover, cellName(1, r), ir[0])
		f.SetCellValue(cover, cellName(2, r), ir[1])
		f.SetCellStyle(cover, cellName(1, r), cellName(1, r), sInfoLabel)
		f.SetCellStyle(cover, cellName(2, r), cellName(2, r), sInfoVal)
		f.SetRowHeight(cover, r, 22)
	}

	f.SetRowHeight(cover, 10, 14)

	// Kotak statistik
	statCols := [][2]string{{"A", "B"}, {"C", "D"}, {"E", "F"}}
	statLabels := []string{"Total JP", "Kelas Dijadwalkan", "Guru Mengajar"}
	statVals := []interface{}{len(entries), len(classNames), len(teacherSet)}
	for i, sc := range statCols {
		hdr := sc[0] + "11"
		hdrEnd := sc[1] + "11"
		val := sc[0] + "12"
		valEnd := sc[1] + "12"
		f.MergeCell(cover, hdr, hdrEnd)
		f.SetCellValue(cover, hdr, statLabels[i])
		f.SetCellStyle(cover, hdr, hdrEnd, sStatHdr)
		f.MergeCell(cover, val, valEnd)
		f.SetCellValue(cover, val, statVals[i])
		f.SetCellStyle(cover, val, valEnd, sStatVal)
	}
	f.SetRowHeight(cover, 11, 26)
	f.SetRowHeight(cover, 12, 48)

	// Sheet: Semua Data
	flatSheet := "Semua Data"
	f.NewSheet(flatSheet)
	f.SetColWidth(flatSheet, "A", "A", 10)
	f.SetColWidth(flatSheet, "B", "B", 12)
	f.SetColWidth(flatSheet, "C", "C", 11)
	f.SetColWidth(flatSheet, "D", "D", 11)
	f.SetColWidth(flatSheet, "E", "E", 30)
	f.SetColWidth(flatSheet, "F", "F", 36)

	flatHdrs := []string{"Kelas", "Hari", "Jam Mulai", "Jam Selesai", "Mata Pelajaran", "Guru"}
	for col, h := range flatHdrs {
		f.SetCellValue(flatSheet, cellName(col+1, 1), h)
	}
	f.SetCellStyle(flatSheet, "A1", "F1", sTableHdr)
	f.SetRowHeight(flatSheet, 1, 26)

	f.SetPanes(flatSheet, &excelize.Panes{
		Freeze: true, YSplit: 1, TopLeftCell: "A2", ActivePane: "bottomLeft",
		Selection: []excelize.Selection{{Pane: "bottomLeft", ActiveCell: "A2", SQRef: "A2"}},
	})

	sorted := sortEntries(entries)
	for i, e := range sorted {
		row := i + 2
		f.SetCellValue(flatSheet, cellName(1, row), e.ClassName)
		f.SetCellValue(flatSheet, cellName(2, row), dayLabels[e.Day])
		f.SetCellValue(flatSheet, cellName(3, row), e.TimeStart)
		f.SetCellValue(flatSheet, cellName(4, row), e.TimeEnd)
		f.SetCellValue(flatSheet, cellName(5, row), e.SubjectName)
		f.SetCellValue(flatSheet, cellName(6, row), e.TeacherName)
		last, _ := excelize.CoordinatesToCellName(6, row)
		if row%2 == 0 {
			f.SetCellStyle(flatSheet, cellName(1, row), last, sDataAlt)
		} else {
			f.SetCellStyle(flatSheet, cellName(1, row), last, sDataNorm)
		}
		f.SetRowHeight(flatSheet, row, 18)
	}

	// Sheet per kelas
	dayCol := map[string]int{
		"monday": 2, "tuesday": 3, "wednesday": 4, "thursday": 5, "friday": 6,
	}

	for _, className := range classNames {
		sheetEntries := classMap[className]
		sn := "Kelas " + className
		f.NewSheet(sn)
		f.SetColWidth(sn, "A", "A", 16)
		f.SetColWidth(sn, "B", "F", 24)

		f.MergeCell(sn, "A1", "F1")
		f.SetCellValue(sn, "A1", "JADWAL KELAS "+className)
		f.SetCellStyle(sn, "A1", "F1", sBanner)
		f.SetRowHeight(sn, 1, 32)

		f.SetCellValue(sn, "A2", "Waktu")
		f.SetCellStyle(sn, "A2", "A2", sTimeHdr)
		for col, day := range days {
			cn := cellName(col+2, 2)
			f.SetCellValue(sn, cn, dayLabels[day])
			f.SetCellStyle(sn, cn, cn, sDayHdr)
		}
		f.SetRowHeight(sn, 2, 28)

		// Kumpulkan slot waktu unik
		slotSet := make(map[string]timeSlot)
		for _, e := range sheetEntries {
			slotSet[e.TimeStart] = timeSlot{e.TimeStart, e.TimeEnd}
		}
		slotList := make([]timeSlot, 0, len(slotSet))
		for _, s := range slotSet {
			slotList = append(slotList, s)
		}
		sortSlots(slotList)

		slotRow := make(map[string]int, len(slotList))
		for i, s := range slotList {
			row := i + 3
			slotRow[s.start] = row

			f.SetCellValue(sn, cellName(1, row), s.start+" – "+s.end)
			f.SetCellStyle(sn, cellName(1, row), cellName(1, row), sTimeData)
			f.SetRowHeight(sn, row, 44)

			last := cellName(6, row)
			if i%2 == 0 {
				f.SetCellStyle(sn, cellName(2, row), last, sDataNorm)
			} else {
				f.SetCellStyle(sn, cellName(2, row), last, sDataAlt)
			}
		}

		// Isi data jadwal
		for _, e := range sheetEntries {
			row, ok := slotRow[e.TimeStart]
			if !ok {
				continue
			}
			col := dayCol[e.Day]
			cn := cellName(col, row)

			if e.TeacherName != "" {
				f.SetCellRichText(sn, cn, []excelize.RichTextRun{
					{
						Text: e.SubjectName,
						Font: &excelize.Font{Bold: true, Size: 10, Color: "1F538D"},
					},
					{
						Text: "\n" + e.TeacherName,
						Font: &excelize.Font{Size: 9, Color: "555555"},
					},
				})
			} else {
				f.SetCellValue(sn, cn, e.SubjectName)
			}
		}

		// Kunci baris header dan kolom waktu
		f.SetPanes(sn, &excelize.Panes{
			Freeze: true, XSplit: 1, YSplit: 2, TopLeftCell: "B3", ActivePane: "bottomRight",
			Selection: []excelize.Selection{{Pane: "bottomRight", ActiveCell: "B3", SQRef: "B3"}},
		})
	}

	idx, _ := f.GetSheetIndex(cover)
	f.SetActiveSheet(idx)
	return f, nil
}

func sortEntries(entries []schedule.ScheduleEntry) []schedule.ScheduleEntry {
	dayOrder := map[string]int{
		"monday": 0, "tuesday": 1, "wednesday": 2, "thursday": 3, "friday": 4,
	}
	result := make([]schedule.ScheduleEntry, len(entries))
	copy(result, entries)
	for i := 0; i < len(result)-1; i++ {
		for j := i + 1; j < len(result); j++ {
			a, b := result[i], result[j]
			swap := a.ClassName > b.ClassName ||
				(a.ClassName == b.ClassName && dayOrder[a.Day] > dayOrder[b.Day]) ||
				(a.ClassName == b.ClassName && dayOrder[a.Day] == dayOrder[b.Day] && a.TimeStart > b.TimeStart)
			if swap {
				result[i], result[j] = result[j], result[i]
			}
		}
	}
	return result
}

func cellName(col, row int) string {
	name, _ := excelize.CoordinatesToCellName(col, row)
	return name
}

func sortStrings(s []string) {
	for i := 0; i < len(s)-1; i++ {
		for j := i + 1; j < len(s); j++ {
			if s[i] > s[j] {
				s[i], s[j] = s[j], s[i]
			}
		}
	}
}

type timeSlot struct{ start, end string }

func sortSlots(slots []timeSlot) {
	for i := 0; i < len(slots)-1; i++ {
		for j := i + 1; j < len(slots); j++ {
			if slots[i].start > slots[j].start {
				slots[i], slots[j] = slots[j], slots[i]
			}
		}
	}
}
