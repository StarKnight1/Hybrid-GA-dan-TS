package algorithm

import (
	"fmt"
	"math/rand"
	"time"
)

// TSConfig holds tunable parameters for the Tabu Search phase.
type TSConfig struct {
	Tenure         int // iterations a move stays forbidden; 0 uses default 15
	MaxIterations  int
	ReportInterval int
	RandSeed       int64
	ShakeCount     int // blocks to evict when stagnant; 0 = disabled
	ShakeAfter     int // iterations without improvement before shaking; 0 = disabled
	PJOKSubjID     uint
	OnSnapshot     func(TSProgress)
}

// TSProgress carries per-iteration metrics emitted via OnSnapshot.
type TSProgress struct {
	Iteration             int
	TabuListSize          int
	CurrentUnplaced       int
	CurrentSoftViolations int
	BestUnplaced          int
	BestSoftViolations    int
	Elapsed               time.Duration
}

// TSResult holds the best solution found by RunTS.
type TSResult struct {
	Chromosome     Chromosome
	Matrix         *ScheduleMatrix
	Unplaced       int
	SoftViolations int
	Iterations     int
	Elapsed        time.Duration
}

// DefaultTSConfig returns sensible defaults for the Tabu Search phase.
func DefaultTSConfig() TSConfig {
	return TSConfig{
		Tenure:         15,
		MaxIterations:  300000,
		ReportInterval: 5000,
		RandSeed:       time.Now().UnixNano(),
		ShakeCount:     10,
		ShakeAfter:     15000,
	}
}

// ── Tabu list management ──────────────────────────────────────────────────────

// moveKey is the composite key for a tabu list entry: (block, position) pair.
type moveKey struct {
	blockID uint
	gene    Gene
}

// isForbidden returns true when the given (blockID, gene) move is currently tabu.
func isForbidden(tabuList map[moveKey]int, blockID uint, gene Gene, iter int) bool {
	expiry, ok := tabuList[moveKey{blockID, gene}]
	return ok && expiry > iter
}

// forbidMove adds (blockID, gene) to the tabu list with an expiry of iter+tenure.
func forbidMove(tabuList map[moveKey]int, blockID uint, gene Gene, iter, tenure int) {
	tabuList[moveKey{blockID, gene}] = iter + tenure
}

// cleanTabuList removes all entries that have expired by iteration iter.
func cleanTabuList(tabuList map[moveKey]int, iter int) {
	for k, expiry := range tabuList {
		if expiry <= iter {
			delete(tabuList, k)
		}
	}
}

// ── Shared TS state ───────────────────────────────────────────────────────────

// tsState carries all mutable state shared between the unplaced-move and swap-move handlers.
type tsState struct {
	grid        *ScheduleMatrix
	placed      []uint
	unplaced    []uint
	currPenalty int
	tabuList    map[moveKey]int
	blockByID   map[uint]MatrixBlock
	groupByID   map[uint][]uint
	validPos    map[uint]map[Gene]struct{}
}

// ── Move handlers ─────────────────────────────────────────────────────────────

