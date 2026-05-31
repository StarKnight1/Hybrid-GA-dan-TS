package algorithm

// MatrixDays mendaftar lima hari sekolah secara berurutan.
var MatrixDays = []string{"monday", "tuesday", "wednesday", "thursday", "friday"}

// Slot mendeskripsikan satu periode mengajar 40 menit dalam satu hari sekolah.
type Slot struct {
	Index     int
	StartTime string
	EndTime   string
	IsBlocked bool
}

// DaySlots memetakan nama hari sekolah ke daftar periode berurutan.
type DaySlots map[string][]Slot

// buildWeekdayPeriods mengembalikan layout standar 9 slot yang digunakan Senin–Kamis.
// Jika blockFirst bernilai true, slot 0 ditandai diblokir (dipakai untuk hari Senin).
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

// buildFridayPeriods mengembalikan layout slot Jumat yang lebih singkat.
// Slot 6–8 diblokir (waktu sholat Jumat), dan slot 3 selesai lebih awal.
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

// GenerateSlots membangun template jadwal mingguan lengkap untuk semua lima hari sekolah.
func GenerateSlots() DaySlots {
	ds := make(DaySlots, 5)
	ds["monday"] = buildWeekdayPeriods(true)
	ds["tuesday"] = buildWeekdayPeriods(false)
	ds["wednesday"] = buildWeekdayPeriods(false)
	ds["thursday"] = buildWeekdayPeriods(false)
	ds["friday"] = buildFridayPeriods()
	return ds
}

// MatrixDayIndex mengembalikan posisi ordinal hari dalam MatrixDays,
// atau len(MatrixDays) jika nama hari tidak dikenali.
func MatrixDayIndex(day string) int {
	for idx, d := range MatrixDays {
		if d == day {
			return idx
		}
	}
	return len(MatrixDays)
}

// MatrixSlotIndexFromTimeStart mencari indeks slot berdasarkan hari dan waktu mulai.
// Mengembalikan (indeks, true) jika berhasil atau (0, false) jika tidak ditemukan.
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
