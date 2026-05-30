package algorithm

import (
	"time"
)

// HybridConfig holds configuration for the combined GA+TS optimiser.
type HybridConfig struct {
	GA                GAConfig
	TS                TSConfig
	ExtraRuns          int            // additional full GA+TS runs beyond the first; 0 = single run
	RetryUntilFeasible bool           // keep retrying until unplaced == 0; overrides ExtraRuns
	MaxAttempts        int            // cap on total runs when RetryUntilFeasible=true; 0 = 1000
	AfterGA           func(GAResult) // callback fired after GA finishes, before TS starts
}

// HybridOutput is the result of the combined GA+TS optimisation.
type HybridResult struct {
	GAPhase        GAResult
	TSPhase        TSResult
	Chromosome     Chromosome
	Matrix         *ScheduleMatrix
	Unplaced       int
	SoftViolations int
	Elapsed        time.Duration
	Runs           int // total GA+TS runs attempted
}

// DefaultHybridConfig returns a HybridConfig with sensible defaults.
func DefaultHybridConfig() HybridConfig {
	gaCfg := DefaultGAConfig()
	gaCfg.PatienceLimit = 100
	return HybridConfig{
		GA: gaCfg,
		TS: DefaultTSConfig(),
	}
}

// hybridDominates returns true when a is strictly better than b.
func hybridDominates(a, b HybridResult) bool {
	return a.Unplaced < b.Unplaced || (a.Unplaced == b.Unplaced && a.SoftViolations < b.SoftViolations)
}

// isFeasible returns true when the result has no unplaced blocks.
func isFeasible(r HybridResult) bool { return r.Unplaced == 0 }

// derivedSeeds produces offset GA and TS seeds for run n to keep runs independent
// but reproducible given the same base configuration.
func derivedSeeds(cfg HybridConfig, n int) (gaSeed, tsSeed int64) {
	return cfg.GA.RandSeed + int64(n)*1234567891,
		cfg.TS.RandSeed + int64(n)*9876543211
}

// ExecuteHybrid runs GA→TS sequentially, repeating for ExtraRuns additional attempts.
// GA explores the solution space globally; TS refines the best GA result using
// matrix-level local search with a tabu list to prevent cycling.
// The best result across all runs is returned. Each run uses an offset seed so runs
// are independent but reproducible. When RetryUntilFeasible is set, the optimiser
// retries beyond ExtraRuns until either unplaced == 0 or MaxAttempts is reached.
func RunHybrid(
	blocks []MatrixBlock,
	candidateIndex map[uint][]Gene,
	daySlots DaySlots,
	cfg HybridConfig,
) HybridResult {
	start := time.Now()
	runs := 0

	runWithOffset := func(n int) HybridResult {
		rc := cfg
		rc.GA.RandSeed, rc.TS.RandSeed = derivedSeeds(cfg, n)
		return singleRun(blocks, candidateIndex, daySlots, rc)
	}

	best := singleRun(blocks, candidateIndex, daySlots, cfg)
	runs++

	maxRuns := cfg.ExtraRuns
	if cfg.RetryUntilFeasible {
		limit := cfg.MaxAttempts
		if limit <= 0 {
			limit = 1000
		}
		maxRuns = limit - 1
	}

	for r := 1; r <= maxRuns; r++ {
		if isFeasible(best) {
			break
		}
		result := runWithOffset(r)
		runs++
		if hybridDominates(result, best) {
			best = result
		}
	}

	best.Elapsed = time.Since(start)
	best.Runs = runs
	return best
}

// singleRun executes one full GA+TS cycle and returns the result.
func singleRun(
	blocks []MatrixBlock,
	candidateIndex map[uint][]Gene,
	daySlots DaySlots,
	cfg HybridConfig,
) HybridResult {
	gaResult := RunGA(blocks, candidateIndex, daySlots, cfg.GA)

	if cfg.AfterGA != nil {
		cfg.AfterGA(gaResult)
	}

	perfect := gaResult.Unplaced == 0 && gaResult.SoftViolations == 0
	if perfect {
		return HybridResult{
			GAPhase:        gaResult,
			Chromosome:     gaResult.Chromosome,
			Matrix:         gaResult.Matrix,
			Unplaced:       0,
			SoftViolations: 0,
		}
	}

	tsResult := RunTS(gaResult, blocks, candidateIndex, daySlots, cfg.TS)

	return HybridResult{
		GAPhase:        gaResult,
		TSPhase:        tsResult,
		Chromosome:     tsResult.Chromosome,
		Matrix:         tsResult.Matrix,
		Unplaced:       tsResult.Unplaced,
		SoftViolations: tsResult.SoftViolations,
	}
}
