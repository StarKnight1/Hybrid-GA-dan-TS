package algorithm

import (
	"reflect"
	"testing"

	teachingassignments "smp_mater_dei_be/internal/teaching_assignments"
)

func TestSplitAssignmentJP(t *testing.T) {
	tests := []struct {
		name   string
		jp     int
		pjok   bool
		want   []int
		wantOK bool
	}{
		{name: "one", jp: 1, want: []int{1}, wantOK: true},
		{name: "two", jp: 2, want: []int{2}, wantOK: true},
		{name: "three regular", jp: 3, want: []int{3}, wantOK: true},
		{name: "three pjok", jp: 3, pjok: true, want: []int{2, 1}, wantOK: true},
		{name: "four", jp: 4, want: []int{2, 2}, wantOK: true},
		{name: "five", jp: 5, want: []int{3, 2}, wantOK: true},
		{name: "six", jp: 6, want: []int{3, 3}, wantOK: true},
		{name: "unsupported", jp: 7},
		{name: "invalid pjok", jp: 2, pjok: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := SplitAssignmentJP(tt.jp, tt.pjok)
			if tt.wantOK && err != nil {
				t.Fatalf("SplitAssignmentJP returned error: %v", err)
			}
			if !tt.wantOK && err == nil {
				t.Fatal("SplitAssignmentJP succeeded; want error")
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("SplitAssignmentJP = %v; want %v", got, tt.want)
			}
		})
	}
}

func TestGenerateMatrixBlocksSplitsTeachingAssignments(t *testing.T) {
	var pjokSubjectID uint = 1
	var regularSubjectID uint = 2
	var classID uint = 10
	var teacherID uint = 5

	assignments := []teachingassignments.TeachingAssignment{
		{
			ID:        1,
			TeacherID: &teacherID,
			SubjectID: regularSubjectID,
			ClassID:   classID,
			JP:        5,
		},
		{
			ID:        2,
			TeacherID: &teacherID,
			SubjectID: pjokSubjectID,
			ClassID:   classID,
			JP:        3,
		},
	}

	blocks, err := GenerateMatrixBlocks(assignments, pjokSubjectID)
	if err != nil {
		t.Fatalf("GenerateMatrixBlocks returned error: %v", err)
	}

	gotDurations := make([]int, 0, len(blocks))
	for _, block := range blocks {
		gotDurations = append(gotDurations, block.Duration)
	}

	wantDurations := []int{3, 2, 2, 1}
	if !reflect.DeepEqual(gotDurations, wantDurations) {
		t.Fatalf("durations = %v; want %v", gotDurations, wantDurations)
	}
}
