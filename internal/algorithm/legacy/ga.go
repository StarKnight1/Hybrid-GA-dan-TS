package algorithm

import (
	"errors"
	"math"
	"math/rand"
	"slices"
	"time"

	"github.com/google/uuid"
)

// lockLevel controls how a placement unit is protected from optimisation moves.
//
//	lockNone — moved freely by mutation, SA repair, ILS perturbation, and restarts.
//	lockSoft — PJOK: skipped by mutation and SA repair; eligible for ILS/restart perturbation.
//	lockHard — SBP: placed once by greedy construction, never moved again by anything.
type lockLevel uint8

const (
	lockNone lockLevel = 0
	lockSoft lockLevel = 1
	lockHard lockLevel = 2
)

type slotCell struct {
	day  string
	slot int
}

type classOccKey struct {
	cell    slotCell
	classID uuid.UUID
}

type teacherOccKey struct {
	cell      slotCell
	teacherID uuid.UUID
}

// LegacyGAProgress is emitted on every progress tick during RunGA.
type LegacyGAProgress struct {
	Generation       int
	TotalGenerations int
	BestFitness      int
	BestViolations   int
	BestUnplaced     int
	Elapsed          time.Duration
	AvgFitness       int
	WorstFitness     int
	DiversityScore   float64
	StagnantGens     int
	SAImprovements   int
	MutationHits     int
	FeasibleCount    int
	Breakdown        ViolationBreakdown
}

// ViolationBreakdown categorises hard-constraint violations by type.
type ViolationBreakdown struct {
	ClassConflicts    int
	TeacherConflicts  int
	SiblingViolations int
}

// LegacyGAConfig holds all tunable parameters for RunGA.
type LegacyGAConfig struct {
	PopulationSize int
	Generations    int
	MutationRate   float64
	EliteCount     int
	SAIterations   int // SA + ILS passes applied to each child after crossover
	Seed           int64
	ProgressEvery  int
	OnProgress     func(LegacyGAProgress)
}

// DefaultLegacyGAConfig returns sensible starting parameters.
func DefaultLegacyGAConfig() LegacyGAConfig {
	return LegacyGAConfig{
		PopulationSize: 100,
		Generations:    300,
		MutationRate:   0.05,
		EliteCount:     5,
		SAIterations:   200,
		Seed:           time.Now().UnixNano(),
		ProgressEvery:  10,
	}
}

// GAChromosome represents one candidate timetable solution (legacy GA).
// UnitSlots[i] is the ValidSlot assigned to units[i].
type GAChromosome struct {
	UnitSlots      []ValidSlot
	Fitness        int
	ViolationCount int
	UnplacedCount  int
	Breakdown      ViolationBreakdown
}

