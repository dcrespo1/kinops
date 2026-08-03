package handlers

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/a-h/templ"
	"github.com/go-chi/chi/v5"

	"github.com/dcrespo1/kinops/internal/domain"
	"github.com/dcrespo1/kinops/internal/kitchen"
	"github.com/dcrespo1/kinops/internal/mealie"
	"github.com/dcrespo1/kinops/internal/scheduling"
	"github.com/dcrespo1/kinops/internal/views/pages"
)

type KitchenService interface {
	DailyView(context.Context, time.Time) (domain.KitchenDailyView, error)
	WeeklyView(context.Context, time.Time) (domain.KitchenWeekView, error)
	RecipeView(context.Context, string, bool, int, time.Time, string) (domain.KitchenRecipeView, error)
	SetFavorite(context.Context, string, bool) error
	CreateMeal(context.Context, domain.KitchenMealInput) (domain.KitchenMeal, error)
	UpdateMeal(context.Context, int64, domain.KitchenMealInput) (domain.KitchenMeal, error)
	DeleteMeal(context.Context, int64) error
	GroceryView(context.Context, string) (domain.KitchenGroceryView, error)
	CreateGroceryItem(context.Context, domain.KitchenGroceryCreate) (domain.KitchenGroceryItem, error)
	UpdateGroceryItem(context.Context, string, domain.KitchenGroceryPatch) (domain.KitchenGroceryItem, error)
	DeleteGroceryItem(context.Context, string) error
}

type KitchenHandler struct {
	service  KitchenService
	logger   *slog.Logger
	location *time.Location
	now      func() time.Time
}

func NewKitchenHandler(service KitchenService, logger *slog.Logger, location *time.Location) *KitchenHandler {
	return &KitchenHandler{service: service, logger: logger, location: location, now: time.Now}
}

func (h *KitchenHandler) Root(w http.ResponseWriter, r *http.Request) {
	http.Redirect(w, r, "/kitchen/daily", http.StatusSeeOther)
}

func (h *KitchenHandler) Daily(w http.ResponseWriter, r *http.Request) {
	date, ok := h.date(w, r)
	if !ok {
		return
	}
	if h.service == nil {
		h.render(w, r, pages.KitchenDaily(domain.KitchenDailyView{Date: date, State: domain.KitchenDisabled, Message: "Connect Mealie to show meals here."}))
		return
	}
	view, err := h.service.DailyView(r.Context(), date)
	if err != nil {
		view = domain.KitchenDailyView{Date: date}
		h.applyError(r, err, &view.State, &view.Message)
	}
	h.render(w, r, pages.KitchenDaily(view))
}

func (h *KitchenHandler) Weekly(w http.ResponseWriter, r *http.Request) {
	date, ok := h.date(w, r)
	if !ok {
		return
	}
	if h.service == nil {
		start := date.AddDate(0, 0, -(int(date.Weekday())+6)%7)
		h.render(w, r, pages.KitchenWeekly(domain.KitchenWeekView{StartDate: start, EndDate: start.AddDate(0, 0, 6), State: domain.KitchenDisabled, Message: "Connect Mealie to show meals here."}))
		return
	}
	view, err := h.service.WeeklyView(r.Context(), date)
	if err != nil {
		start := date.AddDate(0, 0, -(int(date.Weekday())+6)%7)
		view = domain.KitchenWeekView{StartDate: start, EndDate: start.AddDate(0, 0, 6)}
		h.applyError(r, err, &view.State, &view.Message)
	}
	h.render(w, r, pages.KitchenWeekly(view))
}

