package algorithm

import (
	"math/rand"
	"time"

	"github.com/google/uuid"
)

// ── Public types ──────────────────────────────────────────────────────────────

// TSConfig holds all tunable parameters for the Tabu Search phase.
type TSConfig struct {
	MaxIterations    int
	TabuTenure       int // iterations a move stays forbidden
	NeighborhoodSize int // candidate moves evaluated per iteration
	PerturbAfter     int // diversify after this many stagnant iterations
	PerturbCount     int // random moves applied during perturbation
	Seed             int64
	ProgressEvery    int
	PJOKSubjectID    uint
	OnProgress       func(TSProgress)
}

// TSProgress is emitted periodically during TS execution.
type TSProgress struct {
	Iteration       int
	TotalIterations int
	CurrentUnplaced int
	CurrentSoft     int
	BestUnplaced    int
	BestSoft        int
	TabuListSize    int
	Elapsed         time.Duration
}

// TSResult is the output of RunTS.
type TSResult struct {
	Chromosome     Chromosome
	Matrix         *ScheduleMatrix
	Unplaced       int
	SoftViolations int
	Iterations     int
	Elapsed        time.Duration
}

// DefaultTSConfig returns defaults tuned for school-size scheduling problems.
// TabuTenure ≈ sqrt(typical block count); NeighborhoodSize balances coverage vs speed.
func DefaultTSConfig() TSConfig {
	return TSConfig{
		MaxIterations:    1000,
		TabuTenure:       10,
		NeighborhoodSize: 40,
		PerturbAfter:     50,
		PerturbCount:     5,
		ProgressEvery:    50,
	}
}

// ── Internal types ────────────────────────────────────────────────────────────

type tsMoveKind int

const (
	tsMovePlace    tsMoveKind = iota // place an unplaced (logical) block
	tsMoveRelocate                   // move a placed block to a new slot
	tsMoveSwap                       // swap positions of two placed singleton blocks
)

// tsMove encodes one neighbourhood move.
// For Swap: idA moves to (dayA,slotA) and idB moves to (dayB,slotB).
// This means idA's OLD position was (dayB,slotB) and idB's OLD position was (dayA,slotA).
type tsMove struct {
	kind  tsMoveKind
	idA   uuid.UUID
	dayA  string
	slotA int
	idB   uuid.UUID // Swap only
	dayB  string    // Swap only — also encodes idA's old position
	slotB int       // Swap only — also encodes idA's old position
}

type tsTabuEntry struct {
	blockID   uuid.UUID
	day       string
	slot      int
	expiresAt int
}

// ── RunTS ─────────────────────────────────────────────────────────────────────

