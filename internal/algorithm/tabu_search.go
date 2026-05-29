package algorithm

import (
	"fmt"
	"math/rand"
	"time"
)

type TSConfig struct {
	TabuTenure    int // iterations a move stays forbidden; 0 uses default 15
	Iterations    int
	ProgressEvery int
	Seed          int64
	PerturbCount  int // blocks to evict when stagnant; 0 = disabled
	PerturbAfter  int // iterations without improvement before perturbing; 0 = disabled
	PJOKSubjectID uint
	OnProgress    func(TSProgress)
}

type TSProgress struct {
	Iteration             int
	TabuListSize          int
	CurrentUnplaced       int
	CurrentSoftViolations int
	BestUnplaced          int
	BestSoftViolations    int
	Elapsed               time.Duration
}

type TSResult struct {
	Chromosome     Chromosome
	Matrix         *ScheduleMatrix
	Unplaced       int
	SoftViolations int
	Iterations     int
	Elapsed        time.Duration
}

func DefaultTSConfig() TSConfig {
	return TSConfig{
		TabuTenure:    15,
		Iterations:    200000,
		ProgressEvery: 5000,
		Seed:          time.Now().UnixNano(),
		PerturbCount:  10,
		PerturbAfter:  15000,
	}
}

type tabuKey struct {
	blockID uint
	gene    Gene
}

func isTabu(tabuMap map[tabuKey]int, blockID uint, gene Gene, iter int) bool {
	exp, ok := tabuMap[tabuKey{blockID, gene}]
	return ok && exp > iter
}

func addTabu(tabuMap map[tabuKey]int, blockID uint, gene Gene, iter, tenure int) {
	tabuMap[tabuKey{blockID, gene}] = iter + tenure
}

func pruneTabu(tabuMap map[tabuKey]int, iter int) {
	for k, exp := range tabuMap {
		if exp <= iter {
			delete(tabuMap, k)
		}
	}
}

// relocateGroupTS tries to move an entire parallel group to a new free slot.
// Accepts if the move is not tabu; allows tabu moves when aspiration criterion is met
// (result beats global best). Accepted moves record old positions as tabu.
func relocateGroupTS(
	matrix *ScheduleMatrix,
	groupIDs []uint,
	blockByID map[uint]MatrixBlock,
	candidates []Gene,
	blocks []MatrixBlock,
	currentSoft *int,
	tabuMap map[tabuKey]int,
	iter, tenure int,
	bestUnplaced, bestSoft int,
	pjokSubjectID uint,
	rng *rand.Rand,
) {
	if len(candidates) == 0 || len(groupIDs) == 0 {
		return
	}

	orig := make(map[uint]Gene, len(groupIDs))
	for _, id := range groupIDs {
		if p, ok := matrix.Placement(id); ok {
			orig[id] = Gene{Day: p.Day, StartSlot: p.StartSlot}
		}
	}

	for _, id := range groupIDs {
		_ = matrix.RemoveBlock(id)
	}

	gene, ok := findFreeCandidateForGroup(matrix, groupIDs, blockByID, candidates, rng)
	if !ok {
		for _, id := range groupIDs {
			if g, has := orig[id]; has {
				_ = matrix.PlaceBlock(id, g.Day, g.StartSlot)
			}
		}
		return
	}

	anyTabu := false
	for _, id := range groupIDs {
		if isTabu(tabuMap, id, gene, iter) {
			anyTabu = true
			break
		}
	}

	for _, id := range groupIDs {
		_ = matrix.PlaceBlock(id, gene.Day, gene.StartSlot)
	}
	newSoft := CountSoftViolations(matrix, blocks, pjokSubjectID)
	newUnplaced := len(blocks) - matrix.PlacedCount()
	aspiration := newUnplaced < bestUnplaced || (newUnplaced == bestUnplaced && newSoft < bestSoft)

	if anyTabu && !aspiration {
		for _, id := range groupIDs {
			_ = matrix.RemoveBlock(id)
			if g, has := orig[id]; has {
				_ = matrix.PlaceBlock(id, g.Day, g.StartSlot)
			}
		}
		return
	}

	for _, id := range groupIDs {
		if g, has := orig[id]; has {
			addTabu(tabuMap, id, g, iter, tenure)
		}
	}
	*currentSoft = newSoft
}

