package algorithm

import (
	"math/rand"
)

// matrixSnapshot records the (Day, StartSlot) of every placed block at a point in time.
// It allows the Tabu Search to save and restore matrix states efficiently without
// cloning the full grid structure.
type matrixSnapshot map[uint]Gene

// captureMatrix reads every block's current placement from the matrix and returns
// a snapshot. Querying the matrix directly (rather than a placed-block list) ensures
// the snapshot is always consistent with the true grid state.
func captureMatrix(matrix *ScheduleMatrix, blocks []MatrixBlock) matrixSnapshot {
	state := make(matrixSnapshot, len(blocks))
	for _, blk := range blocks {
		if rec, ok := matrix.Placement(blk.ID); ok {
			state[blk.ID] = Gene{Day: rec.Day, StartSlot: rec.StartSlot}
		}
	}
	return state
}

// restoreMatrix builds a fresh ScheduleMatrix from a previously captured snapshot.
// Blocks absent from the snapshot or whose gene is unplaced are left unscheduled;
// the returned integer is the count of such blocks.
func restoreMatrix(snap matrixSnapshot, blocks []MatrixBlock, daySlots DaySlots, pjokSubjectID uint) (*ScheduleMatrix, int) {
	grid := NewScheduleMatrix(nil, nil, blocks, daySlots)
	grid.EnableDayDiversity()
	if pjokSubjectID != 0 {
		grid.ExcludeSubjectFromDayDiversity(pjokSubjectID)
	}
	missing := 0
	for _, blk := range blocks {
		gene, ok := snap[blk.ID]
		if !ok || !gene.IsPlaced() {
			missing++
			continue
		}
		if err := grid.PlaceBlock(blk.ID, gene.Day, gene.StartSlot); err != nil {
			missing++
		}
	}
	return grid, missing
}

// snapshotToChromosome converts a placement snapshot into a Chromosome by encoding
// each block's recorded position as its corresponding gene.
func snapshotToChromosome(snap matrixSnapshot, blocks []MatrixBlock) Chromosome {
	ch := NewChromosome(len(blocks))
	for idx, blk := range blocks {
		if gene, ok := snap[blk.ID]; ok {
			ch.Set(idx, gene)
		}
	}
	return ch
}

// dropID removes the first occurrence of target from the slice using a swap-with-last
// strategy. Order is not preserved; returns the original slice when target is absent.
func dropID(slice []uint, target uint) []uint {
	for idx, val := range slice {
		if val == target {
			slice[idx] = slice[len(slice)-1]
			return slice[:len(slice)-1]
		}
	}
	return slice
}

// conflictsAt finds all block IDs that would conflict with placing block at (day, startSlot).
// Both the class grid and (if the block has a teacher) the teacher grid are checked
// across all slot offsets within the block's duration window.
func conflictsAt(matrix *ScheduleMatrix, block MatrixBlock, day string, startSlot int) []uint {
	seen := make(map[uint]struct{})
	var result []uint

	for offset := 0; offset < block.Duration; offset++ {
		idx := startSlot + offset

		if cell, ok := matrix.ClassCell(block.ClassID, day, idx); ok && cell.State == FilledCell {
			if _, already := seen[cell.BlockID]; !already {
				seen[cell.BlockID] = struct{}{}
				result = append(result, cell.BlockID)
			}
		}

		if block.TeacherID != nil {
			if cell, ok := matrix.TeacherCell(*block.TeacherID, day, idx); ok && cell.State == FilledCell {
				if _, already := seen[cell.BlockID]; !already {
					seen[cell.BlockID] = struct{}{}
					result = append(result, cell.BlockID)
				}
			}
		}
	}

	return result
}

// findGroupSlot scans candidates in a random order and returns the first Gene where
// every block in the parallel group can be placed simultaneously. Returns (Gene{}, false)
// if no such position exists.
func findGroupSlot(matrix *ScheduleMatrix, groupIDs []uint, blockByID map[uint]MatrixBlock, candidates []Gene, rng *rand.Rand) (Gene, bool) {
	if len(candidates) == 0 {
		return Gene{}, false
	}
	startAt := rng.Intn(len(candidates))
	for attempt := 0; attempt < len(candidates); attempt++ {
		pos := candidates[(startAt+attempt)%len(candidates)]
		allClear := true
		for _, id := range groupIDs {
			if err := matrix.CanPlaceBlock(id, pos.Day, pos.StartSlot); err != nil {
				allClear = false
				break
			}
		}
		if allClear {
			return pos, true
		}
	}
	return Gene{}, false
}

// findOpenSlot scans candidates in a random order and returns the first Gene that is
// conflict-free for the given single block. Returns (Gene{}, false) if none is found.
func findOpenSlot(matrix *ScheduleMatrix, block MatrixBlock, candidates []Gene, rng *rand.Rand) (Gene, bool) {
	if len(candidates) == 0 {
		return Gene{}, false
	}
	startAt := rng.Intn(len(candidates))
	for attempt := 0; attempt < len(candidates); attempt++ {
		pos := candidates[(startAt+attempt)%len(candidates)]
		if matrix.CanPlaceBlock(block.ID, pos.Day, pos.StartSlot) == nil {
			return pos, true
		}
	}
	return Gene{}, false
}
