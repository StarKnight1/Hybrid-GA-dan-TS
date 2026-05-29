package algorithm

import (
	"fmt"
	"math/rand"
	"sort"
	"time"

	"github.com/google/uuid"
)

type GAConfig struct {
	PopulationSize  int
	Generations     int
	MutationRate    float64
	EliteCount      int
	TournamentSize  int
	Seed            int64
	ProgressEvery   int
	StagnationLimit int // stop early if best hasn't improved for this many generations; 0 = disabled
	OnProgress      func(GAProgress)
	PJOKSubjectID   uint
}

type GAProgress struct {
	Generation         int
	BestUnplaced       int
	BestSoftViolations int
	AvgUnplaced        float64
	StagnantGens       int
	Elapsed            time.Duration
}

type GAResult struct {
	Chromosome     Chromosome
	Matrix         *ScheduleMatrix
	Unplaced       int
	SoftViolations int
	Generations    int
	Elapsed        time.Duration
}

func DefaultGAConfig() GAConfig {
	return GAConfig{
		PopulationSize: 100,
		Generations:    1000,
		MutationRate:   0.02,
		EliteCount:     5,
		TournamentSize: 3,
		Seed:           time.Now().UnixNano(),
		ProgressEvery:  50,
	}
}

type individual struct {
	chromosome     Chromosome
	unplaced       int
	softViolations int
}

func betterThan(a, b individual) bool {
	if a.unplaced != b.unplaced {
		return a.unplaced < b.unplaced
	}
	return a.softViolations < b.softViolations
}

// RunGA runs a pure genetic algorithm over the given blocks and candidate index.
// It returns the best chromosome found and the decoded matrix.
// A result with Unplaced == 0 is a fully valid schedule.
func RunGA(
	blocks []MatrixBlock,
	candidateIndex map[uuid.UUID][]Gene,
	daySlots DaySlots,
	cfg GAConfig,
) GAResult {
	start := time.Now()
	rng := rand.New(rand.NewSource(cfg.Seed))
	groups := BuildGroupIndex(blocks)

	pop := initPopulation(blocks, candidateIndex, daySlots, cfg.PopulationSize, cfg.PJOKSubjectID, groups, rng)
	sortPopulation(pop)
	best := pop[0]

	stagnantGens := 0
	emit := func(gen int) {
		if cfg.OnProgress != nil {
			cfg.OnProgress(GAProgress{
				Generation:         gen,
				BestUnplaced:       best.unplaced,
				BestSoftViolations: best.softViolations,
				AvgUnplaced:        avgUnplaced(pop),
				StagnantGens:       stagnantGens,
				Elapsed:            time.Since(start),
			})
		}
	}

	// mutateUnplaced diagnostic counters
	var mutateCalls, mutateImproved, mutateTotalBefore, mutateTotalAfter int

	lastGen := 0
	emittedLastGen := false
	for gen := 1; gen <= cfg.Generations; gen++ {
		if best.unplaced == 0 && best.softViolations == 0 {
			break
		}
		lastGen = gen

		next := make([]individual, 0, cfg.PopulationSize)

		for i := 0; i < cfg.EliteCount && i < len(pop); i++ {
			next = append(next, pop[i])
		}

		for len(next) < cfg.PopulationSize {
			parentA := tournamentSelect(pop, cfg.TournamentSize, rng)
			parentB := tournamentSelect(pop, cfg.TournamentSize, rng)
			child := ConstraintAwareCrossover(parentA.chromosome, parentB.chromosome, blocks, groups, candidateIndex, daySlots, cfg.PJOKSubjectID, rng)
			applyMutation(&child, blocks, candidateIndex, groups, cfg.MutationRate, rng)
			matrix, unplaced := DecodeChromosome(child, blocks, daySlots, cfg.PJOKSubjectID)
			if unplaced > 0 {
				beforeUnplaced := unplaced
				mutateUnplaced(&child, blocks, candidateIndex, groups, matrix, rng)
				matrix, unplaced = DecodeChromosome(child, blocks, daySlots, cfg.PJOKSubjectID)
				mutateCalls++
				mutateTotalBefore += beforeUnplaced
				mutateTotalAfter += unplaced
				if unplaced < beforeUnplaced {
					mutateImproved++
				}
			}
			soft := CountSoftViolations(matrix, blocks, cfg.PJOKSubjectID)
			next = append(next, individual{chromosome: child, unplaced: unplaced, softViolations: soft})
		}

		pop = next
		sortPopulation(pop)

		if betterThan(pop[0], best) {
			best = pop[0]
			stagnantGens = 0
		} else {
			stagnantGens++
		}

		if cfg.StagnationLimit > 0 && stagnantGens >= cfg.StagnationLimit {
			if cfg.ProgressEvery > 0 {
				emit(gen)
				emittedLastGen = true
			}
			break
		}

		if cfg.ProgressEvery > 0 && gen%cfg.ProgressEvery == 0 {
			emit(gen)
			emittedLastGen = true
		} else {
			emittedLastGen = false
		}
	}

	if !emittedLastGen {
		emit(lastGen)
	}

	if mutateCalls > 0 {
		avgBefore := float64(mutateTotalBefore) / float64(mutateCalls)
		avgAfter := float64(mutateTotalAfter) / float64(mutateCalls)
		fmt.Printf("[GA diag] mutateUnplaced: calls=%d improved=%d (%.1f%%) avgBefore=%.2f avgAfter=%.2f avgDelta=%.2f\n",
			mutateCalls, mutateImproved,
			float64(mutateImproved)/float64(mutateCalls)*100,
			avgBefore, avgAfter, avgBefore-avgAfter)
	}

	matrix, unplaced := DecodeChromosome(best.chromosome, blocks, daySlots, cfg.PJOKSubjectID)
	soft := CountSoftViolations(matrix, blocks, cfg.PJOKSubjectID)
	return GAResult{
		Chromosome:     best.chromosome,
		Matrix:         matrix,
		Unplaced:       unplaced,
		SoftViolations: soft,
		Generations:    lastGen,
		Elapsed:        time.Since(start),
	}
}

