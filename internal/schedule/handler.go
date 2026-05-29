package schedule

import (
	"net/http"
	"strconv"
	"time"

	"smp_mater_dei_be/internal/platform/logging"
	"smp_mater_dei_be/internal/transport/http/response"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

func GenerateScheduleHandler(c *gin.Context) {
	opts, err := parseGenerateScheduleOptions(c)
	if err != nil {
		response.Fail(c, http.StatusBadRequest, "invalid query parameters", err.Error())
		return
	}

	logToTerminal, err := parseBoolQuery(c, "logToTerminal", true)
	if err != nil {
		response.Fail(c, http.StatusBadRequest, "invalid query parameters", err.Error())
		return
	}

	if logToTerminal {
		logging.Logger.Info("schedule_generation_started",
			zap.Int("population_size", opts.Params.PopulationSize),
			zap.Int("generations", opts.Params.Generations),
			zap.Float64("mutation_rate", opts.Params.MutationRate),
			zap.Int("elite_count", opts.Params.EliteCount),
			zap.Int("sa_iterations", opts.Params.SAIterations),
			zap.Int64("seed", opts.Params.Seed),
			zap.Int("progress_every", opts.Params.ProgressEvery),
		)
		opts.OnProgress = func(p GAProgressSnapshot) {
			logProgressSnapshot("schedule_generation_progress", p)
		}
	}

	start := time.Now()
	result, err := GenerateScheduleWithOptions(opts)
	if err != nil {
		response.Fail(c, http.StatusInternalServerError, "failed to generate schedule", err.Error())
		return
	}
	result.Meta.TotalElapsedMs = time.Since(start).Milliseconds()

	if logToTerminal {
		logScheduleGenerationSummary(result)
	}

	response.OK(c, result, "schedule generated successfully")
}

func GenerateScheduleAndCompareHandler(c *gin.Context) {
	opts, err := parseGenerateScheduleOptions(c)
	if err != nil {
		response.Fail(c, http.StatusBadRequest, "invalid query parameters", err.Error())
		return
	}

	logToTerminal, err := parseBoolQuery(c, "logToTerminal", true)
	if err != nil {
		response.Fail(c, http.StatusBadRequest, "invalid query parameters", err.Error())
		return
	}

	if logToTerminal {
		opts.OnProgress = func(p GAProgressSnapshot) {
			logProgressSnapshot("schedule_generation_progress", p)
		}
	}

	start := time.Now()
	result, err := GenerateScheduleAndCompareWithReal(opts)
	if err != nil {
		response.Fail(c, http.StatusInternalServerError, "failed to generate and compare schedule", err.Error())
		return
	}
	result.Generation.Meta.TotalElapsedMs = time.Since(start).Milliseconds()

	if logToTerminal {
		logScheduleGenerationSummary(result.Generation)
		logScheduleComparisonSummary(result.Comparison)
	}

	if result.Comparison.Checked && result.Comparison.IsSame {
		response.OK(c, result, "schedule generated successfully and matches real schedule")
		return
	}
	if result.Comparison.Checked {
		response.OK(c, result, "schedule generated successfully and differs from real schedule")
		return
	}
	response.OK(c, result, "schedule generated successfully; comparison skipped because violations are not zero")
}

func GenerateScheduleStreamHandler(c *gin.Context) {
	opts, err := parseGenerateScheduleOptions(c)
	if err != nil {
		response.Fail(c, http.StatusBadRequest, "invalid query parameters", err.Error())
		return
	}

	logToTerminal, err := parseBoolQuery(c, "logToTerminal", true)
	if err != nil {
		response.Fail(c, http.StatusBadRequest, "invalid query parameters", err.Error())
		return
	}

	flusher, ok := c.Writer.(http.Flusher)
	if !ok {
		response.Fail(c, http.StatusInternalServerError, "streaming unsupported", "response writer does not implement http.Flusher")
		return
	}

	c.Writer.Header().Set("Content-Type", "text/event-stream")
	c.Writer.Header().Set("Cache-Control", "no-cache")
	c.Writer.Header().Set("Connection", "keep-alive")
	c.Writer.Header().Set("X-Accel-Buffering", "no")

	c.SSEvent("started", gin.H{
		"effectiveGa": opts.Params,
		"defaultGa":   DefaultGAParams(),
		"parameters":  GAParameterSpecs(),
	})
	flusher.Flush()

	opts.CollectProgress = false
	opts.OnProgress = func(p GAProgressSnapshot) {
		if c.Request.Context().Err() != nil {
			return
		}
		if logToTerminal {
			logProgressSnapshot("schedule_generation_progress", p)
		}
		c.SSEvent("progress", p)
		flusher.Flush()
	}

	result, genErr := GenerateScheduleWithOptions(opts)
	if genErr != nil {
		if c.Request.Context().Err() == nil {
			c.SSEvent("error", gin.H{"message": genErr.Error()})
			flusher.Flush()
		}
		return
	}

	if c.Request.Context().Err() == nil {
		if logToTerminal {
			logScheduleGenerationSummary(result)
		}
		c.SSEvent("completed", result)
		flusher.Flush()
	}
}

func GenerateScheduleAndCompareStreamHandler(c *gin.Context) {
	opts, err := parseGenerateScheduleOptions(c)
	if err != nil {
		response.Fail(c, http.StatusBadRequest, "invalid query parameters", err.Error())
		return
	}

	logToTerminal, err := parseBoolQuery(c, "logToTerminal", true)
	if err != nil {
		response.Fail(c, http.StatusBadRequest, "invalid query parameters", err.Error())
		return
	}

	flusher, ok := c.Writer.(http.Flusher)
	if !ok {
		response.Fail(c, http.StatusInternalServerError, "streaming unsupported", "response writer does not implement http.Flusher")
		return
	}

	c.Writer.Header().Set("Content-Type", "text/event-stream")
	c.Writer.Header().Set("Cache-Control", "no-cache")
	c.Writer.Header().Set("Connection", "keep-alive")
	c.Writer.Header().Set("X-Accel-Buffering", "no")

	c.SSEvent("started", gin.H{
		"effectiveGa": opts.Params,
		"defaultGa":   DefaultGAParams(),
		"parameters":  GAParameterSpecs(),
		"compareReal": true,
	})
	flusher.Flush()

	opts.CollectProgress = false
	opts.OnProgress = func(p GAProgressSnapshot) {
		if c.Request.Context().Err() != nil {
			return
		}
		if logToTerminal {
			logProgressSnapshot("schedule_generation_progress", p)
		}
		c.SSEvent("progress", p)
		flusher.Flush()
	}

	result, genErr := GenerateScheduleAndCompareWithReal(opts)
	if genErr != nil {
		if c.Request.Context().Err() == nil {
			c.SSEvent("error", gin.H{"message": genErr.Error()})
			flusher.Flush()
		}
		return
	}

	if c.Request.Context().Err() == nil {
		if logToTerminal {
			logScheduleGenerationSummary(result.Generation)
			logScheduleComparisonSummary(result.Comparison)
		}
		c.SSEvent("completed", result)
		flusher.Flush()
	}
}

func ValidateRealScheduleHandler(c *gin.Context) {
	logToTerminal, err := parseBoolQuery(c, "logToTerminal", true)
	if err != nil {
		response.Fail(c, http.StatusBadRequest, "invalid query parameters", err.Error())
		return
	}

	result, err := EvaluateRealScheduleBaseline()
	if err != nil {
		response.Fail(c, http.StatusInternalServerError, "failed to validate real schedule baseline", err.Error())
		return
	}

	if logToTerminal {
		logRealScheduleValidationSummary(result)
	}

	response.OK(c, result, "real schedule baseline evaluated")
}

func GAInfoHandler(c *gin.Context) {
	response.OK(c, gin.H{
		"defaultGa":  DefaultGAParams(),
		"parameters": GAParameterSpecs(),
	}, "ga parameter information")
}

// DiagnoseSAHandler runs SA-only trials to verify layer-2 health:
// can simulated annealing repair a greedy chromosome to 0 violations?
//
// Query params:
//
//	trials       — number of independent greedy+SA runs (default 5)
//	saIterations — SA passes per trial (default 2000)
//	seed         — base random seed; trial i uses seed+i (default: current unix-nano)
func GenerateV2ScheduleStreamHandler(c *gin.Context) {
	start := time.Now()
	opts, err := parseGenerateScheduleOptions(c)
	if err != nil {
		response.Fail(c, http.StatusBadRequest, "invalid query parameters", err.Error())
		return
	}

	logToTerminal, err := parseBoolQuery(c, "logToTerminal", true)
	if err != nil {
		response.Fail(c, http.StatusBadRequest, "invalid query parameters", err.Error())
		return
	}

	flusher, ok := c.Writer.(http.Flusher)
	if !ok {
		response.Fail(c, http.StatusInternalServerError, "streaming unsupported", "response writer does not implement http.Flusher")
		return
	}

	c.Writer.Header().Set("Content-Type", "text/event-stream")
	c.Writer.Header().Set("Cache-Control", "no-cache")
	c.Writer.Header().Set("Connection", "keep-alive")
	c.Writer.Header().Set("X-Accel-Buffering", "no")

	c.SSEvent("started", gin.H{
		"effectiveGa": opts.Params,
		"defaultGa":   DefaultGAParams(),
		"parameters":  GAParameterSpecs(),
		"compareReal": true,
	})
	flusher.Flush()

	opts.CollectProgress = false
	opts.OnProgress = func(p GAProgressSnapshot) {
		if c.Request.Context().Err() != nil {
			return
		}
		if logToTerminal {
			logProgressSnapshot("v2_schedule_generation_progress", p)
		}
		c.SSEvent("progress", p)
		flusher.Flush()
	}

	result, genErr := GenerateV2Schedule(opts)
	if genErr != nil {
		if c.Request.Context().Err() == nil {
			c.SSEvent("error", gin.H{"message": genErr.Error()})
			flusher.Flush()
		}
		return
	}

	if c.Request.Context().Err() == nil {
		result.Generation.Meta.TotalElapsedMs = time.Since(start).Milliseconds()
		if logToTerminal {
			logScheduleGenerationSummary(result.Generation)
			logScheduleComparisonSummary(result.Comparison)
		}
		c.SSEvent("completed", result)
		flusher.Flush()
	}
}

func DiagnoseSAHandler(c *gin.Context) {
	trials, err := parseIntQuery(c, "trials", 5)
	if err != nil {
		response.Fail(c, http.StatusBadRequest, "invalid query parameters", err.Error())
		return
	}

	saIterations, err := parseIntQuery(c, "saIterations", 2000)
	if err != nil {
		response.Fail(c, http.StatusBadRequest, "invalid query parameters", err.Error())
		return
	}

	seed, err := parseInt64Query(c, "seed", time.Now().UnixNano())
	if err != nil {
		response.Fail(c, http.StatusBadRequest, "invalid query parameters", err.Error())
		return
	}

	result, err := DiagnoseSA(trials, saIterations, seed)
	if err != nil {
		response.Fail(c, http.StatusInternalServerError, "SA diagnostic failed", err.Error())
		return
	}

	response.OK(c, result, "SA diagnostic complete")
}

func parseGenerateScheduleOptions(c *gin.Context) (GenerateScheduleOptions, error) {
	opts := NewGenerateScheduleOptions()
	var err error

	opts.Params.PopulationSize, err = parseIntQuery(c, "populationSize", opts.Params.PopulationSize)
	if err != nil {
		return opts, err
	}

	opts.Params.Generations, err = parseIntQuery(c, "generations", opts.Params.Generations)
	if err != nil {
		return opts, err
	}

	opts.Params.MutationRate, err = parseFloatQuery(c, "mutationRate", opts.Params.MutationRate)
	if err != nil {
		return opts, err
	}

	opts.Params.EliteCount, err = parseIntQuery(c, "eliteCount", opts.Params.EliteCount)
	if err != nil {
		return opts, err
	}

	opts.Params.TournamentSize, err = parseIntQuery(c, "tournamentSize", opts.Params.TournamentSize)
	if err != nil {
		return opts, err
	}

	opts.Params.SAIterations, err = parseIntQuery(c, "saIterations", opts.Params.SAIterations)
	if err != nil {
		return opts, err
	}

	opts.Params.Seed, err = parseInt64Query(c, "seed", opts.Params.Seed)
	if err != nil {
		return opts, err
	}

	opts.Params.ProgressEvery, err = parseIntQuery(c, "progressEvery", opts.Params.ProgressEvery)
	if err != nil {
		return opts, err
	}

	opts.CollectProgress, err = parseBoolQuery(c, "includeProgress", false)
	if err != nil {
		return opts, err
	}

	opts.IncludeSeedWarnings, err = parseBoolQuery(c, "includeSeedWarnings", false)
	if err != nil {
		return opts, err
	}

	return opts, nil
}

func parseIntQuery(c *gin.Context, key string, fallback int) (int, error) {
	raw := c.Query(key)
	if raw == "" {
		return fallback, nil
	}

	v, err := strconv.Atoi(raw)
	if err != nil {
		return 0, err
	}

	return v, nil
}

func parseInt64Query(c *gin.Context, key string, fallback int64) (int64, error) {
	raw := c.Query(key)
	if raw == "" {
		return fallback, nil
	}

	v, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		return 0, err
	}

	return v, nil
}

