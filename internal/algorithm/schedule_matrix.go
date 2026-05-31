package algorithm

import (
	"fmt"
)

// CellState menunjukkan apakah sel grid kosong, diblokir, atau terisi oleh blok.
type CellState uint8

const (
	EmptyCell   CellState = iota // tersedia untuk penempatan
	BlockedCell                  // tidak tersedia permanen (mis. jam istirahat, slot pertama Senin)
	FilledCell                   // terisi oleh sesi pelajaran
)

// MatrixCell menyimpan status hunian satu slot waktu dalam grid kelas atau guru.
type MatrixCell struct {
	State   CellState
	BlockID uint
}

// BlockRecord menyimpan penempatan blok yang sudah terjadwal dalam jadwal pelajaran.
type BlockRecord struct {
	BlockID   uint
	ClassID   uint
	TeacherID *uint
	Day       string
	StartSlot int
	Duration  int
}

// courseDayKey mengidentifikasi pasangan (kelas, mapel) untuk pelacakan sebaran hari.
type courseDayKey struct {
	classID   uint
	subjectID uint
}

// ScheduleMatrix adalah grid kendala dua dimensi yang melacak hunian kelas dan guru.
// Kendala keras (konflik, slot diblokir, sebaran hari) ditegakkan saat penyisipan.
type ScheduleMatrix struct {
	slots map[string][]Slot

	classBoard   map[uint]map[string][]MatrixCell
	teacherBoard map[uint]map[string][]MatrixCell

	blockSet    map[uint]MatrixBlock
	placed      map[uint]BlockRecord
	courseDays  map[courseDayKey]map[string]int // (kelas,mapel) → hari → jumlah
	diversityOn bool
	exemptions  map[uint]bool // mapel yang dikecualikan dari kendala sebaran hari
}

// ── Metode baca ───────────────────────────────────────────────────────────────

// ClassCell mengembalikan sel grid kelas pada hari dan indeks slot tertentu.
func (s *ScheduleMatrix) ClassCell(classID uint, day string, slotIndex int) (MatrixCell, bool) {
	rows, ok := s.classBoard[classID]
	if !ok {
		return MatrixCell{}, false
	}
	return lookupCell(rows, day, slotIndex)
}

// TeacherCell mengembalikan sel grid guru pada hari dan indeks slot tertentu.
func (s *ScheduleMatrix) TeacherCell(teacherID uint, day string, slotIndex int) (MatrixCell, bool) {
	rows, ok := s.teacherBoard[teacherID]
	if !ok {
		return MatrixCell{}, false
	}
	return lookupCell(rows, day, slotIndex)
}

// Placement mengembalikan BlockRecord untuk blok yang sudah ditempatkan, atau false jika belum.
func (s *ScheduleMatrix) Placement(blockID uint) (BlockRecord, bool) {
	rec, ok := s.placed[blockID]
	return rec, ok
}

// PlacedCount mengembalikan jumlah blok yang saat ini sudah ditempatkan di grid.
func (s *ScheduleMatrix) PlacedCount() int {
	return len(s.placed)
}

// ── Metode tulis ──────────────────────────────────────────────────────────────

// CanPlaceBlock memeriksa apakah blok dapat ditempatkan secara legal pada (day, startSlot)
// tanpa melakukan penempatan sebenarnya.
func (s *ScheduleMatrix) CanPlaceBlock(blockID uint, day string, startSlot int) error {
	if _, alreadyPlaced := s.placed[blockID]; alreadyPlaced {
		return fmt.Errorf("block %d is already placed", blockID)
	}
	block, ok := s.blockSet[blockID]
	if !ok {
		return fmt.Errorf("unknown block %d", blockID)
	}
	return s.checkPlacement(block, day, startSlot)
}

// PlaceBlock menempatkan blok pada (day, startSlot), mengembalikan error jika posisi tidak valid.
func (s *ScheduleMatrix) PlaceBlock(blockID uint, day string, startSlot int) error {
	if err := s.CanPlaceBlock(blockID, day, startSlot); err != nil {
		return err
	}
	block := s.blockSet[blockID]
	rec := BlockRecord{
		BlockID:   block.ID,
		ClassID:   block.ClassID,
		TeacherID: block.TeacherID,
		Day:       day,
		StartSlot: startSlot,
		Duration:  block.Duration,
	}
	s.applyPlacement(rec)
	return nil
}

