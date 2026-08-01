package scheduling

import (
	"errors"
	"fmt"

	"github.com/dcrespo1/kinops/internal/domain"
)

func Assignee(schedule domain.Schedule, activePeople []domain.Person, sequenceNo int64) (int64, error) {
	if sequenceNo < 1 {
		return 0, errors.New("sequence number must be positive")
	}
	people := make([]domain.Person, 0, len(activePeople))
	for _, person := range activePeople {
		if person.Active {
			people = append(people, person)
		}
	}
	switch schedule.AssignmentMode {
	case domain.AssignmentFixed:
		if schedule.FixedPersonID == nil {
			return 0, errors.New("fixed schedule has no person")
		}
		for _, person := range people {
			if person.ID == *schedule.FixedPersonID {
				return person.ID, nil
			}
		}
		return 0, fmt.Errorf("fixed person %d is not active", *schedule.FixedPersonID)
	case domain.AssignmentRotate:
		if schedule.RotationStartPersonID == nil {
			return 0, errors.New("rotating schedule has no starting person")
		}
		if len(people) < 2 {
			return 0, errors.New("rotation requires at least two active people")
		}
		start := -1
		for index, person := range people {
			if person.ID == *schedule.RotationStartPersonID {
				start = index
				break
			}
		}
		if start < 0 {
			return 0, fmt.Errorf("rotation start person %d is not active", *schedule.RotationStartPersonID)
		}
		index := (start + int((sequenceNo-1)%int64(len(people)))) % len(people)
		return people[index].ID, nil
	default:
		return 0, fmt.Errorf("unsupported assignment mode %q", schedule.AssignmentMode)
	}
}
