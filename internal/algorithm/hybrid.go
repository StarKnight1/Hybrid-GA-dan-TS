package algorithm

import (
	"context"
	"time"
)

// HybridConfig menyimpan konfigurasi untuk optimiser GA+TS gabungan.
type HybridConfig struct {
	GA                 GAConfig
	TS                 TSConfig
	ExtraRuns          int            // jumlah run GA+TS tambahan setelah run pertama; 0 = satu run saja
	RetryUntilFeasible bool           // terus mencoba sampai unplaced == 0; mengesampingkan ExtraRuns
	MaxAttempts        int            // batas total run saat RetryUntilFeasible=true; 0 = 1000
	AfterGA            func(GAResult) // callback setelah GA selesai, sebelum TS dimulai
}

// HybridResult adalah hasil dari optimasi GA+TS gabungan.
type HybridResult struct {
	GAPhase        GAResult
	TSPhase        TSResult
	Chromosome     Chromosome
	Matrix         *ScheduleMatrix
	Unplaced       int
	SoftViolations int
	Elapsed        time.Duration
	Runs           int // total run GA+TS yang dilakukan
}

// DefaultHybridConfig mengembalikan HybridConfig dengan nilai default yang wajar.
func DefaultHybridConfig() HybridConfig {
	gaCfg := DefaultGAConfig()
	gaCfg.PatienceLimit = 100
	return HybridConfig{
		GA: gaCfg,
		TS: DefaultTSConfig(),
	}
}

// hybridDominates mengembalikan true jika a lebih baik dari b.
func hybridDominates(a, b HybridResult) bool {
	return a.Unplaced < b.Unplaced || (a.Unplaced == b.Unplaced && a.SoftViolations < b.SoftViolations)
}

// isFeasible mengembalikan true jika hasil tidak memiliki blok yang unplaced.
func isFeasible(r HybridResult) bool { return r.Unplaced == 0 }

// derivedSeeds menghasilkan seed GA dan TS yang di-offset untuk run ke-n agar setiap run
// independen namun tetap dapat direproduksi dari konfigurasi dasar yang sama.
func derivedSeeds(cfg HybridConfig, n int) (gaSeed, tsSeed int64) {
	return cfg.GA.RandSeed + int64(n)*1234567891,
		cfg.TS.RandSeed + int64(n)*9876543211
}

// RunHybrid menjalankan GA→TS secara berurutan, diulang sebanyak ExtraRuns tambahan.
// GA menjelajahi ruang solusi secara global; TS memperhalus hasil terbaik GA menggunakan
// pencarian lokal berbasis matriks dengan daftar tabu untuk mencegah siklus.
// Hasil terbaik dari semua run dikembalikan. Setiap run menggunakan seed yang di-offset
// sehingga run bersifat independen namun dapat direproduksi.
// Jika RetryUntilFeasible diaktifkan, optimiser terus mencoba sampai unplaced == 0
// atau MaxAttempts tercapai.
func RunHybrid(
	ctx context.Context,
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
		return singleRun(ctx, blocks, candidateIndex, daySlots, rc)
	}

	best := singleRun(ctx, blocks, candidateIndex, daySlots, cfg)
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
		if ctx.Err() != nil {
			break
		}
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

// singleRun menjalankan satu siklus penuh GA+TS dan mengembalikan hasilnya.
func singleRun(
	ctx context.Context,
	blocks []MatrixBlock,
	candidateIndex map[uint][]Gene,
	daySlots DaySlots,
	cfg HybridConfig,
) HybridResult {
	gaResult := RunGA(ctx, blocks, candidateIndex, daySlots, cfg.GA)

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

	tsResult := RunTS(ctx, gaResult, blocks, candidateIndex, daySlots, cfg.TS)

	return HybridResult{
		GAPhase:        gaResult,
		TSPhase:        tsResult,
		Chromosome:     tsResult.Chromosome,
		Matrix:         tsResult.Matrix,
		Unplaced:       tsResult.Unplaced,
		SoftViolations: tsResult.SoftViolations,
	}
}
