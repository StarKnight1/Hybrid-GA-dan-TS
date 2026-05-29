package schedule

import (
	"errors"
	"fmt"
	"sort"
	"strconv"
	"time"

	"smp_mater_dei_be/internal/algorithm"
	"smp_mater_dei_be/internal/platform/config"
	"smp_mater_dei_be/internal/subjects"
	teachingassignments "smp_mater_dei_be/internal/teaching_assignments"
)


type GenerateScheduleOptions struct {
	Params              GAParams
	CollectProgress     bool
	IncludeSeedWarnings bool
	OnProgress          func(GAProgressSnapshot)
}

type newScheduleBuildContext struct {
	blocks []algorithm.MatrixBlock
	index  map[uint][]algorithm.Gene
	pjokID uint
	input  InputStats
}

func NewGenerateScheduleOptions() GenerateScheduleOptions {
	return GenerateScheduleOptions{Params: DefaultGAParams()}
}

func DefaultGAParams() GAParams {
	cfg := algorithm.DefaultGAConfig()
	return GAParams{
		PopulationSize: cfg.PopulationSize,
		Generations:    cfg.Generations,
		MutationRate:   cfg.MutationRate,
		EliteCount:     cfg.EliteCount,
		TournamentSize: cfg.TournamentSize,
		ProgressEvery:  cfg.ProgressEvery,
	}
}

func GAParameterSpecs() []GAParameterSpec {
	return []GAParameterSpec{
		{
			Name:        "populationSize",
			Type:        "int",
			Default:     "100",
			Min:         "2",
			Max:         "unbounded",
			Description: "Number of chromosomes per generation. Larger values improve exploration but increase runtime.",
		},
		{
			Name:        "generations",
			Type:        "int",
			Default:     strconv.Itoa(DefaultGAParams().Generations),
			Min:         "1",
			Max:         "unbounded",
			Description: "Maximum number of evolutionary iterations.",
		},
		{
			Name:        "mutationRate",
			Type:        "float",
			Default:     strconv.FormatFloat(DefaultGAParams().MutationRate, 'f', -1, 64),
			Min:         "0",
			Max:         "1",
			Description: "Probability of mutating each unit slot in offspring. Keep low so crossover structure is preserved for ILS repair.",
		},
		{
			Name:        "eliteCount",
			Type:        "int",
			Default:     "5",
			Min:         "1",
			Max:         "populationSize-1",
			Description: "Top chromosomes carried unchanged to the next generation.",
		},
		{
			Name:        "tournamentSize",
			Type:        "int",
			Default:     strconv.Itoa(DefaultGAParams().TournamentSize),
			Min:         "1",
			Max:         "unbounded",
			Description: "Tournament selection size used to pick parents each generation.",
		},
		{
			Name:        "seed",
			Type:        "int64",
			Default:     "current unix-nano timestamp",
			Min:         "int64 min",
			Max:         "int64 max",
			Description: "Random seed for reproducible generation. Use the same seed to reproduce the same run.",
		},
		{
			Name:        "progressEvery",
			Type:        "int",
			Default:     strconv.Itoa(DefaultGAParams().ProgressEvery),
			Min:         "1",
			Max:         "unbounded",
			Description: "Emit progress every N generations.",
		},
	}
}

// ── Schedule generation context ──────────────────────────────────────────────

func buildNewScheduleContext() (*newScheduleBuildContext, error) {
	assignments, err := findActiveAssignments()
	if err != nil {
		return nil, err
	}

	pjokID, err := findPJOKSubjectID()
	if err != nil {
		return nil, err
	}

	blocks, err := algorithm.GenerateMatrixBlocks(assignments, pjokID)
	if err != nil {
		return nil, err
	}

	index := algorithm.BuildCandidateIndex(blocks, pjokID, nil)
	logBlockDiagnostics(blocks, index)

	classSet := make(map[uint]struct{}, len(blocks))
	teacherSet := make(map[uint]struct{}, len(blocks))
	for _, b := range blocks {
		classSet[b.ClassID] = struct{}{}
		if b.TeacherID != nil {
			teacherSet[*b.TeacherID] = struct{}{}
		}
	}

	return &newScheduleBuildContext{
		blocks: blocks,
		index:  index,
		pjokID: pjokID,
		input: InputStats{
			ActiveAssignments: len(assignments),
			Blocks:            len(blocks),
			Units:             len(blocks),
			ActiveClasses:     len(classSet),
			Teachers:          len(teacherSet),
		},
	}, nil
}