func (h *KitchenHandler) Recipes(w http.ResponseWriter, r *http.Request) {
	date, ok := h.date(w, r)
	if !ok {
		return
	}
	page := 1
	if value := r.URL.Query().Get("page"); value != "" {
		parsed, err := strconv.Atoi(value)
		if err != nil || parsed < 1 {
			http.Error(w, "page must be a positive number", http.StatusBadRequest)
			return
		}
		page = parsed
	}
	search := strings.TrimSpace(r.URL.Query().Get("q"))
	if len(search) > 100 {
		http.Error(w, "search is too long", http.StatusBadRequest)
		return
	}
	view := domain.KitchenRecipeView{SelectedDate: date, SelectedType: "dinner"}
	if h.service == nil {
		view.State, view.Message = domain.KitchenDisabled, "Connect Mealie to browse recipes."
	} else {
		var err error
		view, err = h.service.RecipeView(r.Context(), search, r.URL.Query().Get("favorites") == "1", page, date, r.URL.Query().Get("entry_type"))
		if err != nil {
			view = domain.KitchenRecipeView{SelectedDate: date, SelectedType: "dinner"}
			h.applyError(r, err, &view.State, &view.Message)
		}
	}
	if isHTMXTarget(r, "recipe-results") && view.State == domain.KitchenReady {
		h.render(w, r, pages.KitchenRecipeResults(view))
		return
	}
	h.render(w, r, pages.KitchenRecipes(view))
}

func (h *KitchenHandler) Groceries(w http.ResponseWriter, r *http.Request) {
	navigationDate := scheduling.Date(h.now(), h.location)
	view := domain.KitchenGroceryView{NavigationDate: navigationDate}
	if h.service == nil {
		view.State, view.Message = domain.KitchenDisabled, "Connect Mealie to manage grocery lists."
		h.render(w, r, pages.KitchenGroceries(view))
		return
	}
	var err error
	view, err = h.service.GroceryView(r.Context(), r.URL.Query().Get("list"))
	view.NavigationDate = navigationDate
	if errors.Is(err, kitchen.ErrGroceryListNotFound) {
		http.Error(w, "shopping list not found", http.StatusBadRequest)
		return
	}
	if err != nil {
		view = domain.KitchenGroceryView{NavigationDate: navigationDate}
		h.applyError(r, err, &view.State, &view.Message)
	}
	h.render(w, r, pages.KitchenGroceries(view))
}

func (h *KitchenHandler) GroceryCreate(w http.ResponseWriter, r *http.Request) {
	if h.service == nil {
		http.Error(w, "Mealie integration is disabled", http.StatusServiceUnavailable)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "invalid form", http.StatusBadRequest)
		return
	}
	quantity, ok := groceryQuantity(w, r.FormValue("quantity"), true)
	if !ok {
		return
	}
	listID := strings.TrimSpace(r.FormValue("list"))
	_, err := h.service.CreateGroceryItem(r.Context(), domain.KitchenGroceryCreate{
		ShoppingListID: listID, Display: r.FormValue("display"), Note: r.FormValue("note"), Quantity: quantity, UnitName: r.FormValue("unit"),
	})
	h.respondGroceryMutation(w, r, listID, "grocery-add-error", err)
}

func (h *KitchenHandler) GroceryMutate(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "invalid form", http.StatusBadRequest)
		return
	}
	switch strings.ToUpper(r.FormValue("_method")) {
	case http.MethodPut, http.MethodPatch:
		h.GroceryUpdate(w, r)
	case http.MethodDelete:
		h.GroceryDelete(w, r)
	default:
		http.Error(w, "unsupported method", http.StatusMethodNotAllowed)
	}
}

func (h *KitchenHandler) GroceryUpdate(w http.ResponseWriter, r *http.Request) {
	if h.service == nil {
		http.Error(w, "Mealie integration is disabled", http.StatusServiceUnavailable)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "invalid form", http.StatusBadRequest)
		return
	}
	itemID := chi.URLParam(r, "itemID")
	if strings.TrimSpace(itemID) == "" {
		http.Error(w, "invalid grocery item ID", http.StatusBadRequest)
		return
	}
	patch := domain.KitchenGroceryPatch{}
	if r.Form.Has("display") {
		value := r.FormValue("display")
		patch.Display = &value
	}
	if r.Form.Has("note") {
		value := r.FormValue("note")
		patch.Note = &value
	}
	if r.Form.Has("unit") {
		value := r.FormValue("unit")
		patch.UnitName = &value
	}
	if r.Form.Has("quantity") {
		value, ok := groceryQuantity(w, r.FormValue("quantity"), false)
		if !ok {
			return
		}
		patch.Quantity = &value
	}
	if r.Form.Has("checked") {
		value, err := strconv.ParseBool(r.FormValue("checked"))
		if err != nil {
			http.Error(w, "checked must be true or false", http.StatusBadRequest)
			return
		}
		patch.Checked = &value
	}
	_, err := h.service.UpdateGroceryItem(r.Context(), itemID, patch)
	h.respondGroceryMutation(w, r, r.FormValue("list"), "grocery-item-error-"+itemID, err)
}

