package schedule

// ScheduleEntry is one generated 1-JP timetable row.
// One JP is always 40 minutes.
type ScheduleEntry struct {
	TeacherID   *uint  `json:"teacherId,omitempty"`
	TeacherName string `json:"teacherName,omitempty"`
	SubjectID   uint   `json:"subjectId"`
	SubjectName string `json:"subjectName,omitempty"`
	ClassID     uint   `json:"classId"`
	ClassName   string `json:"className,omitempty"`
	Day         string `json:"day"`
	TimeStart   string `json:"timeStart"`
	TimeEnd     string `json:"timeEnd"`
}

// GAParams contains tunable parameters for the GA phase.
type GAParams struct {
	PopulationSize int     `json:"populationSize"`
	Generations    int     `json:"generations"`
	MutationRate   float64 `json:"mutationRate"`
	EliteCount     int     `json:"eliteCount"`
	TournamentSize int     `json:"tournamentSize"`
	Seed           int64   `json:"seed"`
	ProgressEvery  int     `json:"progressEvery"`
}

// TSParams contains tunable parameters for the Tabu Search phase.
type TSParams struct {
	MaxIterations    int   `json:"maxIterations"`
	TabuTenure       int   `json:"tabuTenure"`
	NeighborhoodSize int   `json:"neighborhoodSize"`
	PerturbAfter     int   `json:"perturbAfter"`
	PerturbCount     int   `json:"perturbCount"`
	Seed             int64 `json:"seed"`
	ProgressEvery    int   `json:"progressEvery"`
}

// GAProgressSnapshot is one observable progress point emitted during GA execution.
type GAProgressSnapshot struct {
	Generation       int     `json:"generation"`
	TotalGenerations int     `json:"totalGenerations"`
	ProgressPercent  float64 `json:"progressPercent"`
	BestUnplaced     int     `json:"bestUnplaced"`
	BestViolations   int     `json:"bestViolations"`
	AvgUnplaced      float64 `json:"avgUnplaced"`
	StagnantGens     int     `json:"stagnantGens"`
	ElapsedMs        int64   `json:"elapsedMs"`
}

// TSProgressSnapshot is one observable progress point emitted during TS execution.
type TSProgressSnapshot struct {
	Phase             string  `json:"phase"` // always "ts"
	Iteration         int     `json:"iteration"`
	TotalIterations   int     `json:"totalIterations"`
	ProgressPercent   float64 `json:"progressPercent"`
	CurrentUnplaced   int     `json:"currentUnplaced"`
	CurrentSoft       int     `json:"currentSoftViolations"`
	BestUnplaced      int     `json:"bestUnplaced"`
	BestSoft          int     `json:"bestSoftViolations"`
	TabuListSize      int     `json:"tabuListSize"`
	ElapsedMs         int64   `json:"elapsedMs"`
}

// GAPhaseResult carries a summary of what GA achieved, emitted before TS starts.
type GAPhaseResult struct {
	Unplaced       int   `json:"unplaced"`
	SoftViolations int   `json:"softViolations"`
	Generations    int   `json:"generations"`
	ElapsedMs      int64 `json:"elapsedMs"`
}

// SoftBreakdown breaks down soft violations by category.
type SoftBreakdown struct {
	SameDaySplit        int `json:"sameDaySplit"`
	SameDaySplitGrouped int `json:"sameDaySplitGrouped"`
}

// InputStats describes the input domain size.
type InputStats struct {
	ActiveAssignments int `json:"activeAssignments"`
	ActiveClasses     int `json:"activeClasses"`
	Teachers          int `json:"teachers"`
}

// ResultStats describes the final optimization result quality.
type ResultStats struct {
	EntriesGenerated int           `json:"entriesGenerated"`
	Violations       int           `json:"violations"`
	Unplaced         int           `json:"unplaced"`
	SoftBreakdown    SoftBreakdown `json:"softBreakdown"`
}

// ScheduleMeta contains diagnostics and tuning context.
type ScheduleMeta struct {
	Input          InputStats  `json:"input"`
	EffectiveGA    GAParams    `json:"ga"`
	EffectiveTS    *TSParams   `json:"ts,omitempty"`
	Result         ResultStats `json:"result"`
	TotalElapsedMs int64       `json:"totalElapsedMs,omitempty"`
	LoopCount      int         `json:"loopCount,omitempty"`
	SeedWarnings   []string    `json:"seedWarnings,omitempty"`
}

// ScheduleGenerationResult is the full payload returned by the v2 (GA-only) API.
type ScheduleGenerationResult struct {
	Entries  []ScheduleEntry      `json:"entries"`
	Meta     ScheduleMeta         `json:"meta"`
	Progress []GAProgressSnapshot `json:"progress,omitempty"`
}

// ReadableScheduleResult is the human-readable response from the v3/readable endpoint.
// Entries are sorted by class name → day → time and include both IDs and names.
type ReadableScheduleResult struct {
	Entries []ScheduleEntry `json:"entries"`
	Meta    ScheduleMeta    `json:"meta"`
}

// RunSummary is the metadata for one run inside a multi-run batch.
type RunSummary struct {
	Run  int          `json:"run"`
	Meta ScheduleMeta `json:"meta"`
}

// MultiRunResult is the response from the v3/multi-run endpoint.
type MultiRunResult struct {
	Runs           int          `json:"runs"`
	TotalElapsedMs int64        `json:"totalElapsedMs"`
	Results        []RunSummary `json:"results"`
}

// GenerateHybridOptions carries options for the v3 hybrid GA+TS endpoint.
type GenerateHybridOptions struct {
	GA              GAParams
	TS              TSParams
	StagnationLimit int
	Restarts        int
	LoopUntilFeasible bool
	MaxLoops        int
	OnGAProgress    func(GAProgressSnapshot)
	OnGAComplete    func(GAPhaseResult)
	OnTSProgress    func(TSProgressSnapshot)
}
