package domain

import "time"

type RuleType string

const (
	RuleDaily      RuleType = "daily"
	RuleEveryNDays RuleType = "every_n_days"
	RuleWeeklyDays RuleType = "weekly_days"
	RuleMonthlyDay RuleType = "monthly_day"
	RuleOneOff     RuleType = "one_off"
)

type AssignmentMode string

const (
	AssignmentFixed  AssignmentMode = "fixed"
	AssignmentRotate AssignmentMode = "rotate"
)

type InstanceStatus string

const (
	InstancePending InstanceStatus = "pending"
	InstanceDone    InstanceStatus = "done"
	InstanceSkipped InstanceStatus = "skipped"
)

type CompletionEventType string

const (
	EventCompleted CompletionEventType = "completed"
	EventReopened  CompletionEventType = "reopened"
)

// RecurrenceRule is the typed, in-memory form of schedules.rule_params.
// Only the field relevant to Type is populated.
type RecurrenceRule struct {
	Type       RuleType
	Interval   int
	Weekdays   []time.Weekday
	DayOfMonth int
}

type Person struct {
	ID            int64
	Name          string
	Color         string
	CalendarToken string
	Active        bool
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

type HouseholdSettings struct {
	HouseholdEventColor string
	UpdatedAt           time.Time
}

type Chore struct {
	ID          int64
	Name        string
	Description string
	Category    string
	Active      bool
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

type Schedule struct {
	ID                    int64
	ChoreID               int64
	Rule                  RecurrenceRule
	StartDate             time.Time
	EndDate               *time.Time
	AssignmentMode        AssignmentMode
	FixedPersonID         *int64
	RotationStartPersonID *int64
	Active                bool
	CreatedAt             time.Time
	UpdatedAt             time.Time
}

type ChoreInstance struct {
	ID               int64
	ChoreID          int64
	ScheduleID       int64
	SequenceNo       int64
	DueDate          time.Time
	AssignedPersonID int64
	Status           InstanceStatus
	CompletedAt      *time.Time
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

type CompletionLog struct {
	ID              int64
	ChoreInstanceID int64
	PersonID        int64
	EventType       CompletionEventType
	OccurredAt      time.Time
}

type ScheduledInstance struct {
	Instance ChoreInstance
	Chore    Chore
	Assignee Person
}

// DailyInstance remains an alias for the Phase 3 API while calendar views use
// the more general ScheduledInstance name.
type DailyInstance = ScheduledInstance

type PersonDay struct {
	Person  Person
	Overdue []DailyInstance
	Today   []DailyInstance
}

type DailyView struct {
	Date   time.Time
	People []PersonDay
	Events []ScheduledEvent
}

type WeekDay struct {
	Date      time.Time
	Instances []ScheduledInstance
}

type PersonWeek struct {
	Person Person
	Days   []WeekDay
}

type WeekEventDay struct {
	Date   time.Time
	Events []ScheduledEvent
}

type WeekView struct {
	StartDate  time.Time
	EndDate    time.Time
	Today      time.Time
	HorizonEnd time.Time
	People     []PersonWeek
	EventDays  []WeekEventDay
}

type MonthDay struct {
	Date      time.Time
	InMonth   bool
	Instances []ScheduledInstance
	Events    []ScheduledEvent
}

type MonthView struct {
	Month      time.Time
	Today      time.Time
	GridStart  time.Time
	GridEnd    time.Time
	HorizonEnd time.Time
	Weeks      [][]MonthDay
}

type KitchenState string

const (
	KitchenReady        KitchenState = "ready"
	KitchenDisabled     KitchenState = "disabled"
	KitchenUnauthorized KitchenState = "unauthorized"
	KitchenUnavailable  KitchenState = "unavailable"
)

type KitchenRecipe struct {
	ID        string
	Slug      string
	Name      string
	MealieURL string
}

type KitchenRecipeCard struct {
	ID         string
	Slug       string
	Name       string
	ImageURL   string
	MealieURL  string
	Categories []string
	Rating     *float64
	Favorite   bool
	PrepTime   string
	TotalTime  string
}

type KitchenRecipeView struct {
	State         KitchenState
	Message       string
	Stale         bool
	StaleMessage  string
	Recipes       []KitchenRecipeCard
	Search        string
	FavoritesOnly bool
	Page          int
	TotalPages    int
	Total         int
	SelectedDate  time.Time
	SelectedType  string
}

type KitchenMealInput struct {
	Date      time.Time
	EntryType string
	Title     string
	Text      string
	RecipeID  string
}

type KitchenShoppingList struct {
	ID   string
	Name string
}

type KitchenGroceryItem struct {
	ID             string
	ShoppingListID string
	Display        string
	Note           string
	Quantity       float64
	UnitName       string
	Checked        bool
	Position       int
	LabelID        string
	LabelName      string
	LabelColor     string
}

type KitchenGroceryGroup struct {
	LabelID    string
	Label      string
	LabelColor string
	Position   int
	Items      []KitchenGroceryItem
}

type KitchenGroceryView struct {
	State            KitchenState
	Message          string
	Stale            bool
	StaleMessage     string
	NavigationDate   time.Time
	Lists            []KitchenShoppingList
	SelectedListID   string
	SelectedListName string
	NeedsSelection   bool
	ActiveGroups     []KitchenGroceryGroup
	CheckedItems     []KitchenGroceryItem
}

type KitchenGroceryCreate struct {
	ShoppingListID string
	Display        string
	Note           string
	Quantity       float64
	UnitName       string
}

type KitchenGroceryPatch struct {
	Display  *string
	Note     *string
	Quantity *float64
	UnitName *string
	Checked  *bool
}

type KitchenMeal struct {
	ID        int64
	Date      time.Time
	EntryType string
	Title     string
	Text      string
	Recipe    *KitchenRecipe
}

type KitchenMealGroup struct {
	Type  string
	Label string
	Meals []KitchenMeal
}

type KitchenDay struct {
	Date   time.Time
	Groups []KitchenMealGroup
}

type KitchenDailyView struct {
	Date         time.Time
	State        KitchenState
	Message      string
	Stale        bool
	StaleMessage string
	PublicURL    string
	Day          KitchenDay
}

type KitchenWeekView struct {
	StartDate    time.Time
	EndDate      time.Time
	State        KitchenState
	Message      string
	Stale        bool
	StaleMessage string
	PublicURL    string
	Days         []KitchenDay
}

type MealieStatus struct {
	Enabled   bool
	Connected bool
	Version   string
	Message   string
	CheckedAt time.Time
}

type CalendarFeed struct {
	Name   string
	Events []CalendarFeedEvent
}

// CalendarFeedEvent is the outbound, chore-instance-backed ICS representation.
// Household events use the separate types below.
type CalendarFeedEvent struct {
	InstanceID  int64
	DueDate     time.Time
	Summary     string
	Description string
	Category    string
	UpdatedAt   time.Time
}

type EventRuleType string

const (
	EventRuleOneOff     EventRuleType = "one_off"
	EventRuleDaily      EventRuleType = "daily"
	EventRuleEveryNDays EventRuleType = "every_n_days"
	EventRuleWeeklyDays EventRuleType = "weekly_days"
	EventRuleMonthlyDay EventRuleType = "monthly_day"
	EventRuleAnnual     EventRuleType = "annual"
)

const (
	EventCategoryGeneral     = "General"
	EventCategoryBirthday    = "Birthday"
	EventCategoryHoliday     = "Holiday"
	EventCategoryAppointment = "Appointment"
	EventCategoryFamily      = "Family"
	EventCategorySchool      = "School"
	EventCategoryWork        = "Work"
	EventCategorySocial      = "Social"
	EventCategoryTravel      = "Travel/Vacation"
)

var eventCategories = []string{
	EventCategoryGeneral,
	EventCategoryBirthday,
	EventCategoryHoliday,
	EventCategoryAppointment,
	EventCategoryFamily,
	EventCategorySchool,
	EventCategoryWork,
	EventCategorySocial,
	EventCategoryTravel,
}

// EventCategories returns a copy of the curated household event categories.
// Keeping these in code gives views one consistent set of labels and leaves
// the database column migration-free when the set changes.
func EventCategories() []string {
	return append([]string(nil), eventCategories...)
}

func IsEventCategory(value string) bool {
	for _, category := range eventCategories {
		if value == category {
			return true
		}
	}
	return false
}

type EventRecurrenceRule struct {
	Type       EventRuleType
	Interval   int
	Weekdays   []time.Weekday
	DayOfMonth int
	Month      time.Month
}

// HouseholdEvent is a locally managed family-calendar event. Dates are
// household-local. EndDate is exclusive for all-day events and is the actual
// ending local date for timed events.
type HouseholdEvent struct {
	ID                int64
	Title             string
	Description       string
	Location          string
	Category          string
	AllDay            bool
	StartDate         time.Time
	EndDate           time.Time
	StartTime         string
	EndTime           string
	Rule              EventRecurrenceRule
	RecurrenceEndDate *time.Time
	AudiencePersonIDs []int64
	Active            bool
	CreatedAt         time.Time
	UpdatedAt         time.Time
}

type EventOccurrence struct {
	ID        int64
	EventID   int64
	StartDate time.Time
	EndDate   time.Time
	StartAt   *time.Time
	EndAt     *time.Time
	CreatedAt time.Time
	UpdatedAt time.Time
}

type ScheduledEvent struct {
	Occurrence EventOccurrence
	Event      HouseholdEvent
	Audience   []Person
	Color      string
}

type PersonDailyStats struct {
	PersonID  int64
	DueDate   time.Time
	Assigned  int
	Completed int
}

type AnalyticsWindow struct {
	Days        int
	Assigned    int
	Completed   int
	RatePercent int
}

type PersonAnalytics struct {
	Person      Person
	Assigned    int
	Completed   int
	RatePercent int
	Streak      int
	Overdue     int
}

type CompletionActivity struct {
	ID         int64
	ChoreName  string
	PersonName string
	EventType  CompletionEventType
	OccurredAt time.Time
}

type AdminDashboard struct {
	GeneratedAt    time.Time
	TimeZone       string
	HorizonDays    int
	SevenDay       AnalyticsWindow
	ThirtyDay      AnalyticsWindow
	Overdue        int
	People         []PersonAnalytics
	Activity       []CompletionActivity
	CalendarPeople []Person
}