// handleUnplaced attempts to place one unplaced block, displacing at most two placed
// blocks when necessary. Returns the net change in unplaced count (negative = improvement).
func handleUnplaced(st *tsState, blocks []MatrixBlock, candidateIndex map[uint][]Gene, bestUnplaced, bestPenalty, iter, tenure int, pjokID uint, acak *rand.Rand) {
	if len(st.unplaced) == 0 {
		return
	}
	targetID := st.unplaced[acak.Intn(len(st.unplaced))]
	targetBlock := st.blockByID[targetID]
	candidates := candidateIndex[targetID]

	// Parallel group blocks must be placed together.
	if groupIDs, isGrouped := st.groupByID[targetID]; isGrouped {
		if pos, ok := findGroupSlot(st.grid, groupIDs, st.blockByID, candidates, acak); ok {
			for _, id := range groupIDs {
				_ = st.grid.PlaceBlock(id, pos.Day, pos.StartSlot)
				st.unplaced = dropID(st.unplaced, id)
				st.placed = append(st.placed, id)
			}
			st.currPenalty = CountSoftViolations(st.grid, blocks, pjokID)
		}
		return
	}

	if len(candidates) == 0 {
		return
	}
	pos := candidates[acak.Intn(len(candidates))]

	moveTabu := isForbidden(st.tabuList, targetID, pos, iter)
	aspiration := bestUnplaced > 0
	if moveTabu && !aspiration {
		return
	}

	conflicts := conflictsAt(st.grid, targetBlock, pos.Day, pos.StartSlot)

	switch len(conflicts) {
	case 0:
		// Free placement: always accepted (net -1 unplaced).
		if st.grid.PlaceBlock(targetID, pos.Day, pos.StartSlot) == nil {
			st.unplaced = dropID(st.unplaced, targetID)
			st.placed = append(st.placed, targetID)
			st.currPenalty = CountSoftViolations(st.grid, blocks, pjokID)
		}

	case 1:
		// Single displace: evict one placed block, place target, re-place evicted elsewhere.
		displacedID := conflicts[0]
		if _, isGroup := st.groupByID[displacedID]; isGroup {
			break
		}
		displacedBlock := st.blockByID[displacedID]
		origRec, _ := st.grid.Placement(displacedID)
		origGene := Gene{Day: origRec.Day, StartSlot: origRec.StartSlot}

		_ = st.grid.RemoveBlock(displacedID)
		st.placed = dropID(st.placed, displacedID)
		st.unplaced = append(st.unplaced, displacedID)

		if st.grid.PlaceBlock(targetID, pos.Day, pos.StartSlot) != nil {
			_ = st.grid.PlaceBlock(displacedID, origGene.Day, origGene.StartSlot)
			st.unplaced = dropID(st.unplaced, displacedID)
			st.placed = append(st.placed, displacedID)
			break
		}
		st.unplaced = dropID(st.unplaced, targetID)
		st.placed = append(st.placed, targetID)

		if newPos, ok := findOpenSlot(st.grid, displacedBlock, candidateIndex[displacedID], acak); ok {
			_ = st.grid.PlaceBlock(displacedID, newPos.Day, newPos.StartSlot)
			st.unplaced = dropID(st.unplaced, displacedID)
			st.placed = append(st.placed, displacedID)
			forbidMove(st.tabuList, displacedID, origGene, iter, tenure)
		} else {
			// Trade (net 0): accept; record evicted position as tabu.
			forbidMove(st.tabuList, displacedID, origGene, iter, tenure)
		}
		st.currPenalty = CountSoftViolations(st.grid, blocks, pjokID)

	case 2:
		// Chain displace: evict two blocks; reject if neither can be re-placed (net +1).
		d1, d2 := conflicts[0], conflicts[1]
		if _, ok := st.groupByID[d1]; ok {
			break
		}
		if _, ok := st.groupByID[d2]; ok {
			break
		}
		db1, db2 := st.blockByID[d1], st.blockByID[d2]
		origRec1, _ := st.grid.Placement(d1)
		origRec2, _ := st.grid.Placement(d2)
		origG1 := Gene{Day: origRec1.Day, StartSlot: origRec1.StartSlot}
		origG2 := Gene{Day: origRec2.Day, StartSlot: origRec2.StartSlot}

		_ = st.grid.RemoveBlock(d1)
		st.placed = dropID(st.placed, d1)
		st.unplaced = append(st.unplaced, d1)
		_ = st.grid.RemoveBlock(d2)
		st.placed = dropID(st.placed, d2)
		st.unplaced = append(st.unplaced, d2)

		if st.grid.PlaceBlock(targetID, pos.Day, pos.StartSlot) != nil {
			_ = st.grid.PlaceBlock(d1, origG1.Day, origG1.StartSlot)
			st.unplaced = dropID(st.unplaced, d1)
			st.placed = append(st.placed, d1)
			_ = st.grid.PlaceBlock(d2, origG2.Day, origG2.StartSlot)
			st.unplaced = dropID(st.unplaced, d2)
			st.placed = append(st.placed, d2)
			break
		}
		st.unplaced = dropID(st.unplaced, targetID)
		st.placed = append(st.placed, targetID)

		p1, ok1 := findOpenSlot(st.grid, db1, candidateIndex[d1], acak)
		if ok1 {
			_ = st.grid.PlaceBlock(d1, p1.Day, p1.StartSlot)
			st.unplaced = dropID(st.unplaced, d1)
			st.placed = append(st.placed, d1)
		}
		p2, ok2 := findOpenSlot(st.grid, db2, candidateIndex[d2], acak)
		if ok2 {
			_ = st.grid.PlaceBlock(d2, p2.Day, p2.StartSlot)
			st.unplaced = dropID(st.unplaced, d2)
			st.placed = append(st.placed, d2)
		}

		if ok1 || ok2 {
			forbidMove(st.tabuList, d1, origG1, iter, tenure)
			forbidMove(st.tabuList, d2, origG2, iter, tenure)
			st.currPenalty = CountSoftViolations(st.grid, blocks, pjokID)
		} else {
			// net +1: fully revert.
			_ = st.grid.RemoveBlock(targetID)
			st.placed = dropID(st.placed, targetID)
			st.unplaced = append(st.unplaced, targetID)
			st.unplaced = dropID(st.unplaced, d1)
			st.unplaced = dropID(st.unplaced, d2)
			_ = st.grid.PlaceBlock(d1, origG1.Day, origG1.StartSlot)
			_ = st.grid.PlaceBlock(d2, origG2.Day, origG2.StartSlot)
			st.placed = append(st.placed, d1, d2)
		}
	}
}

