package algorithm

import (
	"math/rand"
	"testing"

	"github.com/google/uuid"
)

// buildIndex is a test helper that builds a candidate index for the given blocks
// with no PJOK filtering (pjokSubjectID = uuid.Nil).
func buildIndex(blocks []MatrixBlock) map[uuid.UUID][]Gene {
	return BuildCandidateIndex(blocks, uuid.Nil, nil)
}

// blockOfDuration returns a plain MatrixBlock with the given duration for use in
// candidate index tests. Its SubjectID is distinct from any PJOK ID.
func blockOfDuration(d int) MatrixBlock {
	return MatrixBlock{ID: uuid.New(), ClassID: uuid.New(), SubjectID: uuid.New(), Duration: d}
}

// ── Gene ────────────────────────────────────────────────────────────────────

func TestGeneZeroValueIsUnplaced(t *testing.T) {
	var g Gene
	if g.IsPlaced() {
		t.Fatal("zero-value Gene should be unplaced")
	}
}

func TestGenePlacedWhenDaySet(t *testing.T) {
	g := Gene{Day: "monday", StartSlot: 1}
	if !g.IsPlaced() {
		t.Fatal("Gene with Day set should be placed")
	}
}

// ── Chromosome ──────────────────────────────────────────────────────────────

func TestNewChromosomeAllUnplaced(t *testing.T) {
	c := NewChromosome(5)
	if c.Len() != 5 {
		t.Fatalf("want len 5, got %d", c.Len())
	}
	for i := 0; i < c.Len(); i++ {
		if c.Get(i).IsPlaced() {
			t.Fatalf("gene %d should be unplaced after NewChromosome", i)
		}
	}
}

func TestChromosomeSetAndGet(t *testing.T) {
	c := NewChromosome(3)
	g := Gene{Day: "tuesday", StartSlot: 3}
	c.Set(1, g)
	if got := c.Get(1); got != g {
		t.Fatalf("got %+v, want %+v", got, g)
	}
}

func TestChromosomeCloneIsIndependent(t *testing.T) {
	c := NewChromosome(2)
	c.Set(0, Gene{Day: "monday", StartSlot: 1})

	clone := c.Clone()
	clone.Set(0, Gene{Day: "friday", StartSlot: 4})

	if c.Get(0).Day != "monday" {
		t.Fatalf("original chromosome mutated by clone modification")
	}
}

// ── BuildCandidateIndex ──────────────────────────────────────────────────────

func TestBuildCandidateIndexHasEntriesForEveryBlock(t *testing.T) {
	blocks := []MatrixBlock{
		blockOfDuration(1),
		blockOfDuration(2),
		blockOfDuration(3),
	}
	index := buildIndex(blocks)
	for _, block := range blocks {
		if len(index[block.ID]) == 0 {
			t.Fatalf("no candidates for block %s (duration %d)", block.ID, block.Duration)
		}
	}
}

func TestCandidatesExcludeBlockedSlots(t *testing.T) {
	blocks := []MatrixBlock{blockOfDuration(1), blockOfDuration(2), blockOfDuration(3)}
	index := buildIndex(blocks)
	for _, block := range blocks {
		for _, g := range index[block.ID] {
			if (g.Day == "monday" || g.Day == "friday") && g.StartSlot == 0 {
				t.Fatalf("duration %d: candidate starts at blocked slot 0 on %s", block.Duration, g.Day)
			}
		}
	}
}

func TestCandidatesDuration2AllNonBlockedSlots(t *testing.T) {
	block := blockOfDuration(2)
	index := buildIndex([]MatrixBlock{block})
	slots := GenerateSlots()
	byIndex := func(day string) map[int]Slot {
		m := make(map[int]Slot)
		for _, s := range slots[day] {
			m[s.Index] = s
		}
		return m
	}

	// Every candidate must reference two slots that both exist and are non-blocked.
	// Time breaks between slots are allowed — the school treats each JP as a
	// standalone period and a 2-JP block may span a break.
	for _, g := range index[block.ID] {
		bi := byIndex(g.Day)
		s0, ok0 := bi[g.StartSlot]
		s1, ok1 := bi[g.StartSlot+1]
		if !ok0 || !ok1 {
			t.Fatalf("duration-2 candidate %+v references missing slot", g)
		}
		if s0.IsBlocked || s1.IsBlocked {
			t.Fatalf("duration-2 candidate %+v references a blocked slot", g)
		}
	}
}