// RunTS applies Tabu Search to refine the best GA solution using direct matrix operations.
//
// Unlike Simulated Annealing, TS has no temperature or probabilistic acceptance.
// Instead a tabu list forbids recently-undone moves for TabuTenure iterations, preventing
// cycling. Non-tabu moves are always accepted (even when they worsen the objective),
// enabling diversification. Tabu moves are accepted only when the aspiration criterion
// is met: the resulting solution strictly beats the global best.
//
// Move types are identical to RunSA:
//   - Free placement: unplaced block finds a free slot — always accepted (net -1 unplaced).
//   - Single displace: unplaced block displaces one placed block; displaced is re-placed or traded.
//   - Chain displace: unplaced block displaces two placed blocks; net +1 always rejected.
//   - Swap: two placed blocks exchange positions to reduce soft violations.
//
// Perturbation: when best hasn't improved for PerturbAfter iterations, PerturbCount random
// placed blocks are evicted to escape local optima without a full restart.
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
	rng := rand.New(rand.NewSource(cfg.Seed))

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
		s := make(map[Gene]struct{}, len(candidateIndex[b.ID]))
		for _, g := range candidateIndex[b.ID] {
			s[g] = struct{}{}
		}
		validPos[b.ID] = s
	}

	matrix, _ := rebuildFromSnapshot(snapshotFromMatrix(gaResult.Matrix, blocks), blocks, daySlots, cfg.PJOKSubjectID)

	placed := make([]uint, 0, len(blocks))
	unplaced := make([]uint, 0, len(blocks))
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

	tenure := cfg.TabuTenure
	if tenure <= 0 {
		tenure = 15
	}
	tabuMap := make(map[tabuKey]int)

	perturbStagnant := 0
	perturbCount := 0
	lastUnplacedIter := -1
	fmt.Printf("[TS diag] start: unplaced=%d softViolations=%d tabuTenure=%d\n", bestUnplaced, bestSoft, tenure)

	emit := func(iter int) {
		if cfg.OnProgress != nil {
			cfg.OnProgress(TSProgress{
				Iteration:             iter,
				TabuListSize:          len(tabuMap),
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

		if iter%1000 == 0 {
			pruneTabu(tabuMap, iter)
		}

		doSwap := len(unplaced) == 0 || (len(placed) >= 2 && rng.Float64() >= 0.8)

		if !doSwap && len(unplaced) > 0 {
			// ----- unplaced move -----
			targetID := unplaced[rng.Intn(len(unplaced))]
			targetBlock := blockByID[targetID]
			candidates := candidateIndex[targetID]

			// Grouped blocks must be placed together.
			if groupIDs, isGrouped := groupByID[targetID]; isGrouped {
				if gene, ok := findFreeCandidateForGroup(matrix, groupIDs, blockByID, candidates, rng); ok {
					for _, id := range groupIDs {
						_ = matrix.PlaceBlock(id, gene.Day, gene.StartSlot)
						unplaced = removeID(unplaced, id)
						placed = append(placed, id)
					}
					currentSoft = CountSoftViolations(matrix, blocks, cfg.PJOKSubjectID)
				}
				continue
			}

			if len(candidates) == 0 {
				continue
			}
			gene := candidates[rng.Intn(len(candidates))]

			// Aspiration: placing an unplaced block always improves when bestUnplaced > 0.
			moveTabu := isTabu(tabuMap, targetID, gene, iter)
			aspiration := bestUnplaced > 0
			if moveTabu && !aspiration {
				continue
			}

			conflicts := findConflictingBlocks(matrix, targetBlock, gene.Day, gene.StartSlot)

			switch len(conflicts) {
			case 0:
				// Free placement: net -1, always accept. No old position to record as tabu.
				if matrix.PlaceBlock(targetID, gene.Day, gene.StartSlot) == nil {
					unplaced = removeID(unplaced, targetID)
					placed = append(placed, targetID)
					currentSoft = CountSoftViolations(matrix, blocks, cfg.PJOKSubjectID)
				}

			case 1:
				// Single displace: remove blocking block, place target, re-place displaced.
				displacedID := conflicts[0]
				if _, isGroupMember := groupByID[displacedID]; isGroupMember {
					break
				}
				displacedBlock := blockByID[displacedID]
				origDP, _ := matrix.Placement(displacedID)
				origDGene := Gene{Day: origDP.Day, StartSlot: origDP.StartSlot}

				_ = matrix.RemoveBlock(displacedID)
				placed = removeID(placed, displacedID)
				unplaced = append(unplaced, displacedID)

				if matrix.PlaceBlock(targetID, gene.Day, gene.StartSlot) != nil {
					// Day diversity or other constraint rejected targetID — restore.
					_ = matrix.PlaceBlock(displacedID, origDGene.Day, origDGene.StartSlot)
					unplaced = removeID(unplaced, displacedID)
					placed = append(placed, displacedID)
					break
				}
				unplaced = removeID(unplaced, targetID)
				placed = append(placed, targetID)

				if newGene, ok := findFreeCandidate(matrix, displacedBlock, candidateIndex[displacedID], rng); ok {
					// Re-placed: net -1, always accept. Record displaced's old position as tabu.
					_ = matrix.PlaceBlock(displacedID, newGene.Day, newGene.StartSlot)
					unplaced = removeID(unplaced, displacedID)
					placed = append(placed, displacedID)
					addTabu(tabuMap, displacedID, origDGene, iter, tenure)
					currentSoft = CountSoftViolations(matrix, blocks, cfg.PJOKSubjectID)
				} else {
					// Trade (net 0): in TS, accept unconditionally (tabu/aspiration already checked).
					addTabu(tabuMap, displacedID, origDGene, iter, tenure)
					currentSoft = CountSoftViolations(matrix, blocks, cfg.PJOKSubjectID)
				}

			case 2:
				// Chain displace: remove both blockers, place target, re-place both.
				d1, d2 := conflicts[0], conflicts[1]
				if _, ok := groupByID[d1]; ok {
					break
				}
				if _, ok := groupByID[d2]; ok {
					break
				}
				db1, db2 := blockByID[d1], blockByID[d2]
				origP1, _ := matrix.Placement(d1)
				origP2, _ := matrix.Placement(d2)
				origG1 := Gene{Day: origP1.Day, StartSlot: origP1.StartSlot}
				origG2 := Gene{Day: origP2.Day, StartSlot: origP2.StartSlot}

				_ = matrix.RemoveBlock(d1)
				placed = removeID(placed, d1)
				unplaced = append(unplaced, d1)
				_ = matrix.RemoveBlock(d2)
				placed = removeID(placed, d2)
				unplaced = append(unplaced, d2)

				if matrix.PlaceBlock(targetID, gene.Day, gene.StartSlot) != nil {
					// Restore d1, d2 and abort.
					_ = matrix.PlaceBlock(d1, origG1.Day, origG1.StartSlot)
					unplaced = removeID(unplaced, d1)
					placed = append(placed, d1)
					_ = matrix.PlaceBlock(d2, origG2.Day, origG2.StartSlot)
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

				if ok1 || ok2 {
					// net -1 (both ok) or net 0 (one ok): accept, record tabu for evicted positions.
					addTabu(tabuMap, d1, origG1, iter, tenure)
					addTabu(tabuMap, d2, origG2, iter, tenure)
					currentSoft = CountSoftViolations(matrix, blocks, cfg.PJOKSubjectID)
				} else {
					// net +1: always reject — fully revert.
					_ = matrix.RemoveBlock(targetID)
					placed = removeID(placed, targetID)
					unplaced = append(unplaced, targetID)
					unplaced = removeID(unplaced, d1)
					unplaced = removeID(unplaced, d2)
					_ = matrix.PlaceBlock(d1, origG1.Day, origG1.StartSlot)
					_ = matrix.PlaceBlock(d2, origG2.Day, origG2.StartSlot)
					placed = append(placed, d1, d2)
				}
			}

		} else if len(placed) >= 2 {
			// ----- swap move -----
			i := rng.Intn(len(placed))
			j := rng.Intn(len(placed))
			for j == i {
				j = rng.Intn(len(placed))
			}
			idA, idB := placed[i], placed[j]

			if groupIDs, ok := groupByID[idA]; ok {
				relocateGroupTS(matrix, groupIDs, blockByID, candidateIndex[idA], blocks,
					&currentSoft, tabuMap, iter, tenure, bestUnplaced, bestSoft, cfg.PJOKSubjectID, rng)
				continue
			}
			if groupIDs, ok := groupByID[idB]; ok {
				relocateGroupTS(matrix, groupIDs, blockByID, candidateIndex[idB], blocks,
					&currentSoft, tabuMap, iter, tenure, bestUnplaced, bestSoft, cfg.PJOKSubjectID, rng)
				continue
			}

			pA, okA := matrix.Placement(idA)
			pB, okB := matrix.Placement(idB)
			if !okA || !okB {
				continue
			}

			newGeneA := Gene{Day: pB.Day, StartSlot: pB.StartSlot}
			newGeneB := Gene{Day: pA.Day, StartSlot: pA.StartSlot}

			if _, aOK := validPos[idA][newGeneA]; !aOK {
				continue
			}
			if _, bOK := validPos[idB][newGeneB]; !bOK {
				continue
			}

			swapTabuA := isTabu(tabuMap, idA, newGeneA, iter)
			swapTabuB := isTabu(tabuMap, idB, newGeneB, iter)

			_ = matrix.RemoveBlock(idA)
			_ = matrix.RemoveBlock(idB)
			errA := matrix.PlaceBlock(idA, pB.Day, pB.StartSlot)
			errB := matrix.PlaceBlock(idB, pA.Day, pA.StartSlot)

			if errA != nil || errB != nil {
				if errA == nil {
					_ = matrix.RemoveBlock(idA)
				}
				if errB == nil {
					_ = matrix.RemoveBlock(idB)
				}
				_ = matrix.PlaceBlock(idA, pA.Day, pA.StartSlot)
				_ = matrix.PlaceBlock(idB, pB.Day, pB.StartSlot)
				continue
			}

			newSoft := CountSoftViolations(matrix, blocks, cfg.PJOKSubjectID)
			newUnplaced := len(blocks) - matrix.PlacedCount()
			aspiration := newUnplaced < bestUnplaced || (newUnplaced == bestUnplaced && newSoft < bestSoft)

			if (swapTabuA || swapTabuB) && !aspiration {
				// Tabu and no aspiration: revert.
				_ = matrix.RemoveBlock(idA)
				_ = matrix.RemoveBlock(idB)
				_ = matrix.PlaceBlock(idA, pA.Day, pA.StartSlot)
				_ = matrix.PlaceBlock(idB, pB.Day, pB.StartSlot)
			} else {
				// Accept: record old positions as tabu to prevent immediate undo.
				addTabu(tabuMap, idA, Gene{Day: pA.Day, StartSlot: pA.StartSlot}, iter, tenure)
				addTabu(tabuMap, idB, Gene{Day: pB.Day, StartSlot: pB.StartSlot}, iter, tenure)
				currentSoft = newSoft
			}
		}

		matrixUnplaced := len(blocks) - matrix.PlacedCount()
		if matrixUnplaced < bestUnplaced || (matrixUnplaced == bestUnplaced && currentSoft < bestSoft) {
			bestUnplaced = matrixUnplaced
			bestSoft = currentSoft
			bestSnap = snapshotFromMatrix(matrix, blocks)
			perturbStagnant = 0
			if bestUnplaced == 0 && lastUnplacedIter == -1 {
				lastUnplacedIter = iter
				fmt.Printf("[TS diag] last unplaced block placed at iteration %d (%.1f%% of budget)\n",
					iter, float64(iter)/float64(cfg.Iterations)*100)
			}
		} else {
			perturbStagnant++
		}

		if cfg.PerturbAfter > 0 && cfg.PerturbCount > 0 && perturbStagnant >= cfg.PerturbAfter {
			count := cfg.PerturbCount
			if count > len(placed) {
				count = len(placed)
			}
			evicted := make(map[uint]bool)
			for k := 0; k < count; k++ {
				if len(placed) == 0 {
					break
				}
				idx := rng.Intn(len(placed))
				id := placed[idx]
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
					_ = matrix.RemoveBlock(eid)
					placed = removeID(placed, eid)
					unplaced = append(unplaced, eid)
				}
			}
			currentSoft = CountSoftViolations(matrix, blocks, cfg.PJOKSubjectID)
			perturbStagnant = 0
			perturbCount++
			fmt.Printf("[TS diag] perturbation #%d at iteration %d, evicted %d blocks, unplaced now=%d\n",
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
		fmt.Printf("[TS diag] finished %d iters, unplaced never reached 0 (best=%d), perturbations=%d\n",
			lastIter, bestUnplaced, perturbCount)
	} else {
		fmt.Printf("[TS diag] finished %d iters, unplaced→0 at iter %d, perturbations=%d, finalSoft=%d\n",
			lastIter, lastUnplacedIter, perturbCount, bestSoft)
	}

	bestMatrix, actualUnplaced := rebuildFromSnapshot(bestSnap, blocks, daySlots, cfg.PJOKSubjectID)
	actualSoft := CountSoftViolations(bestMatrix, blocks, cfg.PJOKSubjectID)
	bestChromosome := chromosomeFromSnapshot(bestSnap, blocks)

	return TSResult{
		Chromosome:     bestChromosome,
		Matrix:         bestMatrix,
		Unplaced:       actualUnplaced,
		SoftViolations: actualSoft,
		Iterations:     lastIter,
		Elapsed:        time.Since(start),
	}
}
