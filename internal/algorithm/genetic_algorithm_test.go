package algorithm

import (
	"testing"

	"github.com/google/uuid"
)

// ── helpers ──────────────────────────────────────────────────────────────────

func newTeacherID() *uuid.UUID { id := uuid.New(); return &id }

func smallGACfg() GAConfig {
	cfg := DefaultGAConfig()
	cfg.PopulationSize = 50
	cfg.Generations = 300
	cfg.Seed = 42
	cfg.OnProgress = nil
	return cfg
}

// ── DefaultGAConfig ──────────────────────────────────────────────────────────

func TestDefaultGAConfigIsValid(t *testing.T) {
	cfg := DefaultGAConfig()
	if cfg.PopulationSize <= 0 {
		t.Fatal("PopulationSize must be positive")
	}
	if cfg.Generations <= 0 {
		t.Fatal("Generations must be positive")
	}
	if cfg.MutationRate <= 0 || cfg.MutationRate >= 1 {
		t.Fatalf("MutationRate %f is not in (0,1)", cfg.MutationRate)
	}
	if cfg.EliteCount <= 0 {
		t.Fatal("EliteCount must be positive")
	}
	if cfg.TournamentSize <= 0 {
		t.Fatal("TournamentSize must be positive")
	}
}

// ── Early stop ───────────────────────────────────────────────────────────────

// When every block has a unique class and a unique teacher, any random
// chromosome is conflict-free. The GA should stop at generation 0 (initial
// population already has a valid solution).
func TestRunGAStopsEarlyWhenInitialPopulationIsValid(t *testing.T) {
	blocks := make([]MatrixBlock, 6)
	for i := range blocks {
		blocks[i] = MatrixBlock{
			ID:        uuid.New(),
			ClassID:   uuid.New(),
			TeacherID: newTeacherID(),
			SubjectID: uuid.New(),
			Duration:  1,
		}
	}

	index := BuildCandidateIndex(blocks, uuid.Nil, nil)
	cfg := smallGACfg()

	result := RunGA(blocks, index, nil, cfg)

	if result.Unplaced != 0 {
		t.Fatalf("expected 0 unplaced, got %d", result.Unplaced)
	}
	if result.Generations != 0 {
		t.Fatalf("expected early stop at generation 0, ran %d generations", result.Generations)
	}
}

// ── Convergence ──────────────────────────────────────────────────────────────

// Three teachers each teach two classes, creating teacher and class conflicts
// that the GA must resolve. The problem is small enough that the GA reliably
// reaches zero unplaced within a few generations.
//
//	Teacher1 → ClassA, ClassB
//	Teacher2 → ClassA, ClassC
//	Teacher3 → ClassB, ClassC
func TestRunGAConvergesOnSmallSchedule(t *testing.T) {
	classA, classB, classC := uuid.New(), uuid.New(), uuid.New()
	t1, t2, t3 := newTeacherID(), newTeacherID(), newTeacherID()
	subject := uuid.New()

	blocks := []MatrixBlock{
		{ID: uuid.New(), ClassID: classA, TeacherID: t1, SubjectID: subject, Duration: 2},
		{ID: uuid.New(), ClassID: classB, TeacherID: t1, SubjectID: subject, Duration: 2},
		{ID: uuid.New(), ClassID: classA, TeacherID: t2, SubjectID: subject, Duration: 2},
		{ID: uuid.New(), ClassID: classC, TeacherID: t2, SubjectID: subject, Duration: 2},
		{ID: uuid.New(), ClassID: classB, TeacherID: t3, SubjectID: subject, Duration: 2},
		{ID: uuid.New(), ClassID: classC, TeacherID: t3, SubjectID: subject, Duration: 2},
	}

	index := BuildCandidateIndex(blocks, uuid.Nil, nil)
	result := RunGA(blocks, index, nil, smallGACfg())

	if result.Unplaced != 0 {
		t.Fatalf("GA did not converge: %d blocks unplaced after %d generations",
			result.Unplaced, result.Generations)
	}
}

// A more constrained version: 4 teachers × 4 classes = 16 blocks, all
// duration 2. Each teacher must spread 4 blocks across distinct slots.
func TestRunGAConvergesOnMediumSchedule(t *testing.T) {
	classes := make([]uuid.UUID, 4)
	for i := range classes {
		classes[i] = uuid.New()
	}
	teachers := make([]*uuid.UUID, 4)
	for i := range teachers {
		teachers[i] = newTeacherID()
	}
	subject := uuid.New()

	blocks := make([]MatrixBlock, 0, 16)
	for _, teacher := range teachers {
		for _, class := range classes {
			blocks = append(blocks, MatrixBlock{
				ID:        uuid.New(),
				ClassID:   class,
				TeacherID: teacher,
				SubjectID: subject,
				Duration:  2,
			})
		}
	}

	index := BuildCandidateIndex(blocks, uuid.Nil, nil)

	cfg := smallGACfg()
	cfg.PopulationSize = 100
	cfg.Generations = 500

	result := RunGA(blocks, index, nil, cfg)

	if result.Unplaced != 0 {
		t.Fatalf("GA did not converge: %d blocks unplaced after %d generations",
			result.Unplaced, result.Generations)
	}
}