// handleSwap attempts to swap two placed blocks to reduce soft penalty.
func handleSwap(st *tsState, blocks []MatrixBlock, bestUnplaced, bestPenalty, iter, tenure int, pjokID uint, acak *rand.Rand) {
	if len(st.placed) < 2 {
		return
	}
	i := acak.Intn(len(st.placed))
	j := acak.Intn(len(st.placed))
	for j == i {
		j = acak.Intn(len(st.placed))
	}
	idA, idB := st.placed[i], st.placed[j]

	if groupIDs, ok := st.groupByID[idA]; ok {
		shiftParallelGroup(st.grid, groupIDs, st.blockByID, validSlotsOf(st, idA), blocks,
			&st.currPenalty, st.tabuList, iter, tenure, bestUnplaced, bestPenalty, pjokID, acak)
		return
	}
	if groupIDs, ok := st.groupByID[idB]; ok {
		shiftParallelGroup(st.grid, groupIDs, st.blockByID, validSlotsOf(st, idB), blocks,
			&st.currPenalty, st.tabuList, iter, tenure, bestUnplaced, bestPenalty, pjokID, acak)
		return
	}

	recA, okA := st.grid.Placement(idA)
	recB, okB := st.grid.Placement(idB)
	if !okA || !okB {
		return
	}

	newPosA := Gene{Day: recB.Day, StartSlot: recB.StartSlot}
	newPosB := Gene{Day: recA.Day, StartSlot: recA.StartSlot}

	if _, aOK := st.validPos[idA][newPosA]; !aOK {
		return
	}
	if _, bOK := st.validPos[idB][newPosB]; !bOK {
		return
	}

	tabuA := isForbidden(st.tabuList, idA, newPosA, iter)
	tabuB := isForbidden(st.tabuList, idB, newPosB, iter)

	_ = st.grid.RemoveBlock(idA)
	_ = st.grid.RemoveBlock(idB)
	errA := st.grid.PlaceBlock(idA, recB.Day, recB.StartSlot)
	errB := st.grid.PlaceBlock(idB, recA.Day, recA.StartSlot)

	if errA != nil || errB != nil {
		if errA == nil {
			_ = st.grid.RemoveBlock(idA)
		}
		if errB == nil {
			_ = st.grid.RemoveBlock(idB)
		}
		_ = st.grid.PlaceBlock(idA, recA.Day, recA.StartSlot)
		_ = st.grid.PlaceBlock(idB, recB.Day, recB.StartSlot)
		return
	}

	newPenalty := CountSoftViolations(st.grid, blocks, pjokID)
	newUnplaced := len(blocks) - st.grid.PlacedCount()
	aspiration := newUnplaced < bestUnplaced || (newUnplaced == bestUnplaced && newPenalty < bestPenalty)

	if (tabuA || tabuB) && !aspiration {
		_ = st.grid.RemoveBlock(idA)
		_ = st.grid.RemoveBlock(idB)
		_ = st.grid.PlaceBlock(idA, recA.Day, recA.StartSlot)
		_ = st.grid.PlaceBlock(idB, recB.Day, recB.StartSlot)
	} else {
		forbidMove(st.tabuList, idA, Gene{Day: recA.Day, StartSlot: recA.StartSlot}, iter, tenure)
		forbidMove(st.tabuList, idB, Gene{Day: recB.Day, StartSlot: recB.StartSlot}, iter, tenure)
		st.currPenalty = newPenalty
	}
}

