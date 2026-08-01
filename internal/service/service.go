package service

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/dcrespo1/kinops/internal/domain"
	"github.com/dcrespo1/kinops/internal/scheduling"
	"github.com/dcrespo1/kinops/internal/store"
)

type Service struct {
	repository *store.SQLite
	location   *time.Location
	now        func() time.Time
}

func New(repository *store.SQLite, location *time.Location) *Service {
	return &Service{repository: repository, location: location, now: time.Now}
}

func NewWithClock(repository *store.SQLite, location *time.Location, now func() time.Time) *Service {
	return &Service{repository: repository, location: location, now: now}
}

func (s *Service) ListChores(ctx context.Context) ([]domain.Chore, error) {
	return s.repository.ListChores(ctx, false)
}

func (s *Service) GetChore(ctx context.Context, id int64) (domain.Chore, []domain.Schedule, error) {
	chore, err := s.repository.GetChore(ctx, id)
	if err != nil {
		return domain.Chore{}, nil, err
	}
	schedules, err := s.repository.ListSchedulesByChore(ctx, id, false)
	if err != nil {
		return domain.Chore{}, nil, err
	}
	return chore, schedules, nil
}

func validateChore(chore domain.Chore) (domain.Chore, error) {
	chore.Name = strings.TrimSpace(chore.Name)
	chore.Description = strings.TrimSpace(chore.Description)
	chore.Category = strings.TrimSpace(chore.Category)
	if chore.Name == "" {
		return domain.Chore{}, errors.New("name is required")
	}
	if len(chore.Name) > 200 {
		return domain.Chore{}, errors.New("name must be 200 characters or fewer")
	}
	return chore, nil
}

func (s *Service) CreateChore(ctx context.Context, chore domain.Chore) (domain.Chore, error) {
	chore.Active = true
	valid, err := validateChore(chore)
	if err != nil {
		return domain.Chore{}, err
	}
	if err := s.repository.CreateChore(ctx, &valid); err != nil {
		return domain.Chore{}, err
	}
	return valid, nil
}

func (s *Service) UpdateChore(ctx context.Context, chore domain.Chore) error {
	valid, err := validateChore(chore)
	if err != nil {
		return err
	}
	current, err := s.repository.GetChore(ctx, chore.ID)
	if err != nil {
		return err
	}
	valid.Active = current.Active
	return s.repository.UpdateChore(ctx, valid)
}

func (s *Service) DeactivateChore(ctx context.Context, id int64) error {
	today := scheduling.Date(s.now(), s.location)
	return s.repository.WithinTx(ctx, func(tx *store.SQLite) error {
		if _, err := tx.GetChore(ctx, id); err != nil {
			return err
		}
		schedules, err := tx.ListSchedulesByChore(ctx, id, false)
		if err != nil {
			return err
		}
		for _, schedule := range schedules {
			if err := tx.DeactivateSchedule(ctx, schedule.ID); err != nil {
				return err
			}
			if err := tx.DeletePendingInstancesFrom(ctx, schedule.ID, today); err != nil {
				return err
			}
		}
		return tx.DeactivateChore(ctx, id)
	})
}

func (s *Service) ListPeople(ctx context.Context) ([]domain.Person, error) {
	return s.repository.ListPeople(ctx, false)
}

func (s *Service) CreatePerson(ctx context.Context, person domain.Person) (domain.Person, error) {
	person.Name = strings.TrimSpace(person.Name)
	person.Color = strings.TrimSpace(person.Color)
	if person.Name == "" {
		return domain.Person{}, errors.New("name is required")
	}
	if person.Color == "" {
		return domain.Person{}, errors.New("color is required")
	}
	token, err := generateCalendarToken()
	if err != nil {
		return domain.Person{}, err
	}
	person.CalendarToken = token
	person.Active = true
	if err := s.repository.WithinTx(ctx, func(tx *store.SQLite) error {
		if err := tx.CreatePerson(ctx, &person); err != nil {
			return err
		}
		schedules, err := tx.ListActiveSchedules(ctx)
		if err != nil {
			return err
		}
		for _, schedule := range schedules {
			if schedule.AssignmentMode != domain.AssignmentRotate {
				continue
			}
			if err := s.generate(ctx, tx, schedule, true); err != nil {
				return fmt.Errorf("regenerate rotating schedule %d after adding person: %w", schedule.ID, err)
			}
		}
		return nil
	}); err != nil {
		return domain.Person{}, err
	}
	return person, nil
}

func generateCalendarToken() (string, error) {
	token := make([]byte, 24)
	if _, err := rand.Read(token); err != nil {
		return "", fmt.Errorf("generate calendar token: %w", err)
	}
	return hex.EncodeToString(token), nil
}

func (s *Service) GetSchedule(ctx context.Context, id int64) (domain.Schedule, error) {
	return s.repository.GetSchedule(ctx, id)
}