func buildEntriesFromMatrix(blocks []algorithm.MatrixBlock, matrix *algorithm.ScheduleMatrix, daySlots algorithm.DaySlots) []ScheduleEntry {
	if daySlots == nil {
		daySlots = algorithm.GenerateSlots()
	}

	entries := make([]ScheduleEntry, 0, len(blocks))
	for _, block := range blocks {
		placement, ok := matrix.Placement(block.ID)
		if !ok {
			continue
		}
		for offset := 0; offset < placement.Duration; offset++ {
			start, end := matrixSlotTimeRange(placement.Day, placement.StartSlot+offset, daySlots)
			if start == "" {
				continue
			}
			entries = append(entries, ScheduleEntry{
				TeacherID: block.TeacherID,
				SubjectID: block.SubjectID,
				ClassID:   block.ClassID,
				Day:       placement.Day,
				TimeStart: start,
				TimeEnd:   end,
			})
		}
	}

	sortScheduleEntries(entries)
	return entries
}

func matrixSlotTimeRange(day string, slotIndex int, daySlots algorithm.DaySlots) (string, string) {
	for _, slot := range daySlots[day] {
		if slot.Index == slotIndex {
			return slot.StartTime, slot.EndTime
		}
	}
	return "", ""
}

func toV2ProgressSnapshot(p algorithm.GAProgress, totalGenerations int) GAProgressSnapshot {
	percent := 0.0
	if totalGenerations > 0 {
		percent = float64(p.Generation) / float64(totalGenerations) * 100
		if percent > 100 {
			percent = 100
		}
	}
	return GAProgressSnapshot{
		Generation:       p.Generation,
		TotalGenerations: totalGenerations,
		ProgressPercent:  percent,
		BestUnplaced:     p.BestUnplaced,
		BestViolations:   p.BestSoftViolations,
		AvgFitness:       int(p.AvgUnplaced),
		ElapsedMs:        p.Elapsed.Milliseconds(),
	}
}

// ── V3 schedule generation (hybrid GA+TS) ────────────────────────────────────

func DefaultTSParams() TSParams {
	cfg := algorithm.DefaultTSConfig()
	return TSParams{
		TabuTenure:    cfg.TabuTenure,
		Iterations:    cfg.Iterations,
		ProgressEvery: cfg.ProgressEvery,
		PerturbCount:  cfg.PerturbCount,
		PerturbAfter:  cfg.PerturbAfter,
	}
}