// RunTS refines the schedule produced by GA using Tabu Search.
// It starts from the GA chromosome and iteratively improves toward
// 0 unplaced blocks and 0 soft violations.
func RunTS(
	blocks []MatrixBlock,
	candidateIndex map[uuid.UUID][]Gene,
	daySlots DaySlots,
	gaResult GAResult,
	cfg TSConfig,
) TSResult {
	if daySlots == nil {
		daySlots = GenerateSlots()
	}
	if cfg.Seed == 0 {
		cfg.Seed = time.Now().UnixNano()
	}
	start := time.Now()
	rng := rand.New(rand.NewSource(cfg.Seed))
	groups := BuildGroupIndex(blocks)
	blockIdx := tsBuildBlockLookup(blocks)

	// Decode GA chromosome into a fresh matrix with all constraints active.
	matrix, currentUnplaced := DecodeChromosome(gaResult.Chromosome, blocks, daySlots, cfg.PJOKSubjectID)
	currentSoft := CountSoftViolations(matrix, blocks, cfg.PJOKSubjectID)

	// Track best solution as a chromosome (cheap copy, reconstructable).
	bestChrom := tsBuildChromosome(matrix, blocks)
	bestUnplaced, bestSoft := currentUnplaced, currentSoft

	tabuList := make([]tsTabuEntry, 0, cfg.TabuTenure*20)
	stagnant := 0
	lastIter := 0

	emit := func(iter int) {
		if cfg.OnProgress != nil {
			cfg.OnProgress(TSProgress{
				Iteration:       iter,
				TotalIterations: cfg.MaxIterations,
				CurrentUnplaced: currentUnplaced,
				CurrentSoft:     currentSoft,
				BestUnplaced:    bestUnplaced,
				BestSoft:        bestSoft,
				TabuListSize:    len(tabuList),
				Elapsed:         time.Since(start),
			})
		}
	}

	for iter := 0; iter < cfg.MaxIterations; iter++ {
		lastIter = iter
		if bestUnplaced == 0 && bestSoft == 0 {
			break
		}

		tabuList = tsExpireTabu(tabuList, iter)

		moves := tsNeighbourhood(blocks, blockIdx, candidateIndex, matrix, groups, cfg.NeighborhoodSize, rng)

		// Find best non-tabu move; aspiration overrides tabu when beating global best.
		globalBestFit := tsFit(bestUnplaced, bestSoft)
		chosen := -1
		chosenFit := -1
		chosenUnplaced, chosenSoft := 0, 0

		for mi, m := range moves {
			nu, ns, ok := tsEval(matrix, m, blocks, blockIdx, groups, currentUnplaced, cfg.PJOKSubjectID)
			if !ok {
				continue
			}
			nf := tsFit(nu, ns)
			if tsCheckTabu(tabuList, m, iter) && nf >= globalBestFit {
				continue
			}
			if chosen == -1 || nf < chosenFit {
				chosen, chosenFit = mi, nf
				chosenUnplaced, chosenSoft = nu, ns
			}
		}

		if chosen == -1 {
			// No valid move found — perturb to escape stagnation.
			tsPerturb(matrix, blocks, blockIdx, candidateIndex, groups, cfg.PerturbCount, rng)
			currentUnplaced = len(blocks) - matrix.PlacedCount()
			currentSoft = CountSoftViolations(matrix, blocks, cfg.PJOKSubjectID)
			stagnant++
		} else {
			m := moves[chosen]
			// Read old position BEFORE committing the move (needed for tabu entry).
			oldDay, oldSlot := tsOldPos(matrix, m)
			tsApply(matrix, m, blocks, blockIdx, groups)
			currentUnplaced, currentSoft = chosenUnplaced, chosenSoft

			// Forbid returning to the position we just left.
			tabuList = append(tabuList, tsTabuEntry{m.idA, oldDay, oldSlot, iter + cfg.TabuTenure})
			if m.kind == tsMoveSwap {
				// idB's old position was (dayA, slotA).
				tabuList = append(tabuList, tsTabuEntry{m.idB, m.dayA, m.slotA, iter + cfg.TabuTenure})
			}

			if tsFit(currentUnplaced, currentSoft) < tsFit(bestUnplaced, bestSoft) {
				bestChrom = tsBuildChromosome(matrix, blocks)
				bestUnplaced, bestSoft = currentUnplaced, currentSoft
				stagnant = 0
			} else {
				stagnant++
			}
		}

		// Diversification: reset to best then apply a larger perturbation.
		if stagnant >= cfg.PerturbAfter {
			matrix, currentUnplaced = DecodeChromosome(bestChrom, blocks, daySlots, cfg.PJOKSubjectID)
			currentSoft = CountSoftViolations(matrix, blocks, cfg.PJOKSubjectID)
			tsPerturb(matrix, blocks, blockIdx, candidateIndex, groups, cfg.PerturbCount*2, rng)
			currentUnplaced = len(blocks) - matrix.PlacedCount()
			currentSoft = CountSoftViolations(matrix, blocks, cfg.PJOKSubjectID)
			tabuList = tabuList[:0]
			stagnant = 0
		}

		if cfg.ProgressEvery > 0 && (iter+1)%cfg.ProgressEvery == 0 {
			emit(iter + 1)
		}
	}
	emit(lastIter)

	finalMatrix, finalUnplaced := DecodeChromosome(bestChrom, blocks, daySlots, cfg.PJOKSubjectID)
	finalSoft := CountSoftViolations(finalMatrix, blocks, cfg.PJOKSubjectID)
	return TSResult{
		Chromosome:     bestChrom,
		Matrix:         finalMatrix,
		Unplaced:       finalUnplaced,
		SoftViolations: finalSoft,
		Iterations:     lastIter + 1,
		Elapsed:        time.Since(start),
	}
}

// ── Neighbourhood generation ──────────────────────────────────────────────────

