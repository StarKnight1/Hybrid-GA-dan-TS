package algorithm

import (
	"math/rand"
	"sort"
)

// Gene is the scheduled placement for one MatrixBlock.
// The zero value (Day == "") means the block is not yet placed.
type Gene struct {
	Day       string
	StartSlot int
}

func (g Gene) IsPlaced() bool { return g.Day != "" }

// Chromosome holds one candidate schedule as an ordered slice of Genes.
// Index i in the gene slice corresponds to index i in the associated block slice.
// The chromosome does not own the block slice; callers supply it at every operation.
type Chromosome struct {
	genes []Gene
}

func NewChromosome(n int) Chromosome {
	return Chromosome{genes: make([]Gene, n)}
}

func (c Chromosome) Len() int        { return len(c.genes) }
func (c Chromosome) Get(i int) Gene  { return c.genes[i] }
func (c *Chromosome) Set(i int, g Gene) { c.genes[i] = g }

func (c Chromosome) Clone() Chromosome {
	cp := make([]Gene, len(c.genes))
	copy(cp, c.genes)
	return Chromosome{genes: cp}
}

// pjokCutoffTime is the latest allowed end time for a PJOK 2JP block.
const pjokCutoffTime = "10:50"

// sortEntry is used to order blocks by placement difficulty before building chromosomes.
type sortEntry struct {
	origIdx int
	block   MatrixBlock
	weight  int // lower weight = processed earlier (tighter constraint)
}

// sortByWeight sorts entries ascending by weight in place.
func sortByWeight(entries []sortEntry) {
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].weight < entries[j].weight
	})
}

// ── Soft violation scoring ────────────────────────────────────────────────────

// PenaltyBreakdown holds per-category soft penalty counts for one schedule.
// PJOKOvertime contributes 3× to the weighted optimisation score; others contribute 1×.
type PenaltyBreakdown struct {
	DaySplitCount      int // same-day blocks of the same (class, subject)
	DaySplitGroupCount int // subset of DaySplitCount involving SBP parallel group members
	PJOKOvertime       int // PJOK 2JP blocks ending after pjokCutoffTime
}

// Total returns the unweighted penalty count (sum of all categories without PJOK multiplier).
func (bd PenaltyBreakdown) Total() int {
	return bd.DaySplitCount
}

// BreakdownSoftViolations returns a per-category soft penalty breakdown for a decoded schedule.
// Use Total() to retrieve the weighted optimisation score.
func BreakdownSoftViolations(matrix *ScheduleMatrix, blocks []MatrixBlock, pjokSubjectID uint) PenaltyBreakdown {
	var bd PenaltyBreakdown

	// mark which block IDs are parallel group members
	isGrouped := make(map[uint]bool, len(blocks))
	for _, b := range blocks {
		if b.GroupKey != nil {
			isGrouped[b.ID] = true
		}
	}

	// same-day split: count how many blocks of each (class, subject) land on the same day
	classSubjectBlocks := make(map[courseDayKey][]uint)
	for _, b := range blocks {
		if pjokSubjectID != 0 && b.SubjectID == pjokSubjectID {
			continue
		}
		k := courseDayKey{b.ClassID, b.SubjectID}
		classSubjectBlocks[k] = append(classSubjectBlocks[k], b.ID)
	}
	for _, ids := range classSubjectBlocks {
		if len(ids) < 2 {
			continue
		}
		perDay := make(map[string][]uint)
		for _, id := range ids {
			if rec, ok := matrix.Placement(id); ok {
				perDay[rec.Day] = append(perDay[rec.Day], id)
			}
		}
		for _, dayIDs := range perDay {
			if len(dayIDs) <= 1 {
				continue
			}
			excess := len(dayIDs) - 1
			bd.DaySplitCount += excess
			for _, id := range dayIDs {
				if isGrouped[id] {
					bd.DaySplitGroupCount += excess
					break
				}
			}
		}
	}

	// PJOK 2JP overtime: blocks ending after pjokCutoffTime carry a weight-3 penalty
	if pjokSubjectID != 0 {
		periods := GenerateSlots()
		endTimeAt := make(map[string]map[int]string, len(periods))
		for day, slots := range periods {
			m := make(map[int]string, len(slots))
			for _, s := range slots {
				m[s.Index] = s.EndTime
			}
			endTimeAt[day] = m
		}
		for _, b := range blocks {
			if b.SubjectID != pjokSubjectID || b.Duration != 2 {
				continue
			}
			rec, ok := matrix.Placement(b.ID)
			if !ok {
				continue
			}
			if endTime := endTimeAt[rec.Day][rec.StartSlot+1]; endTime > pjokCutoffTime {
				bd.PJOKOvertime++
			}
		}
	}

	return bd
}