func parseFloatQuery(c *gin.Context, key string, fallback float64) (float64, error) {
	raw := c.Query(key)
	if raw == "" {
		return fallback, nil
	}

	v, err := strconv.ParseFloat(raw, 64)
	if err != nil {
		return 0, err
	}

	return v, nil
}

func parseBoolQuery(c *gin.Context, key string, fallback bool) (bool, error) {
	raw := c.Query(key)
	if raw == "" {
		return fallback, nil
	}

	v, err := strconv.ParseBool(raw)
	if err != nil {
		return false, err
	}

	return v, nil
}

func logProgressSnapshot(event string, p GAProgressSnapshot) {
	fields := []zap.Field{
		zap.Int("generation", p.Generation),
		zap.Int("total_generations", p.TotalGenerations),
		zap.Float64("progress_percent", p.ProgressPercent),
		zap.Int("best_fitness", p.BestFitness),
		zap.Int("best_violations", p.BestViolations),
		zap.Int("best_unplaced", p.BestUnplaced),
		zap.Int64("elapsed_ms", p.ElapsedMs),
		zap.Int("avg_fitness", p.AvgFitness),
		zap.Int("worst_fitness", p.WorstFitness),
		zap.Float64("diversity_score", p.DiversityScore),
		zap.Int("stagnant_gens", p.StagnantGens),
		zap.Int("sa_improvements", p.SAImprovements),
		zap.Int("mutation_hits", p.MutationHits),
		zap.Int("feasible_count", p.FeasibleCount),
		zap.Int("class_conflicts", p.Breakdown.ClassConflicts),
		zap.Int("teacher_conflicts", p.Breakdown.TeacherConflicts),
		zap.Int("sibling_violations", p.Breakdown.SiblingViolations),
	}
	if p.BestDistanceFromSeed != nil {
		fields = append(fields, zap.Int("best_distance_from_seed", *p.BestDistanceFromSeed))
	}
	logging.Logger.Info(event, fields...)
}

