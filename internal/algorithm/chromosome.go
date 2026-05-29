package algorithm

import (
	"math/rand"
	"sort"

	"github.com/google/uuid"
)

// Gene is the scheduled placement for one MatrixBlock.
// The zero value (Day == "") means unplaced.
type Gene struct {
	Day       string
	StartSlot int
}

func (g Gene) IsPlaced() bool { return g.Day != "" }

// Chromosome holds one candidate schedule as an ordered slice of Genes.
// Position i in Genes corresponds to position i in the associated block list.
// The chromosome does not own the block list; callers supply it at every operation.
type Chromosome struct {
	genes []Gene
}

func NewChromosome(n int) Chromosome {
	return Chromosome{genes: make([]Gene, n)}
}

func (c Chromosome) Len() int       { return len(c.genes) }
func (c Chromosome) Get(i int) Gene { return c.genes[i] }

func (c *Chromosome) Set(i int, g Gene) { c.genes[i] = g }

func (c Chromosome) Clone() Chromosome {
	out := make([]Gene, len(c.genes))
	copy(out, c.genes)
	return Chromosome{genes: out}
}

const pjokDeadlineEndTime = "10:50"

// BuildCandidateIndex pre-computes all physically valid (Day, StartSlot) positions
// for every block. A position is valid when every slot in the window is non-blocked.
// PJOK 2JP blocks are hard-restricted to morning slots (ending ≤ 10:50).
// Feasibility confirmed: 18 valid morning slots per teacher, 8 classes per teacher.
func BuildCandidateIndex(blocks []MatrixBlock, pjokSubjectID uuid.UUID, daySlots DaySlots) map[uuid.UUID][]Gene {
	if daySlots == nil {
		daySlots = GenerateSlots()
	}

	baseByDuration := make(map[int][]Gene, 3)
	for d := 1; d <= 3; d++ {
		baseByDuration[d] = validCandidatesForDuration(d, daySlots)
	}

	pjok2JPCandidates := pjokMorningOnly(baseByDuration[2], daySlots)

	index := make(map[uuid.UUID][]Gene, len(blocks))
	for _, block := range blocks {
		if pjokSubjectID != uuid.Nil && block.SubjectID == pjokSubjectID && block.Duration == 2 {
			index[block.ID] = pjok2JPCandidates
		} else {
			index[block.ID] = baseByDuration[block.Duration]
		}
	}
	return index
}

// pjokMorningOnly filters candidates to only positions where the 2JP window ends
// at or before pjokDeadlineEndTime. These are the only valid slots for PJOK 2JP blocks.
func pjokMorningOnly(candidates []Gene, daySlots DaySlots) []Gene {
	slotEnd := make(map[string]map[int]string, len(daySlots))
	for day, slots := range daySlots {
		m := make(map[int]string, len(slots))
		for _, s := range slots {
			m[s.Index] = s.EndTime
		}
		slotEnd[day] = m
	}

	var out []Gene
	for _, g := range candidates {
		if end, ok := slotEnd[g.Day][g.StartSlot+1]; ok && end <= pjokDeadlineEndTime {
			out = append(out, g)
		}
	}
	return out
}

func validCandidatesForDuration(duration int, daySlots DaySlots) []Gene {
	var candidates []Gene
	for _, day := range MatrixDays {
		slots := daySlots[day]

		byIndex := make(map[int]Slot, len(slots))
		for _, s := range slots {
			byIndex[s.Index] = s
		}

		for _, s := range slots {
			if s.IsBlocked {
				continue
			}
			start := s.Index
			valid := true
			for offset := 0; offset < duration; offset++ {
				cur, ok := byIndex[start+offset]
				if !ok || cur.IsBlocked {
					valid = false
					break
				}
			}
			if valid {
				candidates = append(candidates, Gene{Day: day, StartSlot: start})
			}
		}
	}
	return candidates
}

// RandomChromosome creates a chromosome with each block assigned a random valid
// (Day, StartSlot) from candidateIndex. Blocks sharing a GroupKey are assigned
// the same gene so parallel classes are always scheduled together.
func RandomChromosome(blocks []MatrixBlock, candidateIndex map[uuid.UUID][]Gene, groups GroupIndex, rng *rand.Rand) Chromosome {
	c := NewChromosome(len(blocks))
	assigned := make(map[int]bool)
	for i, block := range blocks {
		if assigned[i] {
			continue
		}
		candidates := candidateIndex[block.ID]
		if len(candidates) == 0 {
			continue
		}
		gene := candidates[rng.Intn(len(candidates))]
		c.Set(i, gene)
		if block.GroupKey != nil {
			for _, j := range groups[*block.GroupKey] {
				c.Set(j, gene)
				assigned[j] = true
			}
		}
	}
	return c
}