func TestCandidatesDuration3AllNonBlockedSlots(t *testing.T) {
	block := blockOfDuration(3)
	index := buildIndex([]MatrixBlock{block})
	slots := GenerateSlots()
	byIndex := func(day string) map[int]Slot {
		m := make(map[int]Slot)
		for _, s := range slots[day] {
			m[s.Index] = s
		}
		return m
	}

	// All three slots in every candidate must exist and be non-blocked.
	// Spanning a time break is intentional and allowed.
	for _, g := range index[block.ID] {
		bi := byIndex(g.Day)
		for offset := 0; offset < 3; offset++ {
			s, ok := bi[g.StartSlot+offset]
			if !ok {
				t.Fatalf("duration-3 candidate %+v references missing slot at offset %d", g, offset)
			}
			if s.IsBlocked {
				t.Fatalf("duration-3 candidate %+v references blocked slot at offset %d", g, offset)
			}
		}
	}
}

// Verify concrete valid start slots on Monday for duration 2.
// All non-blocked starts that fit within the 9-slot window are valid,
// including those that span the 09:10→09:30 and 11:30→11:45 breaks.
func TestValidStartSlotsMondayDuration2(t *testing.T) {
	block := blockOfDuration(2)
	index := buildIndex([]MatrixBlock{block})

	// Monday: slot 0 blocked, slots 1-8 available. Duration-2 needs start+1 ≤ 8.
	// Valid starts: 1,2,3,4,5,6,7 (slot 0 is blocked so can't start there).
	wantStarts := map[int]bool{1: true, 2: true, 3: true, 4: true, 5: true, 6: true, 7: true}
	got := map[int]bool{}
	for _, g := range index[block.ID] {
		if g.Day == "monday" {
			got[g.StartSlot] = true
		}
	}
	for s := range wantStarts {
		if !got[s] {
			t.Errorf("monday duration-2: expected start slot %d but it's missing", s)
		}
	}
	if got[0] {
		t.Errorf("monday duration-2: slot 0 should be excluded (blocked)")
	}
}

// ── PJOK candidate index ──────────────────────────────────────────────────────

// PJOK 2JP deadline is a hard constraint: BuildCandidateIndex must restrict
// PJOK 2JP blocks to morning positions only (ending ≤ 10:50).
// Non-PJOK blocks of the same duration retain all valid positions.
func TestPJOK2JPOnlyMorningCandidates(t *testing.T) {
	pjokSubjectID := uuid.New()
	otherSubjectID := uuid.New()

	pjokBlock := MatrixBlock{ID: uuid.New(), ClassID: uuid.New(), SubjectID: pjokSubjectID, Duration: 2}
	otherBlock := MatrixBlock{ID: uuid.New(), ClassID: uuid.New(), SubjectID: otherSubjectID, Duration: 2}
	blocks := []MatrixBlock{pjokBlock, otherBlock}

	daySlots := GenerateSlots()
	index := BuildCandidateIndex(blocks, pjokSubjectID, daySlots)

	// Build slot-end lookup for verification.
	slotEnd := make(map[string]map[int]string)
	for day, slots := range daySlots {
		m := make(map[int]string)
		for _, s := range slots {
			m[s.Index] = s.EndTime
		}
		slotEnd[day] = m
	}

	pjokCandidates := index[pjokBlock.ID]
	if len(pjokCandidates) == 0 {
		t.Fatal("PJOK 2JP block has no candidates — morning filter is too aggressive")
	}
	for _, g := range pjokCandidates {
		end, ok := slotEnd[g.Day][g.StartSlot+1]
		if !ok || end > pjokDeadlineEndTime {
			t.Errorf("PJOK 2JP candidate %s slot %d ends at %q, exceeds deadline %s",
				g.Day, g.StartSlot, end, pjokDeadlineEndTime)
		}
	}

	// Non-PJOK must have more candidates than PJOK (afternoon slots included).
	if len(index[otherBlock.ID]) <= len(pjokCandidates) {
		t.Errorf("non-PJOK duration-2 block should have more candidates than PJOK; got %d vs %d",
			len(index[otherBlock.ID]), len(pjokCandidates))
	}
}

