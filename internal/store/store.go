package store

import (
	"context"
	"errors"
	"time"

	"github.com/dcrespo1/kinops/internal/domain"
)

var ErrNotFound = errors.New("not found")
var ErrConflict = errors.New("conflict")

type People interface {
	GetPerson(context.Context, int64) (domain.Person, error)
	GetActivePersonByCalendarToken(context.Context, string) (domain.Person, error)
	ListPeople(context.Context, bool) ([]domain.Person, error)
	CreatePerson(context.Context, *domain.Person) error
	UpdatePerson(context.Context, domain.Person) error
	DeactivatePerson(context.Context, int64) error
	RotatePersonCalendarToken(context.Context, int64, string) error
}

type Chores interface {
	GetChore(context.Context, int64) (domain.Chore, error)
	ListChores(context.Context, bool) ([]domain.Chore, error)
	CreateChore(context.Context, *domain.Chore) error
	UpdateChore(context.Context, domain.Chore) error
	DeactivateChore(context.Context, int64) error
}

type Schedules interface {
	GetSchedule(context.Context, int64) (domain.Schedule, error)
	ListSchedulesByChore(context.Context, int64, bool) ([]domain.Schedule, error)
	ListActiveSchedules(context.Context) ([]domain.Schedule, error)
	CreateSchedule(context.Context, *domain.Schedule) error
	UpdateSchedule(context.Context, domain.Schedule) error
	DeactivateSchedule(context.Context, int64) error
}

type Instances interface {
	GetInstance(context.Context, int64) (domain.ChoreInstance, error)
	GetDailyInstance(context.Context, int64) (domain.DailyInstance, error)
	ListDailyInstances(context.Context, time.Time) ([]domain.DailyInstance, error)
	ListScheduledInstances(context.Context, time.Time, time.Time) ([]domain.ScheduledInstance, error)
	ListScheduledInstancesForPerson(context.Context, int64, time.Time, time.Time) ([]domain.ScheduledInstance, error)
	ListInstancesBySchedule(context.Context, int64) ([]domain.ChoreInstance, error)
	DeletePendingInstancesFrom(context.Context, int64, time.Time) error
	MaxInstanceSequence(context.Context, int64) (int64, error)
	InstanceDates(context.Context, int64, time.Time, time.Time) (map[string]bool, error)
	CreateInstance(context.Context, *domain.ChoreInstance) error
	TransitionInstance(context.Context, int64, domain.InstanceStatus, domain.InstanceStatus, *time.Time, time.Time) (bool, error)
}

type CompletionLogs interface {
	CreateCompletionLog(context.Context, *domain.CompletionLog) error
	ListRecentCompletionActivity(context.Context, int) ([]domain.CompletionActivity, error)
}

type Analytics interface {
	ListPersonDailyStats(context.Context, time.Time, time.Time) ([]domain.PersonDailyStats, error)
	ListPersonOverdueCounts(context.Context, time.Time) (map[int64]int, error)
}

type Events interface {
	GetEvent(context.Context, int64) (domain.HouseholdEvent, error)
	ListEvents(context.Context, bool) ([]domain.HouseholdEvent, error)
	CreateEvent(context.Context, *domain.HouseholdEvent) error
	UpdateEvent(context.Context, domain.HouseholdEvent) error
	DeactivateEvent(context.Context, int64) error
	ReplaceEventAudience(context.Context, int64, []int64) error
	DeleteEventOccurrencesFrom(context.Context, int64, time.Time) error
	CreateEventOccurrence(context.Context, *domain.EventOccurrence) error
	ListScheduledEvents(context.Context, time.Time, time.Time) ([]domain.ScheduledEvent, error)
}

type Repository interface {
	People
	Chores
	Schedules
	Instances
	CompletionLogs
	Analytics
	Events
}
