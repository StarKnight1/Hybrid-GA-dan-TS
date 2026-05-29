package schedule

import (
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"smp_mater_dei_be/internal/algorithm"
	"smp_mater_dei_be/internal/classes"
	"smp_mater_dei_be/internal/platform/config"
	"smp_mater_dei_be/internal/subjects"
	"smp_mater_dei_be/internal/teachers"
	teachingassignments "smp_mater_dei_be/internal/teaching_assignments"
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

func GenerateSchedule() ([]ScheduleEntry, error) {
	result, err := GenerateV3Schedule(GenerateHybridOptions{
		GA: DefaultGAParams(),
		TS: DefaultTSParams(),
	})
	if err != nil {
		return nil, err
	}
	return result.Generation.Entries, nil
}

func EvaluateRealScheduleBaseline() (*RealScheduleValidationResult, error) {
	pjokID, err := findPJOKSubjectID()
	if err != nil {
		return nil, err
	}

	sbpID, err := findSBPSubjectID()
	if err != nil {
		return nil, err
	}

	teacherNumberMap, err := buildTeacherNumberMap()
	if err != nil {
		return nil, err
	}

	classNameMap, err := buildActiveClassNameMap()
	if err != nil {
		return nil, err
	}

	subjectByTeacher, err := buildSubjectByTeacherNumberMap()
	if err != nil {
		return nil, err
	}

	opts := algorithm.RealScheduleMatrixOptions{
		TeacherNumberToID:      teacherNumberMap,
		ClassNameToID:          classNameMap,
		SubjectByTeacherNumber: subjectByTeacher,
		SBPSubjectID:           sbpID,
	}

	matrixResult, err := algorithm.BuildRealScheduleMatrix(opts)
	if err != nil {
		return nil, err
	}

	entries := buildEntriesFromMatrix(matrixResult.Blocks, matrixResult.Matrix, nil)
	softBd := algorithm.BreakdownSoftViolations(matrixResult.Matrix, matrixResult.Blocks, pjokID)
	softTotal := softBd.SameDaySplit + softBd.PJOKAfterDeadline*3
	unplaced := len(matrixResult.Blocks) - len(matrixResult.Placements)

	result := &RealScheduleValidationResult{Entries: entries}
	result.Meta.Input = InputStats{
		Blocks: len(matrixResult.Blocks),
		Units:  len(matrixResult.Blocks),
	}
	result.Meta.Result = ResultStats{
		EntriesGenerated: len(entries),
		Violations:       softTotal,
		Unplaced:         unplaced,
		SoftBreakdown: SoftBreakdown{
			SameDaySplit:      softBd.SameDaySplit,
			PJOKAfterDeadline: softBd.PJOKAfterDeadline,
		},
	}
	result.Meta.IsFeasible = unplaced == 0 && softTotal == 0

	return result, nil
}

// computeSoftBreakdownFromEntries evaluates soft violations from a ScheduleEntry slice.
// Mirrors CountSoftViolations logic but operates on time-string entries instead of a ScheduleMatrix.
//
// Block boundaries are detected by slot index gaps, not time gaps. This is critical because
// the school timetable has a morning break (slots 2→3: 09:10→09:30) and a lunch break
// (slots 5→6: 11:30→11:45) where consecutive-slot blocks produce non-consecutive times.
// Using time-gap detection would incorrectly split those blocks and inflate SameDaySplit.
func computeSoftBreakdownFromEntries(entries []ScheduleEntry, pjokSubjectID uint) SoftBreakdown {
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
		classID   uint
		subjectID uint
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
		classID   uint
		subjectID uint
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

// ── V2 schedule generation (new matrix-based GA) ─────────────────────────────

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

func GenerateV3Schedule(opts GenerateHybridOptions) (*ScheduleGenerateAndCompareResult, error) {
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
// human-readable teacher/subject/class names instead of IDs.
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
	teacherNames := make(map[uint]string, len(teacherRows))
	for _, t := range teacherRows {
		teacherNames[t.ID] = t.FullName
	}

	var subjectRows []subjects.Subject
	if err := config.DB.Find(&subjectRows).Error; err != nil {
		return nil, err
	}
	subjectNames := make(map[uint]string, len(subjectRows))
	for _, s := range subjectRows {
		subjectNames[s.ID] = s.Name
	}

	var classRows []classes.Class
	if err := config.DB.Where("is_active = ?", true).Find(&classRows).Error; err != nil {
		return nil, err
	}
	classNames := make(map[uint]string, len(classRows))
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

func findSBPSubjectID() (uint, error) {
	var s subjects.Subject
	err := config.DB.Where("name = ?", "Seni Budaya").First(&s).Error
	if err != nil {
		return 0, err
	}
	return s.ID, nil
}

func buildTeacherNumberMap() (map[string]uint, error) {
	var rows []teachers.Teacher
	if err := config.DB.Find(&rows).Error; err != nil {
		return nil, err
	}

	out := make(map[string]uint, len(rows))
	for _, t := range rows {
		out[strconv.Itoa(t.TeacherNumber)] = t.ID
	}

	return out, nil
}

func buildActiveClassNameMap() (map[string]uint, error) {
	var rows []classes.Class
	err := config.DB.Where("is_active = ?", true).Find(&rows).Error
	if err != nil {
		return nil, err
	}

	out := make(map[string]uint, len(rows))
	for _, c := range rows {
		out[c.Name] = c.ID
	}

	return out, nil
}

func buildSubjectByTeacherNumberMap() (map[string]uint, error) {
	type row struct {
		TeacherNumber int  `gorm:"column:teacher_number"`
		SubjectID     uint `gorm:"column:subject_id"`
	}
	var rows []row
	err := config.DB.
		Table("teaching_assignments ta").
		Select("t.teacher_number, ta.subject_id").
		Joins("JOIN teachers t ON t.id = ta.teacher_id").
		Where("ta.deleted_at IS NULL").
		Where("ta.teacher_id IS NOT NULL").
		Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	out := make(map[string]uint, len(rows))
	for _, r := range rows {
		out[strconv.Itoa(r.TeacherNumber)] = r.SubjectID
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
	ID            uint   `json:"id"`
	ClassID       uint   `json:"class_id"`
	ClassName     string `json:"class_name"`
	TeacherID     *uint  `json:"teacher_id"`
	TeacherNumber *int   `json:"teacher_number,omitempty"`
	TeacherName   string `json:"teacher_name,omitempty"`
	SubjectID     uint   `json:"subject_id"`
	SubjectName   string `json:"subject_name"`
	Duration      int    `json:"duration"`
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

	// build teacher lookup: id → (number, name)
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

	// build class lookup: id → name
	var classRows []classes.Class
	if err := config.DB.Where("is_active = ?", true).Find(&classRows).Error; err != nil {
		return nil, err
	}
	classLookup := make(map[uint]string, len(classRows))
	for _, c := range classRows {
		classLookup[c.ID] = c.Name
	}

	// build subject lookup: id → name
	var subjectRows []subjects.Subject
	if err := config.DB.Find(&subjectRows).Error; err != nil {
		return nil, err
	}
	subjectLookup := make(map[uint]string, len(subjectRows))
	for _, s := range subjectRows {
		subjectLookup[s.ID] = s.Name
	}

	// per-class summary accumulator
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

func splitToSingleJP(day string, slotIndex int, jp int) []timeRange {
	out := make([]timeRange, 0, jp)
	for i := 0; i < jp; i++ {
		slots := algorithm.GenerateSlots()
		start, end := matrixSlotTimeRange(day, slotIndex+i, slots)
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
		teacherID = strconv.FormatUint(uint64(*entry.TeacherID), 10)
	}

	parts := []string{
		teacherID,
		strconv.FormatUint(uint64(entry.SubjectID), 10),
		strconv.FormatUint(uint64(entry.ClassID), 10),
		entry.Day,
		entry.TimeStart,
		entry.TimeEnd,
	}
	return strings.Join(parts, "|")
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