// tsNeighbourhood generates candidate moves from the current matrix state.
// Priority: Place moves for unplaced blocks first, then Relocate and Swap.
func tsNeighbourhood(
	blocks []MatrixBlock,
	blockIdx map[uuid.UUID]int,
	candidateIndex map[uuid.UUID][]Gene,
	matrix *ScheduleMatrix,
	groups GroupIndex,
	maxSize int,
	rng *rand.Rand,
) []tsMove {
	type unit struct {
		block   MatrixBlock
		isGroup bool
	}

	var unplaced, placed []unit
	seenGroup := make(map[string]bool)

	for _, b := range blocks {
		if b.GroupKey != nil {
			if seenGroup[*b.GroupKey] {
				continue
			}
			// Use only the first member of each group as representative.
			gidx := groups[*b.GroupKey]
			if len(gidx) == 0 || blocks[gidx[0]].ID != b.ID {
				continue
			}
			seenGroup[*b.GroupKey] = true
		}
		_, isPlaced := matrix.Placement(b.ID)
		u := unit{b, b.GroupKey != nil}
		if isPlaced {
			placed = append(placed, u)
		} else {
			unplaced = append(unplaced, u)
		}
	}

	var moves []tsMove
	slot := tsMax(1, maxSize/3)

	// Place moves.
	for _, u := range unplaced {
		cands := candidateIndex[u.block.ID]
		if len(cands) == 0 {
			continue
		}
		limit := tsMin(len(cands), tsMax(slot/tsMax(1, len(unplaced)), 2))
		perm := rng.Perm(len(cands))
		for j := 0; j < limit; j++ {
			g := cands[perm[j]]
			moves = append(moves, tsMove{kind: tsMovePlace, idA: u.block.ID, dayA: g.Day, slotA: g.StartSlot})
		}
	}

	// Shuffle placed units for variety.
	rng.Shuffle(len(placed), func(i, j int) { placed[i], placed[j] = placed[j], placed[i] })

	// Relocate moves.
	numRelocate := slot
	for _, u := range placed {
		if numRelocate <= 0 {
			break
		}
		cands := candidateIndex[u.block.ID]
		if len(cands) == 0 {
			continue
		}
		cur, ok := matrix.Placement(u.block.ID)
		if !ok {
			continue
		}
		perm := rng.Perm(len(cands))
		for _, pi := range perm {
			g := cands[pi]
			if g.Day == cur.Day && g.StartSlot == cur.StartSlot {
				continue
			}
			moves = append(moves, tsMove{kind: tsMoveRelocate, idA: u.block.ID, dayA: g.Day, slotA: g.StartSlot})
			numRelocate--
			break
		}
	}

	// Swap moves: only between singletons with the same duration.
	numSwap := slot
	outer:
	for i := 0; i < len(placed) && numSwap > 0; i++ {
		if placed[i].isGroup {
			continue
		}
		for j := i + 1; j < len(placed) && numSwap > 0; j++ {
			if placed[j].isGroup {
				continue
			}
			uA, uB := placed[i], placed[j]
			if uA.block.Duration != uB.block.Duration {
				continue
			}
			posA, okA := matrix.Placement(uA.block.ID)
			posB, okB := matrix.Placement(uB.block.ID)
			if !okA || !okB {
				continue
			}
			moves = append(moves, tsMove{
				kind:  tsMoveSwap,
				idA:   uA.block.ID, dayA: posB.Day, slotA: posB.StartSlot,
				idB:   uB.block.ID, dayB: posA.Day, slotB: posA.StartSlot,
			})
			numSwap--
			if len(moves) >= maxSize {
				break outer
			}
		}
	}

	return moves
}

// ── Move evaluation (apply + measure + revert) ────────────────────────────────