func logScheduleGenerationSummary(result *ScheduleGenerationResult) {
	fields := []zap.Field{
		zap.Int("entries_generated", result.Meta.Result.EntriesGenerated),
		zap.Int("best_fitness", result.Meta.Result.BestFitness),
		zap.Int("violations", result.Meta.Result.Violations),
		zap.Int("unplaced", result.Meta.Result.Unplaced),
		zap.Int("class_conflicts", result.Meta.Result.Breakdown.ClassConflicts),
		zap.Int("teacher_conflicts", result.Meta.Result.Breakdown.TeacherConflicts),
		zap.Int("sibling_violations", result.Meta.Result.Breakdown.SiblingViolations),
		zap.Bool("seed_warnings_checked", result.Meta.SeedWarningsChecked),
	}
	if result.Meta.SeedWarningsChecked {
		fields = append(fields, zap.Int("seed_warnings", len(result.Meta.SeedWarnings)))
	}
	logging.Logger.Info("schedule_generation_completed", fields...)
}

func logRealScheduleValidationSummary(result *RealScheduleValidationResult) {
	logging.Logger.Info("real_schedule_baseline_evaluated",
		zap.Bool("is_feasible", result.Meta.IsFeasible),
		zap.Int("entries_generated", result.Meta.Result.EntriesGenerated),
		zap.Int("best_fitness", result.Meta.Result.BestFitness),
		zap.Int("violations", result.Meta.Result.Violations),
		zap.Int("unplaced", result.Meta.Result.Unplaced),
		zap.Int("class_conflicts", result.Meta.Result.Breakdown.ClassConflicts),
		zap.Int("teacher_conflicts", result.Meta.Result.Breakdown.TeacherConflicts),
		zap.Int("sibling_violations", result.Meta.Result.Breakdown.SiblingViolations),
		zap.Int("seed_warnings", len(result.Meta.SeedWarnings)),
	)
}