func GenerateV3Schedule(opts GenerateHybridOptions) (*ScheduleGenerationResult, error) {
	gaParams := opts.GA
	if isZeroGAParams(gaParams) {
		gaParams = DefaultGAParams()
	}
	gaParams = ensureSeed(gaParams)
	if err := validateGAParams(gaParams); err != nil {
		return nil, err
	}

	tsParams := opts.TS
	if tsParams.Iterations == 0 {
		tsParams = DefaultTSParams()
	}
	if tsParams.Seed == 0 {
		tsParams.Seed = time.Now().UnixNano()
	}

	ctx, err := buildNewScheduleContext()
	if err != nil {
		return nil, err
	}

	gaCfg := algorithm.DefaultGAConfig()
	gaCfg.PopulationSize = gaParams.PopulationSize
	gaCfg.Generations = gaParams.Generations
	gaCfg.MutationRate = gaParams.MutationRate
	gaCfg.EliteCount = gaParams.EliteCount
	gaCfg.TournamentSize = gaParams.TournamentSize
	gaCfg.Seed = gaParams.Seed
	gaCfg.ProgressEvery = gaParams.ProgressEvery
	gaCfg.StagnationLimit = opts.StagnationLimit
	gaCfg.PJOKSubjectID = ctx.pjokID
	gaCfg.OnProgress = func(p algorithm.GAProgress) {
		if opts.OnGAProgress != nil {
			snap := toV2ProgressSnapshot(p, gaParams.Generations)
			snap.StagnantGens = p.StagnantGens
			opts.OnGAProgress(snap)
		}
	}

	tsCfg := algorithm.DefaultTSConfig()
	tsCfg.TabuTenure = tsParams.TabuTenure
	tsCfg.Iterations = tsParams.Iterations
	tsCfg.ProgressEvery = tsParams.ProgressEvery
	tsCfg.Seed = tsParams.Seed
	tsCfg.PerturbCount = tsParams.PerturbCount
	tsCfg.PerturbAfter = tsParams.PerturbAfter
	tsCfg.PJOKSubjectID = ctx.pjokID
	tsCfg.OnProgress = func(p algorithm.TSProgress) {
		if opts.OnTSProgress != nil {
			opts.OnTSProgress(toTSProgressSnapshot(p, tsParams.Iterations))
		}
	}

	hybridCfg := algorithm.HybridConfig{
		GA:                gaCfg,
		TS:                tsCfg,
		Restarts:          opts.Restarts,
		LoopUntilFeasible: opts.LoopUntilFeasible,
		MaxLoops:          opts.MaxLoops,
		OnGAComplete: func(r algorithm.GAResult) {
			if opts.OnGAComplete != nil {
				opts.OnGAComplete(GAPhaseResult{
					Unplaced:       r.Unplaced,
					SoftViolations: r.SoftViolations,
					Generations:    r.Generations,
					ElapsedMs:      r.Elapsed.Milliseconds(),
				})
			}
		},
	}
	hybridResult := algorithm.RunHybrid(ctx.blocks, ctx.index, nil, hybridCfg)

	entries := buildEntriesFromMatrix(ctx.blocks, hybridResult.Matrix, nil)
	hybridSoftBd := algorithm.BreakdownSoftViolations(hybridResult.Matrix, ctx.blocks, ctx.pjokID)

	defaultTS := DefaultTSParams()
	generationResult := &ScheduleGenerationResult{
		Entries: entries,
		Meta: ScheduleMeta{
			Input:          ctx.input,
			DefaultGA:      DefaultGAParams(),
			EffectiveGA:    gaParams,
			DefaultTS:      &defaultTS,
			EffectiveTS:    &tsParams,
			TotalElapsedMs: hybridResult.Elapsed.Milliseconds(),
			LoopCount:      hybridResult.Loops,
			Result: ResultStats{
				EntriesGenerated: len(entries),
				BestFitness:      hybridResult.Unplaced,
				Violations:       hybridResult.SoftViolations,
				Unplaced:         hybridResult.Unplaced,
				SoftBreakdown: SoftBreakdown{
					SameDaySplit:        hybridSoftBd.SameDaySplit,
					SameDaySplitGrouped: hybridSoftBd.SameDaySplitGrouped,
					PJOKAfterDeadline:   hybridSoftBd.PJOKAfterDeadline,
				},
			},
		},
	}

	return generationResult, nil
}

// GenerateV3ScheduleReadable runs the hybrid GA+SA and returns the schedule with
func toTSProgressSnapshot(p algorithm.TSProgress, totalIterations int) TSProgressSnapshot {
	percent := 0.0
	if totalIterations > 0 {
		percent = float64(p.Iteration) / float64(totalIterations) * 100
		if percent > 100 {
			percent = 100
		}
	}
	return TSProgressSnapshot{
		Phase:                 "ts",
		Iteration:             p.Iteration,
		TotalIterations:       totalIterations,
		ProgressPercent:       percent,
		TabuListSize:          p.TabuListSize,
		CurrentUnplaced:       p.CurrentUnplaced,
		CurrentSoftViolations: p.CurrentSoftViolations,
		BestUnplaced:          p.BestUnplaced,
		BestSoftViolations:    p.BestSoftViolations,
		ElapsedMs:             p.Elapsed.Milliseconds(),
	}
}

func findActiveAssignments() ([]teachingassignments.TeachingAssignment, error) {
	var assignments []teachingassignments.TeachingAssignment
	err := config.DB.
		Table("teaching_assignments AS ta").
		Select("ta.*").
		Joins("JOIN classes c ON c.id = ta.class_id").
		Where("ta.deleted_at IS NULL").
		Where("c.deleted_at IS NULL").
		Where("c.is_active = ?", true).
		Find(&assignments).Error
	return assignments, err
}

func findPJOKSubjectID() (uint, error) {
	var s subjects.Subject
	err := config.DB.Where("name = ?", "PJOK").First(&s).Error
	if err != nil {
		return 0, err
	}
	return s.ID, nil
}

// ── Utilities ─────────────────────────────────────────────────────────────────

func parseClock(s string) time.Time {
	t, _ := time.Parse("15:04", s)
	return t
}

