package schedule

import (
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"smp_mater_dei_be/internal/algorithm"
	legacyalgo "smp_mater_dei_be/internal/algorithm/legacy"
	"smp_mater_dei_be/internal/classes"
	"smp_mater_dei_be/internal/platform/config"
	"smp_mater_dei_be/internal/subjects"
	"smp_mater_dei_be/internal/teachers"
	teachingassignments "smp_mater_dei_be/internal/teaching_assignments"

	"github.com/google/uuid"
)

type timeRange struct {
	start string
	end   string
}

type GenerateScheduleOptions struct {
	Params              GAParams
	CollectProgress     bool
	IncludeSeedWarnings bool
	OnProgress          func(GAProgressSnapshot)
}

type scheduleBuildContext struct {
	assignments []teachingassignments.TeachingAssignment
	blocks      []legacyalgo.Block
	validSlots  map[uuid.UUID][]legacyalgo.ValidSlot
	units       []legacyalgo.PlacementUnit
	teacherMap  map[string]uuid.UUID
	classMap    map[string]uuid.UUID
	input       InputStats
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
			Name:        "saIterations",
			Type:        "int",
			Default:     strconv.Itoa(DefaultGAParams().SAIterations),
			Min:         "0",
			Max:         "unbounded",
			Description: "SA+ILS passes applied to each child after crossover. SA accepts improving and probabilistic worsening moves; ILS escape perturbs violating units when the temperature cools and no progress is made.",
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

func GenerateSchedule() ([]ScheduleEntry, error) {
	opts := NewGenerateScheduleOptions()
	result, err := GenerateScheduleWithOptions(opts)
	if err != nil {
		return nil, err
	}
	return result.Entries, nil
}

func GenerateScheduleWithOptions(opts GenerateScheduleOptions) (*ScheduleGenerationResult, error) {
	if isZeroGAParams(opts.Params) {
		opts.Params = DefaultGAParams()
	}

	opts.Params = ensureSeed(opts.Params)
	if err := validateGAParams(opts.Params); err != nil {
		return nil, err
	}

	ctx, err := buildScheduleContext()
	if err != nil {
		return nil, err
	}

	var seedWarnings []string
	if opts.IncludeSeedWarnings {
		_, seedWarnings = legacyalgo.BuildSeedChromosome(ctx.units, ctx.validSlots, ctx.teacherMap, ctx.classMap)
	}

	progress := make([]GAProgressSnapshot, 0, opts.Params.Generations/opts.Params.ProgressEvery+2)

	cfg := legacyalgo.DefaultLegacyGAConfig()
	cfg.PopulationSize = opts.Params.PopulationSize
	cfg.Generations = opts.Params.Generations
	cfg.MutationRate = opts.Params.MutationRate
	cfg.EliteCount = opts.Params.EliteCount
	cfg.SAIterations = opts.Params.SAIterations
	cfg.Seed = opts.Params.Seed
	cfg.ProgressEvery = opts.Params.ProgressEvery
	cfg.OnProgress = func(p legacyalgo.LegacyGAProgress) {
		snapshot := toProgressSnapshot(p)
		if opts.CollectProgress {
			progress = append(progress, snapshot)
		}
		if opts.OnProgress != nil {
			opts.OnProgress(snapshot)
		}
	}

	best, err := legacyalgo.LegacyRunGA(ctx.units, ctx.validSlots, cfg)
	if err != nil {
		return nil, err
	}

	entries := buildEntriesFromChromosome(best, ctx.units)

	result := &ScheduleGenerationResult{
		Entries: entries,
		Meta: ScheduleMeta{
			Input:               ctx.input,
			DefaultGA:           DefaultGAParams(),
			EffectiveGA:         opts.Params,
			SeedWarningsChecked: opts.IncludeSeedWarnings,
			Result: ResultStats{
				EntriesGenerated: len(entries),
				BestFitness:      best.Fitness,
				Violations:       best.ViolationCount,
				Unplaced:         best.UnplacedCount,
				Breakdown:        toScheduleBreakdown(best.Breakdown),
			},
			SeedWarnings: seedWarnings,
		},
	}

	if opts.CollectProgress {
		result.Progress = progress
	}

	return result, nil
}

func GenerateScheduleAndCompareWithReal(opts GenerateScheduleOptions) (*ScheduleGenerateAndCompareResult, error) {
	generated, err := GenerateScheduleWithOptions(opts)
	if err != nil {
		return nil, err
	}

	comparison := ScheduleDiffResult{
		Checked:        false,
		Reason:         "comparison skipped because violations are not zero",
		IsSame:         false,
		GeneratedCount: len(generated.Entries),
	}

	if generated.Meta.Result.Violations == 0 {
		realBaseline, err := EvaluateRealScheduleBaseline()
		if err != nil {
			return nil, err
		}
		comparison = compareScheduleEntries(generated.Entries, realBaseline.Entries)
	}

	return &ScheduleGenerateAndCompareResult{
		Generation: generated,
		Comparison: comparison,
	}, nil
}

func EvaluateRealScheduleBaseline() (*RealScheduleValidationResult, error) {
	ctx, err := buildScheduleContext()
	if err != nil {
		return nil, err
	}

	pjokID, err := findPJOKSubjectID()
	if err != nil {
		return nil, err
	}

	seedChromosome, seedWarnings := legacyalgo.BuildSeedChromosome(ctx.units, ctx.validSlots, ctx.teacherMap, ctx.classMap)
	legacyalgo.EvaluateChromosome(seedChromosome, ctx.units)

	entries := buildEntriesFromChromosome(seedChromosome, ctx.units)
	softBd := computeSoftBreakdownFromEntries(entries, pjokID)
	softTotal := softBd.SameDaySplit + softBd.PJOKAfterDeadline*3

	result := &RealScheduleValidationResult{Entries: entries}
	result.Meta.Input = ctx.input
	result.Meta.Result = ResultStats{
		EntriesGenerated: len(entries),
		BestFitness:      seedChromosome.Fitness,
		Violations:       softTotal,
		Unplaced:         seedChromosome.UnplacedCount,
		Breakdown:        toScheduleBreakdown(seedChromosome.Breakdown),
		SoftBreakdown: SoftBreakdown{
			SameDaySplit:      softBd.SameDaySplit,
			PJOKAfterDeadline: softBd.PJOKAfterDeadline,
		},
	}
	result.Meta.SeedWarnings = seedWarnings
	result.Meta.IsFeasible = seedChromosome.ViolationCount == 0 && seedChromosome.UnplacedCount == 0

	return result, nil
}

// computeSoftBreakdownFromEntries evaluates soft violations from a ScheduleEntry slice.
// Mirrors CountSoftViolations logic but operates on time-string entries instead of a ScheduleMatrix.
//
// Block boundaries are detected by slot index gaps, not time gaps. This is critical because
// the school timetable has a morning break (slots 2→3: 09:10→09:30) and a lunch break
// (slots 5→6: 11:30→11:45) where consecutive-slot blocks produce non-consecutive times.
// Using time-gap detection would incorrectly split those blocks and inflate SameDaySplit.
func computeSoftBreakdownFromEntries(entries []ScheduleEntry, pjokSubjectID uuid.UUID) SoftBreakdown {
	var bd SoftBreakdown
	daySlots := algorithm.GenerateSlots()

	// end-time lookup: day → slotIndex → endTime (for PJOK deadline check)
	slotEndTime := make(map[string]map[int]string, len(daySlots))
	for day, slots := range daySlots {
		m := make(map[int]string, len(slots))
		for _, s := range slots {
			m[s.Index] = s.EndTime
		}
		slotEndTime[day] = m
	}

	type groupKey struct {
		classID   uuid.UUID
		subjectID uuid.UUID
		day       string
	}
	type slotEntry struct {
		idx int
		e   ScheduleEntry
	}

	byGroup := make(map[groupKey][]slotEntry, len(entries))
	for _, e := range entries {
		idx, ok := algorithm.MatrixSlotIndexFromTimeStart(e.Day, e.TimeStart, daySlots)
		if !ok {
			continue
		}
		k := groupKey{e.ClassID, e.SubjectID, e.Day}
		byGroup[k] = append(byGroup[k], slotEntry{idx, e})
	}
	for k := range byGroup {
		g := byGroup[k]
		sort.Slice(g, func(i, j int) bool { return g[i].idx < g[j].idx })
		byGroup[k] = g
	}

	// Count distinct blocks per (classID, subjectID, day).
	// A new block starts whenever there is a gap in slot indices.
	type subjectKey struct {
		classID   uuid.UUID
		subjectID uuid.UUID
	}
	dayBlocks := make(map[subjectKey]map[string]int)
	for k, es := range byGroup {
		sk := subjectKey{k.classID, k.subjectID}
		if dayBlocks[sk] == nil {
			dayBlocks[sk] = make(map[string]int)
		}
		count := 1
		for i := 1; i < len(es); i++ {
			if es[i].idx != es[i-1].idx+1 {
				count++
			}
		}
		dayBlocks[sk][k.day] += count
	}

	// Same-day split (weight 1, PJOK excluded)
	for sk, dm := range dayBlocks {
		if sk.subjectID == pjokSubjectID {
			continue
		}
		for _, n := range dm {
			if n > 1 {
				bd.SameDaySplit += n - 1
			}
		}
	}

	// PJOK after deadline (weight 3): pairs of consecutive slot indices = one 2-JP block.
	// Check the end time of the second slot against the 10:50 deadline.
	for k, es := range byGroup {
		if k.subjectID != pjokSubjectID {
			continue
		}
		for i := 0; i < len(es)-1; i++ {
			if es[i+1].idx == es[i].idx+1 {
				if endTime := slotEndTime[k.day][es[i+1].idx]; endTime > "10:50" {
					bd.PJOKAfterDeadline++
				}
				i++ // consume the pair
			}
		}
	}

	return bd
}

// DiagnoseSA runs SA-only trials (no GA) to verify that simulated annealing
// repair alone can reach 0 violations from a greedy starting chromosome.
//
// Each trial uses seed+i so results are reproducible yet independent.
// Default: 5 trials, 2000 SA iterations each.
func DiagnoseSA(trials, saIterations int, baseSeed int64) (*SADiagnosticResult, error) {
	if trials <= 0 {
		trials = 5
	}
	if saIterations <= 0 {
		saIterations = 2000
	}

	ctx, err := buildScheduleContext()
	if err != nil {
		return nil, err
	}

	result := &SADiagnosticResult{Input: ctx.input}
	result.Summary.TrialCount = trials
	result.Summary.SAIterationsPerTrial = saIterations
	result.Summary.MinFinalViolations = int(^uint(0) >> 1)
	result.Summary.MinFinalUnplaced = int(^uint(0) >> 1)

	totalGreedy := 0
	totalGreedyUnplaced := 0
	totalFinal := 0
	totalFinalUnplaced := 0

	for i := 0; i < trials; i++ {
		seed := baseSeed + int64(i)
		t := legacyalgo.DiagnoseGreedySA(ctx.units, ctx.validSlots, saIterations, seed)

		tr := SATrialResult{
			Trial:            i + 1,
			Seed:             seed,
			GreedyViolations: t.GreedyViolations,
			GreedyUnplaced:   t.GreedyUnplaced,
			GreedyBreakdown: GABreakdown{
				ClassConflicts:    t.GreedyBreakdown.ClassConflicts,
				TeacherConflicts:  t.GreedyBreakdown.TeacherConflicts,
				SiblingViolations: t.GreedyBreakdown.SiblingViolations,
			},
			FinalViolations: t.FinalViolations,
			FinalUnplaced:   t.FinalUnplaced,
			FinalBreakdown: GABreakdown{
				ClassConflicts:    t.FinalBreakdown.ClassConflicts,
				TeacherConflicts:  t.FinalBreakdown.TeacherConflicts,
				SiblingViolations: t.FinalBreakdown.SiblingViolations,
			},
			ReachedZero: t.ReachedZero,
		}
		result.Trials = append(result.Trials, tr)

		if t.ReachedZero {
			result.Summary.SuccessCount++
		}
		if t.FinalViolations < result.Summary.MinFinalViolations {
			result.Summary.MinFinalViolations = t.FinalViolations
		}
		if t.FinalUnplaced < result.Summary.MinFinalUnplaced {
			result.Summary.MinFinalUnplaced = t.FinalUnplaced
		}
		totalGreedy += t.GreedyViolations
		totalGreedyUnplaced += t.GreedyUnplaced
		totalFinal += t.FinalViolations
		totalFinalUnplaced += t.FinalUnplaced
	}

	result.Summary.SuccessRate = float64(result.Summary.SuccessCount) / float64(trials)
	result.Summary.MeanGreedyViolations = float64(totalGreedy) / float64(trials)
	result.Summary.MeanGreedyUnplaced = float64(totalGreedyUnplaced) / float64(trials)
	result.Summary.MeanFinalViolations = float64(totalFinal) / float64(trials)
	result.Summary.MeanFinalUnplaced = float64(totalFinalUnplaced) / float64(trials)

	return result, nil
}

// ── V2 schedule generation (new matrix-based GA) ─────────────────────────────

type newScheduleBuildContext struct {
	blocks []algorithm.MatrixBlock
	index  map[uuid.UUID][]algorithm.Gene
	pjokID uuid.UUID
	input  InputStats
}

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

	classSet := make(map[uuid.UUID]struct{}, len(blocks))
	teacherSet := make(map[uuid.UUID]struct{}, len(blocks))
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

func GenerateV2Schedule(opts GenerateScheduleOptions) (*ScheduleGenerateAndCompareResult, error) {
	if isZeroGAParams(opts.Params) {
		opts.Params = DefaultGAParams()
	}
	opts.Params = ensureSeed(opts.Params)
	if err := validateGAParams(opts.Params); err != nil {
		return nil, err
	}

	ctx, err := buildNewScheduleContext()
	if err != nil {
		return nil, err
	}

	cfg := algorithm.DefaultGAConfig()
	cfg.PopulationSize = opts.Params.PopulationSize
	cfg.Generations = opts.Params.Generations
	cfg.MutationRate = opts.Params.MutationRate
	cfg.EliteCount = opts.Params.EliteCount
	cfg.TournamentSize = opts.Params.TournamentSize
	cfg.Seed = opts.Params.Seed
	cfg.ProgressEvery = opts.Params.ProgressEvery
	cfg.PJOKSubjectID = ctx.pjokID
	cfg.OnProgress = func(p algorithm.GAProgress) {
		if opts.OnProgress != nil {
			opts.OnProgress(toV2ProgressSnapshot(p, opts.Params.Generations))
		}
	}

	gaResult := algorithm.RunGA(ctx.blocks, ctx.index, nil, cfg)
	entries := buildEntriesFromMatrix(ctx.blocks, gaResult.Matrix, nil)
	gaSoftBd := algorithm.BreakdownSoftViolations(gaResult.Matrix, ctx.blocks, ctx.pjokID)

	generationResult := &ScheduleGenerationResult{
		Entries: entries,
		Meta: ScheduleMeta{
			Input:       ctx.input,
			DefaultGA:   DefaultGAParams(),
			EffectiveGA: opts.Params,
			Result: ResultStats{
				EntriesGenerated: len(entries),
				BestFitness:      gaResult.Unplaced,
				Violations:       gaResult.SoftViolations,
				Unplaced:         gaResult.Unplaced,
				SoftBreakdown: SoftBreakdown{
					SameDaySplit:        gaSoftBd.SameDaySplit,
					SameDaySplitGrouped: gaSoftBd.SameDaySplitGrouped,
					PJOKAfterDeadline:   gaSoftBd.PJOKAfterDeadline,
				},
			},
		},
	}

	comparison := ScheduleDiffResult{
		Checked:        false,
		Reason:         "comparison skipped: schedule has unplaced blocks",
		GeneratedCount: len(entries),
	}

	if gaResult.Unplaced == 0 {
		realBaseline, err := EvaluateRealScheduleBaseline()
		if err != nil {
			return nil, err
		}
		comparison = compareScheduleEntries(entries, realBaseline.Entries)
	}

	return &ScheduleGenerateAndCompareResult{
		Generation: generationResult,
		Comparison: comparison,
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

// ── V3 schedule generation (hybrid GA+SA) ────────────────────────────────────

func DefaultSAParams() SAParams {
	cfg := algorithm.DefaultSAConfig()
	return SAParams{
		InitialTemperature: cfg.InitialTemperature,
		CoolingRate:        cfg.CoolingRate,
		Iterations:         cfg.Iterations,
		ProgressEvery:      cfg.ProgressEvery,
		PerturbCount:       cfg.PerturbCount,
		PerturbAfter:       cfg.PerturbAfter,
	}
}

func GenerateV3Schedule(opts GenerateHybridOptions) (*ScheduleGenerateAndCompareResult, error) {
	gaParams := opts.GA
	if isZeroGAParams(gaParams) {
		gaParams = DefaultGAParams()
	}
	gaParams = ensureSeed(gaParams)
	if err := validateGAParams(gaParams); err != nil {
		return nil, err
	}

	saParams := opts.SA
	if saParams.Iterations == 0 {
		saParams = DefaultSAParams()
	}
	if saParams.Seed == 0 {
		saParams.Seed = time.Now().UnixNano()
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

	saCfg := algorithm.DefaultSAConfig()
	saCfg.InitialTemperature = saParams.InitialTemperature
	saCfg.CoolingRate = saParams.CoolingRate
	saCfg.Iterations = saParams.Iterations
	saCfg.ProgressEvery = saParams.ProgressEvery
	saCfg.Seed = saParams.Seed
	saCfg.PerturbCount = saParams.PerturbCount
	saCfg.PerturbAfter = saParams.PerturbAfter
	saCfg.PJOKSubjectID = ctx.pjokID
	saCfg.OnProgress = func(p algorithm.SAProgress) {
		if opts.OnSAProgress != nil {
			opts.OnSAProgress(toSAProgressSnapshot(p, saParams.Iterations))
		}
	}

	hybridCfg := algorithm.HybridConfig{
		GA:                gaCfg,
		SA:                saCfg,
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

	defaultSA := DefaultSAParams()
	generationResult := &ScheduleGenerationResult{
		Entries: entries,
		Meta: ScheduleMeta{
			Input:          ctx.input,
			DefaultGA:      DefaultGAParams(),
			EffectiveGA:    gaParams,
			DefaultSA:      &defaultSA,
			EffectiveSA:    &saParams,
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

	comparison := ScheduleDiffResult{
		Checked:        false,
		Reason:         "comparison skipped because schedule still has unplaced blocks",
		GeneratedCount: len(entries),
	}

	if hybridResult.Unplaced == 0 {
		realBaseline, err := EvaluateRealScheduleBaseline()
		if err != nil {
			return nil, err
		}
		comparison = compareScheduleEntries(entries, realBaseline.Entries)
	}

	return &ScheduleGenerateAndCompareResult{
		Generation: generationResult,
		Comparison: comparison,
	}, nil
}

// GenerateV3ScheduleReadable runs the hybrid GA+SA and returns the schedule with
// human-readable teacher/subject/class names instead of UUIDs.
// Entries are sorted by class name → day (Monday first) → time.
func GenerateV3ScheduleReadable(opts GenerateHybridOptions) (*ReadableScheduleResult, error) {
	result, err := GenerateV3Schedule(opts)
	if err != nil {
		return nil, err
	}

	var teacherRows []teachers.Teacher
	if err := config.DB.Find(&teacherRows).Error; err != nil {
		return nil, err
	}
	teacherNames := make(map[uuid.UUID]string, len(teacherRows))
	for _, t := range teacherRows {
		teacherNames[t.ID] = t.FullName
	}

	var subjectRows []subjects.Subject
	if err := config.DB.Find(&subjectRows).Error; err != nil {
		return nil, err
	}
	subjectNames := make(map[uuid.UUID]string, len(subjectRows))
	for _, s := range subjectRows {
		subjectNames[s.ID] = s.Name
	}

	var classRows []classes.Class
	if err := config.DB.Where("is_active = ?", true).Find(&classRows).Error; err != nil {
		return nil, err
	}
	classNames := make(map[uuid.UUID]string, len(classRows))
	for _, cl := range classRows {
		classNames[cl.ID] = cl.Name
	}

	entries := result.Generation.Entries
	readable := make([]ReadableScheduleEntry, 0, len(entries))
	for _, e := range entries {
		re := ReadableScheduleEntry{
			ClassName:   classNames[e.ClassID],
			SubjectName: subjectNames[e.SubjectID],
			Day:         e.Day,
			TimeStart:   e.TimeStart,
			TimeEnd:     e.TimeEnd,
		}
		if e.TeacherID != nil {
			re.TeacherName = teacherNames[*e.TeacherID]
		}
		readable = append(readable, re)
	}

	sort.Slice(readable, func(i, j int) bool {
		if readable[i].ClassName != readable[j].ClassName {
			return readable[i].ClassName < readable[j].ClassName
		}
		di := algorithm.MatrixDayIndex(readable[i].Day)
		dj := algorithm.MatrixDayIndex(readable[j].Day)
		if di != dj {
			return di < dj
		}
		return readable[i].TimeStart < readable[j].TimeStart
	})

	return &ReadableScheduleResult{
		Entries: readable,
		Meta:    result.Generation.Meta,
	}, nil
}

func toSAProgressSnapshot(p algorithm.SAProgress, totalIterations int) SAProgressSnapshot {
	percent := 0.0
	if totalIterations > 0 {
		percent = float64(p.Iteration) / float64(totalIterations) * 100
		if percent > 100 {
			percent = 100
		}
	}
	return SAProgressSnapshot{
		Phase:                 "sa",
		Iteration:             p.Iteration,
		TotalIterations:       totalIterations,
		ProgressPercent:       percent,
		Temperature:           p.Temperature,
		CurrentUnplaced:       p.CurrentUnplaced,
		CurrentSoftViolations: p.CurrentSoftViolations,
		BestUnplaced:          p.BestUnplaced,
		BestSoftViolations:    p.BestSoftViolations,
		ElapsedMs:             p.Elapsed.Milliseconds(),
	}
}

func buildScheduleContext() (*scheduleBuildContext, error) {
	assignments, err := findActiveAssignments()
	if err != nil {
		return nil, err
	}

	pjokID, err := findPJOKSubjectID()
	if err != nil {
		return nil, err
	}

	blocks := legacyalgo.GenerateBlocks(assignments, pjokID)
	validSlots := legacyalgo.GenerateValidSlots(blocks)
	units := legacyalgo.GroupParallelBlocks(blocks)

	teacherMap, err := buildTeacherNumberMap()
	if err != nil {
		return nil, err
	}

	classMap, err := buildActiveClassNameMap()
	if err != nil {
		return nil, err
	}

	return &scheduleBuildContext{
		assignments: assignments,
		blocks:      blocks,
		validSlots:  validSlots,
		units:       units,
		teacherMap:  teacherMap,
		classMap:    classMap,
		input: InputStats{
			ActiveAssignments: len(assignments),
			Blocks:            len(blocks),
			Units:             len(units),
			ActiveClasses:     len(classMap),
			Teachers:          len(teacherMap),
		},
	}, nil
}

func buildEntriesFromChromosome(ch *legacyalgo.GAChromosome, units []legacyalgo.PlacementUnit) []ScheduleEntry {
	entries := make([]ScheduleEntry, 0, len(units)*2)

	n := len(units)
	if len(ch.UnitSlots) < n {
		n = len(ch.UnitSlots)
	}

	for i := 0; i < n; i++ {
		unit := units[i]
		slot := ch.UnitSlots[i]
		if slot.Day == "" {
			continue
		}

		for _, b := range blocksFromUnit(unit) {
			ranges := splitToSingleJP(slot.Day, slot.SlotIndex, b.JP)
			for _, r := range ranges {
				entries = append(entries, ScheduleEntry{
					TeacherID: b.TeacherID,
					SubjectID: b.SubjectID,
					ClassID:   b.ClassID,
					Day:       slot.Day,
					TimeStart: r.start,
					TimeEnd:   r.end,
				})
			}
		}
	}

	sortScheduleEntries(entries)
	return entries
}

func isZeroGAParams(p GAParams) bool {
	return p.PopulationSize == 0 &&
		p.Generations == 0 &&
		p.MutationRate == 0 &&
		p.EliteCount == 0 &&
		p.SAIterations == 0 &&
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
	if p.SAIterations < 0 {
		return errors.New("saIterations must be >= 0")
	}
	if p.ProgressEvery < 1 {
		return errors.New("progressEvery must be >= 1")
	}
	return nil
}

func toProgressSnapshot(p legacyalgo.LegacyGAProgress) GAProgressSnapshot {
	percent := 0.0
	if p.TotalGenerations > 0 {
		percent = (float64(p.Generation) / float64(p.TotalGenerations)) * 100
		if percent > 100 {
			percent = 100
		}
	}

	return GAProgressSnapshot{
		Generation:       p.Generation,
		TotalGenerations: p.TotalGenerations,
		ProgressPercent:  percent,
		BestFitness:      p.BestFitness,
		BestViolations:   p.BestViolations,
		BestUnplaced:     p.BestUnplaced,
		ElapsedMs:        p.Elapsed.Milliseconds(),
		AvgFitness:       p.AvgFitness,
		WorstFitness:     p.WorstFitness,
		DiversityScore:   p.DiversityScore,
		StagnantGens:     p.StagnantGens,
		SAImprovements:   p.SAImprovements,
		MutationHits:     p.MutationHits,
		FeasibleCount:    p.FeasibleCount,
		Breakdown:        toScheduleBreakdown(p.Breakdown),
	}
}

func toScheduleBreakdown(b legacyalgo.ViolationBreakdown) GABreakdown {
	return GABreakdown{
		ClassConflicts:    b.ClassConflicts,
		TeacherConflicts:  b.TeacherConflicts,
		SiblingViolations: b.SiblingViolations,
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

func findPJOKSubjectID() (uuid.UUID, error) {
	var s subjects.Subject
	err := config.DB.Where("name = ?", "PJOK").First(&s).Error
	if err != nil {
		return uuid.Nil, err
	}
	return s.ID, nil
}

func buildTeacherNumberMap() (map[string]uuid.UUID, error) {
	var rows []teachers.Teacher
	if err := config.DB.Find(&rows).Error; err != nil {
		return nil, err
	}

	out := make(map[string]uuid.UUID, len(rows))
	for _, t := range rows {
		out[strconv.Itoa(t.TeacherNumber)] = t.ID
	}

	return out, nil
}

func buildActiveClassNameMap() (map[string]uuid.UUID, error) {
	var rows []classes.Class
	err := config.DB.Where("is_active = ?", true).Find(&rows).Error
	if err != nil {
		return nil, err
	}

	out := make(map[string]uuid.UUID, len(rows))
	for _, c := range rows {
		out[c.Name] = c.ID
	}

	return out, nil
}

// ── Diagnostic routes ─────────────────────────────────────────────────────────

type MatrixSlotDiagnostic struct {
	Days  []string                    `json:"days"`
	Slots map[string][]algorithm.Slot `json:"slots"`
}

func GetMatrixSlotsDiagnostic() MatrixSlotDiagnostic {
	return MatrixSlotDiagnostic{
		Days:  algorithm.MatrixDays,
		Slots: algorithm.GenerateSlots(),
	}
}

type MatrixBlockDiagnosticItem struct {
	ID            uuid.UUID  `json:"id"`
	ClassID       uuid.UUID  `json:"class_id"`
	ClassName     string     `json:"class_name"`
	TeacherID     *uuid.UUID `json:"teacher_id"`
	TeacherNumber *int       `json:"teacher_number,omitempty"`
	TeacherName   string     `json:"teacher_name,omitempty"`
	SubjectID     uuid.UUID  `json:"subject_id"`
	SubjectName   string     `json:"subject_name"`
	Duration      int        `json:"duration"`
}

type ClassBlockSummary struct {
	ClassID    uuid.UUID `json:"class_id"`
	ClassName  string    `json:"class_name"`
	BlockCount int       `json:"block_count"`
	TotalJP    int       `json:"total_jp"`
}

type MatrixBlocksDiagnostic struct {
	TotalBlocks int                         `json:"total_blocks"`
	TotalJP     int                         `json:"total_jp"`
	ByClass     []ClassBlockSummary         `json:"by_class"`
	Blocks      []MatrixBlockDiagnosticItem `json:"blocks"`
}

func GetMatrixBlocksDiagnostic() (*MatrixBlocksDiagnostic, error) {
	ctx, err := buildNewScheduleContext()
	if err != nil {
		return nil, err
	}

	// build teacher lookup: id → (number, name)
	var teacherRows []teachers.Teacher
	if err := config.DB.Find(&teacherRows).Error; err != nil {
		return nil, err
	}
	type teacherInfo struct {
		number int
		name   string
	}
	teacherLookup := make(map[uuid.UUID]teacherInfo, len(teacherRows))
	for _, t := range teacherRows {
		teacherLookup[t.ID] = teacherInfo{number: t.TeacherNumber, name: t.FullName}
	}

	// build class lookup: id → name
	var classRows []classes.Class
	if err := config.DB.Where("is_active = ?", true).Find(&classRows).Error; err != nil {
		return nil, err
	}
	classLookup := make(map[uuid.UUID]string, len(classRows))
	for _, c := range classRows {
		classLookup[c.ID] = c.Name
	}

	// build subject lookup: id → name
	var subjectRows []subjects.Subject
	if err := config.DB.Find(&subjectRows).Error; err != nil {
		return nil, err
	}
	subjectLookup := make(map[uuid.UUID]string, len(subjectRows))
	for _, s := range subjectRows {
		subjectLookup[s.ID] = s.Name
	}

	// per-class summary accumulator
	classSummary := make(map[uuid.UUID]*ClassBlockSummary)

	items := make([]MatrixBlockDiagnosticItem, 0, len(ctx.blocks))
	totalJP := 0
	for _, b := range ctx.blocks {
		item := MatrixBlockDiagnosticItem{
			ID:          b.ID,
			ClassID:     b.ClassID,
			ClassName:   classLookup[b.ClassID],
			TeacherID:   b.TeacherID,
			SubjectID:   b.SubjectID,
			SubjectName: subjectLookup[b.SubjectID],
			Duration:    b.Duration,
		}
		if b.TeacherID != nil {
			if info, ok := teacherLookup[*b.TeacherID]; ok {
				item.TeacherNumber = &info.number
				item.TeacherName = info.name
			}
		}
		items = append(items, item)

		totalJP += b.Duration
		if _, ok := classSummary[b.ClassID]; !ok {
			classSummary[b.ClassID] = &ClassBlockSummary{
				ClassID:   b.ClassID,
				ClassName: classLookup[b.ClassID],
			}
		}
		classSummary[b.ClassID].BlockCount++
		classSummary[b.ClassID].TotalJP += b.Duration
	}

	byClass := make([]ClassBlockSummary, 0, len(classSummary))
	for _, s := range classSummary {
		byClass = append(byClass, *s)
	}
	sort.Slice(byClass, func(i, j int) bool {
		return byClass[i].ClassName < byClass[j].ClassName
	})

	return &MatrixBlocksDiagnostic{
		TotalBlocks: len(ctx.blocks),
		TotalJP:     totalJP,
		ByClass:     byClass,
		Blocks:      items,
	}, nil
}

func blocksFromUnit(unit legacyalgo.PlacementUnit) []*legacyalgo.Block {
	if unit.Block != nil {
		return []*legacyalgo.Block{unit.Block}
	}
	return unit.Blocks
}

func splitToSingleJP(day string, slotIndex int, jp int) []timeRange {
	out := make([]timeRange, 0, jp)
	for i := 0; i < jp; i++ {
		start, end := legacyalgo.SlotTimeRange(day, slotIndex+i)
		if start == "" {
			continue
		}
		out = append(out, timeRange{start: start, end: end})
	}
	return out
}

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
			return entries[i].ClassID.String() < entries[j].ClassID.String()
		}

		if entries[i].SubjectID != entries[j].SubjectID {
			return entries[i].SubjectID.String() < entries[j].SubjectID.String()
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

		return entries[i].TeacherID.String() < entries[j].TeacherID.String()
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

func compareScheduleEntries(generated []ScheduleEntry, real []ScheduleEntry) ScheduleDiffResult {
	generatedCounts, _ := countScheduleEntries(generated)
	realCounts, _ := countScheduleEntries(real)

	isSame := true
	for key, realCount := range realCounts {
		if generatedCounts[key] != realCount {
			isSame = false
			break
		}
	}
	if isSame {
		for key, generatedCount := range generatedCounts {
			if realCounts[key] != generatedCount {
				isSame = false
				break
			}
		}
	}

	return ScheduleDiffResult{
		Checked:           true,
		IsSame:            isSame,
		GeneratedCount:    len(generated),
		RealScheduleCount: len(real),
	}
}

func countScheduleEntries(entries []ScheduleEntry) (map[string]int, map[string]ScheduleEntry) {
	counts := make(map[string]int, len(entries))
	samples := make(map[string]ScheduleEntry, len(entries))

	for _, entry := range entries {
		key := scheduleEntryKey(entry)
		counts[key]++
		if _, exists := samples[key]; !exists {
			samples[key] = entry
		}
	}

	return counts, samples
}

func scheduleEntryKey(entry ScheduleEntry) string {
	teacherID := ""
	if entry.TeacherID != nil {
		teacherID = entry.TeacherID.String()
	}

	parts := []string{
		teacherID,
		entry.SubjectID.String(),
		entry.ClassID.String(),
		entry.Day,
		entry.TimeStart,
		entry.TimeEnd,
	}
	return strings.Join(parts, "|")
}

// GenerateV3MultiRun runs the hybrid GA+SA sequentially for the given number of runs
// and returns only the metadata per run — no schedule entries.
// Each run uses a different seed so results vary; if a base seed is provided it is
// incremented per run so the batch is still reproducible.
func GenerateV3MultiRun(opts GenerateHybridOptions, runs int) (*MultiRunResult, error) {
	if runs < 1 {
		runs = 1
	}

	baseSeedGA := opts.GA.Seed
	baseSeedSA := opts.SA.Seed
	totalStart := time.Now()
	results := make([]RunSummary, 0, runs)

	for i := 0; i < runs; i++ {
		runOpts := opts
		if baseSeedGA != 0 {
			runOpts.GA.Seed = baseSeedGA + int64(i)
		}
		if baseSeedSA != 0 {
			runOpts.SA.Seed = baseSeedSA + int64(i)
		}

		runStart := time.Now()
		result, err := GenerateV3Schedule(runOpts)
		if err != nil {
			return nil, err
		}
		result.Generation.Meta.TotalElapsedMs = time.Since(runStart).Milliseconds()

		results = append(results, RunSummary{
			Run:  i + 1,
			Meta: result.Generation.Meta,
		})
	}

	return &MultiRunResult{
		Runs:           runs,
		TotalElapsedMs: time.Since(totalStart).Milliseconds(),
		Results:        results,
	}, nil
}

func logBlockDiagnostics(blocks []algorithm.MatrixBlock, index map[uuid.UUID][]algorithm.Gene) {
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