// RunGA runs a Genetic Algorithm with Simulated Annealing local search to find
// a zero-violation school timetable.
//
// Placement priority (MCF greedy construction):
//  1. SBP parallel groups (lockHard) — placed first; constrain multiple classes at once.
//  2. PJOK blocks (lockSoft) — placed second; hard time-window constraint.
//  3. 3JP → 2JP → 1JP — longer blocks have fewer fitting windows.
//  4. Fewest candidates — standard MCF tiebreak.
//
// Local search per child: conflict-aware SA with ILS escape when stuck.
// Diversity: soft restart every 15 stagnant gens; hard restart every 50.
// Early exit: stops as soon as a zero-violation chromosome is found.
func LegacyRunGA(units []PlacementUnit, validSlots map[uuid.UUID][]ValidSlot, cfg LegacyGAConfig) (*GAChromosome, error) {
	if len(units) == 0 {
		return nil, errors.New("no placement units")
	}
	if cfg.PopulationSize < 2 {
		cfg.PopulationSize = 2
	}
	if cfg.EliteCount < 1 {
		cfg.EliteCount = 1
	}
	if cfg.EliteCount >= cfg.PopulationSize {
		cfg.EliteCount = cfg.PopulationSize - 1
	}
	if cfg.ProgressEvery <= 0 {
		cfg.ProgressEvery = 10
	}

	started := time.Now()
	var genSAImprovements int
	var genMutationHits int

	emit := func(gen int, best *GAChromosome, pop []*GAChromosome, stagnant int) {
		if cfg.OnProgress == nil || best == nil {
			return
		}

		total := 0
		worst := pop[0].Fitness
		feasibleCount := 0
		for _, c := range pop {
			total += c.Fitness
			if c.Fitness < worst {
				worst = c.Fitness
			}
			if c.ViolationCount == 0 && c.UnplacedCount == 0 {
				feasibleCount++
			}
		}
		avg := total / len(pop)

		diversitySum := 0.0
		for i := range units {
			seen := map[slotCell]struct{}{}
			for _, c := range pop {
				seen[slotCell{day: c.UnitSlots[i].Day, slot: c.UnitSlots[i].SlotIndex}] = struct{}{}
			}
			diversitySum += float64(len(seen))
		}
		if len(units) > 0 {
			diversitySum = (diversitySum / float64(len(units))) / float64(len(pop))
		}

		cfg.OnProgress(LegacyGAProgress{
			Generation:       gen,
			TotalGenerations: cfg.Generations,
			BestFitness:      best.Fitness,
			BestViolations:   best.ViolationCount,
			BestUnplaced:     best.UnplacedCount,
			Elapsed:          time.Since(started),
			AvgFitness:       avg,
			WorstFitness:     worst,
			DiversityScore:   diversitySum,
			StagnantGens:     stagnant,
			SAImprovements:   genSAImprovements,
			MutationHits:     genMutationHits,
			FeasibleCount:    feasibleCount,
			Breakdown:        best.Breakdown,
		})

		genSAImprovements = 0
		genMutationHits = 0
	}

	rng := rand.New(rand.NewSource(cfg.Seed))
	locks := detectLocks(units)

	// First half: MCF-greedy (starts near zero violations).
	// Second half: random (adds search diversity).
	pop := make([]*GAChromosome, 0, cfg.PopulationSize)
	halfPop := cfg.PopulationSize / 2
	for len(pop) < cfg.PopulationSize {
		var ch *GAChromosome
		if len(pop) < halfPop {
			ch = greedyChromosome(units, validSlots, locks, rng)
		} else {
			ch = randomChromosome(units, validSlots, rng)
		}
		evaluateChromosome(ch, units)
		pop = append(pop, ch)
	}

	best := cloneChromosome(findBestChromosome(pop))
	stagnantGens := 0
	emit(0, best, pop, stagnantGens)

	if best.Fitness == 0 && best.ViolationCount == 0 && best.UnplacedCount == 0 {
		return best, nil
	}

	for g := 0; g < cfg.Generations; g++ {
		sortByFitnessDesc(pop)

		currentBest := findBestChromosome(pop)
		if betterChromosome(currentBest, best) {
			best = cloneChromosome(currentBest)
			stagnantGens = 0
		} else {
			stagnantGens++
		}

		// Soft diversity: replace weakest 20% with 30%-perturbed clones of best.
		if stagnantGens > 0 && stagnantGens%15 == 0 && stagnantGens%50 != 0 {
			replaceCount := cfg.PopulationSize / 5
			for i := cfg.PopulationSize - replaceCount; i < cfg.PopulationSize; i++ {
				pop[i] = perturbChromosome(best, 0.30, units, validSlots, locks, rng)
				evaluateChromosome(pop[i], units)
			}
		}

		// Hard restart: keep elite, then fill with a diversified pool so the
		// restarted population can explore different basins of attraction.
		// 1/3 perturbed clones of best (exploit best-known region)
		// 1/3 fresh greedy chromosomes  (new MCF-greedy starting points)
		// 1/3 fully random chromosomes  (extreme diversity)
		if stagnantGens > 0 && stagnantGens%50 == 0 {
			restartSize := cfg.PopulationSize - cfg.EliteCount
			third := max(1, restartSize/3)
			for i := cfg.EliteCount; i < cfg.PopulationSize; i++ {
				offset := i - cfg.EliteCount
				var ch *GAChromosome
				switch {
				case offset < third:
					rate := 0.30 + rng.Float64()*0.40
					ch = perturbChromosome(best, rate, units, validSlots, locks, rng)
				case offset < 2*third:
					ch = greedyChromosome(units, validSlots, locks, rng)
				default:
					ch = randomChromosome(units, validSlots, rng)
				}
				evaluateChromosome(ch, units)
				pop[i] = ch
			}
		}

		next := make([]*GAChromosome, 0, cfg.PopulationSize)
		for i := 0; i < cfg.EliteCount; i++ {
			next = append(next, cloneChromosome(pop[i]))
		}

		for len(next) < cfg.PopulationSize {
			p1 := tournament(pop, 3, rng)
			p2 := tournament(pop, 3, rng)
			child := crossover(p1, p2, units, rng)

			hits := conflictAwareMutate(child, units, validSlots, cfg.MutationRate, locks, rng)
			genMutationHits += hits
			evaluateChromosome(child, units)

			beforeSA := child.Fitness
			repairWithILSSA(child, units, validSlots, cfg.SAIterations, locks, rng)
			if child.Fitness > beforeSA {
				genSAImprovements++
			}

			next = append(next, child)
		}
		pop = next

		if (g+1)%cfg.ProgressEvery == 0 {
			sortByFitnessDesc(pop)
			currentBest = findBestChromosome(pop)
			if betterChromosome(currentBest, best) {
				best = cloneChromosome(currentBest)
				stagnantGens = 0
			}
			emit(g+1, best, pop, stagnantGens)

			if best.Fitness == 0 && best.ViolationCount == 0 && best.UnplacedCount == 0 {
				break
			}
		}
	}

	sortByFitnessDesc(pop)
	if last := findBestChromosome(pop); betterChromosome(last, best) {
		best = cloneChromosome(last)
	}
	emit(cfg.Generations, best, pop, stagnantGens)

	return best, nil
}

// randomChromosome assigns a uniformly random valid slot to each unit.
func randomChromosome(units []PlacementUnit, validSlots map[uuid.UUID][]ValidSlot, rng *rand.Rand) *GAChromosome {
	ch := &GAChromosome{UnitSlots: make([]ValidSlot, len(units))}
	for i := range units {
		cands := unitCandidates(units[i], validSlots)
		if len(cands) == 0 {
			continue
		}
		ch.UnitSlots[i] = cands[rng.Intn(len(cands))]
	}
	return ch
}