// validSlotsOf retrieves the valid candidate list for a block from the tsState's
// validPos set (used only inside handleSwap to pass to shiftParallelGroup).
func validSlotsOf(st *tsState, blockID uint) []Gene {
	m := st.validPos[blockID]
	result := make([]Gene, 0, len(m))
	for g := range m {
		result = append(result, g)
	}
	return result
}

// shiftParallelGroup tries to relocate an entire SBP parallel group to a new slot.
// Accepted moves record old positions as tabu; tabu moves are accepted only via aspiration.
func shiftParallelGroup(
	grid *ScheduleMatrix,
	groupIDs []uint,
	blockByID map[uint]MatrixBlock,
	candidates []Gene,
	blocks []MatrixBlock,
	currPenalty *int,
	tabuList map[moveKey]int,
	iter, tenure int,
	bestUnplaced, bestPenalty int,
	pjokID uint,
	acak *rand.Rand,
) {
	if len(candidates) == 0 || len(groupIDs) == 0 {
		return
	}

	origPos := make(map[uint]Gene, len(groupIDs))
	for _, id := range groupIDs {
		if rec, ok := grid.Placement(id); ok {
			origPos[id] = Gene{Day: rec.Day, StartSlot: rec.StartSlot}
		}
	}

	for _, id := range groupIDs {
		_ = grid.RemoveBlock(id)
	}

	pos, ok := findGroupSlot(grid, groupIDs, blockByID, candidates, acak)
	if !ok {
		for _, id := range groupIDs {
			if g, has := origPos[id]; has {
				_ = grid.PlaceBlock(id, g.Day, g.StartSlot)
			}
		}
		return
	}

	anyTabu := false
	for _, id := range groupIDs {
		if isForbidden(tabuList, id, pos, iter) {
			anyTabu = true
			break
		}
	}

	for _, id := range groupIDs {
		_ = grid.PlaceBlock(id, pos.Day, pos.StartSlot)
	}
	newPenalty := CountSoftViolations(grid, blocks, pjokID)
	newUnplaced := len(blocks) - grid.PlacedCount()
	aspiration := newUnplaced < bestUnplaced || (newUnplaced == bestUnplaced && newPenalty < bestPenalty)

	if anyTabu && !aspiration {
		for _, id := range groupIDs {
			_ = grid.RemoveBlock(id)
			if g, has := origPos[id]; has {
				_ = grid.PlaceBlock(id, g.Day, g.StartSlot)
			}
		}
		return
	}

	for _, id := range groupIDs {
		if g, has := origPos[id]; has {
			forbidMove(tabuList, id, g, iter, tenure)
		}
	}
	*currPenalty = newPenalty
}