// CountSoftViolations computes the weighted soft violation total for a decoded matrix.
// This is the hot-path version used in the GA inner loop — minimal allocations.
// Call BreakdownSoftViolations only for reporting.
func CountSoftViolations(matrix *ScheduleMatrix, blocks []MatrixBlock, pjokSubjectID uint) int {
	total := 0

	classSubjectBlocks := make(map[courseDayKey][]uint)
	for _, b := range blocks {
		if pjokSubjectID != 0 && b.SubjectID == pjokSubjectID {
			continue
		}
		k := courseDayKey{b.ClassID, b.SubjectID}
		classSubjectBlocks[k] = append(classSubjectBlocks[k], b.ID)
	}
	for _, ids := range classSubjectBlocks {
		if len(ids) < 2 {
			continue
		}
		hitPerDay := make(map[string]int)
		for _, id := range ids {
			if rec, ok := matrix.Placement(id); ok {
				hitPerDay[rec.Day]++
			}
		}
		for _, cnt := range hitPerDay {
			if cnt > 1 {
				total += cnt - 1
			}
		}
	}

	return total
}

// ── Candidate index ───────────────────────────────────────────────────────────

// BuildCandidateIndex pre-computes all physically valid (Day, StartSlot) positions
// for every block. A position is valid when all slots in the duration window are unblocked.
// PJOK 2JP blocks are restricted to morning slots ending at or before pjokCutoffTime.
func BuildCandidateIndex(blocks []MatrixBlock, pjokSubjectID uint, daySlots DaySlots) map[uint][]Gene {
	if daySlots == nil {
		daySlots = GenerateSlots()
	}
	perDuration := make(map[int][]Gene, 3)
	for d := 1; d <= 3; d++ {
		perDuration[d] = candidatesForDuration(d, daySlots)
	}
	morningSlots := filterMorningSlots(perDuration[2], daySlots)

	index := make(map[uint][]Gene, len(blocks))
	for _, block := range blocks {
		if pjokSubjectID != 0 && block.SubjectID == pjokSubjectID && block.Duration == 2 {
			index[block.ID] = morningSlots
		} else {
			index[block.ID] = perDuration[block.Duration]
		}
	}
	return index
}

// filterMorningSlots keeps only 2JP candidates whose window ends at or before pjokCutoffTime.
func filterMorningSlots(candidates []Gene, daySlots DaySlots) []Gene {
	endAt := make(map[string]map[int]string, len(daySlots))
	for day, slots := range daySlots {
		m := make(map[int]string, len(slots))
		for _, s := range slots {
			m[s.Index] = s.EndTime
		}
		endAt[day] = m
	}
	var result []Gene
	for _, g := range candidates {
		if t, ok := endAt[g.Day][g.StartSlot+1]; ok && t <= pjokCutoffTime {
			result = append(result, g)
		}
	}
	return result
}

// candidatesForDuration enumerates all valid start positions for a block of the given duration.
func candidatesForDuration(duration int, daySlots DaySlots) []Gene {
	var result []Gene
	for _, day := range MatrixDays {
		slots := daySlots[day]
		byIdx := make(map[int]Slot, len(slots))
		for _, s := range slots {
			byIdx[s.Index] = s
		}
		for _, s := range slots {
			if s.IsBlocked {
				continue
			}
			startIdx := s.Index
			fits := true
			for offset := 0; offset < duration; offset++ {
				next, ok := byIdx[startIdx+offset]
				if !ok || next.IsBlocked {
					fits = false
					break
				}
			}
			if fits {
				result = append(result, Gene{Day: day, StartSlot: startIdx})
			}
		}
	}
	return result
}

// ── Chromosome construction ───────────────────────────────────────────────────