// greedyChromosome constructs a chromosome with most-constrained-first (MCF) ordering
// and conflict-minimising slot selection.
//
// Sort order (tightest constraint first):
//  1. lockHard (SBP) — parallel multi-class constraint; misplacing blocks many downstream units.
//  2. lockSoft (PJOK) — finish-before-11 time window; fewest valid slots.
//  3. Higher JP (3 → 2 → 1) — longer blocks fit in fewer windows.
//  4. Fewest candidates — standard MCF tiebreak.
func greedyChromosome(units []PlacementUnit, validSlots map[uuid.UUID][]ValidSlot, locks []lockLevel, rng *rand.Rand) *GAChromosome {
	ch := &GAChromosome{UnitSlots: make([]ValidSlot, len(units))}
	state := &evalState{
		classCounts:   make(map[classOccKey]int),
		teacherCounts: make(map[teacherOccKey]int),
		siblingCounts: make(map[uuid.UUID]map[string]int),
	}

	candCounts := make([]int, len(units))
	jpValues := make([]int, len(units))
	for i := range units {
		candCounts[i] = len(unitCandidates(units[i], validSlots))
		jpValues[i] = unitMaxJP(units[i])
	}

	order := rng.Perm(len(units))
	slices.SortStableFunc(order, func(a, b int) int {
		la, lb := locks[a], locks[b]
		if la != lb {
			if la > lb {
				return -1
			}
			return 1
		}
		if jpValues[a] != jpValues[b] {
			if jpValues[a] > jpValues[b] {
				return -1
			}
			return 1
		}
		return candCounts[a] - candCounts[b]
	})

	for _, idx := range order {
		cands := unitCandidates(units[idx], validSlots)
		if len(cands) == 0 {
			continue
		}

		bestScore := int(^uint(0) >> 1)
		zeroCands := make([]ValidSlot, 0, len(cands))
		var bestCand ValidSlot

		for _, cand := range cands {
			score := previewUnitScore(state, units[idx], cand)
			if score < bestScore {
				bestScore = score
				bestCand = cand
				if score == 0 {
					zeroCands = zeroCands[:0]
					zeroCands = append(zeroCands, cand)
				}
			} else if score == 0 {
				zeroCands = append(zeroCands, cand)
			}
		}

		chosen := bestCand
		if len(zeroCands) > 1 {
			chosen = zeroCands[rng.Intn(len(zeroCands))]
		}

		ch.UnitSlots[idx] = chosen
		addUnitToState(state, units[idx], chosen)
	}

	return ch
}

// previewUnitScore estimates conflict-points for placing unit at slot against the current state.
// Does not modify state. Scale: class/teacher pair = 10, sibling same-day = 8.
func previewUnitScore(state *evalState, unit PlacementUnit, slot ValidSlot) int {
	if slot.Day == "" {
		return 1_000_000
	}

	score := 0
	jp := unitMaxJP(unit)
	blocks := unitBlocks(unit)

	for offset := 0; offset < jp; offset++ {
		cell := slotCell{day: slot.Day, slot: slot.SlotIndex + offset}
		for _, b := range blocks {
			score += state.classCounts[classOccKey{cell: cell, classID: b.ClassID}] * 10
			if b.TeacherID != nil {
				score += state.teacherCounts[teacherOccKey{cell: cell, teacherID: *b.TeacherID}] * 10
			}
		}
	}

	for _, b := range blocks {
		if b.SiblingGroupID != nil {
			if counts := state.siblingCounts[*b.SiblingGroupID]; counts != nil {
				score += counts[slot.Day] * 8
			}
		}
	}

	return score
}

// EvaluateChromosome is the exported wrapper for baseline validation.
func EvaluateChromosome(ch *GAChromosome, units []PlacementUnit) {
	evaluateChromosome(ch, units)
}

// SATrial holds the before/after result of one greedy-start SA repair trial.
type SATrial struct {
	Seed             int64
	GreedyViolations int
	GreedyUnplaced   int
	GreedyBreakdown  ViolationBreakdown
	FinalViolations  int
	FinalUnplaced    int
	FinalBreakdown   ViolationBreakdown
	ReachedZero      bool
}

// DiagnoseGreedySA runs a single SA-only trial (no GA):
//  1. Build one greedy MCF chromosome.
//  2. Record its violations.
//  3. Apply SA+ILS repair for saIterations passes.
//  4. Record final violations.
//
// Use this to verify that SA repair alone can reach 0 violations
// before committing to a full GA run.
func DiagnoseGreedySA(units []PlacementUnit, validSlots map[uuid.UUID][]ValidSlot, saIterations int, seed int64) SATrial {
	rng := rand.New(rand.NewSource(seed))
	locks := detectLocks(units)

	ch := greedyChromosome(units, validSlots, locks, rng)
	evaluateChromosome(ch, units)

	trial := SATrial{
		Seed:             seed,
		GreedyViolations: ch.ViolationCount,
		GreedyUnplaced:   ch.UnplacedCount,
		GreedyBreakdown:  ch.Breakdown,
	}

	repairWithILSSA(ch, units, validSlots, saIterations, locks, rng)

	trial.FinalViolations = ch.ViolationCount
	trial.FinalUnplaced = ch.UnplacedCount
	trial.FinalBreakdown = ch.Breakdown
	trial.ReachedZero = ch.ViolationCount == 0 && ch.UnplacedCount == 0

	return trial
}

func evaluateChromosome(ch *GAChromosome, units []PlacementUnit) {
	state := buildEvalState(ch, units)
	applyEvalState(ch, state)
}