func logScheduleComparisonSummary(comparison ScheduleDiffResult) {
	logging.Logger.Info("schedule_real_comparison_completed",
		zap.Bool("checked", comparison.Checked),
		zap.Bool("is_same", comparison.IsSame),
		zap.Int("generated_count", comparison.GeneratedCount),
		zap.Int("real_schedule_count", comparison.RealScheduleCount),
		zap.String("reason", comparison.Reason),
	)
}

func GenerateV3ScheduleReadableHandler(c *gin.Context) {
	start := time.Now()
	gaOpts, err := parseGenerateScheduleOptions(c)
	if err != nil {
		response.Fail(c, http.StatusBadRequest, "invalid query parameters", err.Error())
		return
	}

	saInitTemp, err := parseFloatQuery(c, "saInitialTemperature", DefaultSAParams().InitialTemperature)
	if err != nil {
		response.Fail(c, http.StatusBadRequest, "invalid query parameters", err.Error())
		return
	}
	saCoolingRate, err := parseFloatQuery(c, "saCoolingRate", DefaultSAParams().CoolingRate)
	if err != nil {
		response.Fail(c, http.StatusBadRequest, "invalid query parameters", err.Error())
		return
	}
	saIterations, err := parseIntQuery(c, "saIterations", DefaultSAParams().Iterations)
	if err != nil {
		response.Fail(c, http.StatusBadRequest, "invalid query parameters", err.Error())
		return
	}
	saProgressEvery, err := parseIntQuery(c, "saProgressEvery", DefaultSAParams().ProgressEvery)
	if err != nil {
		response.Fail(c, http.StatusBadRequest, "invalid query parameters", err.Error())
		return
	}
	saSeed, err := parseInt64Query(c, "saSeed", 0)
	if err != nil {
		response.Fail(c, http.StatusBadRequest, "invalid query parameters", err.Error())
		return
	}
	saPerturbCount, err := parseIntQuery(c, "saPerturbCount", DefaultSAParams().PerturbCount)
	if err != nil {
		response.Fail(c, http.StatusBadRequest, "invalid query parameters", err.Error())
		return
	}
	saPerturbAfter, err := parseIntQuery(c, "saPerturbAfter", DefaultSAParams().PerturbAfter)
	if err != nil {
		response.Fail(c, http.StatusBadRequest, "invalid query parameters", err.Error())
		return
	}
	stagnationLimit, err := parseIntQuery(c, "stagnationLimit", 100)
	if err != nil {
		response.Fail(c, http.StatusBadRequest, "invalid query parameters", err.Error())
		return
	}
	restarts, err := parseIntQuery(c, "restarts", 0)
	if err != nil {
		response.Fail(c, http.StatusBadRequest, "invalid query parameters", err.Error())
		return
	}

	opts := GenerateHybridOptions{
		GA: gaOpts.Params,
		SA: SAParams{
			InitialTemperature: saInitTemp,
			CoolingRate:        saCoolingRate,
			Iterations:         saIterations,
			ProgressEvery:      saProgressEvery,
			Seed:               saSeed,
			PerturbCount:       saPerturbCount,
			PerturbAfter:       saPerturbAfter,
		},
		StagnationLimit: stagnationLimit,
		Restarts:        restarts,
	}

	result, err := GenerateV3ScheduleReadable(opts)
	if err != nil {
		response.Fail(c, http.StatusInternalServerError, "failed to generate readable schedule", err.Error())
		return
	}

	result.Meta.TotalElapsedMs = time.Since(start).Milliseconds()
	response.OK(c, result, "schedule generated successfully")
}