func tsEval(
	matrix *ScheduleMatrix,
	m tsMove,
	blocks []MatrixBlock,
	blockIdx map[uuid.UUID]int,
	groups GroupIndex,
	currentUnplaced int,
	pjokSubjectID uint,
) (newUnplaced, newSoft int, valid bool) {
	switch m.kind {

	case tsMovePlace:
		ids := tsGroupMembers(m.idA, blocks, blockIdx, groups)
		for _, id := range ids {
			if matrix.CanPlaceBlock(id, m.dayA, m.slotA) != nil {
				return 0, 0, false
			}
		}
		for _, id := range ids {
			_ = matrix.PlaceBlock(id, m.dayA, m.slotA)
		}
		nu := currentUnplaced - len(ids)
		ns := CountSoftViolations(matrix, blocks, pjokSubjectID)
		for _, id := range ids {
			_ = matrix.RemoveBlock(id)
		}
		return nu, ns, true

	case tsMoveRelocate:
		ids := tsGroupMembers(m.idA, blocks, blockIdx, groups)
		saved := make(map[uuid.UUID]BlockPlacement, len(ids))
		for _, id := range ids {
			if p, ok := matrix.Placement(id); ok {
				saved[id] = p
				_ = matrix.RemoveBlock(id)
			}
		}
		feasible := true
		for _, id := range ids {
			if matrix.CanPlaceBlock(id, m.dayA, m.slotA) != nil {
				feasible = false
				break
			}
		}
		if !feasible {
			for _, id := range ids {
				if p, has := saved[id]; has {
					_ = matrix.PlaceBlock(id, p.Day, p.StartSlot)
				}
			}
			return 0, 0, false
		}
		for _, id := range ids {
			_ = matrix.PlaceBlock(id, m.dayA, m.slotA)
		}
		ns := CountSoftViolations(matrix, blocks, pjokSubjectID)
		for _, id := range ids {
			_ = matrix.RemoveBlock(id)
		}
		for _, id := range ids {
			if p, has := saved[id]; has {
				_ = matrix.PlaceBlock(id, p.Day, p.StartSlot)
			}
		}
		return currentUnplaced, ns, true

	case tsMoveSwap:
		// idA was at (dayB,slotB); idB was at (dayA,slotA).
		_ = matrix.RemoveBlock(m.idA)
		_ = matrix.RemoveBlock(m.idB)
		okA := matrix.CanPlaceBlock(m.idA, m.dayA, m.slotA)
		okB := matrix.CanPlaceBlock(m.idB, m.dayB, m.slotB)
		if okA != nil || okB != nil {
			_ = matrix.PlaceBlock(m.idA, m.dayB, m.slotB)
			_ = matrix.PlaceBlock(m.idB, m.dayA, m.slotA)
			return 0, 0, false
		}
		_ = matrix.PlaceBlock(m.idA, m.dayA, m.slotA)
		_ = matrix.PlaceBlock(m.idB, m.dayB, m.slotB)
		ns := CountSoftViolations(matrix, blocks, pjokSubjectID)
		_ = matrix.RemoveBlock(m.idA)
		_ = matrix.RemoveBlock(m.idB)
		_ = matrix.PlaceBlock(m.idA, m.dayB, m.slotB)
		_ = matrix.PlaceBlock(m.idB, m.dayA, m.slotA)
		return currentUnplaced, ns, true
	}
	return 0, 0, false
}

// ── Move application ──────────────────────────────────────────────────────────

func tsApply(matrix *ScheduleMatrix, m tsMove, blocks []MatrixBlock, blockIdx map[uuid.UUID]int, groups GroupIndex) {
	switch m.kind {
	case tsMovePlace:
		for _, id := range tsGroupMembers(m.idA, blocks, blockIdx, groups) {
			_ = matrix.PlaceBlock(id, m.dayA, m.slotA)
		}
	case tsMoveRelocate:
		ids := tsGroupMembers(m.idA, blocks, blockIdx, groups)
		for _, id := range ids {
			_ = matrix.RemoveBlock(id)
		}
		for _, id := range ids {
			_ = matrix.PlaceBlock(id, m.dayA, m.slotA)
		}
	case tsMoveSwap:
		_ = matrix.RemoveBlock(m.idA)
		_ = matrix.RemoveBlock(m.idB)
		_ = matrix.PlaceBlock(m.idA, m.dayA, m.slotA)
		_ = matrix.PlaceBlock(m.idB, m.dayB, m.slotB)
	}
}

// ── Perturbation ──────────────────────────────────────────────────────────────

