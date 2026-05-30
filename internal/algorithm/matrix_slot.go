package algorithm

// MatrixDays lists the five school days in order.
var MatrixDays = []string{"monday", "tuesday", "wednesday", "thursday", "friday"}

// Slot describes one 40-minute teaching period within a school day.
type Slot struct {
	Index     int
	StartTime string
	EndTime   string
	IsBlocked bool
}

// DaySlots maps each school day name to its ordered list of periods.
type DaySlots map[string][]Slot

// buildWeekdayPeriods returns the standard 9-slot layout used Monday–Thursday.
// When blockFirst is true, slot 0 is marked blocked (used for Monday).
func buildWeekdayPeriods(blockFirst bool) []Slot {
	return []Slot{
		{0, "07:10", "07:50", blockFirst},
		{1, "07:50", "08:30", false},
		{2, "08:30", "09:10", false},
		{3, "09:30", "10:10", false},
		{4, "10:10", "10:50", false},
		{5, "10:50", "11:30", false},
		{6, "11:45", "12:25", false},
		{7, "12:25", "13:05", false},
		{8, "13:05", "13:45", false},
	}
}

// buildFridayPeriods returns the abbreviated Friday slot layout.
// Slots 6–8 are blocked (Jum'at prayer break), and slot 3 ends earlier.
func buildFridayPeriods() []Slot {
	return []Slot{
		{0, "07:10", "07:50", true},
		{1, "07:50", "08:30", false},
		{2, "08:30", "09:10", false},
		{3, "09:10", "09:50", false},
		{4, "10:10", "10:50", false},
		{5, "10:50", "11:30", false},
		{6, "", "", true},
		{7, "", "", true},
		{8, "", "", true},
	}
}

// GenerateSlots builds the complete weekly schedule template for all five school days.
func GenerateSlots() DaySlots {
	ds := make(DaySlots, 5)
	ds["monday"] = buildWeekdayPeriods(true)
	ds["tuesday"] = buildWeekdayPeriods(false)
	ds["wednesday"] = buildWeekdayPeriods(false)
	ds["thursday"] = buildWeekdayPeriods(false)
	ds["friday"] = buildFridayPeriods()
	return ds
}

// MatrixDayIndex returns the ordinal position of a day within MatrixDays,
// or len(MatrixDays) if the day name is unrecognized.
func MatrixDayIndex(day string) int {
	for idx, d := range MatrixDays {
		if d == day {
			return idx
		}
	}
	return len(MatrixDays)
}

// MatrixSlotIndexFromTimeStart looks up the slot index for a given day and start time.
// Returns (index, true) on success or (0, false) if not found.
func MatrixSlotIndexFromTimeStart(day, timeStart string, daySlots DaySlots) (int, bool) {
	if daySlots == nil {
		daySlots = GenerateSlots()
	}
	for _, s := range daySlots[day] {
		if s.StartTime == timeStart {
			return s.Index, true
		}
	}
	return 0, false
}