// SmartChromosome creates a chromosome by greedily placing blocks in order of
// constraint tightness: PJOK 2JP blocks first (only 18 morning-only candidates),
// then all other blocks sorted by fewest candidates first.
// For each block, it picks a random candidate that is valid in the current matrix,
// so the result typically decodes to 0 unplaced blocks.
func SmartChromosome(blocks []MatrixBlock, candidateIndex map[uuid.UUID][]Gene, groups GroupIndex, daySlots DaySlots, pjokSubjectID uuid.UUID, rng *rand.Rand) Chromosome {
	type item struct {
		origIdx  int
		block    MatrixBlock
		priority int // lower = processed first
	}
	items := make([]item, len(blocks))
	for i, b := range blocks {
		p := len(candidateIndex[b.ID])
		if pjokSubjectID != uuid.Nil && b.SubjectID == pjokSubjectID && b.Duration == 2 {
			p = -1 // PJOK 2JP always first
		}
		items[i] = item{i, b, p}
	}
	sort.Slice(items, func(i, j int) bool {
		return items[i].priority < items[j].priority
	})

	matrix := NewScheduleMatrix(nil, nil, blocks, daySlots)
	matrix.EnableDayDiversity()
	if pjokSubjectID != uuid.Nil {
		matrix.ExcludeSubjectFromDayDiversity(pjokSubjectID)
	}

	c := NewChromosome(len(blocks))
	processedGroups := make(map[string]bool)

	for _, it := range items {
		i, block := it.origIdx, it.block

		if block.GroupKey != nil {
			if processedGroups[*block.GroupKey] {
				continue
			}
			processedGroups[*block.GroupKey] = true

			candidates := candidateIndex[block.ID]
			for _, pi := range rng.Perm(len(candidates)) {
				g := candidates[pi]
				allOk := true
				for _, j := range groups[*block.GroupKey] {
					if matrix.CanPlaceBlock(blocks[j].ID, g.Day, g.StartSlot) != nil {
						allOk = false
						break
					}
				}
				if allOk {
					for _, j := range groups[*block.GroupKey] {
						c.Set(j, g)
						_ = matrix.PlaceBlock(blocks[j].ID, g.Day, g.StartSlot)
					}
					break
				}
			}
			// If no conflict-free position exists, gene stays zero (unplaced).
		} else {
			candidates := candidateIndex[block.ID]
			for _, pi := range rng.Perm(len(candidates)) {
				g := candidates[pi]
				if matrix.CanPlaceBlock(block.ID, g.Day, g.StartSlot) == nil {
					c.Set(i, g)
					_ = matrix.PlaceBlock(block.ID, g.Day, g.StartSlot)
					break
				}
			}
		}
	}

	return c
}

// MutateGene replaces the gene at position i with a new random candidate for
// that block. Callers are responsible for propagating the gene to group mates.
func MutateGene(c *Chromosome, i int, block MatrixBlock, candidateIndex map[uuid.UUID][]Gene, rng *rand.Rand) {
	candidates := candidateIndex[block.ID]
	if len(candidates) == 0 {
		return
	}
	c.Set(i, candidates[rng.Intn(len(candidates))])
}

// UniformCrossover produces a child chromosome by independently picking each
// gene from parent a or parent b. Blocks sharing a GroupKey always inherit
// from the same parent so the group invariant is preserved.
func UniformCrossover(a, b Chromosome, blocks []MatrixBlock, groups GroupIndex, rng *rand.Rand) Chromosome {
	n := a.Len()
	child := NewChromosome(n)
	assigned := make(map[int]bool)
	for i := 0; i < n; i++ {
		if assigned[i] {
			continue
		}
		var gene Gene
		if rng.Intn(2) == 0 {
			gene = a.Get(i)
		} else {
			gene = b.Get(i)
		}
		child.Set(i, gene)
		if i < len(blocks) && blocks[i].GroupKey != nil {
			for _, j := range groups[*blocks[i].GroupKey] {
				child.Set(j, gene)
				assigned[j] = true
			}
		}
	}
	return child
}

// SoftViolationBreakdown holds per-category soft violation counts.
// PJOKAfterDeadline contributes 3× to the weighted total; all others contribute 1×.
type SoftViolationBreakdown struct {
	SameDaySplit        int // blocks of the same (class, subject) placed on the same day
	SameDaySplitGrouped int // subset of SameDaySplit where at least one block is an SBP group member
	PJOKAfterDeadline   int // PJOK 2JP blocks whose window ends after pjokDeadlineEndTime
}

