package algorithm

var MatrixDays = []string{"monday", "tuesday", "wednesday", "thursday", "friday"}

type Slot struct {
	Index     int
	StartTime string
	EndTime   string
	IsBlocked bool
}

type DaySlots map[string][]Slot

func GenerateSlots() DaySlots {
	return DaySlots{
		"monday": {
			{0, "07:10", "07:50", true},

			{1, "07:50", "08:30", false},
			{2, "08:30", "09:10", false},
			{3, "09:30", "10:10", false},
			{4, "10:10", "10:50", false},
			{5, "10:50", "11:30", false},
			{6, "11:45", "12:25", false},
			{7, "12:25", "13:05", false},
			{8, "13:05", "13:45", false},
		},

		"tuesday": {
			{0, "07:10", "07:50", false},
			{1, "07:50", "08:30", false},
			{2, "08:30", "09:10", false},
			{3, "09:30", "10:10", false},
			{4, "10:10", "10:50", false},
			{5, "10:50", "11:30", false},
			{6, "11:45", "12:25", false},
			{7, "12:25", "13:05", false},
			{8, "13:05", "13:45", false},
		},

		"wednesday": {
			{0, "07:10", "07:50", false},
			{1, "07:50", "08:30", false},
			{2, "08:30", "09:10", false},
			{3, "09:30", "10:10", false},
			{4, "10:10", "10:50", false},
			{5, "10:50", "11:30", false},
			{6, "11:45", "12:25", false},
			{7, "12:25", "13:05", false},
			{8, "13:05", "13:45", false},
		},

		"thursday": {
			{0, "07:10", "07:50", false},
			{1, "07:50", "08:30", false},
			{2, "08:30", "09:10", false},
			{3, "09:30", "10:10", false},
			{4, "10:10", "10:50", false},
			{5, "10:50", "11:30", false},
			{6, "11:45", "12:25", false},
			{7, "12:25", "13:05", false},
			{8, "13:05", "13:45", false},
		},

		"friday": {
			{0, "07:10", "07:50", true},

			{1, "07:50", "08:30", false},
			{2, "08:30", "09:10", false},
			{3, "09:10", "09:50", false},
			{4, "10:10", "10:50", false},
			{5, "10:50", "11:30", false},

			{6, "", "", true},
			{7, "", "", true},
			{8, "", "", true},
		},
	}
}

func MatrixDayIndex(day string) int {
	for i, matrixDay := range MatrixDays {
		if matrixDay == day {
			return i
		}
	}
	return len(MatrixDays)
}

func MatrixSlotIndexFromTimeStart(day, timeStart string, daySlots DaySlots) (int, bool) {
	if daySlots == nil {
		daySlots = GenerateSlots()
	}

	for _, slot := range daySlots[day] {
		if slot.StartTime == timeStart {
			return slot.Index, true
		}
	}
	return 0, false
}
