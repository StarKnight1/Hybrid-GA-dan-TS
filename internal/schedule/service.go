package schedule

import (
	"errors"
	"fmt"
	"sort"
	"strconv"
	"time"

	"smp_mater_dei_be/internal/algorithm"
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

// ── GA parameter defaults & validation ───────────────────────────────────────

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

// DefaultTSParams returns default Tabu Search parameters matching algorithm.DefaultTSConfig.
func DefaultTSParams() TSParams {
	cfg := algorithm.DefaultTSConfig()
	return TSParams{
		MaxIterations:    cfg.MaxIterations,
		TabuTenure:       cfg.TabuTenure,
		NeighborhoodSize: cfg.NeighborhoodSize,
		PerturbAfter:     cfg.PerturbAfter,
		PerturbCount:     cfg.PerturbCount,
		ProgressEvery:    cfg.ProgressEvery,
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
			Description: "Probability of mutating each gene. Keep low so crossover structure is preserved.",
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
			Description: "Tournament selection pool size used to pick parents.",
		},
		{
			Name:        "seed",
			Type:        "int64",
			Default:     "current unix-nano timestamp",
			Min:         "int64 min",
			Max:         "int64 max",
			Description: "Random seed for reproducible generation.",
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

// GAParameterSpec documents one tunable GA parameter.
type GAParameterSpec struct {
	Name        string `json:"name"`
	Type        string `json:"type"`
	Default     string `json:"default"`
	Min         string `json:"min"`
	Max         string `json:"max"`
	Description string `json:"description"`
}

// GenerateScheduleOptions is the options struct for the v2 GA-only endpoint.
type GenerateScheduleOptions struct {
	Params        GAParams
	OnProgress    func(GAProgressSnapshot)
}

func isZeroGAParams(p GAParams) bool {
	return p.PopulationSize == 0 &&
		p.Generations == 0 &&
		p.MutationRate == 0 &&
		p.EliteCount == 0 &&
		p.Seed == 0 &&
		p.ProgressEvery == 0
}

func isZeroTSParams(p TSParams) bool {
	return p.MaxIterations == 0 &&
		p.TabuTenure == 0 &&
		p.NeighborhoodSize == 0 &&
		p.PerturbAfter == 0 &&
		p.PerturbCount == 0 &&
		p.Seed == 0 &&
		p.ProgressEvery == 0
}

func ensureSeed(p GAParams) GAParams {
	if p.Seed == 0 {
		p.Seed = time.Now().UnixNano()
	}
	return p
}

func ensureTSSeed(p TSParams) TSParams {
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

// ── Schedule build context ────────────────────────────────────────────────────

type newScheduleBuildContext struct {
	blocks []algorithm.MatrixBlock
	index  map[uuid.UUID][]algorithm.Gene
	pjokID uint
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
			ActiveClasses:     len(classSet),
			Teachers:          len(teacherSet),
		},
	}, nil
}

// ── V2: Matrix-based GA only ──────────────────────────────────────────────────

func GenerateV2Schedule(opts GenerateScheduleOptions) (*ScheduleGenerationResult, error) {
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
	softBd := algorithm.BreakdownSoftViolations(gaResult.Matrix, ctx.blocks, ctx.pjokID)

	return &ScheduleGenerationResult{
		Entries: entries,
		Meta: ScheduleMeta{
			Input:       ctx.input,
			EffectiveGA: opts.Params,
			Result: ResultStats{
				EntriesGenerated: len(entries),
				Violations:       gaResult.SoftViolations,
				Unplaced:         gaResult.Unplaced,
				SoftBreakdown: SoftBreakdown{
					SameDaySplit:        softBd.SameDaySplit,
					SameDaySplitGrouped: softBd.SameDaySplitGrouped,
				},
			},
		},
	}, nil
}

// ── V3: Hybrid GA + Tabu Search ───────────────────────────────────────────────

// GenerateV3Schedule runs the full hybrid GA+TS pipeline.
// GA performs global exploration; TS refines the result toward
// 0 unplaced blocks and 0 soft violations.
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
	if isZeroTSParams(tsParams) {
		tsParams = DefaultTSParams()
	}
	tsParams = ensureTSSeed(tsParams)

	ctx, err := buildNewScheduleContext()
	if err != nil {
		return nil, err
	}

	// ── GA phase ──────────────────────────────────────────────────────────────
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

	gaResult := algorithm.RunGA(ctx.blocks, ctx.index, nil, gaCfg)

	if opts.OnGAComplete != nil {
		opts.OnGAComplete(GAPhaseResult{
			Unplaced:       gaResult.Unplaced,
			SoftViolations: gaResult.SoftViolations,
			Generations:    gaResult.Generations,
			ElapsedMs:      gaResult.Elapsed.Milliseconds(),
		})
	}

	// ── TS phase ──────────────────────────────────────────────────────────────
	tsCfg := algorithm.DefaultTSConfig()
	tsCfg.MaxIterations = tsParams.MaxIterations
	tsCfg.TabuTenure = tsParams.TabuTenure
	tsCfg.NeighborhoodSize = tsParams.NeighborhoodSize
	tsCfg.PerturbAfter = tsParams.PerturbAfter
	tsCfg.PerturbCount = tsParams.PerturbCount
	tsCfg.Seed = tsParams.Seed
	tsCfg.ProgressEvery = tsParams.ProgressEvery
	tsCfg.PJOKSubjectID = ctx.pjokID
	tsCfg.OnProgress = func(p algorithm.TSProgress) {
		if opts.OnTSProgress != nil {
			opts.OnTSProgress(toTSProgressSnapshot(p, tsParams.MaxIterations))
		}
	}

	tsResult := algorithm.RunTS(ctx.blocks, ctx.index, nil, gaResult, tsCfg)

	// ── Build result ──────────────────────────────────────────────────────────
	entries := buildEntriesFromMatrix(ctx.blocks, tsResult.Matrix, nil)
	softBd := algorithm.BreakdownSoftViolations(tsResult.Matrix, ctx.blocks, ctx.pjokID)
	effectiveTS := tsParams

	return &ScheduleGenerationResult{
		Entries: entries,
		Meta: ScheduleMeta{
			Input:       ctx.input,
			EffectiveGA: gaParams,
			EffectiveTS: &effectiveTS,
			TotalElapsedMs: gaResult.Elapsed.Milliseconds() + tsResult.Elapsed.Milliseconds(),
			Result: ResultStats{
				EntriesGenerated: len(entries),
				Violations:       tsResult.SoftViolations,
				Unplaced:         tsResult.Unplaced,
				SoftBreakdown: SoftBreakdown{
					SameDaySplit:        softBd.SameDaySplit,
					SameDaySplitGrouped: softBd.SameDaySplitGrouped,
				},
			},
		},
	}, nil
}

// resolveEntryNames fetches subject, class, and teacher names from the DB
// and populates the name fields on each entry in-place.
func resolveEntryNames(entries []ScheduleEntry) error {
	var teacherRows []teachers.Teacher
	if err := config.DB.Find(&teacherRows).Error; err != nil {
		return err
	}
	teacherNames := make(map[uint]string, len(teacherRows))
	for _, t := range teacherRows {
		teacherNames[t.ID] = t.FullName
	}

	var subjectRows []subjects.Subject
	if err := config.DB.Find(&subjectRows).Error; err != nil {
		return err
	}
	subjectNames := make(map[uint]string, len(subjectRows))
	for _, s := range subjectRows {
		subjectNames[s.ID] = s.Name
	}

	var classRows []classes.Class
	if err := config.DB.Where("is_active = ?", true).Find(&classRows).Error; err != nil {
		return err
	}
	classNames := make(map[uint]string, len(classRows))
	for _, cl := range classRows {
		classNames[cl.ID] = cl.Name
	}

	for i := range entries {
		entries[i].SubjectName = subjectNames[entries[i].SubjectID]
		entries[i].ClassName = classNames[entries[i].ClassID]
		if entries[i].TeacherID != nil {
			entries[i].TeacherName = teacherNames[*entries[i].TeacherID]
		}
	}
	return nil
}

// GenerateV3ScheduleReadable runs GA+TS and returns the schedule with
// human-readable names alongside IDs.
// Entries are sorted by class name → day (Monday first) → time.
func GenerateV3ScheduleReadable(opts GenerateHybridOptions) (*ReadableScheduleResult, error) {
	result, err := GenerateV3Schedule(opts)
	if err != nil {
		return nil, err
	}

	if err := resolveEntryNames(result.Entries); err != nil {
		return nil, err
	}

	sort.Slice(result.Entries, func(i, j int) bool {
		if result.Entries[i].ClassName != result.Entries[j].ClassName {
			return result.Entries[i].ClassName < result.Entries[j].ClassName
		}
		di := algorithm.MatrixDayIndex(result.Entries[i].Day)
		dj := algorithm.MatrixDayIndex(result.Entries[j].Day)
		if di != dj {
			return di < dj
		}
		return result.Entries[i].TimeStart < result.Entries[j].TimeStart
	})

	return &ReadableScheduleResult{
		Entries: result.Entries,
		Meta:    result.Meta,
	}, nil
}

// GenerateV3MultiRun runs the hybrid GA+TS sequentially for the given number of
// runs and returns only the metadata per run — no schedule entries.
func GenerateV3MultiRun(opts GenerateHybridOptions, runs int) (*MultiRunResult, error) {
	if runs < 1 {
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

		result, err := GenerateV3Schedule(runOpts)
		if err != nil {
			return nil, err
		}

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

// ── Entry building ────────────────────────────────────────────────────────────

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

// ── Progress snapshot converters ──────────────────────────────────────────────

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
		AvgUnplaced:      p.AvgUnplaced,
		StagnantGens:     p.StagnantGens,
		ElapsedMs:        p.Elapsed.Milliseconds(),
	}
}

func toTSProgressSnapshot(p algorithm.TSProgress, totalIterations int) TSProgressSnapshot {
	percent := 0.0
	if totalIterations > 0 {
		percent = float64(p.Iteration) / float64(totalIterations) * 100
		if percent > 100 {
			percent = 100
		}
	}
	return TSProgressSnapshot{
		Phase:           "ts",
		Iteration:       p.Iteration,
		TotalIterations: totalIterations,
		ProgressPercent: percent,
		CurrentUnplaced: p.CurrentUnplaced,
		CurrentSoft:     p.CurrentSoft,
		BestUnplaced:    p.BestUnplaced,
		BestSoft:        p.BestSoft,
		TabuListSize:    p.TabuListSize,
		ElapsedMs:       p.Elapsed.Milliseconds(),
	}
}

// ── DB query helpers ──────────────────────────────────────────────────────────

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

// ── Diagnostic endpoints ──────────────────────────────────────────────────────

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
	ID            uuid.UUID `json:"id"`
	ClassID       uint      `json:"class_id"`
	ClassName     string    `json:"class_name"`
	TeacherID     *uint     `json:"teacher_id"`
	TeacherNumber *int      `json:"teacher_number,omitempty"`
	TeacherName   string    `json:"teacher_name,omitempty"`
	SubjectID     uint      `json:"subject_id"`
	SubjectName   string    `json:"subject_name"`
	Duration      int       `json:"duration"`
}

