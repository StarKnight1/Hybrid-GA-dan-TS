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

// ── V2: GA-only streaming ─────────────────────────────────────────────────────

func GenerateV2ScheduleStreamHandler(c *gin.Context) {
	start := time.Now()
	gaParams, err := parseGAParams(c)
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
		"effectiveGa": gaParams,
		"defaultGa":   DefaultGAParams(),
		"parameters":  GAParameterSpecs(),
	})
	flusher.Flush()

	opts := GenerateScheduleOptions{
		Params: gaParams,
		OnProgress: func(p GAProgressSnapshot) {
			if c.Request.Context().Err() != nil {
				return
			}
			if logToTerminal {
				logGAProgress("v2_ga_progress", p)
			}
			c.SSEvent("progress", p)
			flusher.Flush()
		},
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
		result.Meta.TotalElapsedMs = time.Since(start).Milliseconds()
		if logToTerminal {
			logScheduleSummary("v2_schedule_completed", result)
		}
		c.SSEvent("completed", result)
		flusher.Flush()
	}
}

// ── V3: Hybrid GA+TS streaming ────────────────────────────────────────────────

func GenerateV3ScheduleStreamHandler(c *gin.Context) {
	start := time.Now()

	gaParams, err := parseGAParams(c)
	if err != nil {
		response.Fail(c, http.StatusBadRequest, "invalid query parameters", err.Error())
		return
	}
	tsParams, err := parseTSParams(c)
	if err != nil {
		response.Fail(c, http.StatusBadRequest, "invalid query parameters", err.Error())
		return
	}
	stagnationLimit, err := parseIntQuery(c, "stagnationLimit", 0)
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
		"effectiveGa":     gaParams,
		"defaultGa":       DefaultGAParams(),
		"effectiveTs":     tsParams,
		"defaultTs":       DefaultTSParams(),
		"stagnationLimit": stagnationLimit,
	})
	flusher.Flush()

	opts := GenerateHybridOptions{
		GA:              gaParams,
		TS:              tsParams,
		StagnationLimit: stagnationLimit,
		OnGAProgress: func(p GAProgressSnapshot) {
			if c.Request.Context().Err() != nil {
				return
			}
			if logToTerminal {
				logGAProgress("v3_ga_progress", p)
			}
			c.SSEvent("ga_progress", p)
			flusher.Flush()
		},
		OnGAComplete: func(r GAPhaseResult) {
			if c.Request.Context().Err() != nil {
				return
			}
			c.SSEvent("phase_change", gin.H{"phase": "ts", "gaResult": r})
			flusher.Flush()
		},
		OnTSProgress: func(p TSProgressSnapshot) {
			if c.Request.Context().Err() != nil {
				return
			}
			c.SSEvent("ts_progress", p)
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
		result.Meta.TotalElapsedMs = time.Since(start).Milliseconds()
		_ = resolveEntryNames(result.Entries)
		if logToTerminal {
			logScheduleSummary("v3_schedule_completed", result)
		}
		c.SSEvent("result", result)
		flusher.Flush()
	}
}

func GenerateV3ScheduleReadableHandler(c *gin.Context) {
	start := time.Now()

	gaParams, err := parseGAParams(c)
	if err != nil {
		response.Fail(c, http.StatusBadRequest, "invalid query parameters", err.Error())
		return
	}
	tsParams, err := parseTSParams(c)
	if err != nil {
		response.Fail(c, http.StatusBadRequest, "invalid query parameters", err.Error())
		return
	}
	stagnationLimit, err := parseIntQuery(c, "stagnationLimit", 0)
	if err != nil {
		response.Fail(c, http.StatusBadRequest, "invalid query parameters", err.Error())
		return
	}

	opts := GenerateHybridOptions{
		GA:              gaParams,
		TS:              tsParams,
		StagnationLimit: stagnationLimit,
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
	gaParams, err := parseGAParams(c)
	if err != nil {
		response.Fail(c, http.StatusBadRequest, "invalid query parameters", err.Error())
		return
	}
	tsParams, err := parseTSParams(c)
	if err != nil {
		response.Fail(c, http.StatusBadRequest, "invalid query parameters", err.Error())
		return
	}
	stagnationLimit, err := parseIntQuery(c, "stagnationLimit", 0)
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
		GA:              gaParams,
		TS:              tsParams,
		StagnationLimit: stagnationLimit,
	}

	result, err := GenerateV3MultiRun(opts, runs)
	if err != nil {
		response.Fail(c, http.StatusInternalServerError, "failed to run multi-run generation", err.Error())
		return
	}

	response.OK(c, result, "multi-run generation complete")
}

