package algorithm

import (
	"fmt"
	"sort"

	basealgo "smp_mater_dei_be/internal/algorithm"

	"github.com/google/uuid"
)

// BuildSeedChromosome constructs a chromosome whose slots match the real schedule.
//
// Parameters:
// - units:      placement units passed to RunGA (same order)
// - validSlots: map from GenerateValidSlots
// - teacherMap: teacher_number (string) -> uuid.UUID
// - classMap:   class name (string) -> uuid.UUID
//
// Returns the seed chromosome and warnings for units that could not be matched.
func BuildSeedChromosome(
	units []PlacementUnit,
	validSlots map[uuid.UUID][]ValidSlot,
	teacherMap map[string]uuid.UUID,
	classMap map[string]uuid.UUID,
) (*GAChromosome, []string) {

	type scheduleKey struct {
		teacherID uuid.UUID
		classID   uuid.UUID
		subjectID uuid.UUID
	}

	teacherSubject := buildTeacherSubjectMap(units)

	lookup := make(map[scheduleKey][]basealgo.RealScheduleEntry)
	// realRowStarts[key][day][slotIndex] = exists
	realRowStarts := make(map[scheduleKey]map[string]map[int]struct{})
	keyRowCount := make(map[scheduleKey]int)

	for _, entry := range basealgo.RealSchedule {
		tID, tOK := teacherMap[entry.TeacherNumber]
		cID, cOK := classMap[entry.ClassName]
		if !tOK || !cOK {
			continue
		}
		sID, sOK := teacherSubject[tID]
		if !sOK {
			continue
		}
		slotIdx, slotOK := timeStartToSlot[entry.Day][entry.TimeStart]
		if !slotOK {
			continue
		}
		k := scheduleKey{teacherID: tID, classID: cID, subjectID: sID}
		lookup[k] = append(lookup[k], entry)
		if realRowStarts[k] == nil {
			realRowStarts[k] = make(map[string]map[int]struct{})
		}
		if realRowStarts[k][entry.Day] == nil {
			realRowStarts[k][entry.Day] = make(map[int]struct{})
		}
		realRowStarts[k][entry.Day][slotIdx] = struct{}{}
		keyRowCount[k]++
	}

	for key := range lookup {
		sort.SliceStable(lookup[key], func(i, j int) bool {
			di := dayOrder(lookup[key][i].Day)
			dj := dayOrder(lookup[key][j].Day)
			if di != dj {
				return di < dj
			}
			si := timeStartToSlot[lookup[key][i].Day][lookup[key][i].TimeStart]
			sj := timeStartToSlot[lookup[key][j].Day][lookup[key][j].TimeStart]
			return si < sj
		})
	}

	keyUnitCount := make(map[scheduleKey]int)
	for _, unit := range units {
		if unit.IsParallel || unit.Block == nil || unit.Block.TeacherID == nil {
			continue
		}
		k := scheduleKey{teacherID: *unit.Block.TeacherID, classID: unit.Block.ClassID, subjectID: unit.Block.SubjectID}
		keyUnitCount[k]++
	}

	// If a key has more real rows than units, rows are per-period records.
	keyUsesPeriodRows := make(map[scheduleKey]bool)
	for k, rowCount := range keyRowCount {
		if rowCount > keyUnitCount[k] {
			keyUsesPeriodRows[k] = true
		}
	}

	sbpSlotLookup := make(map[string]ValidSlot)
	for groupKey, real := range basealgo.RealSBPSlots {
		slotIdx := SlotIndexFromTimeStart(real.Day, real.TimeStart)
		if slotIdx < 0 {
			continue
		}
		sbpSlotLookup[groupKey] = ValidSlot{Day: real.Day, SlotIndex: slotIdx}
	}

	ch := &GAChromosome{UnitSlots: make([]ValidSlot, len(units))}
	var warnings []string

	type occupiedInterval struct {
		startSlot int
		endSlot   int // inclusive: startSlot + jp - 1
		unitIdx   int
	}

	type seedState struct {
		usedRealEntry       map[scheduleKey]map[int]struct{}
		usedRealPeriodSlots map[scheduleKey]map[string]map[int]struct{}
		classOcc            map[string][]occupiedInterval
		teacherOcc          map[string][]occupiedInterval
		siblingDays         map[uuid.UUID]map[string]struct{}
	}

	state := &seedState{
		usedRealEntry:       make(map[scheduleKey]map[int]struct{}),
		usedRealPeriodSlots: make(map[scheduleKey]map[string]map[int]struct{}),
		classOcc:            make(map[string][]occupiedInterval),
		teacherOcc:          make(map[string][]occupiedInterval),
		siblingDays:         make(map[uuid.UUID]map[string]struct{}),
	}

	// periodSlotsForBlock returns the slot indices occupied by a jp-period block
	// starting at timeStart on day.
	periodSlotsForBlock := func(day, timeStart string, jp int) []int {
		startSlot, ok := timeStartToSlot[day][timeStart]
		if !ok {
			return nil
		}
		slots := make([]int, jp)
		for i := range jp {
			slots[i] = startSlot + i
		}
		return slots
	}

	periodSlotsExist := func(key scheduleKey, day string, slots []int) bool {
		daySlots, ok := realRowStarts[key][day]
		if !ok {
			return false
		}
		for _, s := range slots {
			if _, exists := daySlots[s]; !exists {
				return false
			}
		}
		return true
	}

	periodSlotsUsed := func(st *seedState, key scheduleKey, day string, slots []int) bool {
		keyUsed := st.usedRealPeriodSlots[key]
		if keyUsed == nil {
			return false
		}
		dayUsed := keyUsed[day]
		if dayUsed == nil {
			return false
		}
		for _, s := range slots {
			if _, used := dayUsed[s]; used {
				return true
			}
		}
		return false
	}

	markPeriodSlotsUsed := func(st *seedState, key scheduleKey, day string, slots []int) {
		if st.usedRealPeriodSlots[key] == nil {
			st.usedRealPeriodSlots[key] = make(map[string]map[int]struct{})
		}
		if st.usedRealPeriodSlots[key][day] == nil {
			st.usedRealPeriodSlots[key][day] = make(map[int]struct{})
		}
		for _, s := range slots {
			st.usedRealPeriodSlots[key][day][s] = struct{}{}
		}
	}

	markPlaced := func(st *seedState, unitIdx int, unit PlacementUnit, slot ValidSlot) {
		if slot.Day == "" {
			return
		}
		jp := unitMaxJP(unit)
		iv := occupiedInterval{
			startSlot: slot.SlotIndex,
			endSlot:   slot.SlotIndex + jp - 1,
			unitIdx:   unitIdx,
		}

		for _, b := range unitBlocks(unit) {
			ck := slot.Day + "|" + b.ClassID.String()
			st.classOcc[ck] = append(st.classOcc[ck], iv)

			if b.TeacherID != nil {
				tk := teacherOccupancyKey(slot.Day, b.TeacherID)
				st.teacherOcc[tk] = append(st.teacherOcc[tk], iv)
			}

			if b.SiblingGroupID != nil {
				if st.siblingDays[*b.SiblingGroupID] == nil {
					st.siblingDays[*b.SiblingGroupID] = map[string]struct{}{}
				}
				st.siblingDays[*b.SiblingGroupID][slot.Day] = struct{}{}
			}
		}
	}

	slotConflictScore := func(st *seedState, unit PlacementUnit, slot ValidSlot) int {
		if slot.Day == "" {
			return 1_000_000
		}

		jp := unitMaxJP(unit)
		start := slot.SlotIndex
		end := start + jp - 1
		score := 0

		for _, b := range unitBlocks(unit) {
			ck := slot.Day + "|" + b.ClassID.String()
			for _, iv := range st.classOcc[ck] {
				if start <= iv.endSlot && iv.startSlot <= end {
					score += 10
				}
			}

			if b.TeacherID != nil {
				tk := teacherOccupancyKey(slot.Day, b.TeacherID)
				for _, iv := range st.teacherOcc[tk] {
					if start <= iv.endSlot && iv.startSlot <= end {
						score += 10
					}
				}
			}

			if b.SiblingGroupID != nil {
				if days := st.siblingDays[*b.SiblingGroupID]; days != nil {
					if _, exists := days[slot.Day]; exists {
						score += 8
					}
				}
			}
		}

		return score
	}

	pickBestCandidate := func(st *seedState, unit PlacementUnit, candidates []ValidSlot) (ValidSlot, int, bool) {
		if len(candidates) == 0 {
			return ValidSlot{}, 0, false
		}

		best := candidates[0]
		bestScore := slotConflictScore(st, unit, best)
		for _, cand := range candidates[1:] {
			score := slotConflictScore(st, unit, cand)
			if score < bestScore {
				best = cand
				bestScore = score
				if bestScore == 0 {
					break
				}
			}
		}

		return best, bestScore, true
	}

	placeNormalUnit := func(st *seedState, unitIdx int, apply bool) (int, []string) {
		unit := units[unitIdx]
		block := unit.Block
		if block == nil {
			return 2_000_000, nil
		}

		var unitWarnings []string

		applySlot := func(slot ValidSlot) {
			if apply {
				ch.UnitSlots[unitIdx] = slot
			}
			markPlaced(st, unitIdx, unit, slot)
		}

		if block.TeacherID == nil {
			unitWarnings = append(unitWarnings, fmt.Sprintf(
				"unit[%d]: block %s has nil teacher and no parallel key; fallback to first candidate",
				unitIdx, block.ID.String(),
			))
			if cands := unitCandidates(unit, validSlots); len(cands) > 0 {
				if chosen, score, ok := pickBestCandidate(st, unit, cands); ok {
					applySlot(chosen)
					return 100 + score, unitWarnings
				}
			}
			return 2_000_000, unitWarnings
		}

		key := scheduleKey{teacherID: *block.TeacherID, classID: block.ClassID, subjectID: block.SubjectID}
		entries := lookup[key]
		if len(entries) == 0 {
			unitWarnings = append(unitWarnings, fmt.Sprintf(
				"unit[%d]: no real schedule entry for teacher %s class %s subject %s; fallback to first candidate",
				unitIdx, block.TeacherID.String(), block.ClassID.String(), block.SubjectID.String(),
			))
			if cands := unitCandidates(unit, validSlots); len(cands) > 0 {
				if chosen, score, ok := pickBestCandidate(st, unit, cands); ok {
					applySlot(chosen)
					return 100 + score, unitWarnings
				}
			}
			return 2_000_000, unitWarnings
		}

		if st.usedRealEntry[key] == nil {
			st.usedRealEntry[key] = map[int]struct{}{}
		}
		usePeriodRows := keyUsesPeriodRows[key]

		bestIdx := -1
		bestScore := int(^uint(0) >> 1)
		var bestSlot ValidSlot
		var bestSlots []int

		for idx, entry := range entries {
			if _, used := st.usedRealEntry[key][idx]; used {
				continue
			}

			vs, found := findValidSlot(unit, validSlots, entry.Day, entry.TimeStart)
			if !found {
				continue
			}

			candidateSlots := []int{SlotIndexFromTimeStart(entry.Day, entry.TimeStart)}
			if usePeriodRows {
				candidateSlots = periodSlotsForBlock(entry.Day, entry.TimeStart, block.JP)
				if !periodSlotsExist(key, entry.Day, candidateSlots) {
					continue
				}
			}

			if periodSlotsUsed(st, key, entry.Day, candidateSlots) {
				continue
			}

			score := slotConflictScore(st, unit, vs)
			if score < bestScore {
				bestScore = score
				bestIdx = idx
				bestSlot = vs
				bestSlots = candidateSlots
				if bestScore == 0 {
					break
				}
			}
		}

		if bestIdx >= 0 {
			st.usedRealEntry[key][bestIdx] = struct{}{}
			markPeriodSlotsUsed(st, key, bestSlot.Day, bestSlots)
			applySlot(bestSlot)
			return bestScore, unitWarnings
		}

		unitWarnings = append(unitWarnings, fmt.Sprintf(
			"unit[%d]: real entries for teacher %s class %s subject %s do not match validSlots; fallback to best candidate",
			unitIdx, block.TeacherID.String(), block.ClassID.String(), block.SubjectID.String(),
		))
		if cands := unitCandidates(unit, validSlots); len(cands) > 0 {
			if chosen, score, ok := pickBestCandidate(st, unit, cands); ok {
				applySlot(chosen)
				if score > 0 {
					unitWarnings = append(unitWarnings, fmt.Sprintf(
						"unit[%d]: fallback candidate still has estimated conflict score %d",
						unitIdx, score,
					))
				}
				return 200 + score, unitWarnings
			}
		}

		unitWarnings = append(unitWarnings, fmt.Sprintf(
			"unit[%d]: no candidates available for teacher %s class %s",
			unitIdx, block.TeacherID.String(), block.ClassID.String(),
		))
		return 2_000_000, unitWarnings
	}

	cloneState := func(src *seedState) *seedState {
		dst := &seedState{
			usedRealEntry:       make(map[scheduleKey]map[int]struct{}, len(src.usedRealEntry)),
			usedRealPeriodSlots: make(map[scheduleKey]map[string]map[int]struct{}, len(src.usedRealPeriodSlots)),
			classOcc:            make(map[string][]occupiedInterval, len(src.classOcc)),
			teacherOcc:          make(map[string][]occupiedInterval, len(src.teacherOcc)),
			siblingDays:         make(map[uuid.UUID]map[string]struct{}, len(src.siblingDays)),
		}

		for k, used := range src.usedRealEntry {
			copied := make(map[int]struct{}, len(used))
			for idx := range used {
				copied[idx] = struct{}{}
			}
			dst.usedRealEntry[k] = copied
		}

		for k, dayMap := range src.usedRealPeriodSlots {
			copiedDayMap := make(map[string]map[int]struct{}, len(dayMap))
			for day, slots := range dayMap {
				copiedSlots := make(map[int]struct{}, len(slots))
				for s := range slots {
					copiedSlots[s] = struct{}{}
				}
				copiedDayMap[day] = copiedSlots
			}
			dst.usedRealPeriodSlots[k] = copiedDayMap
		}

		for k, intervals := range src.classOcc {
			copied := make([]occupiedInterval, len(intervals))
			copy(copied, intervals)
			dst.classOcc[k] = copied
		}

		for k, intervals := range src.teacherOcc {
			copied := make([]occupiedInterval, len(intervals))
			copy(copied, intervals)
			dst.teacherOcc[k] = copied
		}

		for k, days := range src.siblingDays {
			copied := make(map[string]struct{}, len(days))
			for day := range days {
				copied[day] = struct{}{}
			}
			dst.siblingDays[k] = copied
		}

		return dst
	}

	parallelFirst := make([]int, 0, len(units))
	normalSecond := make([]int, 0, len(units))
	for i, unit := range units {
		if unit.IsParallel {
			parallelFirst = append(parallelFirst, i)
			continue
		}
		normalSecond = append(normalSecond, i)
	}

	order := append(parallelFirst, normalSecond...)

	flexiblePartner := make(map[int]int)
	siblingUnits := make(map[uuid.UUID][]int)
	for _, idx := range normalSecond {
		unit := units[idx]
		if unit.Block == nil || unit.Block.SiblingGroupID == nil {
			continue
		}
		if unit.Block.JP != 2 && unit.Block.JP != 3 {
			continue
		}
		siblingUnits[*unit.Block.SiblingGroupID] = append(siblingUnits[*unit.Block.SiblingGroupID], idx)
	}
	for _, idxs := range siblingUnits {
		if len(idxs) != 2 {
			continue
		}
		b0 := units[idxs[0]].Block
		b1 := units[idxs[1]].Block
		if b0 == nil || b1 == nil {
			continue
		}
		if (b0.JP == 2 && b1.JP == 3) || (b0.JP == 3 && b1.JP == 2) {
			flexiblePartner[idxs[0]] = idxs[1]
			flexiblePartner[idxs[1]] = idxs[0]
		}
	}

	placedNormal := make(map[int]struct{})

	simulatePairOrder := func(firstIdx, secondIdx int) (int, int) {
		trial := cloneState(state)
		p1, w1 := placeNormalUnit(trial, firstIdx, false)
		p2, w2 := placeNormalUnit(trial, secondIdx, false)
		return p1 + p2, len(w1) + len(w2)
	}

	for _, i := range order {
		unit := units[i]
		if unit.IsParallel {
			realSlot, ok := sbpSlotLookup[unit.ParallelGroupKey]
			if !ok {
				warnings = append(warnings, fmt.Sprintf(
					"unit[%d]: no real SBP slot for group %q; fallback to first candidate",
					i, unit.ParallelGroupKey,
				))
				if cands := unitCandidates(unit, validSlots); len(cands) > 0 {
					if chosen, _, ok := pickBestCandidate(state, unit, cands); ok {
						ch.UnitSlots[i] = chosen
						markPlaced(state, i, unit, chosen)
					}
				}
				continue
			}

			cands := unitCandidates(unit, validSlots)
			placed := false
			for _, vs := range cands {
				if vs.Day == realSlot.Day && vs.SlotIndex == realSlot.SlotIndex {
					ch.UnitSlots[i] = vs
					markPlaced(state, i, unit, vs)
					placed = true
					break
				}
			}
			if placed {
				continue
			}

			warnings = append(warnings, fmt.Sprintf(
				"unit[%d]: SBP group %q real slot %s slot-%d not in validSlots; fallback to first candidate",
				i, unit.ParallelGroupKey, realSlot.Day, realSlot.SlotIndex,
			))
			if cands := unitCandidates(unit, validSlots); len(cands) > 0 {
				if chosen, _, ok := pickBestCandidate(state, unit, cands); ok {
					ch.UnitSlots[i] = chosen
					markPlaced(state, i, unit, chosen)
				}
			}
			continue
		}

		if _, done := placedNormal[i]; done {
			continue
		}

		if partnerIdx, ok := flexiblePartner[i]; ok {
			if _, partnerDone := placedNormal[partnerIdx]; !partnerDone {
				first := i
				second := partnerIdx

				curPenalty, curWarn := simulatePairOrder(first, second)
				altPenalty, altWarn := simulatePairOrder(second, first)
				if altPenalty < curPenalty || (altPenalty == curPenalty && altWarn < curWarn) {
					first, second = second, first
				}

				_, unitWarnings := placeNormalUnit(state, first, true)
				warnings = append(warnings, unitWarnings...)
				_, unitWarnings = placeNormalUnit(state, second, true)
				warnings = append(warnings, unitWarnings...)

				placedNormal[first] = struct{}{}
				placedNormal[second] = struct{}{}
				continue
			}
		}

		_, unitWarnings := placeNormalUnit(state, i, true)
		warnings = append(warnings, unitWarnings...)
		placedNormal[i] = struct{}{}
	}

	return ch, warnings
}

