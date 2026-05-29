package algorithm

// days defines the ordered weekly schedule days.
var days = []string{"monday", "tuesday", "wednesday", "thursday", "friday"}

// slotsPerDay is the number of 40-minute teaching slots per day (breaks not counted).
//
//	Monday:           8 slots (07:50–13:45, breaks 09:10 and 11:30)
//	Tuesday–Thursday: 9 slots (07:10–13:45, same breaks)
//	Friday:           5 slots (07:50–11:30, break at 09:50)
var slotsPerDay = map[string]int{
	"monday":    8,
	"tuesday":   9,
	"wednesday": 9,
	"thursday":  9,
	"friday":    5,
}

// slotStartTime maps (day, slotIndex) → wall-clock start "HH:MM".
// Index 0 is the first teaching slot of the day; breaks are skipped.
var slotStartTime = map[string][]string{
	"monday":    {"07:50", "08:30", "09:30", "10:10", "10:50", "11:45", "12:25", "13:05"},
	"tuesday":   {"07:10", "07:50", "08:30", "09:30", "10:10", "10:50", "11:45", "12:25", "13:05"},
	"wednesday": {"07:10", "07:50", "08:30", "09:30", "10:10", "10:50", "11:45", "12:25", "13:05"},
	"thursday":  {"07:10", "07:50", "08:30", "09:30", "10:10", "10:50", "11:45", "12:25", "13:05"},
	"friday":    {"07:50", "08:30", "09:10", "10:10", "10:50"},
}

// slotEndTime maps (day, slotIndex) → wall-clock end "HH:MM".
var slotEndTime = map[string][]string{
	"monday":    {"08:30", "09:10", "10:10", "10:50", "11:30", "12:25", "13:05", "13:45"},
	"tuesday":   {"07:50", "08:30", "09:10", "10:10", "10:50", "11:30", "12:25", "13:05", "13:45"},
	"wednesday": {"07:50", "08:30", "09:10", "10:10", "10:50", "11:30", "12:25", "13:05", "13:45"},
	"thursday":  {"07:50", "08:30", "09:10", "10:10", "10:50", "11:30", "12:25", "13:05", "13:45"},
	"friday":    {"08:30", "09:10", "09:50", "10:50", "11:30"},
}

// timeStartToSlot maps (day, "HH:MM") → slotIndex.
// Used to convert real-schedule time strings to grid indices when seeding.
var timeStartToSlot = map[string]map[string]int{
	"monday": {
		"07:50": 0, "08:30": 1, "09:30": 2,
		"10:10": 3, "10:50": 4, "11:45": 5, "12:25": 6, "13:05": 7,
	},
	"tuesday": {
		"07:10": 0, "07:50": 1, "08:30": 2, "09:30": 3,
		"10:10": 4, "10:50": 5, "11:45": 6, "12:25": 7, "13:05": 8,
	},
	"wednesday": {
		"07:10": 0, "07:50": 1, "08:30": 2, "09:30": 3,
		"10:10": 4, "10:50": 5, "11:45": 6, "12:25": 7, "13:05": 8,
	},
	"thursday": {
		"07:10": 0, "07:50": 1, "08:30": 2, "09:30": 3,
		"10:10": 4, "10:50": 5, "11:45": 6, "12:25": 7, "13:05": 8,
	},
	"friday": {
		"07:50": 0, "08:30": 1, "09:10": 2, "10:10": 3, "10:50": 4,
	},
}

// pjokMaxStartSlot is the highest slot index at which a JP=2 PJOK block
// can start and still finish at or before 11:00 wall-clock.
var pjokMaxStartSlot = map[string]int{
	"monday":    2, // 09:30 + 2JP → 10:50
	"tuesday":   3, // 09:30 + 2JP → 10:50
	"wednesday": 3,
	"thursday":  3,
	"friday":    2, // 09:10 + 2JP → 10:50 (crosses 09:50 break)
}

// SlotTimeRange returns the wall-clock start and end strings for a single slot.
func SlotTimeRange(day string, slotIndex int) (start, end string) {
	starts := slotStartTime[day]
	ends := slotEndTime[day]
	if slotIndex < 0 || slotIndex >= len(starts) {
		return "", ""
	}
	return starts[slotIndex], ends[slotIndex]
}

// SlotIndexFromTimeStart converts a wall-clock start time to its slot index.
// Returns -1 if the time is not a valid slot start on that day.
func SlotIndexFromTimeStart(day, timeStart string) int {
	if m, ok := timeStartToSlot[day]; ok {
		if idx, ok := m[timeStart]; ok {
			return idx
		}
	}
	return -1
}
