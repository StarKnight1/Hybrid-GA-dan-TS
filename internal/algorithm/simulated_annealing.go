package algorithm

import (
	"fmt"
	"math"
	"math/rand"
	"time"

	"github.com/google/uuid"
)

type SAConfig struct {
	InitialTemperature float64
	CoolingRate        float64
	Iterations         int
	ProgressEvery      int
	Seed               int64
	PerturbCount       int // blocks to evict when stagnant; 0 = disabled
	PerturbAfter       int // iterations without improvement before perturbing; 0 = disabled
	PJOKSubjectID      uuid.UUID
	OnProgress         func(SAProgress)
}

type SAProgress struct {
	Iteration             int
	Temperature           float64
	CurrentUnplaced       int
	CurrentSoftViolations int
	BestUnplaced          int
	BestSoftViolations    int
	Elapsed               time.Duration
}

type SAResult struct {
	Chromosome     Chromosome
	Matrix         *ScheduleMatrix
	Unplaced       int
	SoftViolations int
	Iterations     int
	Elapsed        time.Duration
}

func DefaultSAConfig() SAConfig {
	return SAConfig{
		InitialTemperature: 2.0,
		CoolingRate:        0.99995,
		Iterations:         200000,
		ProgressEvery:      5000,
		Seed:               time.Now().UnixNano(),
		PerturbCount:       10,
		PerturbAfter:       15000,
	}
}

// placementSnapshot records current block placements without cloning the matrix.
type placementSnapshot map[uuid.UUID]Gene

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