// RemoveBlock mengangkat blok yang sudah ditempatkan dari grid.
func (s *ScheduleMatrix) RemoveBlock(blockID uint) error {
	rec, ok := s.placed[blockID]
	if !ok {
		return fmt.Errorf("block %d is not placed", blockID)
	}
	s.removePlacement(rec)
	delete(s.placed, blockID)
	return nil
}

// MoveBlock memindahkan blok yang sudah ditempatkan ke posisi baru, atau menempatkannya jika belum ada.
func (s *ScheduleMatrix) MoveBlock(blockID uint, day string, startSlot int) error {
	oldRec, wasPlaced := s.placed[blockID]
	if wasPlaced {
		s.removePlacement(oldRec)
		delete(s.placed, blockID)
	}
	if err := s.PlaceBlock(blockID, day, startSlot); err != nil {
		if wasPlaced {
			s.applyPlacement(oldRec)
		}
		return err
	}
	return nil
}

// ── Sebaran hari dan validasi ─────────────────────────────────────────────────

// EnableDayDiversity mengaktifkan kendala bahwa blok (kelas, mapel) yang sama
// harus dijadwalkan pada hari berbeda.
func (s *ScheduleMatrix) EnableDayDiversity() {
	s.diversityOn = true
}

// ExcludeSubjectFromDayDiversity mengecualikan mapel dari kendala sebaran hari.
// PJOK dikecualikan karena blok 2JP (praktik) dan 1JP (teori) berbagi ID mapel
// namun boleh jatuh pada hari yang sama.
func (s *ScheduleMatrix) ExcludeSubjectFromDayDiversity(subjectID uint) {
	if s.exemptions == nil {
		s.exemptions = make(map[uint]bool)
	}
	s.exemptions[subjectID] = true
}

// ValidateIntegrity melakukan pemeriksaan konsistensi penuh pada semua penempatan dan sel grid.
func (s *ScheduleMatrix) ValidateIntegrity() error {
	for blockID, rec := range s.placed {
		block, ok := s.blockSet[blockID]
		if !ok {
			return fmt.Errorf("placement references unknown block %d", blockID)
		}
		if block.ClassID != rec.ClassID {
			return fmt.Errorf("placement class mismatch for block %d", blockID)
		}
		if block.Duration != rec.Duration {
			return fmt.Errorf("placement duration mismatch for block %d", blockID)
		}
		if err := s.verifyPlacementCells(rec); err != nil {
			return err
		}
	}

	for classID, dayRows := range s.classBoard {
		for day, row := range dayRows {
			for slotIdx, cell := range row {
				if cell.State != FilledCell {
					continue
				}
				if err := s.verifyClassCell(classID, day, slotIdx, cell); err != nil {
					return err
				}
			}
		}
	}

	for teacherID, dayRows := range s.teacherBoard {
		for day, row := range dayRows {
			for slotIdx, cell := range row {
				if cell.State != FilledCell {
					continue
				}
				if err := s.verifyTeacherCell(teacherID, day, slotIdx, cell); err != nil {
					return err
				}
			}
		}
	}

	return nil
}

// ── Konstruktor ───────────────────────────────────────────────────────────────

// NewScheduleMatrix membuat grid kendala kosong yang sudah terisi kelas, guru, dan blok.
// Jika daySlots nil, digunakan jadwal mingguan default.
func NewScheduleMatrix(classes []uint, teachers []uint, blocks []MatrixBlock, daySlots DaySlots) *ScheduleMatrix {
	if daySlots == nil {
		daySlots = GenerateSlots()
	}
	s := &ScheduleMatrix{
		slots:        cloneSlots(daySlots),
		classBoard:   make(map[uint]map[string][]MatrixCell),
		teacherBoard: make(map[uint]map[string][]MatrixCell),
		blockSet:     make(map[uint]MatrixBlock, len(blocks)),
		placed:       make(map[uint]BlockRecord),
		courseDays:   make(map[courseDayKey]map[string]int),
	}
	for _, id := range classes {
		s.initClassRows(id)
	}
	for _, id := range teachers {
		s.initTeacherRows(id)
	}
	for _, blk := range blocks {
		s.blockSet[blk.ID] = blk
		s.initClassRows(blk.ClassID)
		if blk.TeacherID != nil {
			s.initTeacherRows(*blk.TeacherID)
		}
	}
	return s
}

