package algorithm

import (
	"fmt"
)

type CellState uint8

const (
	CellEmpty CellState = iota
	CellBlocked
	CellOccupied
)

type MatrixCell struct {
	State   CellState
	BlockID uint
}

type BlockPlacement struct {
	BlockID   uint
	ClassID   uint
	TeacherID *uint
	Day       string
	StartSlot int
	Duration  int
}

type subjectDayKey struct {
	classID   uint
	subjectID uint
}

type ScheduleMatrix struct {
	daySlots map[string][]Slot

	classGrid   map[uint]map[string][]MatrixCell
	teacherGrid map[uint]map[string][]MatrixCell

	blocks      map[uint]MatrixBlock
	placements  map[uint]BlockPlacement
	subjectDays              map[subjectDayKey]map[string]int // (class,subject) → day → count of placed blocks
	enforceDayDiversity      bool
	dayDiversityExclusions   map[uint]bool // subjects exempt from the day-diversity hard constraint
}

// EnableDayDiversity turns on the same-subject-same-day hard constraint.
// Call this on matrices used for schedule generation (not real-schedule validation).
func (s *ScheduleMatrix) EnableDayDiversity() {
	s.enforceDayDiversity = true
}

// ExcludeSubjectFromDayDiversity exempts a subject from the day-diversity hard
// constraint. Use this for PJOK: its 2JP (practice) and 1JP (theory) blocks
// share the same (class, subject) but may legally land on the same day, as the
// manual schedule shows. Must be called after EnableDayDiversity.
func (s *ScheduleMatrix) ExcludeSubjectFromDayDiversity(subjectID uint) {
	if s.dayDiversityExclusions == nil {
		s.dayDiversityExclusions = make(map[uint]bool)
	}
	s.dayDiversityExclusions[subjectID] = true
}

func NewScheduleMatrix(classes []uint, teachers []uint, blocks []MatrixBlock, daySlots DaySlots) *ScheduleMatrix {
	if daySlots == nil {
		daySlots = GenerateSlots()
	}

	s := &ScheduleMatrix{
		daySlots:    copyDaySlots(daySlots),
		classGrid:   make(map[uint]map[string][]MatrixCell),
		teacherGrid: make(map[uint]map[string][]MatrixCell),
		blocks:      make(map[uint]MatrixBlock, len(blocks)),
		placements:  make(map[uint]BlockPlacement),
		subjectDays: make(map[subjectDayKey]map[string]int),
	}

	for _, classID := range classes {
		s.ensureClassGrid(classID)
	}
	for _, teacherID := range teachers {
		s.ensureTeacherGrid(teacherID)
	}
	for _, block := range blocks {
		s.blocks[block.ID] = block
		s.ensureClassGrid(block.ClassID)
		if block.TeacherID != nil {
			s.ensureTeacherGrid(*block.TeacherID)
		}
	}

	return s
}

func (s *ScheduleMatrix) CanPlaceBlock(blockID uint, day string, startSlot int) error {
	if _, placed := s.placements[blockID]; placed {
		return fmt.Errorf("block %d is already placed", blockID)
	}

	block, ok := s.blocks[blockID]
	if !ok {
		return fmt.Errorf("unknown block %d", blockID)
	}

	return s.canPlace(block, day, startSlot)
}

func (s *ScheduleMatrix) PlaceBlock(blockID uint, day string, startSlot int) error {
	if err := s.CanPlaceBlock(blockID, day, startSlot); err != nil {
		return err
	}

	block := s.blocks[blockID]
	placement := BlockPlacement{
		BlockID:   block.ID,
		ClassID:   block.ClassID,
		TeacherID: block.TeacherID,
		Day:       day,
		StartSlot: startSlot,
		Duration:  block.Duration,
	}
	s.writePlacement(placement)
	return nil
}

func (s *ScheduleMatrix) RemoveBlock(blockID uint) error {
	placement, ok := s.placements[blockID]
	if !ok {
		return fmt.Errorf("block %d is not placed", blockID)
	}

	s.clearPlacement(placement)
	delete(s.placements, blockID)
	return nil
}

func (s *ScheduleMatrix) MoveBlock(blockID uint, day string, startSlot int) error {
	oldPlacement, wasPlaced := s.placements[blockID]
	if wasPlaced {
		s.clearPlacement(oldPlacement)
		delete(s.placements, blockID)
	}

	if err := s.PlaceBlock(blockID, day, startSlot); err != nil {
		if wasPlaced {
			s.writePlacement(oldPlacement)
		}
		return err
	}

	return nil
}

func (s *ScheduleMatrix) ClassCell(classID uint, day string, slotIndex int) (MatrixCell, bool) {
	rows, ok := s.classGrid[classID]
	if !ok {
		return MatrixCell{}, false
	}
	return cellFromRows(rows, day, slotIndex)
}

func (s *ScheduleMatrix) TeacherCell(teacherID uint, day string, slotIndex int) (MatrixCell, bool) {
	rows, ok := s.teacherGrid[teacherID]
	if !ok {
		return MatrixCell{}, false
	}
	return cellFromRows(rows, day, slotIndex)
}

