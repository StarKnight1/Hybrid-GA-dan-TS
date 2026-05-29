package schedule

import "github.com/google/uuid"

// ScheduleEntry is one generated 1-JP timetable row.
// One JP is always 40 minutes.
type ScheduleEntry struct {
	TeacherID *uuid.UUID `json:"teacherId,omitempty"`
	SubjectID uuid.UUID  `json:"subjectId"`
	ClassID   uuid.UUID  `json:"classId"`
	Day       string     `json:"day"`
	TimeStart string     `json:"timeStart"`
	TimeEnd   string     `json:"timeEnd"`
}

// GAParams contains tunable parameters for schedule generation.
type GAParams struct {
	PopulationSize int     `json:"populationSize"`
	Generations    int     `json:"generations"`
	MutationRate   float64 `json:"mutationRate"`
	EliteCount     int     `json:"eliteCount"`
	TournamentSize int     `json:"tournamentSize"`
	SAIterations   int     `json:"-"`
	Seed           int64   `json:"seed"`
	ProgressEvery  int     `json:"progressEvery"`
}

// GABreakdown mirrors hard-constraint breakdown counters.
type GABreakdown struct {
	ClassConflicts    int `json:"classConflicts"`
	TeacherConflicts  int `json:"teacherConflicts"`
	SiblingViolations int `json:"siblingViolations"`
}

// SoftBreakdown breaks down soft violations by category.
// SameDaySplit: blocks of the same (class, subject) placed on the same day (weight 1 each).
// SameDaySplitGrouped: subset of SameDaySplit where at least one block is an SBP parallel group member.
// PJOKAfterDeadline: PJOK 2JP blocks ending after 10:50 (weight 3 each in total score).
type SoftBreakdown struct {
	SameDaySplit        int `json:"sameDaySplit"`
	SameDaySplitGrouped int `json:"sameDaySplitGrouped"`
	PJOKAfterDeadline   int `json:"pjokAfterDeadline"`
}

// GAProgressSnapshot is one observable progress point emitted during GA execution.
type GAProgressSnapshot struct {
	Generation           int         `json:"generation"`
	TotalGenerations     int         `json:"totalGenerations"`
	ProgressPercent      float64     `json:"progressPercent"`
	BestFitness          int         `json:"bestFitness"`
	BestViolations       int         `json:"bestViolations"`
	BestUnplaced         int         `json:"bestUnplaced"`
	ElapsedMs            int64       `json:"elapsedMs"`
	AvgFitness           int         `json:"avgFitness"`
	WorstFitness         int         `json:"worstFitness"`
	DiversityScore       float64     `json:"diversityScore"`
	StagnantGens         int         `json:"stagnantGens"`
	SAImprovements       int         `json:"saImprovements"`
	MutationHits         int         `json:"mutationHits"`
	FeasibleCount        int         `json:"feasibleCount"`
	BestDistanceFromSeed *int        `json:"bestDistanceFromSeed,omitempty"`
	Breakdown            GABreakdown `json:"breakdown"`
}

// InputStats describes the input domain size consumed by the scheduler.
type InputStats struct {
	ActiveAssignments int `json:"activeAssignments"`
	Blocks            int `json:"-"`
	Units             int `json:"-"`
	ActiveClasses     int `json:"activeClasses"`
	Teachers          int `json:"teachers"`
}

// ResultStats describes the final optimization result quality.
type ResultStats struct {
	EntriesGenerated int           `json:"entriesGenerated"`
	BestFitness      int           `json:"-"`
	Violations       int           `json:"violations"`
	Unplaced         int           `json:"unplaced"`
	Breakdown        GABreakdown   `json:"-"`
	SoftBreakdown    SoftBreakdown `json:"softBreakdown"`
}

// ScheduleMeta contains diagnostics and tuning context.
type ScheduleMeta struct {
	Input               InputStats  `json:"input"`
	DefaultGA           GAParams    `json:"-"`
	EffectiveGA         GAParams    `json:"ga"`
	DefaultSA           *SAParams   `json:"-"`
	EffectiveSA         *SAParams   `json:"sa,omitempty"`
	Result              ResultStats `json:"result"`
	TotalElapsedMs      int64       `json:"totalElapsedMs,omitempty"`
	LoopCount           int         `json:"loopCount,omitempty"`
	SeedWarningsChecked bool        `json:"-"`
	SeedWarnings        []string    `json:"seedWarnings,omitempty"`
}

// ScheduleGenerationResult is the full payload returned by the API.
type ScheduleGenerationResult struct {
	Entries  []ScheduleEntry      `json:"entries"`
	Meta     ScheduleMeta         `json:"meta"`
	Progress []GAProgressSnapshot `json:"progress,omitempty"`
}

// ScheduleDiffResult describes whether generated GA output matches the mapped real schedule.
type ScheduleDiffResult struct {
	Checked           bool   `json:"checked"`
	Reason            string `json:"reason,omitempty"`
	IsSame            bool   `json:"isSame"`
	GeneratedCount    int    `json:"generatedCount"`
	RealScheduleCount int    `json:"realScheduleCount"`
}

// ScheduleGenerateAndCompareResult returns GA generation details plus comparison output.
type ScheduleGenerateAndCompareResult struct {
	Generation *ScheduleGenerationResult `json:"generation"`
	Comparison ScheduleDiffResult        `json:"comparison"`
}