// repairWithILSSA applies ILS + SA local search to ch.
//
// SA acceptance: improving moves (Δ < 0) always taken. Worsening moves (Δ > 0)
// accepted with probability e^(-Δ/T). Temperature decays geometrically each pass.
//
// ILS escape: when SA has cooled to tempMin and nothing moved (local minimum),
// perturb non-lockHard violating units randomly and reset temperature.
// lockSoft (PJOK) is eligible for ILS perturbation to break deadlocks.
//
// Best-seen tracking: restored at the end if exploratory moves worsened the result.
func repairWithILSSA(ch *GAChromosome, units []PlacementUnit, validSlots map[uuid.UUID][]ValidSlot, maxPasses int, locks []lockLevel, rng *rand.Rand) {
	if maxPasses <= 0 || len(units) == 0 {
		return
	}

	const (
		tempInitial = 15.0 // e^(-10/15) ≈ 0.51: ~50% chance to accept 1 pair worsening
		tempMin     = 0.5  // below this SA is near-greedy; trigger ILS if stuck
	)

	// Decay over all passes so the SA explores at meaningful temperatures
	// throughout, not just the first half.
	coolingRate := math.Pow(tempMin/tempInitial, 1.0/float64(max(1, maxPasses-1)))

	allCands := make([][]ValidSlot, len(units))
	for i := range units {
		allCands[i] = unitCandidates(units[i], validSlots)
	}

	// Precompute candidate sets for O(1) swap-eligibility checks inside the swap phase.
	// Key: slotCell. Allocated once per SA call; reused across all passes.
	candSets := make([]map[slotCell]struct{}, len(units))
	for i := range units {
		set := make(map[slotCell]struct{}, len(allCands[i]))
		for _, c := range allCands[i] {
			set[slotCell{day: c.Day, slot: c.SlotIndex}] = struct{}{}
		}
		candSets[i] = set
	}

	state := buildEvalState(ch, units)
	bestViol := state.classPairs*10 + state.teacherPairs*10 + state.siblingExtra*8 + state.unplaced*100
	bestSlots := make([]ValidSlot, len(ch.UnitSlots))
	copy(bestSlots, ch.UnitSlots)

	temp := tempInitial
	stagnantPasses := 0

	for range maxPasses {
		violating := violatingFromState(state, units, ch)
		if len(violating) == 0 {
			break
		}

		rng.Shuffle(len(violating), func(i, j int) {
			violating[i], violating[j] = violating[j], violating[i]
		})

		// PJOK is now lockNone — its validSlots are already constrained by
		// pjokMaxStartSlot, so SA can reposition it freely within safe slots.
		// lockHard (SBP) is allowed to move when it is already violating.
		anyMovable := false
		for _, ui := range violating {
			if locks[ui] != lockSoft {
				anyMovable = true
				break
			}
		}
		if !anyMovable {
			break
		}

		moved := false

		for _, ui := range violating {
			if locks[ui] == lockSoft {
				continue
			}

			oldSlot := ch.UnitSlots[ui]
			cands := allCands[ui]
			if len(cands) == 0 {
				continue
			}

			removeUnitFromState(state, units[ui], oldSlot)

			// Score all candidates (conflict-aware); find the best.
			currentScore := previewUnitScore(state, units[ui], oldSlot)
			bestScore := currentScore
			bestSlot := oldSlot

			for _, cand := range cands {
				if sameSlot(cand, oldSlot) {
					continue
				}
				score := previewUnitScore(state, units[ui], cand)
				if score < bestScore {
					bestScore = score
					bestSlot = cand
					if bestScore == 0 {
						break
					}
				}
			}

			// SA acceptance: always accept improvements; accept worsening with e^(-Δ/T).
			chosen := oldSlot
			delta := bestScore - currentScore
			switch {
			case delta < 0:
				chosen = bestSlot
			case delta > 0 && temp > tempMin:
				if rng.Float64() < math.Exp(-float64(delta)/temp) {
					chosen = bestSlot
				}
			}

			addUnitToState(state, units[ui], chosen)
			if !sameSlot(chosen, oldSlot) {
				ch.UnitSlots[ui] = chosen
				moved = true
			}
		}

		// Swap phase: at high slot utilization every candidate position for a violating
		// unit is already occupied, so single moves just shift the conflict elsewhere.
		// Swapping two units simultaneously — A→B, B's occupant→A — is zero-displacement:
		// no slot goes unfilled, yet incompatible neighbours can be separated.
		{
			// Build starting-slot → unit index reverse map from current positions.
			// Rebuilt each pass so moves from the single-move phase are reflected.
			cellOccupant := make(map[slotCell]int, len(units))
			for idx := range units {
				s := ch.UnitSlots[idx]
				if s.Day != "" {
					cellOccupant[slotCell{day: s.Day, slot: s.SlotIndex}] = idx
				}
			}

			const topCands = 8
			type scoredCand struct {
				slot  ValidSlot
				score int
			}
			var candBuf [topCands]scoredCand // stack-allocated; avoids heap per unit

			for _, ui := range violating {
				if locks[ui] == lockHard {
					continue
				}
				slotI := ch.UnitSlots[ui]
				if slotI.Day == "" {
					continue
				}

				// Score all candidates with ui temporarily removed; keep top-K by score.
				removeUnitFromState(state, units[ui], slotI)
				topLen := 0
				for _, cand := range allCands[ui] {
					if sameSlot(cand, slotI) {
						continue
					}
					score := previewUnitScore(state, units[ui], cand)
					if topLen < topCands {
						candBuf[topLen] = scoredCand{cand, score}
						topLen++
						for k := topLen - 1; k > 0 && candBuf[k].score < candBuf[k-1].score; k-- {
							candBuf[k], candBuf[k-1] = candBuf[k-1], candBuf[k]
						}
					} else if score < candBuf[topCands-1].score {
						candBuf[topCands-1] = scoredCand{cand, score}
						for k := topCands - 1; k > 0 && candBuf[k].score < candBuf[k-1].score; k-- {
							candBuf[k], candBuf[k-1] = candBuf[k-1], candBuf[k]
						}
					}
				}
				addUnitToState(state, units[ui], slotI) // restore before swap scoring

				swapped := false
				for k := 0; k < topLen && !swapped; k++ {
					targetSlot := candBuf[k].slot
					j, occupied := cellOccupant[slotCell{day: targetSlot.Day, slot: targetSlot.SlotIndex}]
					if !occupied || j == ui || locks[j] == lockHard {
						continue
					}
					// j must be able to legally go to ui's current slot
					if _, ok := candSets[j][slotCell{day: slotI.Day, slot: slotI.SlotIndex}]; !ok {
						continue
					}

					// Apply the swap, measure violation delta, then accept or undo.
					slotJ := ch.UnitSlots[j]
					prevViol := state.classPairs*10 + state.teacherPairs*10 + state.siblingExtra*8 + state.unplaced*100
					removeUnitFromState(state, units[ui], slotI)
					removeUnitFromState(state, units[j], slotJ)
					addUnitToState(state, units[ui], slotJ)
					addUnitToState(state, units[j], slotI)
					newViol := state.classPairs*10 + state.teacherPairs*10 + state.siblingExtra*8 + state.unplaced*100

					delta := newViol - prevViol
					accept := delta < 0 || (delta > 0 && temp > tempMin && rng.Float64() < math.Exp(-float64(delta)/temp))

					if accept {
						ch.UnitSlots[ui] = slotJ
						ch.UnitSlots[j] = slotI
						cellOccupant[slotCell{day: slotJ.Day, slot: slotJ.SlotIndex}] = ui
						cellOccupant[slotCell{day: slotI.Day, slot: slotI.SlotIndex}] = j
						moved = true
						swapped = true
					} else {
						// Undo: reverse the four state operations
						removeUnitFromState(state, units[ui], slotJ)
						removeUnitFromState(state, units[j], slotI)
						addUnitToState(state, units[ui], slotI)
						addUnitToState(state, units[j], slotJ)
					}
				}
			}
		}

		temp *= coolingRate

		curViol := state.classPairs*10 + state.teacherPairs*10 + state.siblingExtra*8 + state.unplaced*100
		if curViol < bestViol {
			bestViol = curViol
			copy(bestSlots, ch.UnitSlots)
			stagnantPasses = 0
		} else {
			stagnantPasses++
		}
		if bestViol == 0 {
			break
		}

		// ILS escape: lift ALL violating units off the board and re-insert them
		// greedily (conflict-minimising) in a shuffled order. This is far stronger
		// than the old 1/3-random approach: SA was stuck because the background
		// configuration left no zero-conflict slots for the violating units; a full
		// greedy re-insertion finds the best available position for each one given
		// the current background, potentially landing in a new basin.
		//
		// Triggers on stagnation OR temperature floor (swap phase keeps moved≈true,
		// so stagnation-based triggering is essential).
		ilsThreshold := max(30, maxPasses/8)
		if stagnantPasses >= ilsThreshold || (temp <= tempMin && !moved) {
			perturbable := make([]int, 0, len(violating))
			for _, ui := range violating {
				if locks[ui] < lockHard {
					perturbable = append(perturbable, ui)
				}
			}
			if len(perturbable) == 0 {
				break
			}

			// Remove all perturbable units from state.
			for _, ui := range perturbable {
				removeUnitFromState(state, units[ui], ch.UnitSlots[ui])
				ch.UnitSlots[ui] = ValidSlot{}
			}

			// Shuffle order for diversity between ILS escapes, then greedy re-insert.
			rng.Shuffle(len(perturbable), func(a, b int) { perturbable[a], perturbable[b] = perturbable[b], perturbable[a] })
			for _, ui := range perturbable {
				cands := allCands[ui]
				if len(cands) == 0 {
					continue
				}
				bestScore := int(^uint(0) >> 1)
				bestCand := cands[0]
				for _, c := range cands {
					if s := previewUnitScore(state, units[ui], c); s < bestScore {
						bestScore = s
						bestCand = c
						if bestScore == 0 {
							break
						}
					}
				}
				ch.UnitSlots[ui] = bestCand
				addUnitToState(state, units[ui], bestCand)
			}
			temp = tempInitial
			stagnantPasses = 0
		}
	}

	// Restore best-seen if exploratory SA/ILS moves ultimately worsened the result.
	curViol := state.classPairs*10 + state.teacherPairs*10 + state.siblingExtra*8 + state.unplaced*100
	if curViol > bestViol {
		copy(ch.UnitSlots, bestSlots)
		state = buildEvalState(ch, units)
	}

	// Backtracking repair: lift remaining conflicting units off the board and
	// re-insert them via zero-conflict DFS. SA+swap can reduce 400+ violations
	// to ~6-10 class-conflict pairs that form an irreducible multi-way deadlock;
	// backtracking resolves these in microseconds by treating them as a tiny CSP.
	if state.classPairs+state.teacherPairs+state.siblingExtra+state.unplaced > 0 {
		backtrackRepair(ch, state, units, allCands, rng)
	}

	applyEvalState(ch, state)
}