func (bd SoftViolationBreakdown) Total() int {
	return bd.SameDaySplit
}

// BreakdownSoftViolations returns a per-category breakdown of soft violations.
// Use Total() for the weighted score used by the optimiser.
func BreakdownSoftViolations(matrix *ScheduleMatrix, blocks []MatrixBlock, pjokSubjectID uuid.UUID) SoftViolationBreakdown {
	var bd SoftViolationBreakdown

	// pre-compute which block IDs belong to a parallel group (SBP)
	groupedID := make(map[uuid.UUID]bool, len(blocks))
	for _, b := range blocks {
		if b.GroupKey != nil {
			groupedID[b.ID] = true
		}
	}

	// ── same-day split-subject penalty ───────────────────────────────────────
	type key struct{ classID, subjectID uuid.UUID }
	groups := make(map[key][]uuid.UUID)
	for _, b := range blocks {
		if pjokSubjectID != uuid.Nil && b.SubjectID == pjokSubjectID {
			continue
		}
		k := key{b.ClassID, b.SubjectID}
		groups[k] = append(groups[k], b.ID)
	}
	for _, ids := range groups {
		if len(ids) < 2 {
			continue
		}
		dayBlocks := make(map[string][]uuid.UUID)
		for _, id := range ids {
			if p, ok := matrix.Placement(id); ok {
				dayBlocks[p.Day] = append(dayBlocks[p.Day], id)
			}
		}
		for _, dayIDs := range dayBlocks {
			if len(dayIDs) <= 1 {
				continue
			}
			v := len(dayIDs) - 1
			bd.SameDaySplit += v
			for _, id := range dayIDs {
				if groupedID[id] {
					bd.SameDaySplitGrouped += v
					break
				}
			}
		}
	}

	// ── PJOK 2JP deadline penalty (weight 3) ─────────────────────────────────
	if pjokSubjectID != uuid.Nil {
		daySlots := GenerateSlots()
		slotEnd := make(map[string]map[int]string, len(daySlots))
		for day, slots := range daySlots {
			m := make(map[int]string, len(slots))
			for _, s := range slots {
				m[s.Index] = s.EndTime
			}
			slotEnd[day] = m
		}
		for _, b := range blocks {
			if b.SubjectID != pjokSubjectID || b.Duration != 2 {
				continue
			}
			p, ok := matrix.Placement(b.ID)
			if !ok {
				continue
			}
			if end := slotEnd[p.Day][p.StartSlot+1]; end > pjokDeadlineEndTime {
				bd.PJOKAfterDeadline++
			}
		}
	}

	return bd
}

// CountSoftViolations returns the weighted soft violation total.
// This is the hot-path version used by the optimiser inner loop — no extra allocations.
// Use BreakdownSoftViolations when you need per-category counts (reporting only).
func CountSoftViolations(matrix *ScheduleMatrix, blocks []MatrixBlock, pjokSubjectID uuid.UUID) int {
	violations := 0

	type key struct{ classID, subjectID uuid.UUID }
	groups := make(map[key][]uuid.UUID)
	for _, b := range blocks {
		if pjokSubjectID != uuid.Nil && b.SubjectID == pjokSubjectID {
			continue
		}
		k := key{b.ClassID, b.SubjectID}
		groups[k] = append(groups[k], b.ID)
	}
	for _, ids := range groups {
		if len(ids) < 2 {
			continue
		}
		dayCount := make(map[string]int)
		for _, id := range ids {
			if p, ok := matrix.Placement(id); ok {
				dayCount[p.Day]++
			}
		}
		for _, count := range dayCount {
			if count > 1 {
				violations += count - 1
			}
		}
	}

	return violations
}

// DecodeChromosome builds a ScheduleMatrix from a chromosome by placing blocks
// in order. Blocks whose gene is unplaced or that conflict with an already-placed
// block are skipped; the returned count is the number of such unplaced blocks.
func DecodeChromosome(c Chromosome, blocks []MatrixBlock, daySlots DaySlots, pjokSubjectID uuid.UUID) (*ScheduleMatrix, int) {
	matrix := NewScheduleMatrix(nil, nil, blocks, daySlots)
	matrix.EnableDayDiversity()
	if pjokSubjectID != uuid.Nil {
		matrix.ExcludeSubjectFromDayDiversity(pjokSubjectID)
	}
	unplaced := 0
	for i, block := range blocks {
		gene := c.Get(i)
		if !gene.IsPlaced() {
			unplaced++
			continue
		}
		if err := matrix.PlaceBlock(block.ID, gene.Day, gene.StartSlot); err != nil {
			unplaced++
		}
	}
	return matrix, unplaced
}