func (s *ScheduleMatrix) Placement(blockID uint) (BlockPlacement, bool) {
	placement, ok := s.placements[blockID]
	return placement, ok
}

func (s *ScheduleMatrix) PlacedCount() int {
	return len(s.placements)
}

func (s *ScheduleMatrix) ValidateIntegrity() error {
	for blockID, placement := range s.placements {
		block, ok := s.blocks[blockID]
		if !ok {
			return fmt.Errorf("placement references unknown block %d", blockID)
		}
		if block.ClassID != placement.ClassID {
			return fmt.Errorf("placement class mismatch for block %d", blockID)
		}
		if block.Duration != placement.Duration {
			return fmt.Errorf("placement duration mismatch for block %d", blockID)
		}
		if err := s.validatePlacementCells(placement); err != nil {
			return err
		}
	}

	for classID, dayRows := range s.classGrid {
		for day, row := range dayRows {
			for slotIndex, cell := range row {
				if cell.State != CellOccupied {
					continue
				}
				if err := s.validateOccupiedClassCell(classID, day, slotIndex, cell); err != nil {
					return err
				}
			}
		}
	}

	for teacherID, dayRows := range s.teacherGrid {
		for day, row := range dayRows {
			for slotIndex, cell := range row {
				if cell.State != CellOccupied {
					continue
				}
				if err := s.validateOccupiedTeacherCell(teacherID, day, slotIndex, cell); err != nil {
					return err
				}
			}
		}
	}

	return nil
}

func (s *ScheduleMatrix) canPlace(block MatrixBlock, day string, startSlot int) error {
	if block.Duration <= 0 {
		return fmt.Errorf("block %d has invalid duration %d", block.ID, block.Duration)
	}
	if _, ok := s.daySlots[day]; !ok {
		return fmt.Errorf("unknown day %q", day)
	}

	s.ensureClassGrid(block.ClassID)
	if block.TeacherID != nil {
		s.ensureTeacherGrid(*block.TeacherID)
	}

	classRow := s.classGrid[block.ClassID][day]
	if startSlot < 0 || startSlot+block.Duration > len(classRow) {
		return fmt.Errorf("block %d does not fit at %s slot %d", block.ID, day, startSlot)
	}

	for offset := 0; offset < block.Duration; offset++ {
		slotIndex := startSlot + offset
		classCell := classRow[slotIndex]
		if classCell.State != CellEmpty {
			return fmt.Errorf("class %d is not free at %s slot %d", block.ClassID, day, slotIndex)
		}

		if block.TeacherID == nil {
			continue
		}
		teacherCell := s.teacherGrid[*block.TeacherID][day][slotIndex]
		if teacherCell.State != CellEmpty {
			return fmt.Errorf("teacher %d is not free at %s slot %d", *block.TeacherID, day, slotIndex)
		}
	}

	// Day-diversity: blocks of the same (class, subject) must be on different days.
	// Excluded subjects (e.g. PJOK, whose 2JP + 1JP may land on the same day) skip this check.
	if s.enforceDayDiversity && !s.dayDiversityExclusions[block.SubjectID] {
		sdKey := subjectDayKey{block.ClassID, block.SubjectID}
		if s.subjectDays[sdKey][day] > 0 {
			return fmt.Errorf("class %d already has subject %d on %s", block.ClassID, block.SubjectID, day)
		}
	}

	return nil
}

func (s *ScheduleMatrix) writePlacement(placement BlockPlacement) {
	cell := MatrixCell{State: CellOccupied, BlockID: placement.BlockID}
	for offset := 0; offset < placement.Duration; offset++ {
		slotIndex := placement.StartSlot + offset
		s.classGrid[placement.ClassID][placement.Day][slotIndex] = cell
		if placement.TeacherID != nil {
			s.teacherGrid[*placement.TeacherID][placement.Day][slotIndex] = cell
		}
	}
	s.placements[placement.BlockID] = placement

	if s.enforceDayDiversity {
		if b, ok := s.blocks[placement.BlockID]; ok {
			sdKey := subjectDayKey{b.ClassID, b.SubjectID}
			if s.subjectDays[sdKey] == nil {
				s.subjectDays[sdKey] = make(map[string]int)
			}
			s.subjectDays[sdKey][placement.Day]++
		}
	}
}

func (s *ScheduleMatrix) clearPlacement(placement BlockPlacement) {
	empty := MatrixCell{State: CellEmpty}
	for offset := 0; offset < placement.Duration; offset++ {
		slotIndex := placement.StartSlot + offset
		s.classGrid[placement.ClassID][placement.Day][slotIndex] = empty
		if placement.TeacherID != nil {
			s.teacherGrid[*placement.TeacherID][placement.Day][slotIndex] = empty
		}
	}

	if s.enforceDayDiversity {
		if b, ok := s.blocks[placement.BlockID]; ok {
			sdKey := subjectDayKey{b.ClassID, b.SubjectID}
			if m := s.subjectDays[sdKey]; m != nil {
				if m[placement.Day]--; m[placement.Day] == 0 {
					delete(m, placement.Day)
				}
			}
		}
	}
}

