package algorithm

import "github.com/google/uuid"

// ValidSlot is a concrete (day, slot-index) position that a block can occupy.
// SlotIndex is 0-based and refers to teaching slots only — breaks are invisible.
type ValidSlot struct {
	Day       string
	SlotIndex int
}

// PlacementUnit is the atomic unit the GA places.
//
// Normal block: one Block, one ClassID locked on placement.
// Parallel group: N Blocks sharing a ParallelGroupKey,
// all ClassIDs placed on the same day+slot simultaneously.
type PlacementUnit struct {
	Block  *Block   // non-nil for a normal single block
	Blocks []*Block // non-nil (len >= 2) for a parallel group

	IsParallel       bool
	ParallelGroupKey string
	LockedClassIDs   []uuid.UUID
}

// GenerateValidSlots returns a map of BlockID → valid slots.
//
// A block of JP=j starting at SlotIndex occupies slots [SlotIndex, SlotIndex+j-1].
// The block must fit within the day's total slot count.
// PJOK (ConstraintFinishBefore11) is further restricted to pjokMaxStartSlot.
// Parallel-group blocks share the same slot list via a cache keyed on GroupKey.
func GenerateValidSlots(blocks []Block) map[uuid.UUID][]ValidSlot {
	result := make(map[uuid.UUID][]ValidSlot)
	parallelCache := make(map[string][]ValidSlot)

	for i := range blocks {
		b := &blocks[i]

		if b.ParallelGroupKey != nil {
			gk := *b.ParallelGroupKey
			if _, exists := parallelCache[gk]; !exists {
				parallelCache[gk] = blockValidSlots(b)
			}
			result[b.ID] = parallelCache[gk]
			continue
		}

		result[b.ID] = blockValidSlots(b)
	}

	return result
}

func blockValidSlots(b *Block) []ValidSlot {
	var slots []ValidSlot
	for _, day := range days {
		total := slotsPerDay[day]
		maxStart := total - b.JP
		if b.Constraint == ConstraintFinishBefore11 {
			if limit, ok := pjokMaxStartSlot[day]; ok && limit < maxStart {
				maxStart = limit
			}
		}
		for start := 0; start <= maxStart; start++ {
			slots = append(slots, ValidSlot{Day: day, SlotIndex: start})
		}
	}
	return slots
}

// GroupParallelBlocks organizes blocks into placement units for the GA.
//
// Blocks sharing a ParallelGroupKey AND SplitGroupKey become one PlacementUnit,
// placed atomically, locking the full JP window across all ClassIDs.
// All other blocks remain individual PlacementUnits.
func GroupParallelBlocks(blocks []Block) []PlacementUnit {
	type groupKey struct {
		parallelGroup string
		splitGroup    string
	}

	grouped := make(map[groupKey][]*Block)
	var order []groupKey

	for i := range blocks {
		b := &blocks[i]
		if b.ParallelGroupKey == nil {
			continue
		}

		sg := ""
		if b.SplitGroupKey != nil {
			sg = *b.SplitGroupKey
		}

		gk := groupKey{parallelGroup: *b.ParallelGroupKey, splitGroup: sg}
		if _, exists := grouped[gk]; !exists {
			order = append(order, gk)
		}
		grouped[gk] = append(grouped[gk], b)
	}

	var units []PlacementUnit

	// Normal (non-parallel) blocks first.
	for i := range blocks {
		b := &blocks[i]
		if b.ParallelGroupKey != nil {
			continue
		}
		units = append(units, PlacementUnit{
			Block:          b,
			IsParallel:     false,
			LockedClassIDs: []uuid.UUID{b.ClassID},
		})
	}

	// Parallel placement units.
	for _, gk := range order {
		groupBlocks := grouped[gk]

		seen := make(map[uuid.UUID]bool)
		var classIDs []uuid.UUID
		for _, b := range groupBlocks {
			if !seen[b.ClassID] {
				seen[b.ClassID] = true
				classIDs = append(classIDs, b.ClassID)
			}
		}

		units = append(units, PlacementUnit{
			Blocks:           groupBlocks,
			IsParallel:       true,
			ParallelGroupKey: gk.parallelGroup,
			LockedClassIDs:   classIDs,
		})
	}

	return units
}
