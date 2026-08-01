package scheduling

import (
	"testing"

	"github.com/dcrespo1/kinops/internal/domain"
)

func TestAssignee(t *testing.T) {
	people := []domain.Person{{ID: 1, Active: true}, {ID: 2, Active: true}, {ID: 3, Active: true}}
	fixedID := int64(2)
	fixed := domain.Schedule{AssignmentMode: domain.AssignmentFixed, FixedPersonID: &fixedID}
	for sequence := int64(1); sequence <= 4; sequence++ {
		got, err := Assignee(fixed, people, sequence)
		if err != nil || got != 2 {
			t.Errorf("fixed sequence %d = %d, %v; want 2", sequence, got, err)
		}
	}

	startID := int64(2)
	rotate := domain.Schedule{AssignmentMode: domain.AssignmentRotate, RotationStartPersonID: &startID}
	want := []int64{2, 3, 1, 2}
	for index, expected := range want {
		got, err := Assignee(rotate, people, int64(index+1))
		if err != nil || got != expected {
			t.Errorf("rotate sequence %d = %d, %v; want %d", index+1, got, err, expected)
		}
	}
}

func TestAssigneeRejectsInactiveFixedPerson(t *testing.T) {
	personID := int64(1)
	schedule := domain.Schedule{AssignmentMode: domain.AssignmentFixed, FixedPersonID: &personID}
	if _, err := Assignee(schedule, []domain.Person{{ID: 1, Active: false}}, 1); err == nil {
		t.Fatal("Assignee() returned nil error")
	}
}
