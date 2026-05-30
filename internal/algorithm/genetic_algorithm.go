package algorithm

import (
	"fmt"
	"math/rand"
	"sort"
	"time"
)

// GAConfig contains tunable parameters for the genetic algorithm.
type GAConfig struct {
	PopSize        int
	MaxGenerations int
	MutationProb   float64
	EliteSize      int
	TournSize      int
	RandSeed       int64
	ReportInterval int
	PatienceLimit  int // stop early when best hasn't improved for this many generations; 0 = disabled
	OnSnapshot     func(GAProgress)
	PJOKSubjID     uint
}

// GAProgress carries per-generation metrics emitted via OnSnapshot.
type GAProgress struct {
	Generation         int
	BestUnplaced       int
	BestSoftViolations int
	AvgUnplaced        float64
	StagnantGens       int
	Elapsed            time.Duration
}

// GAResult holds the best solution found by RunGA.
type GAResult struct {
	Chromosome     Chromosome
	Matrix         *ScheduleMatrix
	Unplaced       int
	SoftViolations int
	Generations    int
	Elapsed        time.Duration
}

// DefaultGAConfig returns sensible defaults suitable for most school scheduling instances.
func DefaultGAConfig() GAConfig {
	return GAConfig{
		PopSize:        100,
		MaxGenerations: 1000,
		MutationProb:   0.02,
		EliteSize:      5,
		TournSize:      3,
		RandSeed:       time.Now().UnixNano(),
		ReportInterval: 50,
	}
}

// candidate is one member of the GA population with its decoded fitness.
type candidate struct {
	genome      Chromosome
	unplaced    int
	softPenalty int
}

// dominates returns true when a is strictly better than b.
// Unplaced count is minimised first, then soft penalty.
func dominates(a, b candidate) bool {
	if a.unplaced != b.unplaced {
		return a.unplaced < b.unplaced
	}
	return a.softPenalty < b.softPenalty
}

// mutDiag accumulates diagnostic counters for the repairUnplaced step.
type mutDiag struct {
	calls, hits, sumBefore, sumAfter int
}

func (d *mutDiag) record(before, after int) {
	d.calls++
	d.sumBefore += before
	d.sumAfter += after
	if after < before {
		d.hits++
	}
}

func (d *mutDiag) print() {
	if d.calls == 0 {
		return
	}
	avgBefore := float64(d.sumBefore) / float64(d.calls)
	avgAfter := float64(d.sumAfter) / float64(d.calls)
	fmt.Printf("[GA diag] repairUnplaced: calls=%d improved=%d (%.1f%%) avgBefore=%.2f avgAfter=%.2f avgDelta=%.2f\n",
		d.calls, d.hits,
		float64(d.hits)/float64(d.calls)*100,
		avgBefore, avgAfter, avgBefore-avgAfter)
}

// RunGA executes the genetic algorithm and returns the best chromosome found.
// A result with Unplaced == 0 is a fully feasible schedule.
func RunGA(
	blocks []MatrixBlock,
	candidateIndex map[uint][]Gene,
	daySlots DaySlots,
	cfg GAConfig,
) GAResult {
	start := time.Now()
	acak := rand.New(rand.NewSource(cfg.RandSeed))
	indeksGrup := BuildGroupIndex(blocks)

	population := buildPopulation(blocks, candidateIndex, daySlots, cfg.PopSize, cfg.PJOKSubjID, indeksGrup, acak)
	rankPopulation(population)
	bestSol := population[0]

	stagnant := 0
	diag := &mutDiag{}

	report := func(gen int) {
		if cfg.OnSnapshot != nil {
			cfg.OnSnapshot(GAProgress{
				Generation:         gen,
				BestUnplaced:       bestSol.unplaced,
				BestSoftViolations: bestSol.softPenalty,
				AvgUnplaced:        meanUnplaced(population),
				StagnantGens:       stagnant,
				Elapsed:            time.Since(start),
			})
		}
	}

	lastGen := 0
	emittedLastGen := false

	for gen := 1; gen <= cfg.MaxGenerations; gen++ {
		if bestSol.unplaced == 0 && bestSol.softPenalty == 0 {
			break
		}
		lastGen = gen

		nextGen := make([]candidate, 0, cfg.PopSize)
		for i := 0; i < cfg.EliteSize && i < len(population); i++ {
			nextGen = append(nextGen, population[i])
		}

		for len(nextGen) < cfg.PopSize {
			indukA := pickWinner(population, cfg.TournSize, acak)
			indukB := pickWinner(population, cfg.TournSize, acak)
			anak := ConstraintAwareCrossover(indukA.genome, indukB.genome, blocks, candidateIndex, indeksGrup, daySlots, cfg.PJOKSubjID, acak)
			mutateAll(&anak, blocks, candidateIndex, indeksGrup, cfg.MutationProb, acak)
			grid, missing := DecodeChromosome(anak, blocks, daySlots, cfg.PJOKSubjID)
			if missing > 0 {
				beforeMissing := missing
				repairUnplaced(&anak, blocks, candidateIndex, indeksGrup, grid, acak)
				grid, missing = DecodeChromosome(anak, blocks, daySlots, cfg.PJOKSubjID)
				diag.record(beforeMissing, missing)
			}
			penalty := CountSoftViolations(grid, blocks, cfg.PJOKSubjID)
			nextGen = append(nextGen, candidate{genome: anak, unplaced: missing, softPenalty: penalty})
		}

		population = nextGen
		rankPopulation(population)

		if dominates(population[0], bestSol) {
			bestSol = population[0]
			stagnant = 0
		} else {
			stagnant++
		}

		if cfg.PatienceLimit > 0 && stagnant >= cfg.PatienceLimit {
			if cfg.ReportInterval > 0 {
				report(gen)
				emittedLastGen = true
			}
			break
		}

		if cfg.ReportInterval > 0 && gen%cfg.ReportInterval == 0 {
			report(gen)
			emittedLastGen = true
		} else {
			emittedLastGen = false
		}
	}

	if !emittedLastGen {
		report(lastGen)
	}
	diag.print()

	finalGrid, finalUnplaced := DecodeChromosome(bestSol.genome, blocks, daySlots, cfg.PJOKSubjID)
	finalPenalty := CountSoftViolations(finalGrid, blocks, cfg.PJOKSubjID)
	return GAResult{
		Chromosome:     bestSol.genome,
		Matrix:         finalGrid,
		Unplaced:       finalUnplaced,
		SoftViolations: finalPenalty,
		Generations:    lastGen,
		Elapsed:        time.Since(start),
	}
}