func buildTeacherSubjectMap(units []PlacementUnit) map[uuid.UUID]uuid.UUID {
	teacherSubjects := make(map[uuid.UUID]map[uuid.UUID]struct{})

	for _, unit := range units {
		for _, b := range unitBlocks(unit) {
			if b.TeacherID == nil {
				continue
			}

			tid := *b.TeacherID
			if teacherSubjects[tid] == nil {
				teacherSubjects[tid] = make(map[uuid.UUID]struct{})
			}
			teacherSubjects[tid][b.SubjectID] = struct{}{}
		}
	}

	out := make(map[uuid.UUID]uuid.UUID, len(teacherSubjects))
	for teacherID, subjects := range teacherSubjects {
		if len(subjects) != 1 {
			continue
		}
		for subjectID := range subjects {
			out[teacherID] = subjectID
		}
	}

	return out
}

func dayOrder(day string) int {
	switch day {
	case "monday":
		return 1
	case "tuesday":
		return 2
	case "wednesday":
		return 3
	case "thursday":
		return 4
	case "friday":
		return 5
	default:
		return 99
	}
}

func findValidSlot(unit PlacementUnit, validSlots map[uuid.UUID][]ValidSlot, day, timeStart string) (ValidSlot, bool) {
	slotIdx := SlotIndexFromTimeStart(day, timeStart)
	if slotIdx < 0 {
		return ValidSlot{}, false
	}
	cands := unitCandidates(unit, validSlots)
	for _, vs := range cands {
		if vs.Day == day && vs.SlotIndex == slotIdx {
			return vs, true
		}
	}
	return ValidSlot{}, false
}