func initPopulation(
	blocks []MatrixBlock,
	candidateIndex map[uuid.UUID][]Gene,
	daySlots DaySlots,
	size int,
	pjokSubjectID uint,
	groups GroupIndex,
	rng *rand.Rand,
) []individual {
	pop := make([]individual, size)
	for i := range pop {
		c := SmartChromosome(blocks, candidateIndex, groups, daySlots, pjokSubjectID, rng)
		matrix, unplaced := DecodeChromosome(c, blocks, daySlots, pjokSubjectID)
		soft := CountSoftViolations(matrix, blocks, pjokSubjectID)
		pop[i] = individual{chromosome: c, unplaced: unplaced, softViolations: soft}
	}
	return pop
}

func tournamentSelect(pop []individual, k int, rng *rand.Rand) individual {
	best := pop[rng.Intn(len(pop))]
	for i := 1; i < k; i++ {
		candidate := pop[rng.Intn(len(pop))]
		if betterThan(candidate, best) {
			best = candidate
		}
	}
	return best
}

func applyMutation(c *Chromosome, blocks []MatrixBlock, candidateIndex map[uuid.UUID][]Gene, groups GroupIndex, rate float64, rng *rand.Rand) {
	processed := make(map[string]bool)
	for i, block := range blocks {
		if block.GroupKey != nil {
			if processed[*block.GroupKey] {
				continue
			}
			processed[*block.GroupKey] = true
			if rng.Float64() < rate {
				candidates := candidateIndex[block.ID]
				if len(candidates) > 0 {
					gene := candidates[rng.Intn(len(candidates))]
					for _, j := range groups[*block.GroupKey] {
						c.Set(j, gene)
					}
				}
			}
		} else {
			if rng.Float64() < rate {
				MutateGene(c, i, block, candidateIndex, rng)
			}
		}
	}
}

// mutateUnplaced force-reassigns every block that failed to place in the decoded
// matrix. Unplaced blocks have nothing to lose, so giving them a fresh random
// candidate is always worth trying. Group members are reassigned together.
func mutateUnplaced(c *Chromosome, blocks []MatrixBlock, candidateIndex map[uuid.UUID][]Gene, groups GroupIndex, matrix *ScheduleMatrix, rng *rand.Rand) {
	processed := make(map[string]bool)
	for i, block := range blocks {
		if block.GroupKey != nil {
			if processed[*block.GroupKey] {
				continue
			}
			processed[*block.GroupKey] = true
			// Only reassign if any member of the group is unplaced.
			anyUnplaced := false
			for _, j := range groups[*block.GroupKey] {
				if _, ok := matrix.Placement(blocks[j].ID); !ok {
					anyUnplaced = true
					break
				}
			}
			if anyUnplaced {
				candidates := candidateIndex[block.ID]
				if len(candidates) > 0 {
					gene := candidates[rng.Intn(len(candidates))]
					for _, j := range groups[*block.GroupKey] {
						c.Set(j, gene)
					}
				}
			}
		} else {
			if _, placed := matrix.Placement(block.ID); !placed {
				MutateGene(c, i, block, candidateIndex, rng)
			}
		}
	}
}

func sortPopulation(pop []individual) {
	sort.Slice(pop, func(i, j int) bool {
		return betterThan(pop[i], pop[j])
	})
}

func avgUnplaced(pop []individual) float64 {
	if len(pop) == 0 {
		return 0
	}
	total := 0
	for _, ind := range pop {
		total += ind.unplaced
	}
	return float64(total) / float64(len(pop))
}