// backtrackRepair removes all remaining violating units from the schedule and
// attempts to re-insert them one by one, only accepting zero-conflict slots.
// A node budget caps wall time; if exhausted the original slots are restored.
func backtrackRepair(ch *GAChromosome, state *evalState, units []PlacementUnit, allCands [][]ValidSlot, rng *rand.Rand) {
	violating := violatingFromState(state, units, ch)
	if len(violating) == 0 {
		return
	}

	// Key by unit index, not slice position.
	savedSlots := make(map[int]ValidSlot, len(violating))
	for _, ui := range violating {
		savedSlots[ui] = ch.UnitSlots[ui]
		removeUnitFromState(state, units[ui], ch.UnitSlots[ui])
		ch.UnitSlots[ui] = ValidSlot{}
	}

	// Fail-first ordering: place the most-constrained units (fewest zero-conflict
	// candidates against the clean background) first. This prunes dead ends early
	// and dramatically reduces DFS node count vs. random ordering.
	type uiCount struct{ ui, cnt int }
	ucs := make([]uiCount, len(violating))
	for i, ui := range violating {
		cnt := 0
		for _, c := range allCands[ui] {
			if previewUnitScore(state, units[ui], c) == 0 {
				cnt++
			}
		}
		ucs[i] = uiCount{ui, cnt}
	}
	slices.SortFunc(ucs, func(a, b uiCount) int { return a.cnt - b.cnt })
	for i, uc := range ucs {
		violating[i] = uc.ui
	}

	budget := 100_000
	if !btPlace(ch, state, units, allCands, violating, 0, rng, &budget) {
		// No conflict-free assignment found within budget; restore original positions.
		for _, ui := range violating {
			addUnitToState(state, units[ui], savedSlots[ui])
			ch.UnitSlots[ui] = savedSlots[ui]
		}
	}
}