func (h *KitchenHandler) GroceryDelete(w http.ResponseWriter, r *http.Request) {
	if h.service == nil {
		http.Error(w, "Mealie integration is disabled", http.StatusServiceUnavailable)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "invalid form", http.StatusBadRequest)
		return
	}
	itemID := chi.URLParam(r, "itemID")
	if strings.TrimSpace(itemID) == "" {
		http.Error(w, "invalid grocery item ID", http.StatusBadRequest)
		return
	}
	err := h.service.DeleteGroceryItem(r.Context(), itemID)
	h.respondGroceryMutation(w, r, groceryListID(r), "grocery-item-error-"+itemID, err)
}

func (h *KitchenHandler) Favorite(w http.ResponseWriter, r *http.Request) {
	if h.service == nil {
		http.Error(w, "Mealie integration is disabled", http.StatusServiceUnavailable)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "invalid form", http.StatusBadRequest)
		return
	}
	slug := chi.URLParam(r, "recipeSlug")
	favorite := strings.ToUpper(r.FormValue("_method")) != http.MethodDelete && r.Method != http.MethodDelete
	err := h.service.SetFavorite(r.Context(), slug, favorite)
	if isHTMX(r) {
		message := ""
		current := favorite
		if err != nil {
			message = kitchenMutationError(err)
			current = !favorite
			h.logger.Warn("update kitchen favorite", "error", err)
		}
		h.render(w, r, pages.KitchenFavoriteControl(slug, current, safeKitchenReturn(r.FormValue("return_to")), message))
		return
	}
	if err != nil {
		http.Error(w, kitchenMutationError(err), kitchenErrorStatus(err))
		return
	}
	http.Redirect(w, r, safeKitchenReturn(r.FormValue("return_to")), http.StatusSeeOther)
}

func (h *KitchenHandler) MealCreate(w http.ResponseWriter, r *http.Request) {
	input, ok := h.mealInput(w, r)
	if !ok {
		return
	}
	_, err := h.service.CreateMeal(r.Context(), input)
	h.respondMealMutation(w, r, input.Date, "Meal added.", err)
}

func (h *KitchenHandler) MealMutate(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "invalid form", http.StatusBadRequest)
		return
	}
	switch strings.ToUpper(r.FormValue("_method")) {
	case http.MethodPut:
		h.MealUpdate(w, r)
	case http.MethodDelete:
		h.MealDelete(w, r)
	default:
		http.Error(w, "unsupported method", http.StatusMethodNotAllowed)
	}
}

func (h *KitchenHandler) MealUpdate(w http.ResponseWriter, r *http.Request) {
	id, ok := kitchenID(w, r)
	if !ok {
		return
	}
	input, ok := h.mealInput(w, r)
	if !ok {
		return
	}
	meal, err := h.service.UpdateMeal(r.Context(), id, input)
	if err != nil {
		h.respondMealCardError(w, r, id, err)
		return
	}
	if isHTMX(r) {
		h.render(w, r, pages.KitchenMealCard(meal))
		return
	}
	http.Redirect(w, r, safeKitchenReturn(r.FormValue("return_to")), http.StatusSeeOther)
}

func (h *KitchenHandler) MealDelete(w http.ResponseWriter, r *http.Request) {
	id, ok := kitchenID(w, r)
	if !ok {
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "invalid form", http.StatusBadRequest)
		return
	}
	err := h.service.DeleteMeal(r.Context(), id)
	if err != nil {
		h.respondMealCardError(w, r, id, err)
		return
	}
	if isHTMX(r) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		return
	}
	http.Redirect(w, r, safeKitchenReturn(r.FormValue("return_to")), http.StatusSeeOther)
}