// ── RandomChromosome ────────────────────────────────────────────────────────

func TestRandomChromosomeAllPlaced(t *testing.T) {
	blocks := []MatrixBlock{
		blockOfDuration(1),
		blockOfDuration(2),
		blockOfDuration(3),
	}
	index := buildIndex(blocks)
	rng := rand.New(rand.NewSource(42))
	c := RandomChromosome(blocks, index, GroupIndex{}, rng)

	for i, block := range blocks {
		if !c.Get(i).IsPlaced() {
			t.Fatalf("block %d (duration %d) was not placed by RandomChromosome", i, block.Duration)
		}
	}
}

func TestRandomChromosomeGeneWithinCandidateSet(t *testing.T) {
	block := blockOfDuration(2)
	blocks := []MatrixBlock{block}
	index := buildIndex(blocks)
	rng := rand.New(rand.NewSource(99))

	candidateSet := make(map[Gene]bool, len(index[block.ID]))
	for _, cand := range index[block.ID] {
		candidateSet[cand] = true
	}

	for trial := 0; trial < 50; trial++ {
		c := RandomChromosome(blocks, index, GroupIndex{}, rng)
		g := c.Get(0)
		if !candidateSet[g] {
			t.Fatalf("trial %d: gene %+v is not in the valid candidate set for duration 2", trial, g)
		}
	}
}

// ── MutateGene ──────────────────────────────────────────────────────────────

func TestMutateGeneChangesPlacement(t *testing.T) {
	block := blockOfDuration(1)
	index := buildIndex([]MatrixBlock{block})
	rng := rand.New(rand.NewSource(7))

	c := NewChromosome(1)
	c.Set(0, Gene{Day: "monday", StartSlot: 1})

	changed := false
	for trial := 0; trial < 100; trial++ {
		c2 := c.Clone()
		MutateGene(&c2, 0, block, index, rng)
		if c2.Get(0) != c.Get(0) {
			changed = true
			break
		}
	}
	if !changed {
		t.Fatal("MutateGene produced the same gene in 100 attempts — likely broken")
	}
}

// ── UniformCrossover ────────────────────────────────────────────────────────

func TestUniformCrossoverGeneFromParents(t *testing.T) {
	n := 10
	a := NewChromosome(n)
	b := NewChromosome(n)
	for i := 0; i < n; i++ {
		a.Set(i, Gene{Day: "monday", StartSlot: i + 1})
		b.Set(i, Gene{Day: "friday", StartSlot: i + 1})
	}

	rng := rand.New(rand.NewSource(13))
	child := UniformCrossover(a, b, nil, GroupIndex{}, rng)

	for i := 0; i < n; i++ {
		g := child.Get(i)
		fromA := a.Get(i)
		fromB := b.Get(i)
		if g != fromA && g != fromB {
			t.Fatalf("gene %d (%+v) came from neither parent A (%+v) nor B (%+v)", i, g, fromA, fromB)
		}
	}
}

func TestUniformCrossoverUsesFromBothParents(t *testing.T) {
	n := 20
	a := NewChromosome(n)
	b := NewChromosome(n)
	for i := 0; i < n; i++ {
		a.Set(i, Gene{Day: "monday", StartSlot: 1})
		b.Set(i, Gene{Day: "friday", StartSlot: 1})
	}

	rng := rand.New(rand.NewSource(21))
	child := UniformCrossover(a, b, nil, GroupIndex{}, rng)

	sawMonday, sawFriday := false, false
	for i := 0; i < n; i++ {
		if child.Get(i).Day == "monday" {
			sawMonday = true
		}
		if child.Get(i).Day == "friday" {
			sawFriday = true
		}
	}
	if !sawMonday || !sawFriday {
		t.Fatal("UniformCrossover should draw genes from both parents across 20 positions")
	}
}

