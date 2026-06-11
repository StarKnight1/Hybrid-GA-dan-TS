package algorithm

import (
	"math/rand"
	"sort"
)

// Gene adalah penempatan terjadwal untuk satu MatrixBlock.
// Nilai kosong (Day == "") berarti blok belum ditempatkan.
type Gene struct {
	Day       string
	StartSlot int
}

func (g Gene) IsPlaced() bool { return g.Day != "" }

// Chromosome menyimpan satu solusi jadwal kandidat sebagai slice Gene berurutan.
// Indeks i pada slice gene bersesuaian dengan indeks i pada slice blok terkait.
// Chromosome tidak memiliki slice blok; pemanggil menyediakannya di setiap operasi.
type Chromosome struct {
	genes []Gene
}

func NewChromosome(n int) Chromosome {
	return Chromosome{genes: make([]Gene, n)}
}

func (c Chromosome) Len() int           { return len(c.genes) }
func (c Chromosome) Get(i int) Gene     { return c.genes[i] }
func (c *Chromosome) Set(i int, g Gene) { c.genes[i] = g }

func (c Chromosome) Clone() Chromosome {
	cp := make([]Gene, len(c.genes))
	copy(cp, c.genes)
	return Chromosome{genes: cp}
}

// pjokCutoffTime adalah batas waktu selesai untuk blok PJOK 2JP.
const pjokCutoffTime = "10:50"

// sortEntry mengurutkan blok berdasarkan kesulitan penempatan sebelum membuat kromosom.
type sortEntry struct {
	origIdx int
	block   MatrixBlock
	weight  int // bobot lebih kecil = diproses lebih awal (batasan lebih ketat)
}

// sortByWeight mengurutkan entries secara menaik berdasarkan bobot.
func sortByWeight(entries []sortEntry) {
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].weight < entries[j].weight
	})
}

// ── Perhitungan pelanggaran ringan ────────────────────────────────────────────

// PenaltyBreakdown menyimpan jumlah penalti ringan per kategori untuk satu jadwal.
// PJOKOvertime berkontribusi 3× pada skor optimasi; kategori lain berkontribusi 1×.
type PenaltyBreakdown struct {
	DaySplitCount      int // blok (kelas, mapel) yang jatuh pada hari yang sama
	DaySplitGroupCount int // subset DaySplitCount yang melibatkan anggota grup paralel SBP
	PJOKOvertime       int // blok PJOK 2JP yang selesai setelah pjokCutoffTime
}

// Total mengembalikan jumlah penalti tanpa pembobotan (tanpa multiplier PJOK).
func (bd PenaltyBreakdown) Total() int {
	return bd.DaySplitCount
}

// BreakdownSoftViolations mengembalikan rincian penalti ringan per kategori untuk jadwal hasil decode.
// Gunakan Total() untuk mendapatkan skor optimasi berbobot.
func BreakdownSoftViolations(matrix *ScheduleMatrix, blocks []MatrixBlock, pjokSubjectID uint) PenaltyBreakdown {
	var bd PenaltyBreakdown

	// tandai blok yang merupakan anggota grup paralel
	isGrouped := make(map[uint]bool, len(blocks))
	for _, b := range blocks {
		if b.GroupKey != nil {
			isGrouped[b.ID] = true
		}
	}

	// hitung blok (kelas, mapel) yang jatuh pada hari yang sama
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

	// blok PJOK 2JP yang selesai setelah pjokCutoffTime dikenai penalti bobot 3
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

// CountSoftViolations selalu mengembalikan 0 karena Day Diversity (hard constraint) mencegah
// penempatan blok (kelas, mapel) yang sama pada hari yang sama. Setiap potensi konflik
// diselesaikan saat penempatan — blok menjadi unplaced, bukan placed-with-violation.
// Fungsi ini dipertahankan untuk kompatibilitas API; gunakan BreakdownSoftViolations untuk pelaporan.
func CountSoftViolations(_ *ScheduleMatrix, _ []MatrixBlock, _ uint) int {
	return 0
}

// ── Indeks kandidat ───────────────────────────────────────────────────────────

// BuildCandidateIndex menghitung semua posisi (Day, StartSlot) yang valid secara fisik
// untuk setiap blok. Posisi valid jika semua slot dalam rentang durasi tidak diblokir.
// Blok PJOK 2JP dibatasi pada slot pagi yang selesai tidak melebihi pjokCutoffTime.
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

// filterMorningSlots menyaring kandidat 2JP yang berakhir tidak melebihi pjokCutoffTime.
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

// candidatesForDuration menghitung semua posisi awal yang valid untuk blok berdurasi tertentu.
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

// ── Pembentukan kromosom ──────────────────────────────────────────────────────

// DecodeChromosome menerjemahkan kromosom menjadi ScheduleMatrix dengan menempatkan setiap blok
// pada posisi (Day, StartSlot) yang dikodekan. Blok yang belum ditempatkan atau konflik
// dilewati; bilangan bulat yang dikembalikan menunjukkan jumlah kegagalan tersebut.
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

// RandomChromosome membuat kromosom dengan setiap blok diberi posisi acak yang valid
// dari candidateIndex. Anggota grup paralel berbagi gen yang sama.
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

// SmartChromosome membangun kromosom secara greedy dari blok paling terkendala ke paling longgar.
// Blok PJOK 2JP (bobot -1) selalu diproses pertama; blok lain diurutkan berdasarkan jumlah kandidat.
// Setiap blok mendapat kandidat acak bebas konflik dari matriks yang sedang dibangun,
// sehingga hasilnya biasanya menghasilkan 0 blok yang tidak ditempatkan.
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

// MutateGene mengganti gen pada posisi i dengan kandidat acak baru dari candidateIndex.
// Propagasi grup adalah tanggung jawab pemanggil.
func MutateGene(c *Chromosome, i int, block MatrixBlock, candidateIndex map[uint][]Gene, rng *rand.Rand) {
	candidates := candidateIndex[block.ID]
	if len(candidates) == 0 {
		return
	}
	c.Set(i, candidates[rng.Intn(len(candidates))])
}

// ConstraintAwareCrossover menghasilkan kromosom keturunan dari dua induk.
// Blok diproses dalam urutan ketertataan (PJOK 2JP pertama, lalu kandidat tersedikit),
// dan untuk setiap blok dipilih gen yang bebas konflik di matriks anak yang sedang dibangun.
// Jika kedua induk valid, dipilih secara acak; jika tidak ada yang valid,
// diambil kandidat bebas konflik acak dari candidateIndex.
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

// groupFitsAt mengembalikan true jika setiap blok dalam grup dapat ditempatkan
// pada gen yang diberikan secara simultan di kondisi matriks saat ini.
func groupFitsAt(groupIndices []int, blocks []MatrixBlock, gene Gene, matrix *ScheduleMatrix) bool {
	for _, j := range groupIndices {
		if matrix.CanPlaceBlock(blocks[j].ID, gene.Day, gene.StartSlot) != nil {
			return false
		}
	}
	return true
}