// buildPopulation initialises the GA population using SmartChromosome for each member.
func buildPopulation(
	blocks []MatrixBlock,
	candidateIndex map[uint][]Gene,
	daySlots DaySlots,
	size int,
	pjokSubjID uint,
	groups GroupIndex,
	acak *rand.Rand,
) []candidate {
	result := make([]candidate, size)
	for idx := 0; idx < size; idx++ {
		sol := SmartChromosome(blocks, candidateIndex, groups, daySlots, pjokSubjID, acak)
		grid, missing := DecodeChromosome(sol, blocks, daySlots, pjokSubjID)
		penalty := CountSoftViolations(grid, blocks, pjokSubjID)
		result[idx] = candidate{genome: sol, unplaced: missing, softPenalty: penalty}
	}
	return result
}

// pickWinner runs a k-way tournament and returns the fittest candidate.
func pickWinner(population []candidate, k int, acak *rand.Rand) candidate {
	winner := population[acak.Intn(len(population))]
	for round := 1; round < k; round++ {
		rival := population[acak.Intn(len(population))]
		if dominates(rival, winner) {
			winner = rival
		}
	}
	return winner
}

// mutateAll applies random gene replacement across the chromosome at the given probability.
// Group members are always mutated together to preserve synchronisation.
func mutateAll(c *Chromosome, blocks []MatrixBlock, candidateIndex map[uint][]Gene, groups GroupIndex, prob float64, acak *rand.Rand) {
	visitedGroups := make(map[string]bool)
	for idx, block := range blocks {
		switch block.GroupKey != nil {
		case true:
			if visitedGroups[*block.GroupKey] {
				continue
			}
			visitedGroups[*block.GroupKey] = true
			if acak.Float64() < prob {
				candidates := candidateIndex[block.ID]
				if len(candidates) > 0 {
					gene := candidates[acak.Intn(len(candidates))]
					for _, j := range groups[*block.GroupKey] {
						c.Set(j, gene)
					}
				}
			}
		default:
			if acak.Float64() < prob {
				MutateGene(c, idx, block, candidateIndex, acak)
			}
		}
	}
}

// repairUnplaced force-reassigns every block that failed to decode in the grid.
// An unplaced block has no positional value to preserve, so any random candidate is acceptable.
func repairUnplaced(c *Chromosome, blocks []MatrixBlock, candidateIndex map[uint][]Gene, groups GroupIndex, grid *ScheduleMatrix, acak *rand.Rand) {
	visitedGroups := make(map[string]bool)
	for idx, block := range blocks {
		if block.GroupKey != nil {
			if visitedGroups[*block.GroupKey] {
				continue
			}
			visitedGroups[*block.GroupKey] = true
			anyMissing := false
			for _, j := range groups[*block.GroupKey] {
				if _, ok := grid.Placement(blocks[j].ID); !ok {
					anyMissing = true
					break
				}
			}
			if anyMissing {
				candidates := candidateIndex[block.ID]
				if len(candidates) > 0 {
					gene := candidates[acak.Intn(len(candidates))]
					for _, j := range groups[*block.GroupKey] {
						c.Set(j, gene)
					}
				}
			}
		} else {
			if _, placed := grid.Placement(block.ID); !placed {
				MutateGene(c, idx, block, candidateIndex, acak)
			}
		}
	}
}

// rankPopulation sorts the population ascending by (unplaced, softPenalty).
func rankPopulation(population []candidate) {
	sort.Slice(population, func(i, j int) bool {
		return dominates(population[i], population[j])
	})
}

// meanUnplaced computes the average unplaced block count across the entire population.
func meanUnplaced(population []candidate) float64 {
	if len(population) == 0 {
		return 0
	}
	total := 0
	for _, m := range population {
		total += m.unplaced
	}
	return float64(total) / float64(len(population))
}