// ── DecodeChromosome ────────────────────────────────────────────────────────

func TestDecodeChromosomePlacesAllNonConflicting(t *testing.T) {
	classA := uuid.New()
	classB := uuid.New()
	teacherA := uuid.New()
	teacherB := uuid.New()
	blocks := []MatrixBlock{
		{ID: uuid.New(), ClassID: classA, TeacherID: &teacherA, Duration: 1},
		{ID: uuid.New(), ClassID: classB, TeacherID: &teacherB, Duration: 1},
	}

	c := NewChromosome(2)
	c.Set(0, Gene{Day: "monday", StartSlot: 1})
	c.Set(1, Gene{Day: "monday", StartSlot: 1}) // different classes/teachers, no conflict

	matrix, unplaced := DecodeChromosome(c, blocks, nil, uuid.Nil)
	if unplaced != 0 {
		t.Fatalf("expected 0 unplaced, got %d", unplaced)
	}
	for _, block := range blocks {
		if _, ok := matrix.Placement(block.ID); !ok {
			t.Fatalf("block %s was not placed", block.ID)
		}
	}
}

func TestDecodeChromosomeClassConflictCountsUnplaced(t *testing.T) {
	classID := uuid.New()
	blocks := []MatrixBlock{
		{ID: uuid.New(), ClassID: classID, Duration: 1},
		{ID: uuid.New(), ClassID: classID, Duration: 1},
	}

	c := NewChromosome(2)
	c.Set(0, Gene{Day: "monday", StartSlot: 1})
	c.Set(1, Gene{Day: "monday", StartSlot: 1}) // same class, same slot → conflict

	_, unplaced := DecodeChromosome(c, blocks, nil, uuid.Nil)
	if unplaced != 1 {
		t.Fatalf("expected 1 unplaced from class conflict, got %d", unplaced)
	}
}

func TestDecodeChromosomeTeacherConflictCountsUnplaced(t *testing.T) {
	classA := uuid.New()
	classB := uuid.New()
	teacher := uuid.New()
	blocks := []MatrixBlock{
		{ID: uuid.New(), ClassID: classA, TeacherID: &teacher, Duration: 1},
		{ID: uuid.New(), ClassID: classB, TeacherID: &teacher, Duration: 1},
	}

	c := NewChromosome(2)
	c.Set(0, Gene{Day: "tuesday", StartSlot: 2})
	c.Set(1, Gene{Day: "tuesday", StartSlot: 2}) // same teacher, same slot → conflict

	_, unplaced := DecodeChromosome(c, blocks, nil, uuid.Nil)
	if unplaced != 1 {
		t.Fatalf("expected 1 unplaced from teacher conflict, got %d", unplaced)
	}
}

func TestDecodeChromosomeUnplacedGeneCountsAsUnplaced(t *testing.T) {
	blocks := []MatrixBlock{
		{ID: uuid.New(), ClassID: uuid.New(), Duration: 1},
	}
	c := NewChromosome(1) // gene stays at zero value (unplaced)

	_, unplaced := DecodeChromosome(c, blocks, nil, uuid.Nil)
	if unplaced != 1 {
		t.Fatalf("expected 1 unplaced for zero-value gene, got %d", unplaced)
	}
}

func TestDecodeChromosomePlacementMatchesGene(t *testing.T) {
	block := MatrixBlock{ID: uuid.New(), ClassID: uuid.New(), Duration: 2}
	blocks := []MatrixBlock{block}

	c := NewChromosome(1)
	c.Set(0, Gene{Day: "wednesday", StartSlot: 3})

	matrix, unplaced := DecodeChromosome(c, blocks, nil, uuid.Nil)
	if unplaced != 0 {
		t.Fatalf("expected 0 unplaced, got %d", unplaced)
	}
	p, ok := matrix.Placement(block.ID)
	if !ok {
		t.Fatal("block not placed")
	}
	if p.Day != "wednesday" || p.StartSlot != 3 {
		t.Fatalf("placement mismatch: got day=%s slot=%d", p.Day, p.StartSlot)
	}
}