func (h *KitchenHandler) mealInput(w http.ResponseWriter, r *http.Request) (domain.KitchenMealInput, bool) {
	if h.service == nil {
		http.Error(w, "Mealie integration is disabled", http.StatusServiceUnavailable)
		return domain.KitchenMealInput{}, false
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "invalid form", http.StatusBadRequest)
		return domain.KitchenMealInput{}, false
	}
	date, err := scheduling.ParseDate(r.FormValue("date"), h.location)
	if err != nil {
		http.Error(w, "date must use YYYY-MM-DD", http.StatusBadRequest)
		return domain.KitchenMealInput{}, false
	}
	return domain.KitchenMealInput{Date: date, EntryType: r.FormValue("entry_type"), Title: r.FormValue("title"), Text: r.FormValue("text"), RecipeID: r.FormValue("recipe_id")}, true
}

func (h *KitchenHandler) respondMealMutation(w http.ResponseWriter, r *http.Request, date time.Time, success string, err error) {
	if isHTMX(r) {
		if err != nil {
			h.logger.Warn("mutate kitchen meal", "error", err)
			h.render(w, r, pages.KitchenMutationResult(false, kitchenMutationError(err), ""))
			return
		}
		h.render(w, r, pages.KitchenMutationResult(true, success, date.Format(scheduling.DateLayout)))
		return
	}
	if err != nil {
		http.Error(w, kitchenMutationError(err), kitchenErrorStatus(err))
		return
	}
	http.Redirect(w, r, "/kitchen/daily?date="+date.Format(scheduling.DateLayout), http.StatusSeeOther)
}

func (h *KitchenHandler) respondMealCardError(w http.ResponseWriter, r *http.Request, id int64, err error) {
	h.logger.Warn("mutate kitchen meal", "meal_id", id, "error", err)
	if isHTMX(r) {
		w.Header().Set("HX-Retarget", fmt.Sprintf("#meal-%d .kitchen-meal-edit", id))
		w.Header().Set("HX-Reswap", "beforeend")
		h.render(w, r, pages.KitchenMutationResult(false, kitchenMutationError(err), ""))
		return
	}
	http.Error(w, kitchenMutationError(err), kitchenErrorStatus(err))
}

func (h *KitchenHandler) date(w http.ResponseWriter, r *http.Request) (time.Time, bool) {
	value := r.URL.Query().Get("date")
	if value == "" {
		return scheduling.Date(h.now(), h.location), true
	}
	date, err := scheduling.ParseDate(value, h.location)
	if err != nil {
		http.Error(w, "date must use YYYY-MM-DD", http.StatusBadRequest)
		return time.Time{}, false
	}
	return date, true
}

func (h *KitchenHandler) applyError(r *http.Request, err error, state *domain.KitchenState, message *string) {
	switch {
	case errors.Is(err, mealie.ErrUnauthorized), errors.Is(err, mealie.ErrForbidden):
		*state, *message = domain.KitchenUnauthorized, "Mealie rejected the configured API token."
	default:
		*state, *message = domain.KitchenUnavailable, "Mealie is unavailable right now. Chores and events are still available."
	}
	h.logger.Warn("kitchen request degraded", "path", r.URL.Path, "error", err)
}

func (h *KitchenHandler) render(w http.ResponseWriter, r *http.Request, component templ.Component) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := component.Render(r.Context(), w); err != nil {
		h.logger.Error("render kitchen response", "error", err)
	}
}