// RealScheduleValidationResult reports violation metrics when evaluating
// the mapped real schedule directly, without GA optimization.
type RealScheduleValidationResult struct {
	Entries []ScheduleEntry `json:"entries"`
	Meta    struct {
		Input        InputStats  `json:"input"`
		Result       ResultStats `json:"result"`
		SeedWarnings []string    `json:"seedWarnings,omitempty"`
		IsFeasible   bool        `json:"isFeasible"`
	} `json:"meta"`
}

// SATrialResult holds before/after metrics for one SA-only diagnostic trial.
type SATrialResult struct {
	Trial            int         `json:"trial"`
	Seed             int64       `json:"seed"`
	GreedyViolations int         `json:"greedyViolations"`
	GreedyUnplaced   int         `json:"greedyUnplaced"`
	GreedyBreakdown  GABreakdown `json:"greedyBreakdown"`
	FinalViolations  int         `json:"finalViolations"`
	FinalUnplaced    int         `json:"finalUnplaced"`
	FinalBreakdown   GABreakdown `json:"finalBreakdown"`
	ReachedZero      bool        `json:"reachedZero"`
}

// SADiagnosticResult is the full response from the SA-only diagnostic endpoint.
type SADiagnosticResult struct {
	Input   InputStats      `json:"input"`
	Trials  []SATrialResult `json:"trials"`
	Summary struct {
		TrialCount           int     `json:"trialCount"`
		SAIterationsPerTrial int     `json:"saIterationsPerTrial"`
		SuccessCount         int     `json:"successCount"`
		SuccessRate          float64 `json:"successRate"`
		MeanGreedyViolations float64 `json:"meanGreedyViolations"`
		MeanGreedyUnplaced   float64 `json:"meanGreedyUnplaced"`
		MeanFinalViolations  float64 `json:"meanFinalViolations"`
		MeanFinalUnplaced    float64 `json:"meanFinalUnplaced"`
		MinFinalViolations   int     `json:"minFinalViolations"`
		MinFinalUnplaced     int     `json:"minFinalUnplaced"`
	} `json:"summary"`
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

// SAParams contains tunable parameters for the SA phase of the hybrid.
type SAParams struct {
	InitialTemperature float64 `json:"initialTemperature"`
	CoolingRate        float64 `json:"coolingRate"`
	Iterations         int     `json:"iterations"`
	ProgressEvery      int     `json:"progressEvery"`
	Seed               int64   `json:"seed"`
	PerturbCount       int     `json:"perturbCount"`
	PerturbAfter       int     `json:"perturbAfter"`
}

// SAProgressSnapshot is one observable progress point emitted during SA execution.
type SAProgressSnapshot struct {
	Phase                 string  `json:"phase"` // always "sa"
	Iteration             int     `json:"iteration"`
	TotalIterations       int     `json:"totalIterations"`
	ProgressPercent       float64 `json:"progressPercent"`
	Temperature           float64 `json:"temperature"`
	CurrentUnplaced       int     `json:"currentUnplaced"`
	CurrentSoftViolations int     `json:"currentSoftViolations"`
	BestUnplaced          int     `json:"bestUnplaced"`
	BestSoftViolations    int     `json:"bestSoftViolations"`
	ElapsedMs             int64   `json:"elapsedMs"`
}

// GAPhaseResult carries a summary of what GA achieved, emitted as a phase_change event
// so the client knows SA is starting and what GA's final state was.
type GAPhaseResult struct {
	Unplaced       int `json:"unplaced"`
	SoftViolations int `json:"softViolations"`
	Generations    int `json:"generations"`
	ElapsedMs      int64 `json:"elapsedMs"`
}

// ReadableScheduleEntry is one timetable row with human-readable names instead of UUIDs.
type ReadableScheduleEntry struct {
	ClassName   string `json:"className"`
	SubjectName string `json:"subjectName"`
	TeacherName string `json:"teacherName"`
	Day         string `json:"day"`
	TimeStart   string `json:"timeStart"`
	TimeEnd     string `json:"timeEnd"`
}

// ReadableScheduleResult is the human-readable response from the v3/readable endpoint.
type ReadableScheduleResult struct {
	Entries []ReadableScheduleEntry `json:"entries"`
	Meta    ScheduleMeta            `json:"meta"`
}

// RunSummary is the metadata for one run inside a multi-run batch.
type RunSummary struct {
	Run  int          `json:"run"`
	Meta ScheduleMeta `json:"meta"`
}

// MultiRunResult is the response from the v3/multi-run endpoint.
// Contains only metadata per run — no schedule entries.
type MultiRunResult struct {
	Runs           int          `json:"runs"`
	TotalElapsedMs int64        `json:"totalElapsedMs"`
	Results        []RunSummary `json:"results"`
}

// GenerateHybridOptions carries options for the v3 hybrid GA+SA endpoint.
type GenerateHybridOptions struct {
	GA              GAParams
	SA              SAParams
	StagnationLimit   int  // GA stops after this many generations without improvement; 0 = disabled
	Restarts          int  // additional full GA+SA runs; 0 = single run
	LoopUntilFeasible bool // keep retrying until unplaced == 0; ignores Restarts limit
	MaxLoops          int  // max attempts when LoopUntilFeasible=true; 0 = 1000
	OnGAProgress    func(GAProgressSnapshot)
	OnGAComplete    func(GAPhaseResult) // fires when GA finishes, before SA starts
	OnSAProgress    func(SAProgressSnapshot)
}