// ── Helper privat ─────────────────────────────────────────────────────────────

// checkPlacement memvalidasi semua kendala untuk menempatkan blok pada (day, startSlot).
func (s *ScheduleMatrix) checkPlacement(block MatrixBlock, day string, startSlot int) error {
	if block.Duration <= 0 {
		return fmt.Errorf("block %d has invalid duration %d", block.ID, block.Duration)
	}
	if _, ok := s.slots[day]; !ok {
		return fmt.Errorf("unknown day %q", day)
	}
	s.initClassRows(block.ClassID)
	if block.TeacherID != nil {
		s.initTeacherRows(*block.TeacherID)
	}

	classRow := s.classBoard[block.ClassID][day]
	if startSlot < 0 || startSlot+block.Duration > len(classRow) {
		return fmt.Errorf("block %d does not fit at %s slot %d", block.ID, day, startSlot)
	}
	for offset := 0; offset < block.Duration; offset++ {
		idx := startSlot + offset
		if classRow[idx].State != EmptyCell {
			return fmt.Errorf("class %d is not free at %s slot %d", block.ClassID, day, idx)
		}
		if block.TeacherID != nil {
			if s.teacherBoard[*block.TeacherID][day][idx].State != EmptyCell {
				return fmt.Errorf("teacher %d is not free at %s slot %d", *block.TeacherID, day, idx)
			}
		}
	}

	return s.checkDaySpread(block, day)
}

// checkDaySpread menegakkan bahwa blok (kelas, mapel) yang sama tidak jatuh pada hari yang sama,
// kecuali mapel tersebut dikecualikan.
func (s *ScheduleMatrix) checkDaySpread(block MatrixBlock, day string) error {
	if !s.diversityOn || s.exemptions[block.SubjectID] {
		return nil
	}
	key := courseDayKey{block.ClassID, block.SubjectID}
	if s.courseDays[key][day] > 0 {
		return fmt.Errorf("class %d already has subject %d on %s", block.ClassID, block.SubjectID, day)
	}
	return nil
}

func (s *ScheduleMatrix) applyPlacement(rec BlockRecord) {
	cell := MatrixCell{State: FilledCell, BlockID: rec.BlockID}
	for offset := 0; offset < rec.Duration; offset++ {
		idx := rec.StartSlot + offset
		s.classBoard[rec.ClassID][rec.Day][idx] = cell
		if rec.TeacherID != nil {
			s.teacherBoard[*rec.TeacherID][rec.Day][idx] = cell
		}
	}
	s.placed[rec.BlockID] = rec
	if s.diversityOn {
		if blk, ok := s.blockSet[rec.BlockID]; ok {
			key := courseDayKey{blk.ClassID, blk.SubjectID}
			if s.courseDays[key] == nil {
				s.courseDays[key] = make(map[string]int)
			}
			s.courseDays[key][rec.Day]++
		}
	}
}

func (s *ScheduleMatrix) removePlacement(rec BlockRecord) {
	empty := MatrixCell{State: EmptyCell}
	for offset := 0; offset < rec.Duration; offset++ {
		idx := rec.StartSlot + offset
		s.classBoard[rec.ClassID][rec.Day][idx] = empty
		if rec.TeacherID != nil {
			s.teacherBoard[*rec.TeacherID][rec.Day][idx] = empty
		}
	}
	if s.diversityOn {
		if blk, ok := s.blockSet[rec.BlockID]; ok {
			key := courseDayKey{blk.ClassID, blk.SubjectID}
			if m := s.courseDays[key]; m != nil {
				if m[rec.Day]--; m[rec.Day] == 0 {
					delete(m, rec.Day)
				}
			}
		}
	}
}

func (s *ScheduleMatrix) initClassRows(classID uint) {
	if _, exists := s.classBoard[classID]; !exists {
		s.classBoard[classID] = s.buildGridRows()
	}
}

func (s *ScheduleMatrix) initTeacherRows(teacherID uint) {
	if _, exists := s.teacherBoard[teacherID]; !exists {
		s.teacherBoard[teacherID] = s.buildGridRows()
	}
}

func (s *ScheduleMatrix) buildGridRows() map[string][]MatrixCell {
	rows := make(map[string][]MatrixCell, len(MatrixDays))
	for _, day := range MatrixDays {
		rows[day] = buildRow(s.slots[day])
	}
	return rows
}

