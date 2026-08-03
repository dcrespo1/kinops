package app

import (
	"database/sql"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"github.com/dcrespo1/kinops/internal/auth"
	"github.com/dcrespo1/kinops/internal/config"
	"github.com/dcrespo1/kinops/internal/handlers"
	"github.com/dcrespo1/kinops/internal/service"
	"github.com/dcrespo1/kinops/internal/store"
)

type Dependencies struct {
	DB                  *sql.DB
	Logger              *slog.Logger
	Config              config.Config
	ManagementService   handlers.ManagementService
	DailyService        handlers.DailyService
	CalendarService     handlers.CalendarService
	CalendarFeedService handlers.CalendarFeedService
	AdminService        handlers.AdminService
	KitchenService      handlers.KitchenService
	MealieStatusService handlers.MealieStatusService
	AdminAuth           *auth.Manager
}

func NewRouter(deps Dependencies) http.Handler {
	router := chi.NewRouter()

	router.Use(middleware.RequestID)
	router.Use(middleware.Recoverer)
	router.Use(middleware.Compress(5))

	router.Handle(
		"/static/*",
		http.StripPrefix(
			"/static/",
			http.FileServer(http.Dir("./web/static")),
		),
	)

	health := handlers.NewHealthHandler(deps.DB)
	location := deps.Config.Location
	if location == nil {
		location = time.UTC
	}
	managementService := deps.ManagementService
	if managementService == nil {
		managementService = service.New(store.NewSQLite(deps.DB), location)
	}
	management := handlers.NewManagementHandler(managementService, deps.Logger, location)
	eventService, ok := managementService.(handlers.PublicEventService)
	if !ok {
		eventService = service.New(store.NewSQLite(deps.DB), location)
	}
	events := handlers.NewEventHandler(eventService, deps.Logger, location)
	dailyService := deps.DailyService
	if dailyService == nil {
		if shared, ok := managementService.(handlers.DailyService); ok {
			dailyService = shared
		} else {
			dailyService = service.New(store.NewSQLite(deps.DB), location)
		}
	}
	daily := handlers.NewDailyHandler(dailyService, deps.Logger, location)
	calendarService := deps.CalendarService
	if calendarService == nil {
		if shared, ok := managementService.(handlers.CalendarService); ok {
			calendarService = shared
		} else {
			calendarService = service.New(store.NewSQLite(deps.DB), location)
		}
	}
	calendar := handlers.NewCalendarHandler(calendarService, deps.Logger, location)
	calendarFeedService := deps.CalendarFeedService
	if calendarFeedService == nil {
		if shared, ok := managementService.(handlers.CalendarFeedService); ok {
			calendarFeedService = shared
		} else {
			calendarFeedService = service.New(store.NewSQLite(deps.DB), location)
		}
	}
	calendarFeed := handlers.NewCalendarFeedHandler(calendarFeedService, deps.Logger)
	kitchenHandler := handlers.NewKitchenHandler(deps.KitchenService, deps.Logger, location)
	var admin *handlers.AdminHandler
	if deps.AdminAuth != nil {
		adminService := deps.AdminService
		if adminService == nil {
			if shared, ok := managementService.(handlers.AdminService); ok {
				adminService = shared
			} else {
				adminService = service.New(store.NewSQLite(deps.DB), location)
			}
		}
		admin = handlers.NewAdminHandler(adminService, deps.AdminAuth, deps.Logger, deps.MealieStatusService)
	}

	router.Get("/healthz", health.Get)
	router.Get("/", daily.Get)
	router.Get("/daily", daily.Get)
	router.Get("/weekly", calendar.Weekly)
	router.Get("/monthly", calendar.Monthly)
	router.Get("/calendar/{personToken}.ics", calendarFeed.Get)
	router.Get("/kitchen", kitchenHandler.Root)
	router.Get("/kitchen/daily", kitchenHandler.Daily)
	router.Get("/kitchen/weekly", kitchenHandler.Weekly)
	router.Get("/kitchen/recipes", kitchenHandler.Recipes)
	router.Post("/kitchen/recipes/{recipeSlug}/favorite", kitchenHandler.Favorite)
	router.Delete("/kitchen/recipes/{recipeSlug}/favorite", kitchenHandler.Favorite)
	router.Post("/kitchen/meals", kitchenHandler.MealCreate)
	router.Post("/kitchen/meals/{mealID}", kitchenHandler.MealMutate)
	router.Put("/kitchen/meals/{mealID}", kitchenHandler.MealUpdate)
	router.Delete("/kitchen/meals/{mealID}", kitchenHandler.MealDelete)
	router.Get("/kitchen/groceries", kitchenHandler.Groceries)
	router.Post("/kitchen/groceries/items", kitchenHandler.GroceryCreate)
	router.Post("/kitchen/groceries/items/{itemID}", kitchenHandler.GroceryMutate)
	router.Put("/kitchen/groceries/items/{itemID}", kitchenHandler.GroceryUpdate)
	router.Patch("/kitchen/groceries/items/{itemID}", kitchenHandler.GroceryUpdate)
	router.Delete("/kitchen/groceries/items/{itemID}", kitchenHandler.GroceryDelete)
	router.Patch("/instances/{instanceID}/complete", daily.Complete)
	router.Post("/instances/{instanceID}/complete", daily.Complete)
	router.Patch("/instances/{instanceID}/reopen", daily.Reopen)
	router.Post("/instances/{instanceID}/reopen", daily.Reopen)
	router.Get("/chores", management.ChoreIndex)
	router.Get("/chores/new", management.ChoreNew)
	router.Post("/chores", management.ChoreCreate)
	router.Get("/chores/{choreID}", management.ChoreShow)
	router.Get("/chores/{choreID}/edit", management.ChoreEdit)
	router.Post("/chores/{choreID}", management.ChoreMutate)
	router.Put("/chores/{choreID}", management.ChoreUpdate)
	router.Delete("/chores/{choreID}", management.ChoreDelete)
	router.Get("/chores/{choreID}/schedules/new", management.ScheduleNew)
	router.Post("/chores/{choreID}/schedules", management.ScheduleCreate)
	router.Get("/schedules/{scheduleID}/edit", management.ScheduleEdit)
	router.Post("/schedules/{scheduleID}", management.ScheduleMutate)
	router.Put("/schedules/{scheduleID}", management.ScheduleUpdate)
	router.Delete("/schedules/{scheduleID}", management.ScheduleDelete)
	router.Get("/people", management.PeopleIndex)
	router.Post("/people", management.PersonCreate)
	router.Post("/people/household-event-color", management.HouseholdEventColorUpdate)
	router.Get("/events", events.Index)
	router.Get("/events/new", events.New)
	router.Post("/events", events.Create)
	router.Get("/events/{eventID}/edit", events.Edit)
	router.Post("/events/{eventID}", events.Mutate)
	// Compatibility for a form rendered by the initial admin-only release.
	router.Get("/admin/events", func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, "/events", http.StatusSeeOther) })
	router.Get("/admin/events/new", func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, "/events/new", http.StatusSeeOther) })
	router.Post("/admin/events", events.Create)
	router.Post("/admin/events/{eventID}", events.Mutate)
	if admin != nil {
		router.Get("/admin/login", admin.LoginPage)
		router.Post("/admin/login", admin.Login)
		router.Group(func(protected chi.Router) {
			protected.Use(deps.AdminAuth.Require)
			protected.Get("/admin", admin.Dashboard)
			protected.Post("/admin/logout", admin.Logout)
			protected.Post("/admin/calendar/{personID}/rotate", admin.RotateCalendarToken)
		})
	}

	return router
}