// DecodeChromosome translates a chromosome into a ScheduleMatrix by placing each block
// at its encoded (Day, StartSlot). Blocks that are unplaced or that conflict with
// already-placed blocks are skipped; the returned integer counts those failures.
func DecodeChromosome(c Chromosome, blocks []MatrixBlock, daySlots DaySlots, pjokSubjectID uint) (*ScheduleMatrix, int) {
	grid := NewScheduleMatrix(nil, nil, blocks, daySlots)
	grid.EnableDayDiversity()
	if pjokSubjectID != 0 {
		grid.ExcludeSubjectFromDayDiversity(pjokSubjectID)
	}
	missing := 0
	for idx, block := range blocks {
		gene := c.Get(idx)
		if !gene.IsPlaced() {
			missing++
			continue
		}
		if err := grid.PlaceBlock(block.ID, gene.Day, gene.StartSlot); err != nil {
			missing++
		}
	}
	return grid, missing
}

// RandomChromosome creates a chromosome with each block assigned a uniformly random
// valid position from candidateIndex. Parallel group members share the same gene.
func RandomChromosome(blocks []MatrixBlock, candidateIndex map[uint][]Gene, groups GroupIndex, rng *rand.Rand) Chromosome {
	ch := NewChromosome(len(blocks))
	visited := make(map[int]bool)
	for i, block := range blocks {
		if visited[i] {
			continue
		}
		candidates := candidateIndex[block.ID]
		if len(candidates) == 0 {
			continue
		}
		gene := candidates[rng.Intn(len(candidates))]
		ch.Set(i, gene)
		if block.GroupKey != nil {
			for _, j := range groups[*block.GroupKey] {
				ch.Set(j, gene)
				visited[j] = true
			}
		}
	}
	return ch
}

// SmartChromosome builds a chromosome by placing blocks greedily from most constrained
// to least constrained. PJOK 2JP blocks (weight -1) are always first; remaining blocks
// are ordered by number of available candidates. Each block receives a random conflict-free
// candidate from the partially built matrix, so the result usually decodes with 0 unplaced.
func SmartChromosome(blocks []MatrixBlock, candidateIndex map[uint][]Gene, groups GroupIndex, daySlots DaySlots, pjokSubjectID uint, rng *rand.Rand) Chromosome {
	entries := make([]sortEntry, len(blocks))
	for i, b := range blocks {
		w := len(candidateIndex[b.ID])
		if pjokSubjectID != 0 && b.SubjectID == pjokSubjectID && b.Duration == 2 {
			w = -1
		}
		entries[i] = sortEntry{i, b, w}
	}
	sortByWeight(entries)

	grid := NewScheduleMatrix(nil, nil, blocks, daySlots)
	grid.EnableDayDiversity()
	if pjokSubjectID != 0 {
		grid.ExcludeSubjectFromDayDiversity(pjokSubjectID)
	}

	ch := NewChromosome(len(blocks))
	visitedGroups := make(map[string]bool)

	for _, entry := range entries {
		i, block := entry.origIdx, entry.block

		if block.GroupKey != nil {
			if visitedGroups[*block.GroupKey] {
				continue
			}
			visitedGroups[*block.GroupKey] = true

			candidates := candidateIndex[block.ID]
			for _, pi := range rng.Perm(len(candidates)) {
				g := candidates[pi]
				if groupFitsAt(groups[*block.GroupKey], blocks, g, grid) {
					for _, j := range groups[*block.GroupKey] {
						ch.Set(j, g)
						_ = grid.PlaceBlock(blocks[j].ID, g.Day, g.StartSlot)
					}
					break
				}
			}
		} else {
			candidates := candidateIndex[block.ID]
			for _, pi := range rng.Perm(len(candidates)) {
				g := candidates[pi]
				if grid.CanPlaceBlock(block.ID, g.Day, g.StartSlot) == nil {
					ch.Set(i, g)
					_ = grid.PlaceBlock(block.ID, g.Day, g.StartSlot)
					break
				}
			}
		}
	}

	return ch
}

// MutateGene replaces the gene at position i with a new random candidate from candidateIndex.
// Group propagation is the caller's responsibility.
func MutateGene(c *Chromosome, i int, block MatrixBlock, candidateIndex map[uint][]Gene, rng *rand.Rand) {
	candidates := candidateIndex[block.ID]
	if len(candidates) == 0 {
		return
	}
	c.Set(i, candidates[rng.Intn(len(candidates))])
}

