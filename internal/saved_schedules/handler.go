package savedschedules

import (
	"encoding/json"
	"fmt"
	"net/http"
	"smp_mater_dei_be/internal/schedule"
	"smp_mater_dei_be/internal/transport/http/response"
	"strconv"
	"strings"

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

	// ── Sheet: Semua Data (flat) ─────────────────────────────────────────────
	flatSheet := "Semua Data"
	f.SetSheetName("Sheet1", flatSheet)
	headers := []string{"Kelas", "Hari", "Jam Mulai", "Jam Selesai", "Mata Pelajaran", "Guru"}
	for col, h := range headers {
		cell, _ := excelize.CoordinatesToCellName(col+1, 1)
		f.SetCellValue(flatSheet, cell, h)
	}
	for i, e := range entries {
		row := i + 2
		f.SetCellValue(flatSheet, cellName(1, row), e.ClassName)
		f.SetCellValue(flatSheet, cellName(2, row), dayLabels[e.Day])
		f.SetCellValue(flatSheet, cellName(3, row), e.TimeStart)
		f.SetCellValue(flatSheet, cellName(4, row), e.TimeEnd)
		f.SetCellValue(flatSheet, cellName(5, row), e.SubjectName)
		f.SetCellValue(flatSheet, cellName(6, row), e.TeacherName)
	}

	// ── Per-class sheets ─────────────────────────────────────────────────────
	classMap := make(map[string][]schedule.ScheduleEntry)
	for _, e := range entries {
		classMap[e.ClassName] = append(classMap[e.ClassName], e)
	}

	// collect and sort class names
	classNames := make([]string, 0, len(classMap))
	for cn := range classMap {
		classNames = append(classNames, cn)
	}
	sortStrings(classNames)

	for _, className := range classNames {
		sheetEntries := classMap[className]
		sheetName := "Kelas " + className
		f.NewSheet(sheetName)

		// header row: time | Senin | Selasa | ... | Jumat
		f.SetCellValue(sheetName, cellName(1, 1), "Waktu")
		for col, day := range days {
			f.SetCellValue(sheetName, cellName(col+2, 1), dayLabels[day])
		}

		// collect unique time slots
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
			row := i + 2
			slotRow[s.start] = row
			f.SetCellValue(sheetName, cellName(1, row), s.start+" - "+s.end)
		}

		dayCol := map[string]int{
			"monday": 2, "tuesday": 3, "wednesday": 4, "thursday": 5, "friday": 6,
		}

		for _, e := range sheetEntries {
			row, ok := slotRow[e.TimeStart]
			if !ok {
				continue
			}
			col := dayCol[e.Day]
			cur, _ := f.GetCellValue(sheetName, cellName(col, row))
			val := e.SubjectName
			if e.TeacherName != "" {
				val += "\n" + e.TeacherName
			}
			if cur != "" {
				val = cur + "\n" + val
			}
			f.SetCellValue(sheetName, cellName(col, row), val)
		}
	}

	// set active sheet
	idx, _ := f.GetSheetIndex(flatSheet)
	f.SetActiveSheet(idx)

	return f, nil
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
