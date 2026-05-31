package algorithm

import (
	"math/rand"
)

// matrixSnapshot mencatat posisi (Day, StartSlot) setiap blok yang sudah ditempatkan pada suatu waktu.
// Memungkinkan Tabu Search menyimpan dan memulihkan state matriks secara efisien tanpa
// menyalin seluruh struktur grid.
type matrixSnapshot map[uint]Gene

// captureMatrix membaca penempatan blok saat ini dari matriks dan mengembalikan snapshot.
// Query langsung ke matriks memastikan snapshot selalu konsisten dengan kondisi grid yang sebenarnya.
func captureMatrix(matrix *ScheduleMatrix, blocks []MatrixBlock) matrixSnapshot {
	state := make(matrixSnapshot, len(blocks))
	for _, blk := range blocks {
		if rec, ok := matrix.Placement(blk.ID); ok {
			state[blk.ID] = Gene{Day: rec.Day, StartSlot: rec.StartSlot}
		}
	}
	return state
}

// restoreMatrix membangun ScheduleMatrix baru dari snapshot yang sebelumnya diambil.
// Blok yang tidak ada dalam snapshot atau gennya kosong dibiarkan tanpa jadwal;
// bilangan bulat yang dikembalikan adalah jumlah blok tersebut.
func restoreMatrix(snap matrixSnapshot, blocks []MatrixBlock, daySlots DaySlots, pjokSubjectID uint) (*ScheduleMatrix, int) {
	grid := NewScheduleMatrix(nil, nil, blocks, daySlots)
	grid.EnableDayDiversity()
	if pjokSubjectID != 0 {
		grid.ExcludeSubjectFromDayDiversity(pjokSubjectID)
	}
	missing := 0
	for _, blk := range blocks {
		gene, ok := snap[blk.ID]
		if !ok || !gene.IsPlaced() {
			missing++
			continue
		}
		if err := grid.PlaceBlock(blk.ID, gene.Day, gene.StartSlot); err != nil {
			missing++
		}
	}
	return grid, missing
}

// snapshotToChromosome mengubah snapshot penempatan menjadi Chromosome dengan mengkodekan
// posisi setiap blok sebagai gen yang bersesuaian.
func snapshotToChromosome(snap matrixSnapshot, blocks []MatrixBlock) Chromosome {
	ch := NewChromosome(len(blocks))
	for idx, blk := range blocks {
		if gene, ok := snap[blk.ID]; ok {
			ch.Set(idx, gene)
		}
	}
	return ch
}

// dropID menghapus kemunculan pertama target dari slice menggunakan strategi tukar-dengan-terakhir.
// Urutan tidak dipertahankan; mengembalikan slice asli jika target tidak ditemukan.
func dropID(slice []uint, target uint) []uint {
	for idx, val := range slice {
		if val == target {
			slice[idx] = slice[len(slice)-1]
			return slice[:len(slice)-1]
		}
	}
	return slice
}

// conflictsAt mencari semua ID blok yang akan konflik jika blok ditempatkan pada (day, startSlot).
// Grid kelas dan (jika blok punya guru) grid guru diperiksa untuk semua offset dalam durasi blok.
func conflictsAt(matrix *ScheduleMatrix, block MatrixBlock, day string, startSlot int) []uint {
	seen := make(map[uint]struct{})
	var result []uint

	for offset := 0; offset < block.Duration; offset++ {
		idx := startSlot + offset

		if cell, ok := matrix.ClassCell(block.ClassID, day, idx); ok && cell.State == FilledCell {
			if _, already := seen[cell.BlockID]; !already {
				seen[cell.BlockID] = struct{}{}
				result = append(result, cell.BlockID)
			}
		}

		if block.TeacherID != nil {
			if cell, ok := matrix.TeacherCell(*block.TeacherID, day, idx); ok && cell.State == FilledCell {
				if _, already := seen[cell.BlockID]; !already {
					seen[cell.BlockID] = struct{}{}
					result = append(result, cell.BlockID)
				}
			}
		}
	}

	return result
}

// findGroupSlot memindai kandidat secara acak dan mengembalikan Gen pertama di mana
// setiap blok dalam grup paralel dapat ditempatkan secara simultan.
// Mengembalikan (Gene{}, false) jika tidak ada posisi yang ditemukan.
func findGroupSlot(matrix *ScheduleMatrix, groupIDs []uint, blockByID map[uint]MatrixBlock, candidates []Gene, rng *rand.Rand) (Gene, bool) {
	if len(candidates) == 0 {
		return Gene{}, false
	}
	startAt := rng.Intn(len(candidates))
	for attempt := 0; attempt < len(candidates); attempt++ {
		pos := candidates[(startAt+attempt)%len(candidates)]
		allClear := true
		for _, id := range groupIDs {
			if err := matrix.CanPlaceBlock(id, pos.Day, pos.StartSlot); err != nil {
				allClear = false
				break
			}
		}
		if allClear {
			return pos, true
		}
	}
	return Gene{}, false
}

// findOpenSlot memindai kandidat secara acak dan mengembalikan Gen pertama yang bebas konflik
// untuk satu blok tertentu. Mengembalikan (Gene{}, false) jika tidak ada yang ditemukan.
func findOpenSlot(matrix *ScheduleMatrix, block MatrixBlock, candidates []Gene, rng *rand.Rand) (Gene, bool) {
	if len(candidates) == 0 {
		return Gene{}, false
	}
	startAt := rng.Intn(len(candidates))
	for attempt := 0; attempt < len(candidates); attempt++ {
		pos := candidates[(startAt+attempt)%len(candidates)]
		if matrix.CanPlaceBlock(block.ID, pos.Day, pos.StartSlot) == nil {
			return pos, true
		}
	}
	return Gene{}, false
}
