package algorithm

import (
	"fmt"

	teachingassignments "smp_mater_dei_be/internal/teaching_assignments"
)

// GenerateMatrixBlocks mengubah penugasan mengajar menjadi MatrixBlock yang dapat dijadwalkan.
// Blok grup paralel (SBP) ditempatkan pertama dalam slice yang dikembalikan agar
// DecodeChromosome dapat memprioritaskan mereka sebelum slot kelas lain terisi.
func GenerateMatrixBlocks(assignments []teachingassignments.TeachingAssignment, pjokSubjectID uint) ([]MatrixBlock, error) {
	parallelList := make([]MatrixBlock, 0, len(assignments))
	singleList := make([]MatrixBlock, 0, len(assignments)*2)

	nextID := uint(0)
	for _, assign := range assignments {
		durationList, err := SplitAssignmentJP(assign.JP, assign.SubjectID == pjokSubjectID)
		if err != nil {
			return nil, fmt.Errorf("split assignment %d: %w", assign.ID, err)
		}
		for _, dur := range durationList {
			nextID++
			blk := MatrixBlock{
				ID:        nextID,
				TeacherID: assign.TeacherID,
				SubjectID: assign.SubjectID,
				ClassID:   assign.ClassID,
				Duration:  dur,
				GroupKey:  assign.GroupKey,
			}
			switch assign.GroupKey != nil {
			case true:
				parallelList = append(parallelList, blk)
			default:
				singleList = append(singleList, blk)
			}
		}
	}

	return append(parallelList, singleList...), nil
}

// SplitAssignmentJP memecah total JP mingguan menjadi durasi blok konkret.
// PJOK harus selalu 3 JP dan dipecah menjadi [2, 1] (praktik + teori).
// Mapel lain mengikuti tabel dekomposisi standar.
func SplitAssignmentJP(jp int, isPJOK bool) ([]int, error) {
	if isPJOK {
		if jp != 3 {
			return nil, fmt.Errorf("PJOK assignment must be 3 JP, got %d", jp)
		}
		return []int{2, 1}, nil
	}

	table := map[int][]int{
		1: {1},
		2: {2},
		3: {3},
		4: {2, 2},
		5: {3, 2},
		6: {3, 3},
	}
	result, ok := table[jp]
	if !ok {
		return nil, fmt.Errorf("unsupported JP total %d", jp)
	}
	return result, nil
}

// GroupIndex memetakan GroupKey ke indeks semua blok anggota dalam slice blok tertentu.
// Setiap blok yang berbagi GroupKey yang sama harus dijadwalkan pada (Day, StartSlot) yang sama.
type GroupIndex map[string][]int

// BuildGroupIndex membangun GroupIndex dari slice blok dengan memindai GroupKey setiap blok.
func BuildGroupIndex(blocks []MatrixBlock) GroupIndex {
	peta := make(GroupIndex)
	for posisi, unit := range blocks {
		if unit.GroupKey == nil {
			continue
		}
		peta[*unit.GroupKey] = append(peta[*unit.GroupKey], posisi)
	}
	return peta
}