// btPlace is the recursive DFS worker for backtrackRepair.
// It places units[violating[idx]] in a zero-conflict slot, then recurses.
func btPlace(ch *GAChromosome, state *evalState, units []PlacementUnit, allCands [][]ValidSlot, violating []int, idx int, rng *rand.Rand, budget *int) bool {
	*budget--
	if *budget < 0 {
		return false
	}
	if idx == len(violating) {
		return true
	}

	ui := violating[idx]
	cands := allCands[ui]

	zeroCands := make([]ValidSlot, 0, 8)
	for _, c := range cands {
		if previewUnitScore(state, units[ui], c) == 0 {
			zeroCands = append(zeroCands, c)
		}
	}
	if len(zeroCands) == 0 {
		return false
	}

	rng.Shuffle(len(zeroCands), func(a, b int) { zeroCands[a], zeroCands[b] = zeroCands[b], zeroCands[a] })

	for _, cand := range zeroCands {
		ch.UnitSlots[ui] = cand
		addUnitToState(state, units[ui], cand)
		if btPlace(ch, state, units, allCands, violating, idx+1, rng, budget) {
			return true
		}
		removeUnitFromState(state, units[ui], cand)
		ch.UnitSlots[ui] = ValidSlot{}
	}
	return false
}

// violatingFromState returns indices of units that conflict according to the
// already-maintained evalState — O(n) integer map lookups, no time parsing.
func violatingFromState(state *evalState, units []PlacementUnit, ch *GAChromosome) []int {
	result := make([]int, 0, len(units))
	for i := range units {
		slot := ch.UnitSlots[i]
		if slot.Day == "" {
			result = append(result, i)
			continue
		}
		if unitConflictsInState(state, units[i], slot) {
			result = append(result, i)
		}
	}
	return result
}

func unitConflictsInState(state *evalState, unit PlacementUnit, slot ValidSlot) bool {
	jp := unitMaxJP(unit)
	blocks := unitBlocks(unit)

	for offset := 0; offset < jp; offset++ {
		cell := slotCell{day: slot.Day, slot: slot.SlotIndex + offset}
		for _, b := range blocks {
			if state.classCounts[classOccKey{cell: cell, classID: b.ClassID}] > 1 {
				return true
			}
			if b.TeacherID != nil && state.teacherCounts[teacherOccKey{cell: cell, teacherID: *b.TeacherID}] > 1 {
				return true
			}
		}
	}

	for _, b := range blocks {
		if b.SiblingGroupID != nil && state.siblingCounts[*b.SiblingGroupID][slot.Day] > 1 {
			return true
		}
	}
	return false
}

// ── Delta evaluation ───────────────────────────────────────────────────────

type evalState struct {
	classCounts   map[classOccKey]int
	teacherCounts map[teacherOccKey]int
	siblingCounts map[uuid.UUID]map[string]int

	classPairs   int
	teacherPairs int
	siblingExtra int
	unplaced     int
}

func buildEvalState(ch *GAChromosome, units []PlacementUnit) *evalState {
	state := &evalState{
		classCounts:   make(map[classOccKey]int),
		teacherCounts: make(map[teacherOccKey]int),
		siblingCounts: make(map[uuid.UUID]map[string]int),
	}

	n := min(len(units), len(ch.UnitSlots))
	for i := range n {
		addUnitToState(state, units[i], ch.UnitSlots[i])
	}
	for i := n; i < len(units); i++ {
		addUnitToState(state, units[i], ValidSlot{})
	}

	return state
}

func applyEvalState(ch *GAChromosome, state *evalState) {
	viol := state.classPairs*10 + state.teacherPairs*10 + state.siblingExtra*8 + state.unplaced*100
	ch.ViolationCount = viol
	ch.UnplacedCount = state.unplaced
	ch.Breakdown = ViolationBreakdown{
		ClassConflicts:    state.classPairs,
		TeacherConflicts:  state.teacherPairs,
		SiblingViolations: state.siblingExtra,
	}
	ch.Fitness = -viol
}

func addUnitToState(state *evalState, unit PlacementUnit, slot ValidSlot) {
	if slot.Day == "" {
		state.unplaced++
		return
	}

	jp := unitMaxJP(unit)
	blocks := unitBlocks(unit)

	for offset := 0; offset < jp; offset++ {
		cell := slotCell{day: slot.Day, slot: slot.SlotIndex + offset}
		for _, b := range blocks {
			ck := classOccKey{cell: cell, classID: b.ClassID}
			cc := state.classCounts[ck]
			state.classPairs += cc
			state.classCounts[ck] = cc + 1

			if b.TeacherID != nil {
				tk := teacherOccKey{cell: cell, teacherID: *b.TeacherID}
				tc := state.teacherCounts[tk]
				state.teacherPairs += tc
				state.teacherCounts[tk] = tc + 1
			}
		}
	}

	for _, b := range blocks {
		if b.SiblingGroupID != nil {
			if state.siblingCounts[*b.SiblingGroupID] == nil {
				state.siblingCounts[*b.SiblingGroupID] = make(map[string]int)
			}
			current := state.siblingCounts[*b.SiblingGroupID][slot.Day]
			if current >= 1 {
				state.siblingExtra++
			}
			state.siblingCounts[*b.SiblingGroupID][slot.Day] = current + 1
		}
	}
}