type ClassBlockSummary struct {
	ClassID    uint   `json:"class_id"`
	ClassName  string `json:"class_name"`
	BlockCount int    `json:"block_count"`
	TotalJP    int    `json:"total_jp"`
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

	var teacherRows []teachers.Teacher
	if err := config.DB.Find(&teacherRows).Error; err != nil {
		return nil, err
	}
	type teacherInfo struct {
		number int
		name   string
	}
	teacherLookup := make(map[uint]teacherInfo, len(teacherRows))
	for _, t := range teacherRows {
		teacherLookup[t.ID] = teacherInfo{number: t.TeacherNumber, name: t.FullName}
	}

	var classRows []classes.Class
	if err := config.DB.Where("is_active = ?", true).Find(&classRows).Error; err != nil {
		return nil, err
	}
	classLookup := make(map[uint]string, len(classRows))
	for _, c := range classRows {
		classLookup[c.ID] = c.Name
	}

	var subjectRows []subjects.Subject
	if err := config.DB.Find(&subjectRows).Error; err != nil {
		return nil, err
	}
	subjectLookup := make(map[uint]string, len(subjectRows))
	for _, s := range subjectRows {
		subjectLookup[s.ID] = s.Name
	}

	classSummary := make(map[uint]*ClassBlockSummary)
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

// ── Sort & utility helpers ────────────────────────────────────────────────────

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
		if entries[i].TeacherID == nil {
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

func parseClock(s string) time.Time {
	t, _ := time.Parse("15:04", s)
	return t
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