func (s *Service) CreateSchedule(ctx context.Context, schedule domain.Schedule) (domain.Schedule, error) {
	schedule.Active = true
	schedule.StartDate = scheduling.Date(schedule.StartDate, s.location)
	if schedule.EndDate != nil {
		value := scheduling.Date(*schedule.EndDate, s.location)
		schedule.EndDate = &value
	}
	if err := scheduling.ValidateSchedule(schedule); err != nil {
		return domain.Schedule{}, err
	}
	err := s.repository.WithinTx(ctx, func(tx *store.SQLite) error {
		chore, err := tx.GetChore(ctx, schedule.ChoreID)
		if err != nil {
			return err
		}
		if !chore.Active {
			return errors.New("cannot schedule an inactive chore")
		}
		if err := validateAssignmentPeople(ctx, tx, schedule); err != nil {
			return err
		}
		if err := tx.CreateSchedule(ctx, &schedule); err != nil {
			return err
		}
		return s.generate(ctx, tx, schedule, false)
	})
	return schedule, err
}

func (s *Service) UpdateSchedule(ctx context.Context, schedule domain.Schedule) error {
	schedule.StartDate = scheduling.Date(schedule.StartDate, s.location)
	if schedule.EndDate != nil {
		value := scheduling.Date(*schedule.EndDate, s.location)
		schedule.EndDate = &value
	}
	if err := scheduling.ValidateSchedule(schedule); err != nil {
		return err
	}
	return s.repository.WithinTx(ctx, func(tx *store.SQLite) error {
		current, err := tx.GetSchedule(ctx, schedule.ID)
		if err != nil {
			return err
		}
		schedule.ChoreID = current.ChoreID
		schedule.Active = current.Active
		if err := validateAssignmentPeople(ctx, tx, schedule); err != nil {
			return err
		}
		if err := tx.UpdateSchedule(ctx, schedule); err != nil {
			return err
		}
		return s.generate(ctx, tx, schedule, true)
	})
}

func (s *Service) DeactivateSchedule(ctx context.Context, id int64) error {
	today := scheduling.Date(s.now(), s.location)
	return s.repository.WithinTx(ctx, func(tx *store.SQLite) error {
		if _, err := tx.GetSchedule(ctx, id); err != nil {
			return err
		}
		if err := tx.DeactivateSchedule(ctx, id); err != nil {
			return err
		}
		return tx.DeletePendingInstancesFrom(ctx, id, today)
	})
}

func validateAssignmentPeople(ctx context.Context, repository store.People, schedule domain.Schedule) error {
	people, err := repository.ListPeople(ctx, false)
	if err != nil {
		return err
	}
	_, err = scheduling.Assignee(schedule, people, 1)
	return err
}

func (s *Service) generate(ctx context.Context, repository store.Repository, schedule domain.Schedule, replace bool) error {
	today := scheduling.Date(s.now(), s.location)
	through := today.AddDate(0, 0, 60)
	schedule.StartDate = scheduling.RebaseDate(schedule.StartDate, s.location)
	if schedule.EndDate != nil {
		value := scheduling.RebaseDate(*schedule.EndDate, s.location)
		schedule.EndDate = &value
	}
	if replace {
		if err := repository.DeletePendingInstancesFrom(ctx, schedule.ID, today); err != nil {
			return err
		}
	}
	if !schedule.Active {
		return nil
	}
	dates, err := scheduling.Occurrences(schedule.Rule, schedule.StartDate, schedule.EndDate, today, through)
	if err != nil {
		return err
	}
	existing, err := repository.InstanceDates(ctx, schedule.ID, today, through)
	if err != nil {
		return err
	}
	sequence, err := repository.MaxInstanceSequence(ctx, schedule.ID)
	if err != nil {
		return err
	}
	people, err := repository.ListPeople(ctx, false)
	if err != nil {
		return err
	}
	for _, dueDate := range dates {
		if existing[dueDate.Format(scheduling.DateLayout)] {
			continue
		}
		sequence++
		personID, err := scheduling.Assignee(schedule, people, sequence)
		if err != nil {
			return err
		}
		instance := domain.ChoreInstance{ChoreID: schedule.ChoreID, ScheduleID: schedule.ID, SequenceNo: sequence, DueDate: dueDate, AssignedPersonID: personID, Status: domain.InstancePending}
		if err := repository.CreateInstance(ctx, &instance); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) EnsureHorizon(ctx context.Context) error {
	schedules, err := s.repository.ListActiveSchedules(ctx)
	if err != nil {
		return err
	}
	for _, schedule := range schedules {
		if err := s.repository.WithinTx(ctx, func(tx *store.SQLite) error { return s.generate(ctx, tx, schedule, false) }); err != nil {
			return fmt.Errorf("ensure schedule %d horizon: %w", schedule.ID, err)
		}
	}
	return nil
}
