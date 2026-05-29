package algorithm

import (
	"time"

	"github.com/google/uuid"
)

type HybridConfig struct {
	GA                GAConfig
	SA                SAConfig
	Restarts          int            // additional full GA+SA runs; 0 = single run
	LoopUntilFeasible bool           // keep retrying until unplaced == 0; ignores Restarts limit
	MaxLoops          int            // max attempts when LoopUntilFeasible=true; 0 = 1000
	OnGAComplete      func(GAResult) // fires after GA finishes, before SA starts
}

type HybridResult struct {
	GAPhase        GAResult
	SAPhase        SAResult
	Chromosome     Chromosome
	Matrix         *ScheduleMatrix
	Unplaced       int
	SoftViolations int
	Elapsed        time.Duration
	Loops          int // total full GA+SA runs attempted
}

func DefaultHybridConfig() HybridConfig {
	gaCfg := DefaultGAConfig()
	gaCfg.StagnationLimit = 100
	return HybridConfig{
		GA: gaCfg,
		SA: DefaultSAConfig(),
	}
}

// RunHybrid runs GA then SA sequentially, repeating for Restarts additional runs.
// GA explores the search space globally; SA refines the best GA result using matrix-level
// local search. The best result across all runs is returned. Each restart uses an offset
// seed so runs are independent but reproducible.
// If LoopUntilFeasible is true, it keeps retrying beyond Restarts until unplaced == 0,
// up to MaxLoops total attempts (default 1000 when MaxLoops == 0).
func RunHybrid(
	blocks []MatrixBlock,
	candidateIndex map[uuid.UUID][]Gene,
	daySlots DaySlots,
	cfg HybridConfig,
) HybridResult {
	start := time.Now()
	loops := 0

	isBetter := func(a, b HybridResult) bool {
		return a.Unplaced < b.Unplaced || (a.Unplaced == b.Unplaced && a.SoftViolations < b.SoftViolations)
	}
	isFeasible := func(r HybridResult) bool { return r.Unplaced == 0 }

	seedOffset := func(n int) (gaS, saS int64) {
		return cfg.GA.Seed + int64(n)*1234567891, cfg.SA.Seed + int64(n)*9876543211
	}
	runWithOffset := func(n int) HybridResult {
		rc := cfg
		rc.GA.Seed, rc.SA.Seed = seedOffset(n)
		return runHybridOnce(blocks, candidateIndex, daySlots, rc)
	}

	best := runHybridOnce(blocks, candidateIndex, daySlots, cfg)
	loops++

	restarts := cfg.Restarts
	if cfg.LoopUntilFeasible {
		maxLoops := cfg.MaxLoops
		if maxLoops <= 0 {
			maxLoops = 1000
		}
		restarts = maxLoops - 1
	}

	for r := 1; r <= restarts; r++ {
		if isFeasible(best) {
			break
		}
		result := runWithOffset(r)
		loops++
		if isBetter(result, best) {
			best = result
		}
	}

	best.Elapsed = time.Since(start)
	best.Loops = loops
	return best
}

func runHybridOnce(
	blocks []MatrixBlock,
	candidateIndex map[uuid.UUID][]Gene,
	daySlots DaySlots,
	cfg HybridConfig,
) HybridResult {
	gaResult := RunGA(blocks, candidateIndex, daySlots, cfg.GA)

	if cfg.OnGAComplete != nil {
		cfg.OnGAComplete(gaResult)
	}

	if gaResult.Unplaced == 0 && gaResult.SoftViolations == 0 {
		return HybridResult{
			GAPhase:        gaResult,
			Chromosome:     gaResult.Chromosome,
			Matrix:         gaResult.Matrix,
			Unplaced:       0,
			SoftViolations: 0,
		}
	}

	saResult := RunSA(gaResult, blocks, candidateIndex, daySlots, cfg.SA)

	return HybridResult{
		GAPhase:        gaResult,
		SAPhase:        saResult,
		Chromosome:     saResult.Chromosome,
		Matrix:         saResult.Matrix,
		Unplaced:       saResult.Unplaced,
		SoftViolations: saResult.SoftViolations,
	}
}
