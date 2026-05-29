package algorithm

import (
	"math/rand"
)

// placementSnapshot records current block placements without cloning the matrix.
type placementSnapshot map[uint]Gene

// snapshotFromMatrix builds a snapshot by querying the matrix for every block,
// bypassing the placed list entirely. This guarantees the snapshot matches
// the true matrix state even if the placed list diverged due to silent errors.
func snapshotFromMatrix(matrix *ScheduleMatrix, blocks []MatrixBlock) placementSnapshot {
	snap := make(placementSnapshot, len(blocks))
	for _, b := range blocks {
		if p, ok := matrix.Placement(b.ID); ok {
			snap[b.ID] = Gene{Day: p.Day, StartSlot: p.StartSlot}
		}
	}
	return snap
}

func rebuildFromSnapshot(snap placementSnapshot, blocks []MatrixBlock, daySlots DaySlots, pjokSubjectID uint) (*ScheduleMatrix, int) {
	matrix := NewScheduleMatrix(nil, nil, blocks, daySlots)
	matrix.EnableDayDiversity()
	if pjokSubjectID != 0 {
		matrix.ExcludeSubjectFromDayDiversity(pjokSubjectID)
	}
	unplaced := 0
	for _, b := range blocks {
		g, ok := snap[b.ID]
		if !ok || !g.IsPlaced() {
			unplaced++
			continue
		}
		if err := matrix.PlaceBlock(b.ID, g.Day, g.StartSlot); err != nil {
			unplaced++
		}
	}
	return matrix, unplaced
}

func chromosomeFromSnapshot(snap placementSnapshot, blocks []MatrixBlock) Chromosome {
	c := NewChromosome(len(blocks))
	for i, b := range blocks {
		if g, ok := snap[b.ID]; ok {
			c.Set(i, g)
		}
	}
	return c
}

// removeID removes the first occurrence of id using swap-with-last (order not preserved).
func removeID(s []uint, id uint) []uint {
	for i, v := range s {
		if v == id {
			s[i] = s[len(s)-1]
			return s[:len(s)-1]
		}
	}
	return s
}

// findConflictingBlocks returns all block IDs that would prevent placing block at (day, startSlot).
func findConflictingBlocks(matrix *ScheduleMatrix, block MatrixBlock, day string, startSlot int) []uint {
	seen := make(map[uint]struct{})
	var conflicts []uint

	for offset := 0; offset < block.Duration; offset++ {
		cell, ok := matrix.ClassCell(block.ClassID, day, startSlot+offset)
		if !ok || cell.State != CellOccupied {
			continue
		}
		if _, already := seen[cell.BlockID]; !already {
			seen[cell.BlockID] = struct{}{}
			conflicts = append(conflicts, cell.BlockID)
		}
	}

	if block.TeacherID != nil {
		for offset := 0; offset < block.Duration; offset++ {
			cell, ok := matrix.TeacherCell(*block.TeacherID, day, startSlot+offset)
			if !ok || cell.State != CellOccupied {
				continue
			}
			if _, already := seen[cell.BlockID]; !already {
				seen[cell.BlockID] = struct{}{}
				conflicts = append(conflicts, cell.BlockID)
			}
		}
	}

	return conflicts
}

// findFreeCandidateForGroup returns the first candidate where every block in the
// group can be placed simultaneously (i.e. no class conflict for any member).
// SBP group blocks have no teacher so only the class grid is checked.
func findFreeCandidateForGroup(matrix *ScheduleMatrix, groupIDs []uint, blockByID map[uint]MatrixBlock, candidates []Gene, rng *rand.Rand) (Gene, bool) {
	if len(candidates) == 0 {
		return Gene{}, false
	}
	start := rng.Intn(len(candidates))
	for i := 0; i < len(candidates); i++ {
		g := candidates[(start+i)%len(candidates)]
		allFree := true
		for _, id := range groupIDs {
			if err := matrix.CanPlaceBlock(id, g.Day, g.StartSlot); err != nil {
				allFree = false
				break
			}
		}
		if allFree {
			return g, true
		}
	}
	return Gene{}, false
}

// findFreeCandidate scans candidates in random order and returns the first free slot for block.
func findFreeCandidate(matrix *ScheduleMatrix, block MatrixBlock, candidates []Gene, rng *rand.Rand) (Gene, bool) {
	if len(candidates) == 0 {
		return Gene{}, false
	}
	start := rng.Intn(len(candidates))
	for i := 0; i < len(candidates); i++ {
		g := candidates[(start+i)%len(candidates)]
		if err := matrix.CanPlaceBlock(block.ID, g.Day, g.StartSlot); err == nil {
			return g, true
		}
	}
	return Gene{}, false
}