func (h *KitchenHandler) respondGroceryMutation(w http.ResponseWriter, r *http.Request, listID, errorTarget string, mutationErr error) {
	listID = strings.TrimSpace(listID)
	if mutationErr != nil {
		h.logger.Warn("mutate kitchen grocery", "error", mutationErr)
		if isHTMX(r) {
			if !validGroceryTarget(errorTarget) {
				errorTarget = "grocery-errors"
			}
			w.Header().Set("HX-Retarget", "#"+errorTarget)
			w.Header().Set("HX-Reswap", "innerHTML")
			h.render(w, r, pages.KitchenMutationResult(false, kitchenMutationError(mutationErr), ""))
			return
		}
		http.Error(w, kitchenMutationError(mutationErr), kitchenErrorStatus(mutationErr))
		return
	}
	if isHTMX(r) {
		view, err := h.service.GroceryView(r.Context(), listID)
		if err != nil {
			h.logger.Warn("refresh kitchen groceries after mutation", "error", err)
			w.Header().Set("HX-Retarget", "#grocery-errors")
			w.Header().Set("HX-Reswap", "innerHTML")
			h.render(w, r, pages.KitchenMutationResult(false, "Saved in Mealie. Refresh the list to see the change.", ""))
			return
		}
		view.NavigationDate = scheduling.Date(h.now(), h.location)
		h.render(w, r, pages.KitchenGroceryContent(view))
		return
	}
	values := url.Values{}
	if listID != "" {
		values.Set("list", listID)
	}
	target := "/kitchen/groceries"
	if encoded := values.Encode(); encoded != "" {
		target += "?" + encoded
	}
	http.Redirect(w, r, target, http.StatusSeeOther)
}

func groceryQuantity(w http.ResponseWriter, raw string, defaultOne bool) (float64, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" && defaultOne {
		return 1, true
	}
	quantity, err := strconv.ParseFloat(raw, 64)
	if err != nil || quantity < 0 {
		http.Error(w, "quantity must be zero or greater", http.StatusBadRequest)
		return 0, false
	}
	return quantity, true
}

func validGroceryTarget(value string) bool {
	if value == "grocery-add-error" || value == "grocery-errors" {
		return true
	}
	if !strings.HasPrefix(value, "grocery-item-error-") || len(value) > 140 {
		return false
	}
	for _, character := range strings.TrimPrefix(value, "grocery-item-error-") {
		if (character < 'a' || character > 'z') && (character < 'A' || character > 'Z') && (character < '0' || character > '9') && character != '-' {
			return false
		}
	}
	return true
}

func groceryListID(r *http.Request) string {
	if value := strings.TrimSpace(r.FormValue("list")); value != "" {
		return value
	}
	return strings.TrimSpace(r.URL.Query().Get("list"))
}

func kitchenID(w http.ResponseWriter, r *http.Request) (int64, bool) {
	id, err := strconv.ParseInt(chi.URLParam(r, "mealID"), 10, 64)
	if err != nil || id < 1 {
		http.Error(w, "invalid meal ID", http.StatusBadRequest)
		return 0, false
	}
	return id, true
}

func isHTMX(r *http.Request) bool { return r.Header.Get("HX-Request") == "true" }

func isHTMXTarget(r *http.Request, target string) bool {
	return isHTMX(r) && r.Header.Get("HX-Target") == target
}

func safeKitchenReturn(raw string) string {
	parsed, err := url.Parse(raw)
	if err != nil || parsed.IsAbs() || parsed.Host != "" || !strings.HasPrefix(parsed.Path, "/kitchen/") {
		return "/kitchen/recipes"
	}
	return parsed.RequestURI()
}

func kitchenMutationError(err error) string {
	switch {
	case errors.Is(err, mealie.ErrUnauthorized), errors.Is(err, mealie.ErrForbidden):
		return "Mealie rejected the configured API token."
	case errors.Is(err, mealie.ErrValidation):
		return "Mealie rejected those meal details."
	case errors.Is(err, mealie.ErrNotFound):
		return "That Mealie item no longer exists."
	case errors.Is(err, mealie.ErrRateLimited):
		return "Mealie is busy. Please try again shortly."
	case errors.Is(err, mealie.ErrUnavailable), errors.Is(err, context.DeadlineExceeded):
		return "Mealie is unavailable right now."
	default:
		return "Check the meal details and try again."
	}
}

func kitchenErrorStatus(err error) int {
	switch {
	case errors.Is(err, mealie.ErrUnauthorized):
		return http.StatusUnauthorized
	case errors.Is(err, mealie.ErrForbidden):
		return http.StatusForbidden
	case errors.Is(err, mealie.ErrNotFound):
		return http.StatusNotFound
	case errors.Is(err, mealie.ErrUnavailable), errors.Is(err, mealie.ErrRateLimited), errors.Is(err, context.DeadlineExceeded):
		return http.StatusServiceUnavailable
	default:
		return http.StatusUnprocessableEntity
	}
}