func removeUnitFromState(state *evalState, unit PlacementUnit, slot ValidSlot) {
	if slot.Day == "" {
		if state.unplaced > 0 {
			state.unplaced--
		}
		return
	}

	jp := unitMaxJP(unit)
	blocks := unitBlocks(unit)

	for offset := 0; offset < jp; offset++ {
		cell := slotCell{day: slot.Day, slot: slot.SlotIndex + offset}
		for _, b := range blocks {
			ck := classOccKey{cell: cell, classID: b.ClassID}
			cc := state.classCounts[ck]
			if cc > 0 {
				state.classPairs -= cc - 1
				if cc == 1 {
					delete(state.classCounts, ck)
				} else {
					state.classCounts[ck] = cc - 1
				}
			}

			if b.TeacherID != nil {
				tk := teacherOccKey{cell: cell, teacherID: *b.TeacherID}
				tc := state.teacherCounts[tk]
				if tc > 0 {
					state.teacherPairs -= tc - 1
					if tc == 1 {
						delete(state.teacherCounts, tk)
					} else {
						state.teacherCounts[tk] = tc - 1
					}
				}
			}
		}
	}

	for _, b := range blocks {
		if b.SiblingGroupID != nil {
			dayCounts := state.siblingCounts[*b.SiblingGroupID]
			if dayCounts == nil {
				continue
			}
			current := dayCounts[slot.Day]
			if current <= 0 {
				continue
			}
			if current > 1 {
				state.siblingExtra--
			}
			if current == 1 {
				delete(dayCounts, slot.Day)
				if len(dayCounts) == 0 {
					delete(state.siblingCounts, *b.SiblingGroupID)
				}
			} else {
				dayCounts[slot.Day] = current - 1
			}
		}
	}
}

// ── Genetic operators ──────────────────────────────────────────────────────

// crossover produces a child by conflict-aware selection from two parents.
//
// For each unit (in a random order to avoid systematic bias), the child
// inherits whichever parent's slot creates fewer conflicts against the
// partial child state built so far. When both parents agree the slot is
// inherited directly. Ties are broken randomly.
//
// Building the evalState incrementally means later gene choices are informed
// by earlier placements, preserving compatible clusters from both parents
// instead of mixing slots that immediately conflict with each other.
func crossover(a, b *GAChromosome, units []PlacementUnit, rng *rand.Rand) *GAChromosome {
	n := len(a.UnitSlots)
	out := &GAChromosome{UnitSlots: make([]ValidSlot, n)}
	state := &evalState{
		classCounts:   make(map[classOccKey]int),
		teacherCounts: make(map[teacherOccKey]int),
		siblingCounts: make(map[uuid.UUID]map[string]int),
	}

	for _, i := range rng.Perm(n) {
		slotA := a.UnitSlots[i]
		slotB := b.UnitSlots[i]

		var chosen ValidSlot
		if sameSlot(slotA, slotB) {
			chosen = slotA
		} else {
			scoreA := previewUnitScore(state, units[i], slotA)
			scoreB := previewUnitScore(state, units[i], slotB)
			switch {
			case scoreA < scoreB:
				chosen = slotA
			case scoreB < scoreA:
				chosen = slotB
			default:
				if rng.Float64() < 0.5 {
					chosen = slotA
				} else {
					chosen = slotB
				}
			}
		}

		out.UnitSlots[i] = chosen
		addUnitToState(state, units[i], chosen)
	}
	return out
}

// conflictAwareMutate applies a two-tier mutation to ch:
//   - Violating units: scored against all placed units; moved to a slot drawn from
//     the top-3 least-conflicting candidates (directed random).
//   - Non-violating units: moved to a uniformly random candidate with probability rate.
//
// lockHard (SBP) units are skipped entirely; all other units including PJOK are eligible.
// The evalState is updated incrementally so later units are scored against
// already-repositioned earlier ones.
func conflictAwareMutate(ch *GAChromosome, units []PlacementUnit, validSlots map[uuid.UUID][]ValidSlot, rate float64, locks []lockLevel, rng *rand.Rand) int {
	const topK = 3

	state := buildEvalState(ch, units)
	violating := violatingFromState(state, units, ch)

	violatingSet := make(map[int]struct{}, len(violating))
	for _, ui := range violating {
		violatingSet[ui] = struct{}{}
	}

	hits := 0
	for i := range units {
		if locks[i] >= lockSoft {
			continue
		}

		_, isViolating := violatingSet[i]
		if !isViolating && rng.Float64() >= rate {
			continue
		}

		cands := unitCandidates(units[i], validSlots)
		if len(cands) == 0 {
			continue
		}

		old := ch.UnitSlots[i]
		removeUnitFromState(state, units[i], old)

		var newSlot ValidSlot
		if isViolating && len(cands) > 1 {
			type cs struct {
				slot  ValidSlot
				score int
			}
			// Partial selection: keep the topK lowest-score candidates without
			// sorting the full list. Fixed-size array lives on the stack.
			var top [topK]cs
			topLen := 0
			for _, c := range cands {
				s := previewUnitScore(state, units[i], c)
				if topLen < topK {
					top[topLen] = cs{c, s}
					topLen++
					for j := topLen - 1; j > 0 && top[j].score < top[j-1].score; j-- {
						top[j], top[j-1] = top[j-1], top[j]
					}
				} else if s < top[topK-1].score {
					top[topK-1] = cs{c, s}
					for j := topK - 1; j > 0 && top[j].score < top[j-1].score; j-- {
						top[j], top[j-1] = top[j-1], top[j]
					}
				}
			}
			newSlot = top[rng.Intn(topLen)].slot
		} else {
			newSlot = old
			if len(cands) > 1 {
				for range 5 {
					c := cands[rng.Intn(len(cands))]
					if !sameSlot(c, old) {
						newSlot = c
						break
					}
				}
				if sameSlot(newSlot, old) {
					for _, c := range cands {
						if !sameSlot(c, old) {
							newSlot = c
							break
						}
					}
				}
			}
		}

		addUnitToState(state, units[i], newSlot)
		if !sameSlot(newSlot, old) {
			ch.UnitSlots[i] = newSlot
			hits++
		}
	}
	return hits
}