func (s *ScheduleMatrix) verifyPlacementCells(rec BlockRecord) error {
	for offset := 0; offset < rec.Duration; offset++ {
		idx := rec.StartSlot + offset
		if s.slotIsBlocked(rec.Day, idx) {
			return fmt.Errorf("block %d is placed on blocked slot %s %d", rec.BlockID, rec.Day, idx)
		}
		classCell, ok := s.ClassCell(rec.ClassID, rec.Day, idx)
		if !ok || classCell.State != FilledCell || classCell.BlockID != rec.BlockID {
			return fmt.Errorf("class grid missing block %d at %s slot %d", rec.BlockID, rec.Day, idx)
		}
		if rec.TeacherID == nil {
			continue
		}
		teacherCell, ok := s.TeacherCell(*rec.TeacherID, rec.Day, idx)
		if !ok || teacherCell.State != FilledCell || teacherCell.BlockID != rec.BlockID {
			return fmt.Errorf("teacher grid missing block %d at %s slot %d", rec.BlockID, rec.Day, idx)
		}
	}
	return nil
}

func (s *ScheduleMatrix) verifyClassCell(classID uint, day string, slotIdx int, cell MatrixCell) error {
	rec, ok := s.placed[cell.BlockID]
	if !ok {
		return fmt.Errorf("class grid has untracked block %d at %s slot %d", cell.BlockID, day, slotIdx)
	}
	if rec.ClassID != classID || rec.Day != day {
		return fmt.Errorf("class grid block %d disagrees with placement metadata", cell.BlockID)
	}
	if slotIdx < rec.StartSlot || slotIdx >= rec.StartSlot+rec.Duration {
		return fmt.Errorf("class grid block %d is outside its placement window", cell.BlockID)
	}
	return nil
}

func (s *ScheduleMatrix) verifyTeacherCell(teacherID uint, day string, slotIdx int, cell MatrixCell) error {
	rec, ok := s.placed[cell.BlockID]
	if !ok {
		return fmt.Errorf("teacher grid has untracked block %d at %s slot %d", cell.BlockID, day, slotIdx)
	}
	if rec.TeacherID == nil || *rec.TeacherID != teacherID || rec.Day != day {
		return fmt.Errorf("teacher grid block %d disagrees with placement metadata", cell.BlockID)
	}
	if slotIdx < rec.StartSlot || slotIdx >= rec.StartSlot+rec.Duration {
		return fmt.Errorf("teacher grid block %d is outside its placement window", cell.BlockID)
	}
	return nil
}

func (s *ScheduleMatrix) slotIsBlocked(day string, slotIndex int) bool {
	for _, s := range s.slots[day] {
		if s.Index == slotIndex {
			return s.IsBlocked
		}
	}
	return true
}

// lookupCell mengambil sel dari peta day→row berdasarkan hari dan indeks slot.
func lookupCell(rows map[string][]MatrixCell, day string, slotIndex int) (MatrixCell, bool) {
	row, ok := rows[day]
	if !ok || slotIndex < 0 || slotIndex >= len(row) {
		return MatrixCell{}, false
	}
	return row[slotIndex], true
}

// buildRow mengubah daftar slot menjadi baris MatrixCell, menandai setiap sel diblokir atau kosong.
func buildRow(slots []Slot) []MatrixCell {
	maxIdx := -1
	for _, s := range slots {
		if s.Index > maxIdx {
			maxIdx = s.Index
		}
	}
	if maxIdx < 0 {
		return nil
	}
	row := make([]MatrixCell, maxIdx+1)
	for i := range row {
		row[i] = MatrixCell{State: BlockedCell}
	}
	for _, s := range slots {
		if s.IsBlocked {
			row[s.Index] = MatrixCell{State: BlockedCell}
		} else {
			row[s.Index] = MatrixCell{State: EmptyCell}
		}
	}
	return row
}

// cloneSlots melakukan salinan dalam dari peta DaySlots.
func cloneSlots(src DaySlots) map[string][]Slot {
	dst := make(map[string][]Slot, len(src))
	for day, periods := range src {
		cp := make([]Slot, len(periods))
		copy(cp, periods)
		dst[day] = cp
	}
	return dst
}