func GenerateV3MultiRunStreamHandler(c *gin.Context) {
	gaParams, err := parseGAParams(c)
	if err != nil {
		response.Fail(c, http.StatusBadRequest, "invalid query parameters", err.Error())
		return
	}
	tsParams, err := parseTSParams(c)
	if err != nil {
		response.Fail(c, http.StatusBadRequest, "invalid query parameters", err.Error())
		return
	}
	stagnationLimit, err := parseIntQuery(c, "stagnationLimit", 0)
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

	c.SSEvent("started", gin.H{
		"runs":            runs,
		"effectiveGa":     gaParams,
		"defaultGa":       DefaultGAParams(),
		"effectiveTs":     tsParams,
		"defaultTs":       DefaultTSParams(),
		"stagnationLimit": stagnationLimit,
	})
	flusher.Flush()

	baseOpts := GenerateHybridOptions{
		GA:              gaParams,
		TS:              tsParams,
		StagnationLimit: stagnationLimit,
	}

	totalStart := time.Now()
	results := make([]RunSummary, 0, runs)
	baseSeedGA := gaParams.Seed
	baseSeedTS := tsParams.Seed

	for i := 0; i < runs; i++ {
		if c.Request.Context().Err() != nil {
			return
		}

		runNum := i + 1
		runOpts := baseOpts
		if baseSeedGA != 0 {
			runOpts.GA.Seed = baseSeedGA + int64(i)
		}
		if baseSeedTS != 0 {
			runOpts.TS.Seed = baseSeedTS + int64(i)
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
			c.SSEvent("phase_change", gin.H{"run": runNum, "phase": "ts", "gaResult": r})
			flusher.Flush()
		}
		runOpts.OnTSProgress = func(p TSProgressSnapshot) {
			if c.Request.Context().Err() != nil {
				return
			}
			c.SSEvent("ts_progress", gin.H{"run": runNum, "progress": p})
			flusher.Flush()
		}

		result, genErr := GenerateV3Schedule(runOpts)
		if genErr != nil {
			if c.Request.Context().Err() == nil {
				c.SSEvent("error", gin.H{"run": runNum, "message": genErr.Error()})
				flusher.Flush()
			}
			return
		}

		summary := RunSummary{Run: runNum, Meta: result.Meta}
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

// ── Diagnostic handlers ───────────────────────────────────────────────────────

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

// ── Query parameter parsers ───────────────────────────────────────────────────

func parseGAParams(c *gin.Context) (GAParams, error) {
	def := DefaultGAParams()
	var err error
	p := def

	p.PopulationSize, err = parseIntQuery(c, "populationSize", def.PopulationSize)
	if err != nil {
		return p, err
	}
	p.Generations, err = parseIntQuery(c, "generations", def.Generations)
	if err != nil {
		return p, err
	}
	p.MutationRate, err = parseFloatQuery(c, "mutationRate", def.MutationRate)
	if err != nil {
		return p, err
	}
	p.EliteCount, err = parseIntQuery(c, "eliteCount", def.EliteCount)
	if err != nil {
		return p, err
	}
	p.TournamentSize, err = parseIntQuery(c, "tournamentSize", def.TournamentSize)
	if err != nil {
		return p, err
	}
	p.Seed, err = parseInt64Query(c, "seed", 0)
	if err != nil {
		return p, err
	}
	p.ProgressEvery, err = parseIntQuery(c, "progressEvery", def.ProgressEvery)
	if err != nil {
		return p, err
	}
	return p, nil
}

func parseTSParams(c *gin.Context) (TSParams, error) {
	def := DefaultTSParams()
	var err error
	p := def

	p.MaxIterations, err = parseIntQuery(c, "tsMaxIterations", def.MaxIterations)
	if err != nil {
		return p, err
	}
	p.TabuTenure, err = parseIntQuery(c, "tsTabuTenure", def.TabuTenure)
	if err != nil {
		return p, err
	}
	p.NeighborhoodSize, err = parseIntQuery(c, "tsNeighborhoodSize", def.NeighborhoodSize)
	if err != nil {
		return p, err
	}
	p.PerturbAfter, err = parseIntQuery(c, "tsPerturbAfter", def.PerturbAfter)
	if err != nil {
		return p, err
	}
	p.PerturbCount, err = parseIntQuery(c, "tsPerturbCount", def.PerturbCount)
	if err != nil {
		return p, err
	}
	p.Seed, err = parseInt64Query(c, "tsSeed", 0)
	if err != nil {
		return p, err
	}
	p.ProgressEvery, err = parseIntQuery(c, "tsProgressEvery", def.ProgressEvery)
	if err != nil {
		return p, err
	}
	return p, nil
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

// ── Logging helpers ───────────────────────────────────────────────────────────

func logGAProgress(event string, p GAProgressSnapshot) {
	logging.Logger.Info(event,
		zap.Int("generation", p.Generation),
		zap.Int("total_generations", p.TotalGenerations),
		zap.Float64("progress_percent", p.ProgressPercent),
		zap.Int("best_unplaced", p.BestUnplaced),
		zap.Int("best_violations", p.BestViolations),
		zap.Float64("avg_unplaced", p.AvgUnplaced),
		zap.Int("stagnant_gens", p.StagnantGens),
		zap.Int64("elapsed_ms", p.ElapsedMs),
	)
}

func logScheduleSummary(event string, result *ScheduleGenerationResult) {
	logging.Logger.Info(event,
		zap.Int("entries_generated", result.Meta.Result.EntriesGenerated),
		zap.Int("violations", result.Meta.Result.Violations),
		zap.Int("unplaced", result.Meta.Result.Unplaced),
		zap.Int("same_day_split", result.Meta.Result.SoftBreakdown.SameDaySplit),
		zap.Int64("elapsed_ms", result.Meta.TotalElapsedMs),
	)
}