func (s *ScheduleMatrix) ensureClassGrid(classID uint) {
	if _, ok := s.classGrid[classID]; ok {
		return
	}
	s.classGrid[classID] = s.newGridRows()
}

func (s *ScheduleMatrix) ensureTeacherGrid(teacherID uint) {
	if _, ok := s.teacherGrid[teacherID]; ok {
		return
	}
	s.teacherGrid[teacherID] = s.newGridRows()
}

func (s *ScheduleMatrix) newGridRows() map[string][]MatrixCell {
	rows := make(map[string][]MatrixCell, len(MatrixDays))
	for _, day := range MatrixDays {
		rows[day] = newMatrixRow(s.daySlots[day])
	}
	return rows
}

func (s *ScheduleMatrix) validatePlacementCells(placement BlockPlacement) error {
	for offset := 0; offset < placement.Duration; offset++ {
		slotIndex := placement.StartSlot + offset
		if s.isBlockedSlot(placement.Day, slotIndex) {
			return fmt.Errorf("block %d is placed on blocked slot %s %d", placement.BlockID, placement.Day, slotIndex)
		}

		classCell, ok := s.ClassCell(placement.ClassID, placement.Day, slotIndex)
		if !ok || classCell.State != CellOccupied || classCell.BlockID != placement.BlockID {
			return fmt.Errorf("class grid missing block %d at %s slot %d", placement.BlockID, placement.Day, slotIndex)
		}

		if placement.TeacherID == nil {
			continue
		}
		teacherCell, ok := s.TeacherCell(*placement.TeacherID, placement.Day, slotIndex)
		if !ok || teacherCell.State != CellOccupied || teacherCell.BlockID != placement.BlockID {
			return fmt.Errorf("teacher grid missing block %d at %s slot %d", placement.BlockID, placement.Day, slotIndex)
		}
	}
	return nil
}

func (s *ScheduleMatrix) validateOccupiedClassCell(classID uint, day string, slotIndex int, cell MatrixCell) error {
	placement, ok := s.placements[cell.BlockID]
	if !ok {
		return fmt.Errorf("class grid has untracked block %d at %s slot %d", cell.BlockID, day, slotIndex)
	}
	if placement.ClassID != classID || placement.Day != day {
		return fmt.Errorf("class grid block %d disagrees with placement metadata", cell.BlockID)
	}
	if slotIndex < placement.StartSlot || slotIndex >= placement.StartSlot+placement.Duration {
		return fmt.Errorf("class grid block %d is outside its placement window", cell.BlockID)
	}
	return nil
}

func (s *ScheduleMatrix) validateOccupiedTeacherCell(teacherID uint, day string, slotIndex int, cell MatrixCell) error {
	placement, ok := s.placements[cell.BlockID]
	if !ok {
		return fmt.Errorf("teacher grid has untracked block %d at %s slot %d", cell.BlockID, day, slotIndex)
	}
	if placement.TeacherID == nil || *placement.TeacherID != teacherID || placement.Day != day {
		return fmt.Errorf("teacher grid block %d disagrees with placement metadata", cell.BlockID)
	}
	if slotIndex < placement.StartSlot || slotIndex >= placement.StartSlot+placement.Duration {
		return fmt.Errorf("teacher grid block %d is outside its placement window", cell.BlockID)
	}
	return nil
}

func (s *ScheduleMatrix) isBlockedSlot(day string, slotIndex int) bool {
	for _, slot := range s.daySlots[day] {
		if slot.Index == slotIndex {
			return slot.IsBlocked
		}
	}
	return true
}

func cellFromRows(rows map[string][]MatrixCell, day string, slotIndex int) (MatrixCell, bool) {
	row, ok := rows[day]
	if !ok || slotIndex < 0 || slotIndex >= len(row) {
		return MatrixCell{}, false
	}
	return row[slotIndex], true
}

func newMatrixRow(slots []Slot) []MatrixCell {
	maxIndex := -1
	for _, slot := range slots {
		if slot.Index > maxIndex {
			maxIndex = slot.Index
		}
	}
	if maxIndex < 0 {
		return nil
	}

	row := make([]MatrixCell, maxIndex+1)
	for i := range row {
		row[i] = MatrixCell{State: CellBlocked}
	}
	for _, slot := range slots {
		if slot.IsBlocked {
			row[slot.Index] = MatrixCell{State: CellBlocked}
		} else {
			row[slot.Index] = MatrixCell{State: CellEmpty}
		}
	}
	return row
}

func copyDaySlots(src DaySlots) map[string][]Slot {
	dst := make(map[string][]Slot, len(src))
	for day, slots := range src {
		copied := make([]Slot, len(slots))
		copy(copied, slots)
		dst[day] = copied
	}
	return dst
}