// ── Population helpers ─────────────────────────────────────────────────────

func tournament(pop []*GAChromosome, k int, rng *rand.Rand) *GAChromosome {
	best := pop[rng.Intn(len(pop))]
	for i := 1; i < k; i++ {
		c := pop[rng.Intn(len(pop))]
		if betterChromosome(c, best) {
			best = c
		}
	}
	return best
}

func sortByFitnessDesc(pop []*GAChromosome) {
	slices.SortFunc(pop, func(a, b *GAChromosome) int {
		if betterChromosome(a, b) {
			return -1
		}
		if betterChromosome(b, a) {
			return 1
		}
		return 0
	})
}

func findBestChromosome(pop []*GAChromosome) *GAChromosome {
	best := pop[0]
	for i := 1; i < len(pop); i++ {
		if betterChromosome(pop[i], best) {
			best = pop[i]
		}
	}
	return best
}

// betterChromosome returns true if a is strictly better than b.
// Primary: higher fitness. Secondary: fewer violations. Tertiary: fewer unplaced.
func betterChromosome(a, b *GAChromosome) bool {
	if a == nil {
		return false
	}
	if b == nil {
		return true
	}
	if a.Fitness != b.Fitness {
		return a.Fitness > b.Fitness
	}
	if a.ViolationCount != b.ViolationCount {
		return a.ViolationCount < b.ViolationCount
	}
	if a.UnplacedCount != b.UnplacedCount {
		return a.UnplacedCount < b.UnplacedCount
	}
	return false
}

// ── Chromosome utilities ───────────────────────────────────────────────────

func sameSlot(a, b ValidSlot) bool {
	return a.Day == b.Day && a.SlotIndex == b.SlotIndex
}

func cloneChromosome(c *GAChromosome) *GAChromosome {
	out := &GAChromosome{
		UnitSlots:      make([]ValidSlot, len(c.UnitSlots)),
		Fitness:        c.Fitness,
		ViolationCount: c.ViolationCount,
		UnplacedCount:  c.UnplacedCount,
		Breakdown:      c.Breakdown,
	}
	copy(out.UnitSlots, c.UnitSlots)
	return out
}

// perturbChromosome clones src and randomly reassigns each non-lockHard unit with
// probability rate. lockSoft (PJOK) is eligible — it may need to shift to escape
// the current local minimum's basin of attraction.
func perturbChromosome(src *GAChromosome, rate float64, units []PlacementUnit, validSlots map[uuid.UUID][]ValidSlot, locks []lockLevel, rng *rand.Rand) *GAChromosome {
	ch := cloneChromosome(src)
	for i := range units {
		if locks[i] == lockHard {
			continue
		}
		if rng.Float64() < rate {
			cands := unitCandidates(units[i], validSlots)
			if len(cands) > 0 {
				ch.UnitSlots[i] = cands[rng.Intn(len(cands))]
			}
		}
	}
	return ch
}

func unitCandidates(u PlacementUnit, validSlots map[uuid.UUID][]ValidSlot) []ValidSlot {
	if u.Block != nil {
		return validSlots[u.Block.ID]
	}
	if len(u.Blocks) > 0 {
		return validSlots[u.Blocks[0].ID]
	}
	return nil
}

func unitBlocks(u PlacementUnit) []*Block {
	if u.Block != nil {
		return []*Block{u.Block}
	}
	return u.Blocks
}

// detectLocks classifies every unit into a lock level:
//
//	lockHard — SBP parallel groups (len(Blocks) > 1): never moved by mutation;
//	           moved by SA only when already violating.
//	lockNone — all other units (including PJOK): freely moved by mutation and SA.
//	           PJOK's finish-before-11 constraint is pre-encoded in its validSlots
//	           via pjokMaxStartSlot, so repositioning within those slots is always safe.
func detectLocks(units []PlacementUnit) []lockLevel {
	locks := make([]lockLevel, len(units))
	for i, u := range units {
		if len(u.Blocks) > 1 {
			locks[i] = lockHard
		}
	}
	return locks
}

// unitMaxJP returns the highest JP value among all blocks in a placement unit.
func unitMaxJP(u PlacementUnit) int {
	jp := 0
	for _, b := range unitBlocks(u) {
		if b.JP > jp {
			jp = b.JP
		}
	}
	return jp
}

// ── Shared helpers ─────────────────────────────────────────────────────────

func teacherOccupancyKey(day string, teacherID *uuid.UUID) string {
	if teacherID == nil {
		return day + "|nil"
	}
	return day + "|" + teacherID.String()
}