// tsPerturb relocates count random placed logical units to random candidates.
// Errors are ignored — best-effort disruption is sufficient for diversification.
func tsPerturb(
	matrix *ScheduleMatrix,
	blocks []MatrixBlock,
	blockIdx map[uuid.UUID]int,
	candidateIndex map[uuid.UUID][]Gene,
	groups GroupIndex,
	count int,
	rng *rand.Rand,
) {
	var reps []MatrixBlock
	seen := make(map[string]bool)
	for _, b := range blocks {
		if b.GroupKey != nil {
			if seen[*b.GroupKey] {
				continue
			}
			gidx := groups[*b.GroupKey]
			if len(gidx) == 0 || blocks[gidx[0]].ID != b.ID {
				continue
			}
			seen[*b.GroupKey] = true
		}
		if _, ok := matrix.Placement(b.ID); ok {
			reps = append(reps, b)
		}
	}
	if len(reps) == 0 {
		return
	}
	rng.Shuffle(len(reps), func(i, j int) { reps[i], reps[j] = reps[j], reps[i] })
	for i := 0; i < tsMin(count, len(reps)); i++ {
		b := reps[i]
		cands := candidateIndex[b.ID]
		if len(cands) == 0 {
			continue
		}
		g := cands[rng.Intn(len(cands))]
		ids := tsGroupMembers(b.ID, blocks, blockIdx, groups)
		for _, id := range ids {
			_ = matrix.RemoveBlock(id)
		}
		for _, id := range ids {
			_ = matrix.PlaceBlock(id, g.Day, g.StartSlot)
		}
	}
}

// ── Tabu list helpers ─────────────────────────────────────────────────────────

func tsExpireTabu(list []tsTabuEntry, currentIter int) []tsTabuEntry {
	out := list[:0]
	for _, e := range list {
		if e.expiresAt > currentIter {
			out = append(out, e)
		}
	}
	return out
}

// tsCheckTabu returns true if the move is forbidden by the active tabu list.
// A move is tabu when placing its primary block at the target position is forbidden.
func tsCheckTabu(list []tsTabuEntry, m tsMove, currentIter int) bool {
	for _, e := range list {
		if e.expiresAt <= currentIter {
			continue
		}
		switch m.kind {
		case tsMovePlace, tsMoveRelocate:
			if e.blockID == m.idA && e.day == m.dayA && e.slot == m.slotA {
				return true
			}
		case tsMoveSwap:
			if (e.blockID == m.idA && e.day == m.dayA && e.slot == m.slotA) ||
				(e.blockID == m.idB && e.day == m.dayB && e.slot == m.slotB) {
				return true
			}
		}
	}
	return false
}

// tsOldPos returns the "from" position of the primary block, used as the tabu entry
// added after the move executes. For Swap, returns idA's old position = (dayB, slotB).
func tsOldPos(matrix *ScheduleMatrix, m tsMove) (day string, slot int) {
	switch m.kind {
	case tsMoveRelocate:
		if p, ok := matrix.Placement(m.idA); ok {
			return p.Day, p.StartSlot
		}
	case tsMoveSwap:
		return m.dayB, m.slotB
	}
	return "", 0
}

// ── Utility helpers ───────────────────────────────────────────────────────────

func tsFit(unplaced, soft int) int { return unplaced*1000 + soft }

func tsBuildBlockLookup(blocks []MatrixBlock) map[uuid.UUID]int {
	m := make(map[uuid.UUID]int, len(blocks))
	for i, b := range blocks {
		m[b.ID] = i
	}
	return m
}

// tsBuildChromosome constructs a chromosome from the current matrix placements.
func tsBuildChromosome(matrix *ScheduleMatrix, blocks []MatrixBlock) Chromosome {
	c := NewChromosome(len(blocks))
	for i, b := range blocks {
		if p, ok := matrix.Placement(b.ID); ok {
			c.Set(i, Gene{Day: p.Day, StartSlot: p.StartSlot})
		}
	}
	return c
}

// tsGroupMembers returns all block IDs belonging to the same group as repID.
// Returns {repID} if repID is not a group member.
func tsGroupMembers(repID uuid.UUID, blocks []MatrixBlock, blockIdx map[uuid.UUID]int, groups GroupIndex) []uuid.UUID {
	i, ok := blockIdx[repID]
	if !ok || blocks[i].GroupKey == nil {
		return []uuid.UUID{repID}
	}
	indices := groups[*blocks[i].GroupKey]
	ids := make([]uuid.UUID, len(indices))
	for k, j := range indices {
		ids[k] = blocks[j].ID
	}
	return ids
}

func tsMin(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func tsMax(a, b int) int {
	if a > b {
		return a
	}
	return b
}