func sortScheduleEntries(entries []ScheduleEntry) {
	sort.Slice(entries, func(i, j int) bool {
		di := dayOrder(entries[i].Day)
		dj := dayOrder(entries[j].Day)
		if di != dj {
			return di < dj
		}

		ti := parseClock(entries[i].TimeStart)
		tj := parseClock(entries[j].TimeStart)
		if !ti.Equal(tj) {
			return ti.Before(tj)
		}

		if entries[i].ClassID != entries[j].ClassID {
			return entries[i].ClassID < entries[j].ClassID
		}

		if entries[i].SubjectID != entries[j].SubjectID {
			return entries[i].SubjectID < entries[j].SubjectID
		}

		if entries[i].TeacherID == nil && entries[j].TeacherID != nil {
			return true
		}
		if entries[i].TeacherID != nil && entries[j].TeacherID == nil {
			return false
		}
		if entries[i].TeacherID == nil && entries[j].TeacherID == nil {
			return false
		}

		return *entries[i].TeacherID < *entries[j].TeacherID
	})
}

func dayOrder(day string) int {
	switch day {
	case "monday":
		return 1
	case "tuesday":
		return 2
	case "wednesday":
		return 3
	case "thursday":
		return 4
	case "friday":
		return 5
	default:
		return 99
	}
}


// GenerateV3MultiRun runs the hybrid GA+SA sequentially for the given number of runs
func GenerateV3MultiRun(runs int, opts GenerateHybridOptions) (*MultiRunResult, error) {
	if runs <= 0 {
		runs = 1
	}

	baseSeedGA := opts.GA.Seed
	baseSeedTS := opts.TS.Seed
	totalStart := time.Now()
	results := make([]RunSummary, 0, runs)

	for i := 0; i < runs; i++ {
		runOpts := opts
		if baseSeedGA != 0 {
			runOpts.GA.Seed = baseSeedGA + int64(i)
		}
		if baseSeedTS != 0 {
			runOpts.TS.Seed = baseSeedTS + int64(i)
		}

		runStart := time.Now()
		result, err := GenerateV3Schedule(runOpts)
		if err != nil {
			return nil, err
		}
		result.Meta.TotalElapsedMs = time.Since(runStart).Milliseconds()

		results = append(results, RunSummary{
			Run:  i + 1,
			Meta: result.Meta,
		})
	}

	return &MultiRunResult{
		Runs:           runs,
		TotalElapsedMs: time.Since(totalStart).Milliseconds(),
		Results:        results,
	}, nil
}

func logBlockDiagnostics(blocks []algorithm.MatrixBlock, index map[uint][]algorithm.Gene) {
	sbpCount := 0
	for _, b := range blocks {
		if b.GroupKey != nil {
			sbpCount++
		}
	}

	minCandidates := int(^uint(0) >> 1)
	maxCandidates := 0
	totalCandidates := 0
	for _, b := range blocks {
		n := len(index[b.ID])
		totalCandidates += n
		if n < minCandidates {
			minCandidates = n
		}
		if n > maxCandidates {
			maxCandidates = n
		}
	}
	avgCandidates := 0.0
	if len(blocks) > 0 {
		avgCandidates = float64(totalCandidates) / float64(len(blocks))
		if minCandidates == int(^uint(0)>>1) {
			minCandidates = 0
		}
	}

	lowCount := 0
	for _, b := range blocks {
		if len(index[b.ID]) <= 3 {
			lowCount++
		}
	}

	fmt.Printf("[block diag] total=%d sbp=%d non-sbp=%d | candidates: min=%d max=%d avg=%.1f | blocks with ≤3 candidates=%d\n",
		len(blocks), sbpCount, len(blocks)-sbpCount,
		minCandidates, maxCandidates, avgCandidates, lowCount)
}

func isZeroGAParams(p GAParams) bool {
	return p.PopulationSize == 0 &&
		p.Generations == 0 &&
		p.MutationRate == 0 &&
		p.EliteCount == 0 &&
		p.Seed == 0 &&
		p.ProgressEvery == 0
}

func ensureSeed(p GAParams) GAParams {
	if p.Seed == 0 {
		p.Seed = time.Now().UnixNano()
	}
	return p
}

func validateGAParams(p GAParams) error {
	if p.PopulationSize < 2 {
		return errors.New("populationSize must be >= 2")
	}
	if p.Generations < 1 {
		return errors.New("generations must be >= 1")
	}
	if p.MutationRate < 0 || p.MutationRate > 1 {
		return errors.New("mutationRate must be between 0 and 1")
	}
	if p.EliteCount < 1 || p.EliteCount >= p.PopulationSize {
		return errors.New("eliteCount must be >= 1 and < populationSize")
	}
	if p.TournamentSize < 1 {
		return errors.New("tournamentSize must be >= 1")
	}
	if p.ProgressEvery < 1 {
		return errors.New("progressEvery must be >= 1")
	}
	return nil
}