func GenerateV3MultiRunHandler(c *gin.Context) {
	gaOpts, err := parseGenerateScheduleOptions(c)
	if err != nil {
		response.Fail(c, http.StatusBadRequest, "invalid query parameters", err.Error())
		return
	}

	saInitTemp, err := parseFloatQuery(c, "saInitialTemperature", DefaultSAParams().InitialTemperature)
	if err != nil {
		response.Fail(c, http.StatusBadRequest, "invalid query parameters", err.Error())
		return
	}
	saCoolingRate, err := parseFloatQuery(c, "saCoolingRate", DefaultSAParams().CoolingRate)
	if err != nil {
		response.Fail(c, http.StatusBadRequest, "invalid query parameters", err.Error())
		return
	}
	saIterations, err := parseIntQuery(c, "saIterations", DefaultSAParams().Iterations)
	if err != nil {
		response.Fail(c, http.StatusBadRequest, "invalid query parameters", err.Error())
		return
	}
	saProgressEvery, err := parseIntQuery(c, "saProgressEvery", DefaultSAParams().ProgressEvery)
	if err != nil {
		response.Fail(c, http.StatusBadRequest, "invalid query parameters", err.Error())
		return
	}
	saSeed, err := parseInt64Query(c, "saSeed", 0)
	if err != nil {
		response.Fail(c, http.StatusBadRequest, "invalid query parameters", err.Error())
		return
	}
	saPerturbCount, err := parseIntQuery(c, "saPerturbCount", DefaultSAParams().PerturbCount)
	if err != nil {
		response.Fail(c, http.StatusBadRequest, "invalid query parameters", err.Error())
		return
	}
	saPerturbAfter, err := parseIntQuery(c, "saPerturbAfter", DefaultSAParams().PerturbAfter)
	if err != nil {
		response.Fail(c, http.StatusBadRequest, "invalid query parameters", err.Error())
		return
	}
	stagnationLimit, err := parseIntQuery(c, "stagnationLimit", 100)
	if err != nil {
		response.Fail(c, http.StatusBadRequest, "invalid query parameters", err.Error())
		return
	}
	restarts, err := parseIntQuery(c, "restarts", 0)
	if err != nil {
		response.Fail(c, http.StatusBadRequest, "invalid query parameters", err.Error())
		return
	}
	runs, err := parseIntQuery(c, "runs", 5)
	if err != nil {
		response.Fail(c, http.StatusBadRequest, "invalid query parameters", err.Error())
		return
	}

	opts := GenerateHybridOptions{
		GA: gaOpts.Params,
		SA: SAParams{
			InitialTemperature: saInitTemp,
			CoolingRate:        saCoolingRate,
			Iterations:         saIterations,
			ProgressEvery:      saProgressEvery,
			Seed:               saSeed,
			PerturbCount:       saPerturbCount,
			PerturbAfter:       saPerturbAfter,
		},
		StagnationLimit: stagnationLimit,
		Restarts:        restarts,
	}

	result, err := GenerateV3MultiRun(opts, runs)
	if err != nil {
		response.Fail(c, http.StatusInternalServerError, "failed to run multi-run schedule generation", err.Error())
		return
	}

	response.OK(c, result, "multi-run schedule generation complete")
}