// ── Main entry point ──────────────────────────────────────────────────────────

// RunTS refines the best GA solution using Tabu Search with direct matrix operations.
//
// Accepted move types:
//   - Free placement: unplaced block → empty slot (net -1 unplaced, always accepted)
//   - Single displace: unplaced block displaces one placed block which is re-placed elsewhere
//   - Chain displace: unplaced block displaces two placed blocks (rejected if net +1)
//   - Swap: two placed blocks exchange slots to reduce soft penalty
//
// The tabu list forbids recently undone moves for Tenure iterations, preventing cycling.
// Tabu moves are accepted when the aspiration criterion is met (result beats global best).
// Perturbation (shaking): when best is stagnant for ShakeAfter iterations, ShakeCount
// random placed blocks are evicted to escape local optima.
func RunTS(
	gaResult GAResult,
	blocks []MatrixBlock,
	candidateIndex map[uint][]Gene,
	daySlots DaySlots,
	cfg TSConfig,
) TSResult {
	start := time.Now()
	if daySlots == nil {
		daySlots = GenerateSlots()
	}
	acak := rand.New(rand.NewSource(cfg.RandSeed))

	blockByID := make(map[uint]MatrixBlock, len(blocks))
	for _, b := range blocks {
		blockByID[b.ID] = b
	}

	groups := BuildGroupIndex(blocks)
	groupByID := make(map[uint][]uint)
	for _, indices := range groups {
		ids := make([]uint, len(indices))
		for i, idx := range indices {
			ids[i] = blocks[idx].ID
		}
		for _, id := range ids {
			groupByID[id] = ids
		}
	}

	validPos := make(map[uint]map[Gene]struct{}, len(blocks))
	for _, b := range blocks {
		posSet := make(map[Gene]struct{}, len(candidateIndex[b.ID]))
		for _, g := range candidateIndex[b.ID] {
			posSet[g] = struct{}{}
		}
		validPos[b.ID] = posSet
	}

	grid, _ := restoreMatrix(captureMatrix(gaResult.Matrix, blocks), blocks, daySlots, cfg.PJOKSubjID)

	placed := make([]uint, 0, len(blocks))
	unplaced := make([]uint, 0, len(blocks))
	for _, b := range blocks {
		if _, ok := grid.Placement(b.ID); ok {
			placed = append(placed, b.ID)
		} else {
			unplaced = append(unplaced, b.ID)
		}
	}

	tenure := cfg.Tenure
	if tenure <= 0 {
		tenure = 15
	}

	pjokID := cfg.PJOKSubjID
	st := &tsState{
		grid:        grid,
		placed:      placed,
		unplaced:    unplaced,
		currPenalty: CountSoftViolations(grid, blocks, pjokID),
		tabuList:    make(map[moveKey]int),
		blockByID:   blockByID,
		groupByID:   groupByID,
		validPos:    validPos,
	}

	bestUnplaced := len(blocks) - grid.PlacedCount()
	bestPenalty := st.currPenalty
	bestSnap := captureMatrix(grid, blocks)

	shakeStagnant := 0
	shakeCount := 0
	lastFeasibleIter := -1

	fmt.Printf("[tabu-search] start: unplaced=%d penalty=%d tenure=%d\n", bestUnplaced, bestPenalty, tenure)

	report := func(iter int) {
		if cfg.OnSnapshot != nil {
			cfg.OnSnapshot(TSProgress{
				Iteration:             iter,
				TabuListSize:          len(st.tabuList),
				CurrentUnplaced:       len(blocks) - st.grid.PlacedCount(),
				CurrentSoftViolations: st.currPenalty,
				BestUnplaced:          bestUnplaced,
				BestSoftViolations:    bestPenalty,
				Elapsed:               time.Since(start),
			})
		}
	}

	lastIter := 0
	emittedLast := false

	for iter := 1; iter <= cfg.MaxIterations; iter++ {
		if bestUnplaced == 0 && bestPenalty == 0 {
			break
		}
		lastIter = iter

		if iter%1000 == 0 {
			cleanTabuList(st.tabuList, iter)
		}

		doSwap := len(st.unplaced) == 0 || (len(st.placed) >= 2 && acak.Float64() >= 0.8)

		if !doSwap {
			handleUnplaced(st, blocks, candidateIndex, bestUnplaced, bestPenalty, iter, tenure, pjokID, acak)
		} else {
			handleSwap(st, blocks, bestUnplaced, bestPenalty, iter, tenure, pjokID, acak)
		}

		currUnplaced := len(blocks) - st.grid.PlacedCount()
		if currUnplaced < bestUnplaced || (currUnplaced == bestUnplaced && st.currPenalty < bestPenalty) {
			bestUnplaced = currUnplaced
			bestPenalty = st.currPenalty
			bestSnap = captureMatrix(st.grid, blocks)
			shakeStagnant = 0
			if bestUnplaced == 0 && lastFeasibleIter == -1 {
				lastFeasibleIter = iter
				fmt.Printf("[tabu-search] all blocks placed at iteration %d (%.1f%% of budget)\n",
					iter, float64(iter)/float64(cfg.MaxIterations)*100)
			}
		} else {
			shakeStagnant++
		}

		if cfg.ShakeAfter > 0 && cfg.ShakeCount > 0 && shakeStagnant >= cfg.ShakeAfter {
			evictCount := cfg.ShakeCount
			if evictCount > len(st.placed) {
				evictCount = len(st.placed)
			}
			evicted := make(map[uint]bool)
			for k := 0; k < evictCount; k++ {
				if len(st.placed) == 0 {
					break
				}
				idx := acak.Intn(len(st.placed))
				id := st.placed[idx]
				if evicted[id] {
					continue
				}
				toEvict := []uint{id}
				if groupIDs, ok := groupByID[id]; ok {
					toEvict = groupIDs
				}
				for _, eid := range toEvict {
					if evicted[eid] {
						continue
					}
					evicted[eid] = true
					_ = st.grid.RemoveBlock(eid)
					st.placed = dropID(st.placed, eid)
					st.unplaced = append(st.unplaced, eid)
				}
			}
			st.currPenalty = CountSoftViolations(st.grid, blocks, cfg.PJOKSubjID)
			shakeStagnant = 0
			shakeCount++
			fmt.Printf("[tabu-search] shake #%d at iteration %d, evicted %d blocks, unplaced now=%d\n",
				shakeCount, iter, evictCount, len(st.unplaced))
		}

		if cfg.ReportInterval > 0 && iter%cfg.ReportInterval == 0 {
			report(iter)
			emittedLast = true
		} else {
			emittedLast = false
		}
	}

	if !emittedLast {
		report(lastIter)
	}

	if lastFeasibleIter == -1 {
		fmt.Printf("[tabu-search] finished %d iters, unplaced never reached 0 (best=%d), shakes=%d\n",
			lastIter, bestUnplaced, shakeCount)
	} else {
		fmt.Printf("[tabu-search] finished %d iters, feasible at iter %d, shakes=%d, finalPenalty=%d\n",
			lastIter, lastFeasibleIter, shakeCount, bestPenalty)
	}

	bestGrid, actualUnplaced := restoreMatrix(bestSnap, blocks, daySlots, cfg.PJOKSubjID)
	actualPenalty := CountSoftViolations(bestGrid, blocks, cfg.PJOKSubjID)
	bestChromosome := snapshotToChromosome(bestSnap, blocks)

	return TSResult{
		Chromosome:     bestChromosome,
		Matrix:         bestGrid,
		Unplaced:       actualUnplaced,
		SoftViolations: actualPenalty,
		Iterations:     lastIter,
		Elapsed:        time.Since(start),
	}
}
