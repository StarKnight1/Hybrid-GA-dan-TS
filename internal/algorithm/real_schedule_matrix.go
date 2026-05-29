package algorithm

import (
	"fmt"
	"sort"

	"github.com/google/uuid"
)

const realScheduleSBPTeacherNumber = "SBP"

type RealScheduleMatrixOptions struct {
	TeacherNumberToID      map[string]uuid.UUID
	ClassNameToID          map[string]uuid.UUID
	SubjectByTeacherNumber map[string]uuid.UUID
	SBPSubjectID           uuid.UUID
	DaySlots               DaySlots
}

type RealScheduleMatrixResult struct {
	Matrix     *ScheduleMatrix
	Blocks     []MatrixBlock
	Placements []BlockPlacement
}

func BuildRealScheduleMatrix(opts RealScheduleMatrixOptions) (*RealScheduleMatrixResult, error) {
	return BuildScheduleMatrixFromRealEntries(RealSchedule, opts)
}

func BuildScheduleMatrixFromRealEntries(entries []RealScheduleEntry, opts RealScheduleMatrixOptions) (*RealScheduleMatrixResult, error) {
	daySlots := opts.DaySlots
	if daySlots == nil {
		daySlots = GenerateSlots()
	}

	type entryWithSlot struct {
		entry     RealScheduleEntry
		slotIndex int
	}

	type scheduleKey struct {
		teacherNumber string
		className     string
	}

	grouped := make(map[scheduleKey][]entryWithSlot)
	for _, entry := range entries {
		if _, ok := opts.ClassNameToID[entry.ClassName]; !ok {
			return nil, fmt.Errorf("real schedule class %q is not mapped", entry.ClassName)
		}
		if entry.TeacherNumber != realScheduleSBPTeacherNumber {
			if _, ok := opts.TeacherNumberToID[entry.TeacherNumber]; !ok {
				return nil, fmt.Errorf("real schedule teacher %q is not mapped", entry.TeacherNumber)
			}
		}

		slotIndex, ok := MatrixSlotIndexFromTimeStart(entry.Day, entry.TimeStart, daySlots)
		if !ok {
			return nil, fmt.Errorf("real schedule time %s %s is not a matrix slot", entry.Day, entry.TimeStart)
		}

		key := scheduleKey{teacherNumber: entry.TeacherNumber, className: entry.ClassName}
		grouped[key] = append(grouped[key], entryWithSlot{entry: entry, slotIndex: slotIndex})
	}

	keys := make([]scheduleKey, 0, len(grouped))
	for key := range grouped {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool {
		if keys[i].className != keys[j].className {
			return keys[i].className < keys[j].className
		}
		return keys[i].teacherNumber < keys[j].teacherNumber
	})

	type pendingPlacement struct {
		block     MatrixBlock
		day       string
		startSlot int
	}

	planned := make([]pendingPlacement, 0, len(entries))
	for _, key := range keys {
		rows := grouped[key]
		sort.SliceStable(rows, func(i, j int) bool {
			di := MatrixDayIndex(rows[i].entry.Day)
			dj := MatrixDayIndex(rows[j].entry.Day)
			if di != dj {
				return di < dj
			}
			if rows[i].slotIndex != rows[j].slotIndex {
				return rows[i].slotIndex < rows[j].slotIndex
			}
			return rows[i].entry.TimeStart < rows[j].entry.TimeStart
		})

		for i := 0; i < len(rows); {
			start := rows[i]
			duration := 1
			lastSlot := start.slotIndex

			for i+duration < len(rows) {
				next := rows[i+duration]
				if next.entry.Day != start.entry.Day || next.slotIndex != lastSlot+1 {
					break
				}
				duration++
				lastSlot = next.slotIndex
			}

			block, err := realScheduleBlockFromGroup(key.teacherNumber, key.className, start.entry.Day, start.slotIndex, duration, opts, daySlots)
			if err != nil {
				return nil, err
			}
			planned = append(planned, pendingPlacement{
				block:     block,
				day:       start.entry.Day,
				startSlot: start.slotIndex,
			})

			i += duration
		}
	}

	blocks := make([]MatrixBlock, len(planned))
	for i, placement := range planned {
		blocks[i] = placement.block
	}

	matrix := NewScheduleMatrix(nil, nil, blocks, daySlots)
	placements := make([]BlockPlacement, 0, len(planned))
	for _, plannedPlacement := range planned {
		if err := matrix.PlaceBlock(plannedPlacement.block.ID, plannedPlacement.day, plannedPlacement.startSlot); err != nil {
			return nil, fmt.Errorf("place real schedule block %s: %w", plannedPlacement.block.ID, err)
		}
		placement, ok := matrix.Placement(plannedPlacement.block.ID)
		if ok {
			placements = append(placements, placement)
		}
	}
	if err := matrix.ValidateIntegrity(); err != nil {
		return nil, err
	}

	return &RealScheduleMatrixResult{
		Matrix:     matrix,
		Blocks:     blocks,
		Placements: placements,
	}, nil
}

func realScheduleBlockFromGroup(
	teacherNumber string,
	className string,
	day string,
	startSlot int,
	duration int,
	opts RealScheduleMatrixOptions,
	daySlots DaySlots,
) (MatrixBlock, error) {
	classID := opts.ClassNameToID[className]

	var teacherID *uuid.UUID
	if teacherNumber != realScheduleSBPTeacherNumber {
		id := opts.TeacherNumberToID[teacherNumber]
		teacherID = &id
	}

	subjectID, err := realScheduleSubjectID(teacherNumber, opts)
	if err != nil {
		return MatrixBlock{}, err
	}

	block := MatrixBlock{
		ID:        realScheduleBlockID(teacherNumber, className, day, startSlot, duration),
		TeacherID: teacherID,
		SubjectID: subjectID,
		ClassID:   classID,
		Duration:  duration,
	}

	return block, nil
}

func realScheduleSubjectID(teacherNumber string, opts RealScheduleMatrixOptions) (uuid.UUID, error) {
	if teacherNumber == realScheduleSBPTeacherNumber && opts.SBPSubjectID != uuid.Nil {
		return opts.SBPSubjectID, nil
	}

	subjectID, ok := opts.SubjectByTeacherNumber[teacherNumber]
	if !ok || subjectID == uuid.Nil {
		return uuid.Nil, fmt.Errorf("real schedule teacher %q has no subject mapping", teacherNumber)
	}
	return subjectID, nil
}

func realScheduleBlockID(teacherNumber, className, day string, startSlot, duration int) uuid.UUID {
	name := fmt.Sprintf("real-schedule|%s|%s|%s|%d|%d", teacherNumber, className, day, startSlot, duration)
	return uuid.NewSHA1(uuid.NameSpaceOID, []byte(name))
}