func GenerateV3MultiRunStreamHandler(c *gin.Context) {
	gaOpts, err := parseGenerateScheduleOptions(c)
	if err != nil {
		response.Fail(c, http.StatusBadRequest, "invalid query parameters", err.Error())
		return
	}

	saInitTemp, err := parseFloatQuery(c, "saInitialTemperature", DefaultSAParams().InitialTemperature)
	if err != nil {
		response.Fail(c, http.StatusBadRequest, "invalid query parameters", err.Error())
		return
	}
	saCoolingRate, err := parseFloatQuery(c, "saCoolingRate", DefaultSAParams().CoolingRate)
	if err != nil {
		response.Fail(c, http.StatusBadRequest, "invalid query parameters", err.Error())
		return
	}
	saIterations, err := parseIntQuery(c, "saIterations", DefaultSAParams().Iterations)
	if err != nil {
		response.Fail(c, http.StatusBadRequest, "invalid query parameters", err.Error())
		return
	}
	saProgressEvery, err := parseIntQuery(c, "saProgressEvery", DefaultSAParams().ProgressEvery)
	if err != nil {
		response.Fail(c, http.StatusBadRequest, "invalid query parameters", err.Error())
		return
	}
	saSeed, err := parseInt64Query(c, "saSeed", 0)
	if err != nil {
		response.Fail(c, http.StatusBadRequest, "invalid query parameters", err.Error())
		return
	}
	saPerturbCount, err := parseIntQuery(c, "saPerturbCount", DefaultSAParams().PerturbCount)
	if err != nil {
		response.Fail(c, http.StatusBadRequest, "invalid query parameters", err.Error())
		return
	}
	saPerturbAfter, err := parseIntQuery(c, "saPerturbAfter", DefaultSAParams().PerturbAfter)
	if err != nil {
		response.Fail(c, http.StatusBadRequest, "invalid query parameters", err.Error())
		return
	}
	stagnationLimit, err := parseIntQuery(c, "stagnationLimit", 100)
	if err != nil {
		response.Fail(c, http.StatusBadRequest, "invalid query parameters", err.Error())
		return
	}
	restarts, err := parseIntQuery(c, "restarts", 0)
	if err != nil {
		response.Fail(c, http.StatusBadRequest, "invalid query parameters", err.Error())
		return
	}
	runs, err := parseIntQuery(c, "runs", 5)
	if err != nil {
		response.Fail(c, http.StatusBadRequest, "invalid query parameters", err.Error())
		return
	}
	if runs < 1 {
		runs = 1
	}

	flusher, ok := c.Writer.(http.Flusher)
	if !ok {
		response.Fail(c, http.StatusInternalServerError, "streaming unsupported", "response writer does not implement http.Flusher")
		return
	}

	c.Writer.Header().Set("Content-Type", "text/event-stream")
	c.Writer.Header().Set("Cache-Control", "no-cache")
	c.Writer.Header().Set("Connection", "keep-alive")
	c.Writer.Header().Set("X-Accel-Buffering", "no")

	effectiveSA := SAParams{
		InitialTemperature: saInitTemp,
		CoolingRate:        saCoolingRate,
		Iterations:         saIterations,
		ProgressEvery:      saProgressEvery,
		Seed:               saSeed,
		PerturbCount:       saPerturbCount,
		PerturbAfter:       saPerturbAfter,
	}

	baseOpts := GenerateHybridOptions{
		GA:              gaOpts.Params,
		SA:              effectiveSA,
		StagnationLimit: stagnationLimit,
		Restarts:        restarts,
	}

	c.SSEvent("started", gin.H{
		"runs":            runs,
		"effectiveGa":     gaOpts.Params,
		"defaultGa":       DefaultGAParams(),
		"effectiveSa":     effectiveSA,
		"defaultSa":       DefaultSAParams(),
		"stagnationLimit": stagnationLimit,
		"restarts":        restarts,
	})
	flusher.Flush()

	totalStart := time.Now()
	results := make([]RunSummary, 0, runs)

	baseSeedGA := baseOpts.GA.Seed
	baseSeedSA := baseOpts.SA.Seed

	for i := 0; i < runs; i++ {
		if c.Request.Context().Err() != nil {
			return
		}

		runNum := i + 1
		runOpts := baseOpts
		if baseSeedGA != 0 {
			runOpts.GA.Seed = baseSeedGA + int64(i)
		}
		if baseSeedSA != 0 {
			runOpts.SA.Seed = baseSeedSA + int64(i)
		}

		c.SSEvent("run_start", gin.H{"run": runNum, "totalRuns": runs})
		flusher.Flush()

		runOpts.OnGAProgress = func(p GAProgressSnapshot) {
			if c.Request.Context().Err() != nil {
				return
			}
			c.SSEvent("ga_progress", gin.H{"run": runNum, "progress": p})
			flusher.Flush()
		}
		runOpts.OnGAComplete = func(r GAPhaseResult) {
			if c.Request.Context().Err() != nil {
				return
			}
			c.SSEvent("phase_change", gin.H{"run": runNum, "phase": "sa", "gaResult": r})
			flusher.Flush()
		}
		runOpts.OnSAProgress = func(p SAProgressSnapshot) {
			if c.Request.Context().Err() != nil {
				return
			}
			c.SSEvent("sa_progress", gin.H{"run": runNum, "progress": p})
			flusher.Flush()
		}

		runStart := time.Now()
		result, genErr := GenerateV3Schedule(runOpts)
		if genErr != nil {
			if c.Request.Context().Err() == nil {
				c.SSEvent("error", gin.H{"run": runNum, "message": genErr.Error()})
				flusher.Flush()
			}
			return
		}

		result.Generation.Meta.TotalElapsedMs = time.Since(runStart).Milliseconds()
		summary := RunSummary{Run: runNum, Meta: result.Generation.Meta}
		results = append(results, summary)

		if c.Request.Context().Err() == nil {
			c.SSEvent("run_complete", summary)
			flusher.Flush()
		}
	}

	if c.Request.Context().Err() == nil {
		c.SSEvent("completed", MultiRunResult{
			Runs:           runs,
			TotalElapsedMs: time.Since(totalStart).Milliseconds(),
			Results:        results,
		})
		flusher.Flush()
	}
}

