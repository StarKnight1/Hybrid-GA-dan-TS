package schedule

// ScheduleEntry adalah satu baris jadwal hasil generate sebesar 1 JP (40 menit).
type ScheduleEntry struct {
	TeacherID   *uint  `json:"teacherId,omitempty"`
	SubjectID   uint   `json:"subjectId"`
	ClassID     uint   `json:"classId"`
	SubjectName string `json:"subjectName"`
	TeacherName string `json:"teacherName,omitempty"`
	ClassName   string `json:"className"`
	Day         string `json:"day"`
	TimeStart   string `json:"timeStart"`
	TimeEnd     string `json:"timeEnd"`
}

// GAParams menyimpan parameter yang dapat diatur untuk generate jadwal.
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

// GABreakdown mencerminkan penghitung rincian kendala keras.
type GABreakdown struct {
	ClassConflicts    int `json:"classConflicts"`
	TeacherConflicts  int `json:"teacherConflicts"`
	SiblingViolations int `json:"siblingViolations"`
}

// SoftBreakdown merinci pelanggaran ringan per kategori.
// SameDaySplit: blok (kelas, mapel) yang sama ditempatkan pada hari yang sama (bobot 1 masing-masing).
// SameDaySplitGrouped: subset SameDaySplit yang melibatkan anggota grup paralel SBP.
// PJOKAfterDeadline: blok PJOK 2JP yang selesai setelah 10:50 (bobot 3 masing-masing dalam total skor).
type SoftBreakdown struct {
	SameDaySplit        int `json:"sameDaySplit"`
	SameDaySplitGrouped int `json:"sameDaySplitGrouped"`
	PJOKAfterDeadline   int `json:"pjokAfterDeadline"`
}

// GAProgressSnapshot adalah satu titik progres yang dapat diamati selama eksekusi GA.
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

// InputStats mendeskripsikan ukuran domain input yang digunakan oleh penjadwal.
type InputStats struct {
	ActiveAssignments int `json:"activeAssignments"`
	Blocks            int `json:"-"`
	Units             int `json:"-"`
	ActiveClasses     int `json:"activeClasses"`
	Teachers          int `json:"teachers"`
}

// ResultStats mendeskripsikan kualitas hasil optimasi akhir.
type ResultStats struct {
	EntriesGenerated int           `json:"entriesGenerated"`
	BestFitness      int           `json:"-"`
	Violations       int           `json:"violations"`
	Unplaced         int           `json:"unplaced"`
	Breakdown        GABreakdown   `json:"-"`
	SoftBreakdown    SoftBreakdown `json:"softBreakdown"`
}

// ScheduleMeta menyimpan diagnostik dan konteks penyetelan.
type ScheduleMeta struct {
	Input               InputStats  `json:"input"`
	DefaultGA           GAParams    `json:"-"`
	EffectiveGA         GAParams    `json:"ga"`
	DefaultTS           *TSParams   `json:"-"`
	EffectiveTS         *TSParams   `json:"ts,omitempty"`
	Result              ResultStats `json:"result"`
	TotalElapsedMs      int64       `json:"totalElapsedMs,omitempty"`
	LoopCount           int         `json:"loopCount,omitempty"`
	SeedWarningsChecked bool        `json:"-"`
	SeedWarnings        []string    `json:"seedWarnings,omitempty"`
}

// ScheduleGenerationResult adalah payload lengkap yang dikembalikan oleh API.
type ScheduleGenerationResult struct {
	Entries  []ScheduleEntry      `json:"entries"`
	Meta     ScheduleMeta         `json:"meta"`
	Progress []GAProgressSnapshot `json:"progress,omitempty"`
}

// GAParameterSpec mendokumentasikan satu parameter GA yang dapat diatur.
type GAParameterSpec struct {
	Name        string `json:"name"`
	Type        string `json:"type"`
	Default     string `json:"default"`
	Min         string `json:"min"`
	Max         string `json:"max"`
	Description string `json:"description"`
}

// TSParams menyimpan parameter yang dapat diatur untuk fase TS pada hybrid.
type TSParams struct {
	TabuTenure    int   `json:"tabuTenure"`
	Iterations    int   `json:"iterations"`
	ProgressEvery int   `json:"progressEvery"`
	Seed          int64 `json:"seed"`
	PerturbCount  int   `json:"perturbCount"`
	PerturbAfter  int   `json:"perturbAfter"`
}

// TSProgressSnapshot adalah satu titik progres yang dapat diamati selama eksekusi TS.
type TSProgressSnapshot struct {
	Phase                 string  `json:"phase"` // selalu "ts"
	Iteration             int     `json:"iteration"`
	TotalIterations       int     `json:"totalIterations"`
	ProgressPercent       float64 `json:"progressPercent"`
	TabuListSize          int     `json:"tabuListSize"`
	CurrentUnplaced       int     `json:"currentUnplaced"`
	CurrentSoftViolations int     `json:"currentSoftViolations"`
	BestUnplaced          int     `json:"bestUnplaced"`
	BestSoftViolations    int     `json:"bestSoftViolations"`
	ElapsedMs             int64   `json:"elapsedMs"`
}

// GAPhaseResult membawa ringkasan hasil GA, dikirim sebagai event phase_change
// sehingga klien mengetahui TS akan dimulai dan kondisi akhir GA.
type GAPhaseResult struct {
	Unplaced       int   `json:"unplaced"`
	SoftViolations int   `json:"softViolations"`
	Generations    int   `json:"generations"`
	ElapsedMs      int64 `json:"elapsedMs"`
}

// ReadableScheduleEntry adalah satu baris jadwal dengan nama yang mudah dibaca manusia.
type ReadableScheduleEntry struct {
	ClassName   string `json:"className"`
	SubjectName string `json:"subjectName"`
	TeacherName string `json:"teacherName"`
	Day         string `json:"day"`
	TimeStart   string `json:"timeStart"`
	TimeEnd     string `json:"timeEnd"`
}

// ReadableScheduleResult adalah respons yang mudah dibaca dari endpoint v3/readable.
type ReadableScheduleResult struct {
	Entries []ReadableScheduleEntry `json:"entries"`
	Meta    ScheduleMeta            `json:"meta"`
}

// RunSummary adalah metadata untuk satu run dalam batch multi-run.
type RunSummary struct {
	Run  int          `json:"run"`
	Meta ScheduleMeta `json:"meta"`
}

// MultiRunResult adalah respons dari endpoint v3/multi-run.
// Hanya berisi metadata per run — tanpa entri jadwal.
type MultiRunResult struct {
	Runs           int          `json:"runs"`
	TotalElapsedMs int64        `json:"totalElapsedMs"`
	Results        []RunSummary `json:"results"`
}

// GenerateHybridOptions membawa opsi untuk endpoint hybrid GA+TS v3.
type GenerateHybridOptions struct {
	GA                GAParams
	TS                TSParams
	StagnationLimit   int  // GA berhenti setelah N generasi tanpa peningkatan; 0 = nonaktif
	Restarts          int  // jumlah run GA+TS tambahan; 0 = satu run saja
	LoopUntilFeasible bool // terus mencoba sampai unplaced == 0; mengabaikan batas Restarts
	MaxLoops          int  // maks percobaan saat LoopUntilFeasible=true; 0 = 1000
	OnGAProgress      func(GAProgressSnapshot)
	OnGAComplete      func(GAPhaseResult) // dipanggil saat GA selesai, sebelum TS dimulai
	OnTSProgress      func(TSProgressSnapshot)
}