// ConstraintAwareCrossover produces an offspring chromosome from two parents. Blocks are
// processed in constraint-tightness order (PJOK 2JP first, then fewest candidates first),
// and for each block the gene that is conflict-free in the partially built child matrix is
// preferred. When both parents are valid the choice is random; when neither is valid a
// random conflict-free fallback is chosen from candidateIndex.
func ConstraintAwareCrossover(
	a, b Chromosome,
	blocks []MatrixBlock,
	candidateIndex map[uint][]Gene,
	groups GroupIndex,
	daySlots DaySlots,
	pjokSubjectID uint,
	rng *rand.Rand,
) Chromosome {
	entries := make([]sortEntry, len(blocks))
	for i, bl := range blocks {
		w := len(candidateIndex[bl.ID])
		if pjokSubjectID != 0 && bl.SubjectID == pjokSubjectID && bl.Duration == 2 {
			w = -1
		}
		entries[i] = sortEntry{i, bl, w}
	}
	sortByWeight(entries)

	grid := NewScheduleMatrix(nil, nil, blocks, daySlots)
	grid.EnableDayDiversity()
	if pjokSubjectID != 0 {
		grid.ExcludeSubjectFromDayDiversity(pjokSubjectID)
	}

	offspring := NewChromosome(len(blocks))
	visitedGroups := make(map[string]bool)

	for _, entry := range entries {
		i, block := entry.origIdx, entry.block

		if block.GroupKey != nil {
			if visitedGroups[*block.GroupKey] {
				continue
			}
			visitedGroups[*block.GroupKey] = true

			gA, gB := a.Get(i), b.Get(i)
			validA := gA.IsPlaced() && groupFitsAt(groups[*block.GroupKey], blocks, gA, grid)
			validB := gB.IsPlaced() && groupFitsAt(groups[*block.GroupKey], blocks, gB, grid)

			var chosen Gene
			switch {
			case validA && validB:
				if rng.Intn(2) == 0 {
					chosen = gA
				} else {
					chosen = gB
				}
			case validA:
				chosen = gA
			case validB:
				chosen = gB
			default:
				for _, pi := range rng.Perm(len(candidateIndex[block.ID])) {
					g := candidateIndex[block.ID][pi]
					if groupFitsAt(groups[*block.GroupKey], blocks, g, grid) {
						chosen = g
						break
					}
				}
			}

			if chosen.IsPlaced() {
				for _, j := range groups[*block.GroupKey] {
					offspring.Set(j, chosen)
					_ = grid.PlaceBlock(blocks[j].ID, chosen.Day, chosen.StartSlot)
				}
			}
		} else {
			gA, gB := a.Get(i), b.Get(i)
			validA := gA.IsPlaced() && grid.CanPlaceBlock(block.ID, gA.Day, gA.StartSlot) == nil
			validB := gB.IsPlaced() && grid.CanPlaceBlock(block.ID, gB.Day, gB.StartSlot) == nil

			var chosen Gene
			switch {
			case validA && validB:
				if rng.Intn(2) == 0 {
					chosen = gA
				} else {
					chosen = gB
				}
			case validA:
				chosen = gA
			case validB:
				chosen = gB
			default:
				for _, pi := range rng.Perm(len(candidateIndex[block.ID])) {
					g := candidateIndex[block.ID][pi]
					if grid.CanPlaceBlock(block.ID, g.Day, g.StartSlot) == nil {
						chosen = g
						break
					}
				}
			}

			if chosen.IsPlaced() {
				offspring.Set(i, chosen)
				_ = grid.PlaceBlock(block.ID, chosen.Day, chosen.StartSlot)
			}
		}
	}

	return offspring
}

// groupFitsAt returns true when every block in the group can be placed at the given gene
// simultaneously in the current matrix state.
func groupFitsAt(groupIndices []int, blocks []MatrixBlock, gene Gene, matrix *ScheduleMatrix) bool {
	for _, j := range groupIndices {
		if matrix.CanPlaceBlock(blocks[j].ID, gene.Day, gene.StartSlot) != nil {
			return false
		}
	}
	return true
}