// ── Result correctness ───────────────────────────────────────────────────────

func TestRunGAResultMatrixPassesIntegrity(t *testing.T) {
	classA, classB := uuid.New(), uuid.New()
	teacher := newTeacherID()
	subject := uuid.New()

	blocks := []MatrixBlock{
		{ID: uuid.New(), ClassID: classA, TeacherID: teacher, SubjectID: subject, Duration: 2},
		{ID: uuid.New(), ClassID: classB, TeacherID: teacher, SubjectID: subject, Duration: 2},
	}

	index := BuildCandidateIndex(blocks, uuid.Nil, nil)
	result := RunGA(blocks, index, nil, smallGACfg())

	if result.Unplaced != 0 {
		t.Fatalf("expected 0 unplaced, got %d", result.Unplaced)
	}
	if err := result.Matrix.ValidateIntegrity(); err != nil {
		t.Fatalf("matrix integrity failed: %v", err)
	}
}

func TestRunGAUnplacedMatchesMatrix(t *testing.T) {
	classA, classB := uuid.New(), uuid.New()
	teacher := newTeacherID()
	subject := uuid.New()

	blocks := []MatrixBlock{
		{ID: uuid.New(), ClassID: classA, TeacherID: teacher, SubjectID: subject, Duration: 1},
		{ID: uuid.New(), ClassID: classB, TeacherID: teacher, SubjectID: subject, Duration: 1},
	}

	index := BuildCandidateIndex(blocks, uuid.Nil, nil)
	result := RunGA(blocks, index, nil, smallGACfg())

	// Re-decode the returned chromosome independently and compare unplaced count.
	_, reDecoded := DecodeChromosome(result.Chromosome, blocks, nil, uuid.Nil)
	if reDecoded != result.Unplaced {
		t.Fatalf("result.Unplaced %d does not match re-decoded count %d", result.Unplaced, reDecoded)
	}
}

// ── Progress callback ─────────────────────────────────────────────────────────

func TestRunGAProgressCallbackIsInvoked(t *testing.T) {
	blocks := make([]MatrixBlock, 4)
	for i := range blocks {
		blocks[i] = MatrixBlock{
			ID:        uuid.New(),
			ClassID:   uuid.New(),
			TeacherID: newTeacherID(),
			SubjectID: uuid.New(),
			Duration:  1,
		}
	}

	index := BuildCandidateIndex(blocks, uuid.Nil, nil)

	called := 0
	cfg := smallGACfg()
	cfg.ProgressEvery = 1
	cfg.OnProgress = func(p GAProgress) {
		called++
		if p.BestUnplaced < 0 {
			t.Errorf("BestUnplaced must not be negative")
		}
		if p.AvgUnplaced < 0 {
			t.Errorf("AvgUnplaced must not be negative")
		}
	}

	RunGA(blocks, index, nil, cfg)

	if called == 0 {
		t.Fatal("OnProgress was never called")
	}
}

func TestRunGAProgressBestUnplacedNeverIncreases(t *testing.T) {
	classA, classB, classC := uuid.New(), uuid.New(), uuid.New()
	t1, t2 := newTeacherID(), newTeacherID()
	subject := uuid.New()

	blocks := []MatrixBlock{
		{ID: uuid.New(), ClassID: classA, TeacherID: t1, SubjectID: subject, Duration: 2},
		{ID: uuid.New(), ClassID: classB, TeacherID: t1, SubjectID: subject, Duration: 2},
		{ID: uuid.New(), ClassID: classA, TeacherID: t2, SubjectID: subject, Duration: 2},
		{ID: uuid.New(), ClassID: classC, TeacherID: t2, SubjectID: subject, Duration: 2},
	}

	index := BuildCandidateIndex(blocks, uuid.Nil, nil)

	prev := -1
	cfg := smallGACfg()
	cfg.ProgressEvery = 1
	cfg.OnProgress = func(p GAProgress) {
		if prev == -1 {
			prev = p.BestUnplaced
			return
		}
		if p.BestUnplaced > prev {
			t.Errorf("BestUnplaced increased from %d to %d — elitism is broken", prev, p.BestUnplaced)
		}
		prev = p.BestUnplaced
	}

	RunGA(blocks, index, nil, cfg)
}