func rebuildFromSnapshot(snap placementSnapshot, blocks []MatrixBlock, daySlots DaySlots, pjokSubjectID uuid.UUID) (*ScheduleMatrix, int) {
	matrix := NewScheduleMatrix(nil, nil, blocks, daySlots)
	matrix.EnableDayDiversity()
	if pjokSubjectID != uuid.Nil {
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
func removeID(s []uuid.UUID, id uuid.UUID) []uuid.UUID {
	for i, v := range s {
		if v == id {
			s[i] = s[len(s)-1]
			return s[:len(s)-1]
		}
	}
	return s
}

// findConflictingBlocks returns all block IDs that would prevent placing block at (day, startSlot).
func findConflictingBlocks(matrix *ScheduleMatrix, block MatrixBlock, day string, startSlot int) []uuid.UUID {
	seen := make(map[uuid.UUID]struct{})
	var conflicts []uuid.UUID

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
func findFreeCandidateForGroup(matrix *ScheduleMatrix, groupIDs []uuid.UUID, blockByID map[uuid.UUID]MatrixBlock, candidates []Gene, rng *rand.Rand) (Gene, bool) {
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

// relocateGroup tries to move an entire parallel group (SBP) to a new free slot.
// It saves the current positions, removes all members, finds a free candidate for
// the whole group, places them there, and accepts/rejects via Boltzmann criterion.
// If rejected or no free slot found, it restores the original positions.
func relocateGroup(
	matrix *ScheduleMatrix,
	groupIDs []uuid.UUID,
	blockByID map[uuid.UUID]MatrixBlock,
	candidates []Gene,
	blocks []MatrixBlock,
	currentSoft *int,
	T float64,
	pjokSubjectID uuid.UUID,
	rng *rand.Rand,
) {
	if len(candidates) == 0 || len(groupIDs) == 0 {
		return
	}

	// Save current positions.
	orig := make(map[uuid.UUID]Gene, len(groupIDs))
	for _, id := range groupIDs {
		if p, ok := matrix.Placement(id); ok {
			orig[id] = Gene{Day: p.Day, StartSlot: p.StartSlot}
		}
	}

	// Remove all group members.
	for _, id := range groupIDs {
		_ = matrix.RemoveBlock(id)
	}

	// Find a new free slot for the whole group.
	gene, ok := findFreeCandidateForGroup(matrix, groupIDs, blockByID, candidates, rng)
	if !ok {
		// No free slot found — restore and abort.
		for _, id := range groupIDs {
			if g, hasOrig := orig[id]; hasOrig {
				_ = matrix.PlaceBlock(id, g.Day, g.StartSlot)
			}
		}
		return
	}

	// Place at new position.
	for _, id := range groupIDs {
		_ = matrix.PlaceBlock(id, gene.Day, gene.StartSlot)
	}

	newSoft := CountSoftViolations(matrix, blocks, pjokSubjectID)
	delta := float64(newSoft - *currentSoft)
	if delta <= 0 || rng.Float64() < math.Exp(-delta/T) {
		*currentSoft = newSoft
	} else {
		// Reject — restore original positions.
		for _, id := range groupIDs {
			_ = matrix.RemoveBlock(id)
			if g, hasOrig := orig[id]; hasOrig {
				_ = matrix.PlaceBlock(id, g.Day, g.StartSlot)
			}
		}
	}
}

// RunSA applies simulated annealing to the best GA solution using direct matrix operations.
//
// The chromosome-based approach has a decode-ordering artifact: DecodeChromosome processes
// blocks in fixed array order, so late-array blocks permanently lose conflicts regardless of
// temperature. This bypasses that by operating directly on ScheduleMatrix with explicit
// PlaceBlock/RemoveBlock calls — no re-decode, no ordering bias.
//
// Four move types:
//   - Free placement: unplaced block finds a free slot — always accepted (net -1 unplaced).
//   - Single displace: unplaced block displaces exactly one placed block; displaced block is
//     re-placed at a new free slot (net -1, always accept) or traded (net 0, Boltzmann).
//   - Chain displace: unplaced block displaces two placed blocks; re-place outcomes determine
//     net delta and acceptance (net -1 always; net 0 Boltzmann; net +1 always rejected).
//   - Swap: two placed blocks exchange positions to reduce soft violations (Boltzmann).
//     Also runs 20% of the time when unplaced > 0 to rearrange placed blocks and open slots.
//
// Perturbation: when best hasn't improved for PerturbAfter iterations, PerturbCount random
// placed blocks are evicted, allowing SA to escape local optima without a full restart.
func RunSA(
	gaResult GAResult,
	blocks []MatrixBlock,
	candidateIndex map[uuid.UUID][]Gene,
	daySlots DaySlots,
	cfg SAConfig,
) SAResult {
	start := time.Now()
	if daySlots == nil {
		daySlots = GenerateSlots()
	}
	rng := rand.New(rand.NewSource(cfg.Seed))

	blockByID := make(map[uuid.UUID]MatrixBlock, len(blocks))
	for _, b := range blocks {
		blockByID[b.ID] = b
	}

	// Build group lookup: blockID → sibling block IDs (including self).
	groups := BuildGroupIndex(blocks)
	groupByID := make(map[uuid.UUID][]uuid.UUID)
	for _, indices := range groups {
		ids := make([]uuid.UUID, len(indices))
		for i, idx := range indices {
			ids[i] = blocks[idx].ID
		}
		for _, id := range ids {
			groupByID[id] = ids
		}
	}

	// Build a fast set for candidate-set validation in the swap move.
	// Swapping two blocks exchanges their positions; the new position must be in
	// each block's candidate set (e.g. PJOK 2JP must stay in morning slots).
	validPos := make(map[uuid.UUID]map[Gene]struct{}, len(blocks))
	for _, b := range blocks {
		s := make(map[Gene]struct{}, len(candidateIndex[b.ID]))
		for _, g := range candidateIndex[b.ID] {
			s[g] = struct{}{}
		}
		validPos[b.ID] = s
	}

	// Build an owned matrix from the GA result so SA can mutate it freely.
	matrix, _ := rebuildFromSnapshot(snapshotFromMatrix(gaResult.Matrix, blocks), blocks, daySlots, cfg.PJOKSubjectID)

	placed := make([]uuid.UUID, 0, len(blocks))
	unplaced := make([]uuid.UUID, 0, len(blocks))
	for _, b := range blocks {
		if _, ok := matrix.Placement(b.ID); ok {
			placed = append(placed, b.ID)
		} else {
			unplaced = append(unplaced, b.ID)
		}
	}

	currentSoft := CountSoftViolations(matrix, blocks, cfg.PJOKSubjectID)
	bestUnplaced := len(blocks) - matrix.PlacedCount()
	bestSoft := currentSoft
	bestSnap := snapshotFromMatrix(matrix, blocks)

	T := cfg.InitialTemperature
	perturbStagnant := 0

	// diagnostic counters
	perturbCount := 0
	lastUnplacedIter := -1 // iteration when bestUnplaced first reached 0
	fmt.Printf("[SA diag] start: unplaced=%d softViolations=%d\n", bestUnplaced, bestSoft)

	emit := func(iter int) {
		if cfg.OnProgress != nil {
			cfg.OnProgress(SAProgress{
				Iteration:             iter,
				Temperature:           T,
				CurrentUnplaced:       len(blocks) - matrix.PlacedCount(),
				CurrentSoftViolations: currentSoft,
				BestUnplaced:          bestUnplaced,
				BestSoftViolations:    bestSoft,
				Elapsed:               time.Since(start),
			})
		}
	}

	lastIter := 0
	emittedLast := false

	for iter := 1; iter <= cfg.Iterations; iter++ {
		if bestUnplaced == 0 && bestSoft == 0 {
			break
		}
		lastIter = iter

		// Decide move type: unplaced move (80%) vs swap (20%) when blocks remain unplaced.
		// When all blocks are placed, always do swap to improve soft violations.
		doSwap := len(unplaced) == 0 || (len(placed) >= 2 && rng.Float64() >= 0.8)

		if !doSwap && len(unplaced) > 0 {
			// ----- unplaced move -----
			targetID := unplaced[rng.Intn(len(unplaced))]
			targetBlock := blockByID[targetID]
			candidates := candidateIndex[targetID]

			// Grouped blocks (SBP parallel classes) must be placed together at the
			// same slot. Find a candidate free for every member and place all at once.
			if groupIDs, isGrouped := groupByID[targetID]; isGrouped {
				if gene, ok := findFreeCandidateForGroup(matrix, groupIDs, blockByID, candidates, rng); ok {
					for _, id := range groupIDs {
						_ = matrix.PlaceBlock(id, gene.Day, gene.StartSlot)
						unplaced = removeID(unplaced, id)
						placed = append(placed, id)
					}
					currentSoft = CountSoftViolations(matrix, blocks, cfg.PJOKSubjectID)
				}
				// No free slot found for the whole group this iteration — skip.
				T *= cfg.CoolingRate
				continue
			}

			if len(candidates) > 0 {
				gene := candidates[rng.Intn(len(candidates))]
				conflicts := findConflictingBlocks(matrix, targetBlock, gene.Day, gene.StartSlot)

				switch len(conflicts) {
				case 0:
					// Free placement: always accept.
					// findConflictingBlocks only checks class/teacher grids, not day diversity.
					// Check the actual PlaceBlock result before updating tracking.
					if matrix.PlaceBlock(targetID, gene.Day, gene.StartSlot) == nil {
						unplaced = removeID(unplaced, targetID)
						placed = append(placed, targetID)
						currentSoft = CountSoftViolations(matrix, blocks, cfg.PJOKSubjectID)
					}

				case 1:
					// Single displace: remove the one blocking block, place target, try to re-place displaced.
					displacedID := conflicts[0]
					// Displacing a single group member breaks the group invariant — skip.
					if _, isGroupMember := groupByID[displacedID]; isGroupMember {
						break
					}
					displacedBlock := blockByID[displacedID]
					origDisplacedPlacement, _ := matrix.Placement(displacedID)

					_ = matrix.RemoveBlock(displacedID)
					placed = removeID(placed, displacedID)
					unplaced = append(unplaced, displacedID)

					if matrix.PlaceBlock(targetID, gene.Day, gene.StartSlot) != nil {
						// targetID rejected (e.g. day diversity) — restore displaced and abort.
						_ = matrix.PlaceBlock(displacedID, origDisplacedPlacement.Day, origDisplacedPlacement.StartSlot)
						unplaced = removeID(unplaced, displacedID)
						placed = append(placed, displacedID)
						break
					}
					unplaced = removeID(unplaced, targetID)
					placed = append(placed, targetID)

					if newGene, ok := findFreeCandidate(matrix, displacedBlock, candidateIndex[displacedID], rng); ok {
						// Re-placed displaced: net -1 unplaced, always accept.
						_ = matrix.PlaceBlock(displacedID, newGene.Day, newGene.StartSlot)
						unplaced = removeID(unplaced, displacedID)
						placed = append(placed, displacedID)
						currentSoft = CountSoftViolations(matrix, blocks, cfg.PJOKSubjectID)
					} else {
						// Trade: net 0, Boltzmann on soft delta.
						newSoft := CountSoftViolations(matrix, blocks, cfg.PJOKSubjectID)
						softDelta := float64(newSoft-currentSoft) * 0.01
						if softDelta <= 0 || rng.Float64() < math.Exp(-softDelta/T) {
							currentSoft = newSoft
						} else {
							// reject: restore original positions
							_ = matrix.RemoveBlock(targetID)
							placed = removeID(placed, targetID)
							unplaced = append(unplaced, targetID)
							_ = matrix.PlaceBlock(displacedID, origDisplacedPlacement.Day, origDisplacedPlacement.StartSlot)
							unplaced = removeID(unplaced, displacedID)
							placed = append(placed, displacedID)
						}
					}

				case 2:
					// Chain displace: remove both blocking blocks, place target, try to re-place both.
					d1, d2 := conflicts[0], conflicts[1]
					// Displacing a single group member breaks the group invariant — skip.
					if _, ok := groupByID[d1]; ok {
						break
					}
					if _, ok := groupByID[d2]; ok {
						break
					}
					db1, db2 := blockByID[d1], blockByID[d2]
					origP1, _ := matrix.Placement(d1)
					origP2, _ := matrix.Placement(d2)

					_ = matrix.RemoveBlock(d1)
					placed = removeID(placed, d1)
					unplaced = append(unplaced, d1)
					_ = matrix.RemoveBlock(d2)
					placed = removeID(placed, d2)
					unplaced = append(unplaced, d2)

					if matrix.PlaceBlock(targetID, gene.Day, gene.StartSlot) != nil {
						// targetID rejected — restore d1, d2 and abort.
						_ = matrix.PlaceBlock(d1, origP1.Day, origP1.StartSlot)
						unplaced = removeID(unplaced, d1)
						placed = append(placed, d1)
						_ = matrix.PlaceBlock(d2, origP2.Day, origP2.StartSlot)
						unplaced = removeID(unplaced, d2)
						placed = append(placed, d2)
						break
					}
					unplaced = removeID(unplaced, targetID)
					placed = append(placed, targetID)

					g1, ok1 := findFreeCandidate(matrix, db1, candidateIndex[d1], rng)
					if ok1 {
						_ = matrix.PlaceBlock(d1, g1.Day, g1.StartSlot)
						unplaced = removeID(unplaced, d1)
						placed = append(placed, d1)
					}
					g2, ok2 := findFreeCandidate(matrix, db2, candidateIndex[d2], rng)
					if ok2 {
						_ = matrix.PlaceBlock(d2, g2.Day, g2.StartSlot)
						unplaced = removeID(unplaced, d2)
						placed = append(placed, d2)
					}

					if ok1 && ok2 {
						// Both re-placed: net -1, always accept.
						currentSoft = CountSoftViolations(matrix, blocks, cfg.PJOKSubjectID)

					} else if ok1 && !ok2 {
						// d2 couldn't be re-placed: net 0 trade, Boltzmann.
						newSoft := CountSoftViolations(matrix, blocks, cfg.PJOKSubjectID)
						softDelta := float64(newSoft-currentSoft) * 0.01
						if softDelta <= 0 || rng.Float64() < math.Exp(-softDelta/T) {
							currentSoft = newSoft
						} else {
							_ = matrix.RemoveBlock(d1)
							_ = matrix.RemoveBlock(targetID)
							placed = removeID(placed, d1)
							placed = removeID(placed, targetID)
							unplaced = append(unplaced, targetID)
							unplaced = removeID(unplaced, d2)
							_ = matrix.PlaceBlock(d1, origP1.Day, origP1.StartSlot)
							_ = matrix.PlaceBlock(d2, origP2.Day, origP2.StartSlot)
							placed = append(placed, d1, d2)
						}

					} else if !ok1 && ok2 {
						// d1 couldn't be re-placed: net 0 trade, Boltzmann.
						newSoft := CountSoftViolations(matrix, blocks, cfg.PJOKSubjectID)
						softDelta := float64(newSoft-currentSoft) * 0.01
						if softDelta <= 0 || rng.Float64() < math.Exp(-softDelta/T) {
							currentSoft = newSoft
						} else {
							_ = matrix.RemoveBlock(d2)
							_ = matrix.RemoveBlock(targetID)
							placed = removeID(placed, d2)
							placed = removeID(placed, targetID)
							unplaced = append(unplaced, targetID)
							unplaced = removeID(unplaced, d1)
							_ = matrix.PlaceBlock(d1, origP1.Day, origP1.StartSlot)
							_ = matrix.PlaceBlock(d2, origP2.Day, origP2.StartSlot)
							placed = append(placed, d1, d2)
						}

					} else {
						// Neither re-placed: net +1 — always reject and fully revert.
						_ = matrix.RemoveBlock(targetID)
						placed = removeID(placed, targetID)
						unplaced = append(unplaced, targetID)
						unplaced = removeID(unplaced, d1)
						unplaced = removeID(unplaced, d2)
						_ = matrix.PlaceBlock(d1, origP1.Day, origP1.StartSlot)
						_ = matrix.PlaceBlock(d2, origP2.Day, origP2.StartSlot)
						placed = append(placed, d1, d2)
					}

					// len(conflicts) > 2: skip — too many simultaneous displacements
				}
			}
		} else if len(placed) >= 2 {
			// ----- swap move (soft violations + rearranging for unplaced blocks) -----
			i := rng.Intn(len(placed))
			j := rng.Intn(len(placed))
			for j == i {
				j = rng.Intn(len(placed))
			}
			idA, idB := placed[i], placed[j]
			// When a group member is picked, try to relocate the whole group to a
			// new free slot instead of just skipping the iteration.
			if groupIDs, ok := groupByID[idA]; ok {
				relocateGroup(matrix, groupIDs, blockByID, candidateIndex[idA], blocks, &currentSoft, T, cfg.PJOKSubjectID, rng)
				T *= cfg.CoolingRate
				continue
			}
			if groupIDs, ok := groupByID[idB]; ok {
				relocateGroup(matrix, groupIDs, blockByID, candidateIndex[idB], blocks, &currentSoft, T, cfg.PJOKSubjectID, rng)
				T *= cfg.CoolingRate
				continue
			}
			pA, okA := matrix.Placement(idA)
			pB, okB := matrix.Placement(idB)

			if okA && okB {
				// Reject if either block's new position falls outside its candidate set.
				// This prevents PJOK 2JP (morning-only) from being swapped into afternoon slots.
				newGeneA := Gene{Day: pB.Day, StartSlot: pB.StartSlot}
				newGeneB := Gene{Day: pA.Day, StartSlot: pA.StartSlot}
				if _, aOK := validPos[idA][newGeneA]; !aOK {
					T *= cfg.CoolingRate
					continue
				}
				if _, bOK := validPos[idB][newGeneB]; !bOK {
					T *= cfg.CoolingRate
					continue
				}
				_ = matrix.RemoveBlock(idA)
				_ = matrix.RemoveBlock(idB)
				errA := matrix.PlaceBlock(idA, pB.Day, pB.StartSlot)
				errB := matrix.PlaceBlock(idB, pA.Day, pA.StartSlot)

				if errA != nil || errB != nil {
					// Swap not feasible — undo partial placement, restore originals.
					if errA == nil {
						_ = matrix.RemoveBlock(idA)
					}
					if errB == nil {
						_ = matrix.RemoveBlock(idB)
					}
					_ = matrix.PlaceBlock(idA, pA.Day, pA.StartSlot)
					_ = matrix.PlaceBlock(idB, pB.Day, pB.StartSlot)
				} else {
					newSoft := CountSoftViolations(matrix, blocks, cfg.PJOKSubjectID)
					delta := float64(newSoft - currentSoft)
					if delta <= 0 || rng.Float64() < math.Exp(-delta/T) {
						currentSoft = newSoft
					} else {
						_ = matrix.RemoveBlock(idA)
						_ = matrix.RemoveBlock(idB)
						_ = matrix.PlaceBlock(idA, pA.Day, pA.StartSlot)
						_ = matrix.PlaceBlock(idB, pB.Day, pB.StartSlot)
					}
				}
			}
		}

		T *= cfg.CoolingRate

		matrixUnplaced := len(blocks) - matrix.PlacedCount()
		if matrixUnplaced < bestUnplaced || (matrixUnplaced == bestUnplaced && currentSoft < bestSoft) {
			bestUnplaced = matrixUnplaced
			bestSoft = currentSoft
			bestSnap = snapshotFromMatrix(matrix, blocks)
			perturbStagnant = 0
			if bestUnplaced == 0 && lastUnplacedIter == -1 {
				lastUnplacedIter = iter
				fmt.Printf("[SA diag] last unplaced block placed at iteration %d (%.1f%% of budget)\n",
					iter, float64(iter)/float64(cfg.Iterations)*100)
			}
		} else {
			perturbStagnant++
		}

		// Perturbation: when stagnant too long, evict random placed blocks to escape local optimum.
		if cfg.PerturbAfter > 0 && cfg.PerturbCount > 0 && perturbStagnant >= cfg.PerturbAfter {
			count := cfg.PerturbCount
			if count > len(placed) {
				count = len(placed)
			}
			evicted := make(map[uuid.UUID]bool)
			for k := 0; k < count; k++ {
				if len(placed) == 0 {
					break
				}
				idx := rng.Intn(len(placed))
				id := placed[idx]
				if evicted[id] {
					continue
				}
				// Evict entire group together so the group invariant is maintained.
				toEvict := []uuid.UUID{id}
				if groupIDs, ok := groupByID[id]; ok {
					toEvict = groupIDs
				}
				for _, eid := range toEvict {
					if evicted[eid] {
						continue
					}
					evicted[eid] = true
					_ = matrix.RemoveBlock(eid)
					placed = removeID(placed, eid)
					unplaced = append(unplaced, eid)
				}
			}
			currentSoft = CountSoftViolations(matrix, blocks, cfg.PJOKSubjectID)
			perturbStagnant = 0
			perturbCount++
			fmt.Printf("[SA diag] perturbation #%d at iteration %d, evicted %d blocks, unplaced now=%d\n",
				perturbCount, iter, count, len(unplaced))
		}

		if cfg.ProgressEvery > 0 && iter%cfg.ProgressEvery == 0 {
			emit(iter)
			emittedLast = true
		} else {
			emittedLast = false
		}
	}

	if !emittedLast {
		emit(lastIter)
	}

	if lastUnplacedIter == -1 {
		fmt.Printf("[SA diag] finished %d iters, unplaced never reached 0 (best=%d), perturbations=%d\n",
			lastIter, bestUnplaced, perturbCount)
	} else {
		fmt.Printf("[SA diag] finished %d iters, unplaced→0 at iter %d, perturbations=%d, finalSoft=%d\n",
			lastIter, lastUnplacedIter, perturbCount, bestSoft)
	}

	bestMatrix, actualUnplaced := rebuildFromSnapshot(bestSnap, blocks, daySlots, cfg.PJOKSubjectID)
	actualSoft := CountSoftViolations(bestMatrix, blocks, cfg.PJOKSubjectID)
	bestChromosome := chromosomeFromSnapshot(bestSnap, blocks)

	return SAResult{
		Chromosome:     bestChromosome,
		Matrix:         bestMatrix,
		Unplaced:       actualUnplaced,
		SoftViolations: actualSoft,
		Iterations:     lastIter,
		Elapsed:        time.Since(start),
	}
}