func DiagnoseMatrixSlotsHandler(c *gin.Context) {
	response.OK(c, GetMatrixSlotsDiagnostic(), "matrix slots")
}

func DiagnoseMatrixBlocksHandler(c *gin.Context) {
	result, err := GetMatrixBlocksDiagnostic()
	if err != nil {
		response.Fail(c, http.StatusInternalServerError, "failed to load matrix blocks", err.Error())
		return
	}
	response.OK(c, result, "matrix blocks")
}

func GenerateV3ScheduleStreamHandler(c *gin.Context) {
	start := time.Now()
	gaOpts, err := parseGenerateScheduleOptions(c)
	if err != nil {
		response.Fail(c, http.StatusBadRequest, "invalid query parameters", err.Error())
		return
	}

	saInitTemp, err := parseFloatQuery(c, "saInitialTemperature", DefaultSAParams().InitialTemperature)
	if err != nil {
		response.Fail(c, http.StatusBadRequest, "invalid query parameters", err.Error())
		return
	}
	saCoolingRate, err := parseFloatQuery(c, "saCoolingRate", DefaultSAParams().CoolingRate)
	if err != nil {
		response.Fail(c, http.StatusBadRequest, "invalid query parameters", err.Error())
		return
	}
	saIterations, err := parseIntQuery(c, "saIterations", DefaultSAParams().Iterations)
	if err != nil {
		response.Fail(c, http.StatusBadRequest, "invalid query parameters", err.Error())
		return
	}
	saProgressEvery, err := parseIntQuery(c, "saProgressEvery", DefaultSAParams().ProgressEvery)
	if err != nil {
		response.Fail(c, http.StatusBadRequest, "invalid query parameters", err.Error())
		return
	}
	saSeed, err := parseInt64Query(c, "saSeed", 0)
	if err != nil {
		response.Fail(c, http.StatusBadRequest, "invalid query parameters", err.Error())
		return
	}
	saPerturbCount, err := parseIntQuery(c, "saPerturbCount", DefaultSAParams().PerturbCount)
	if err != nil {
		response.Fail(c, http.StatusBadRequest, "invalid query parameters", err.Error())
		return
	}
	saPerturbAfter, err := parseIntQuery(c, "saPerturbAfter", DefaultSAParams().PerturbAfter)
	if err != nil {
		response.Fail(c, http.StatusBadRequest, "invalid query parameters", err.Error())
		return
	}
	stagnationLimit, err := parseIntQuery(c, "stagnationLimit", 100)
	if err != nil {
		response.Fail(c, http.StatusBadRequest, "invalid query parameters", err.Error())
		return
	}
	restarts, err := parseIntQuery(c, "restarts", 0)
	if err != nil {
		response.Fail(c, http.StatusBadRequest, "invalid query parameters", err.Error())
		return
	}
	loopUntilFeasible, err := parseBoolQuery(c, "loopUntilFeasible", false)
	if err != nil {
		response.Fail(c, http.StatusBadRequest, "invalid query parameters", err.Error())
		return
	}
	maxLoops, err := parseIntQuery(c, "maxLoops", 0)
	if err != nil {
		response.Fail(c, http.StatusBadRequest, "invalid query parameters", err.Error())
		return
	}

	logToTerminal, err := parseBoolQuery(c, "logToTerminal", true)
	if err != nil {
		response.Fail(c, http.StatusBadRequest, "invalid query parameters", err.Error())
		return
	}

	flusher, ok := c.Writer.(http.Flusher)
	if !ok {
		response.Fail(c, http.StatusInternalServerError, "streaming unsupported", "response writer does not implement http.Flusher")
		return
	}

	c.Writer.Header().Set("Content-Type", "text/event-stream")
	c.Writer.Header().Set("Cache-Control", "no-cache")
	c.Writer.Header().Set("Connection", "keep-alive")
	c.Writer.Header().Set("X-Accel-Buffering", "no")

	effectiveSA := SAParams{
		InitialTemperature: saInitTemp,
		CoolingRate:        saCoolingRate,
		Iterations:         saIterations,
		ProgressEvery:      saProgressEvery,
		Seed:               saSeed,
		PerturbCount:       saPerturbCount,
		PerturbAfter:       saPerturbAfter,
	}

	c.SSEvent("started", gin.H{
		"effectiveGa":     gaOpts.Params,
		"defaultGa":       DefaultGAParams(),
		"effectiveSa":     effectiveSA,
		"defaultSa":       DefaultSAParams(),
		"stagnationLimit": stagnationLimit,
		"restarts":        restarts,
	})
	flusher.Flush()

	opts := GenerateHybridOptions{
		GA:                gaOpts.Params,
		SA:                effectiveSA,
		StagnationLimit:   stagnationLimit,
		Restarts:          restarts,
		LoopUntilFeasible: loopUntilFeasible,
		MaxLoops:          maxLoops,
		OnGAProgress: func(p GAProgressSnapshot) {
			if c.Request.Context().Err() != nil {
				return
			}
			if logToTerminal {
				logProgressSnapshot("v3_ga_progress", p)
			}
			c.SSEvent("ga_progress", p)
			flusher.Flush()
		},
		OnGAComplete: func(r GAPhaseResult) {
			if c.Request.Context().Err() != nil {
				return
			}
			c.SSEvent("phase_change", gin.H{"phase": "sa", "gaResult": r})
			flusher.Flush()
		},
		OnSAProgress: func(p SAProgressSnapshot) {
			if c.Request.Context().Err() != nil {
				return
			}
			c.SSEvent("sa_progress", p)
			flusher.Flush()
		},
	}

	result, genErr := GenerateV3Schedule(opts)
	if genErr != nil {
		if c.Request.Context().Err() == nil {
			c.SSEvent("error", gin.H{"message": genErr.Error()})
			flusher.Flush()
		}
		return
	}

	if c.Request.Context().Err() == nil {
		result.Generation.Meta.TotalElapsedMs = time.Since(start).Milliseconds()
		if logToTerminal {
			logScheduleGenerationSummary(result.Generation)
			logScheduleComparisonSummary(result.Comparison)
		}
		c.SSEvent("completed", result)
		flusher.Flush()
	}
}
