package schedule

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"time"

	"smp_mater_dei_be/internal/algorithm"
	"smp_mater_dei_be/internal/classes"
	"smp_mater_dei_be/internal/platform/config"
	"smp_mater_dei_be/internal/subjects"
	teachingassignments "smp_mater_dei_be/internal/teaching_assignments"
	"smp_mater_dei_be/internal/teachers"
)


type GenerateScheduleOptions struct {
	Params              GAParams
	CollectProgress     bool
	IncludeSeedWarnings bool
	OnProgress          func(GAProgressSnapshot)
}

type newScheduleBuildContext struct {
	blocks       []algorithm.MatrixBlock
	index        map[uint][]algorithm.Gene
	pjokID       uint
	input        InputStats
	subjectNames map[uint]string
	teacherNames map[uint]string
	classNames   map[uint]string
}

func NewGenerateScheduleOptions() GenerateScheduleOptions {
	return GenerateScheduleOptions{Params: DefaultGAParams()}
}

func DefaultGAParams() GAParams {
	cfg := algorithm.DefaultGAConfig()
	return GAParams{
		PopulationSize: cfg.PopSize,
		Generations:    cfg.MaxGenerations,
		MutationRate:   cfg.MutationProb,
		EliteCount:     cfg.EliteSize,
		TournamentSize: cfg.TournSize,
		ProgressEvery:  cfg.ReportInterval,
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

// ── Konteks pembuatan jadwal ──────────────────────────────────────────────────

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

	subjectNames, err := loadSubjectNames()
	if err != nil {
		return nil, err
	}
	teacherNames, err := loadTeacherNames()
	if err != nil {
		return nil, err
	}
	classNames, err := loadClassNames()
	if err != nil {
		return nil, err
	}

	return &newScheduleBuildContext{
		blocks:       blocks,
		index:        index,
		pjokID:       pjokID,
		subjectNames: subjectNames,
		teacherNames: teacherNames,
		classNames:   classNames,
		input: InputStats{
			ActiveAssignments: len(assignments),
			Blocks:            len(blocks),
			Units:             len(blocks),
			ActiveClasses:     len(classSet),
			Teachers:          len(teacherSet),
		},
	}, nil
}

func buildEntriesFromMatrix(
	blocks []algorithm.MatrixBlock,
	matrix *algorithm.ScheduleMatrix,
	daySlots algorithm.DaySlots,
	subjectNames map[uint]string,
	teacherNames map[uint]string,
	classNames map[uint]string,
) []ScheduleEntry {
	if daySlots == nil {
		daySlots = algorithm.GenerateSlots()
	}

	entries := make([]ScheduleEntry, 0, len(blocks))
	for _, block := range blocks {
		placement, ok := matrix.Placement(block.ID)
		if !ok {
			continue
		}
		subjName := subjectNames[block.SubjectID]
		clsName := classNames[block.ClassID]
		var tchName string
		if block.TeacherID != nil {
			tchName = teacherNames[*block.TeacherID]
		}
		for offset := 0; offset < placement.Duration; offset++ {
			start, end := matrixSlotTimeRange(placement.Day, placement.StartSlot+offset, daySlots)
			if start == "" {
				continue
			}
			entries = append(entries, ScheduleEntry{
				TeacherID:   block.TeacherID,
				SubjectID:   block.SubjectID,
				ClassID:     block.ClassID,
				SubjectName: subjName,
				TeacherName: tchName,
				ClassName:   clsName,
				Day:         placement.Day,
				TimeStart:   start,
				TimeEnd:     end,
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

// ── Generate jadwal V3 (hybrid GA+TS) ────────────────────────────────────────

func DefaultTSParams() TSParams {
	cfg := algorithm.DefaultTSConfig()
	return TSParams{
		TabuTenure:    cfg.Tenure,
		Iterations:    cfg.MaxIterations,
		ProgressEvery: cfg.ReportInterval,
		PerturbCount:  cfg.ShakeCount,
		PerturbAfter:  cfg.ShakeAfter,
	}
}

func GenerateV3Schedule(ctx context.Context, opts GenerateHybridOptions) (*ScheduleGenerationResult, error) {
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

	sched, err := buildNewScheduleContext()
	if err != nil {
		return nil, err
	}

	gaCfg := algorithm.DefaultGAConfig()
	gaCfg.PopSize = gaParams.PopulationSize
	gaCfg.MaxGenerations = gaParams.Generations
	gaCfg.MutationProb = gaParams.MutationRate
	gaCfg.EliteSize = gaParams.EliteCount
	gaCfg.TournSize = gaParams.TournamentSize
	gaCfg.RandSeed = gaParams.Seed
	gaCfg.ReportInterval = gaParams.ProgressEvery
	gaCfg.PatienceLimit = opts.StagnationLimit
	gaCfg.PJOKSubjID = sched.pjokID
	gaCfg.OnSnapshot = func(p algorithm.GAProgress) {
		if opts.OnGAProgress != nil {
			snap := toV2ProgressSnapshot(p, gaParams.Generations)
			snap.StagnantGens = p.StagnantGens
			opts.OnGAProgress(snap)
		}
	}

	tsCfg := algorithm.DefaultTSConfig()
	tsCfg.Tenure = tsParams.TabuTenure
	tsCfg.MaxIterations = tsParams.Iterations
	tsCfg.ReportInterval = tsParams.ProgressEvery
	tsCfg.RandSeed = tsParams.Seed
	tsCfg.ShakeCount = tsParams.PerturbCount
	tsCfg.ShakeAfter = tsParams.PerturbAfter
	tsCfg.PJOKSubjID = sched.pjokID
	tsCfg.OnSnapshot = func(p algorithm.TSProgress) {
		if opts.OnTSProgress != nil {
			opts.OnTSProgress(toTSProgressSnapshot(p, tsParams.Iterations))
		}
	}

	hybridCfg := algorithm.HybridConfig{
		GA:                  gaCfg,
		TS:                  tsCfg,
		ExtraRuns:           opts.Restarts,
		RetryUntilFeasible:  opts.LoopUntilFeasible,
		MaxAttempts:         opts.MaxLoops,
		AfterGA: func(r algorithm.GAResult) {
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
	hybridResult := algorithm.RunHybrid(ctx, sched.blocks, sched.index, nil, hybridCfg)

	entries := buildEntriesFromMatrix(sched.blocks, hybridResult.Matrix, nil, sched.subjectNames, sched.teacherNames, sched.classNames)
	hybridSoftBd := algorithm.BreakdownSoftViolations(hybridResult.Matrix, sched.blocks, sched.pjokID)

	defaultTS := DefaultTSParams()
	generationResult := &ScheduleGenerationResult{
		Entries: entries,
		Meta: ScheduleMeta{
			Input:          sched.input,
			DefaultGA:      DefaultGAParams(),
			EffectiveGA:    gaParams,
			DefaultTS:      &defaultTS,
			EffectiveTS:    &tsParams,
			TotalElapsedMs: hybridResult.Elapsed.Milliseconds(),
			LoopCount:      hybridResult.Runs,
			Result: ResultStats{
				EntriesGenerated: len(entries),
				BestFitness:      hybridResult.Unplaced,
				Violations:       hybridResult.SoftViolations,
				Unplaced:         hybridResult.Unplaced,
				SoftBreakdown: SoftBreakdown{
					SameDaySplit:        hybridSoftBd.DaySplitCount,
					SameDaySplitGrouped: hybridSoftBd.DaySplitGroupCount,
					PJOKAfterDeadline:   hybridSoftBd.PJOKOvertime,
				},
			},
		},
	}

	return generationResult, nil
}

// toTSProgressSnapshot mengubah progres TS internal menjadi format snapshot untuk klien.
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

func loadSubjectNames() (map[uint]string, error) {
	var rows []subjects.Subject
	if err := config.DB.Find(&rows).Error; err != nil {
		return nil, err
	}
	m := make(map[uint]string, len(rows))
	for _, s := range rows {
		m[s.ID] = s.Name
	}
	return m, nil
}

func loadTeacherNames() (map[uint]string, error) {
	var rows []teachers.Teacher
	if err := config.DB.Find(&rows).Error; err != nil {
		return nil, err
	}
	m := make(map[uint]string, len(rows))
	for _, t := range rows {
		m[t.ID] = t.FullName
	}
	return m, nil
}

func loadClassNames() (map[uint]string, error) {
	var rows []classes.Class
	if err := config.DB.Find(&rows).Error; err != nil {
		return nil, err
	}
	m := make(map[uint]string, len(rows))
	for _, cl := range rows {
		m[cl.ID] = cl.Name
	}
	return m, nil
}

// ── Fungsi utilitas ───────────────────────────────────────────────────────────

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


// GenerateV3MultiRun menjalankan hybrid GA+TS secara berurutan sebanyak jumlah run yang ditentukan.
func GenerateV3MultiRun(ctx context.Context, runs int, opts GenerateHybridOptions) (*MultiRunResult, error) {
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
		result, err := GenerateV3Schedule(ctx, runOpts)
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
