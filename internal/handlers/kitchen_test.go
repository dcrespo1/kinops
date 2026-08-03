package handlers

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/dcrespo1/kinops/internal/domain"
	"github.com/dcrespo1/kinops/internal/mealie"
)

type fakeKitchenService struct {
	daily          domain.KitchenDailyView
	week           domain.KitchenWeekView
	recipes        domain.KitchenRecipeView
	meal           domain.KitchenMeal
	groceries      domain.KitchenGroceryView
	grocery        domain.KitchenGroceryItem
	err            error
	date           time.Time
	favorite       bool
	deleted        int64
	deletedGrocery string
	groceryList    string
}

func (f *fakeKitchenService) DailyView(_ context.Context, date time.Time) (domain.KitchenDailyView, error) {
	f.date = date
	return f.daily, f.err
}
func (f *fakeKitchenService) WeeklyView(_ context.Context, date time.Time) (domain.KitchenWeekView, error) {
	f.date = date
	return f.week, f.err
}
func (f *fakeKitchenService) RecipeView(_ context.Context, _ string, _ bool, _ int, date time.Time, _ string) (domain.KitchenRecipeView, error) {
	f.date = date
	return f.recipes, f.err
}
func (f *fakeKitchenService) SetFavorite(_ context.Context, _ string, favorite bool) error {
	f.favorite = favorite
	return f.err
}
func (f *fakeKitchenService) CreateMeal(_ context.Context, _ domain.KitchenMealInput) (domain.KitchenMeal, error) {
	return f.meal, f.err
}
func (f *fakeKitchenService) UpdateMeal(_ context.Context, _ int64, _ domain.KitchenMealInput) (domain.KitchenMeal, error) {
	return f.meal, f.err
}
func (f *fakeKitchenService) DeleteMeal(_ context.Context, id int64) error {
	f.deleted = id
	return f.err
}
func (f *fakeKitchenService) GroceryView(_ context.Context, listID string) (domain.KitchenGroceryView, error) {
	f.groceryList = listID
	return f.groceries, f.err
}
func (f *fakeKitchenService) CreateGroceryItem(_ context.Context, _ domain.KitchenGroceryCreate) (domain.KitchenGroceryItem, error) {
	return f.grocery, f.err
}
func (f *fakeKitchenService) UpdateGroceryItem(_ context.Context, _ string, _ domain.KitchenGroceryPatch) (domain.KitchenGroceryItem, error) {
	return f.grocery, f.err
}
func (f *fakeKitchenService) DeleteGroceryItem(_ context.Context, id string) error {
	f.deletedGrocery = id
	return f.err
}

func TestKitchenHandlerDailyAndDisabledStates(t *testing.T) {
	disabled := NewKitchenHandler(nil, discardLogger(), time.UTC)
	recorder := httptest.NewRecorder()
	disabled.Daily(recorder, httptest.NewRequest(http.MethodGet, "/kitchen/daily?date=2026-08-02", nil))
	if recorder.Code != http.StatusOK || !strings.Contains(recorder.Body.String(), "Connect Mealie") {
		t.Fatalf("disabled response = %d %s", recorder.Code, recorder.Body.String())
	}

	meal := domain.KitchenMeal{ID: 1, Title: "Tacos"}
	service := &fakeKitchenService{daily: domain.KitchenDailyView{Date: time.Date(2026, 8, 2, 0, 0, 0, 0, time.UTC), State: domain.KitchenReady, Day: domain.KitchenDay{Groups: []domain.KitchenMealGroup{{Label: "Dinner", Meals: []domain.KitchenMeal{meal}}}}}}
	handler := NewKitchenHandler(service, discardLogger(), time.UTC)
	recorder = httptest.NewRecorder()
	handler.Daily(recorder, httptest.NewRequest(http.MethodGet, "/kitchen/daily?date=2026-08-02", nil))
	if recorder.Code != http.StatusOK || !strings.Contains(recorder.Body.String(), "Tacos") || service.date.Format("2006-01-02") != "2026-08-02" {
		t.Fatalf("ready response = %d %s", recorder.Code, recorder.Body.String())
	}
}

func TestKitchenHandlerValidationAndUpstreamStates(t *testing.T) {
	handler := NewKitchenHandler(&fakeKitchenService{err: mealie.ErrUnauthorized}, discardLogger(), time.UTC)
	recorder := httptest.NewRecorder()
	handler.Weekly(recorder, httptest.NewRequest(http.MethodGet, "/kitchen/weekly?date=2026-08-05", nil))
	if recorder.Code != http.StatusOK || !strings.Contains(recorder.Body.String(), "rejected the configured API token") {
		t.Fatalf("unauthorized response = %d %s", recorder.Code, recorder.Body.String())
	}

	handler = NewKitchenHandler(&fakeKitchenService{err: errors.New("offline")}, discardLogger(), time.UTC)
	recorder = httptest.NewRecorder()
	handler.Daily(recorder, httptest.NewRequest(http.MethodGet, "/kitchen/daily?date=bad", nil))
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("invalid date status = %d", recorder.Code)
	}
}

func TestKitchenHandlerRecipesAndFavoriteFragments(t *testing.T) {
	date := time.Date(2026, 8, 3, 0, 0, 0, 0, time.UTC)
	service := &fakeKitchenService{recipes: domain.KitchenRecipeView{
		State: domain.KitchenReady, SelectedDate: date, SelectedType: "dinner", Page: 1, TotalPages: 1, Total: 1,
		Recipes: []domain.KitchenRecipeCard{{ID: "recipe-id", Slug: "soup", Name: "Soup"}},
	}}
	handler := NewKitchenHandler(service, discardLogger(), time.UTC)
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/kitchen/recipes?date=2026-08-03&entry_type=dinner", nil)
	handler.Recipes(recorder, request)
	if recorder.Code != http.StatusOK || !strings.Contains(recorder.Body.String(), "Soup") || !strings.Contains(recorder.Body.String(), "recipe-search") {
		t.Fatalf("recipe page = %d %s", recorder.Code, recorder.Body.String())
	}

	router := chi.NewRouter()
	router.Post("/kitchen/recipes/{recipeSlug}/favorite", handler.Favorite)
	recorder = httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodPost, "/kitchen/recipes/soup/favorite", strings.NewReader("return_to=%2Fkitchen%2Frecipes"))
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.Header.Set("HX-Request", "true")
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK || !service.favorite || !strings.Contains(recorder.Body.String(), "★") {
		t.Fatalf("favorite fragment = %d %s", recorder.Code, recorder.Body.String())
	}
}

func TestKitchenHandlerMealMutationFragments(t *testing.T) {
	date := time.Date(2026, 8, 3, 0, 0, 0, 0, time.UTC)
	service := &fakeKitchenService{meal: domain.KitchenMeal{ID: 8, Date: date, EntryType: "dinner", Title: "Soup"}}
	handler := NewKitchenHandler(service, discardLogger(), time.UTC)
	router := chi.NewRouter()
	router.Post("/kitchen/meals", handler.MealCreate)
	router.Delete("/kitchen/meals/{mealID}", handler.MealDelete)

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/kitchen/meals", strings.NewReader("date=2026-08-03&entry_type=dinner&recipe_id=recipe-id"))
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.Header.Set("HX-Request", "true")
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK || !strings.Contains(recorder.Body.String(), "Meal added") || !strings.Contains(recorder.Body.String(), "2026-08-03") {
		t.Fatalf("create fragment = %d %s", recorder.Code, recorder.Body.String())
	}

	recorder = httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodDelete, "/kitchen/meals/8", nil)
	request.Header.Set("HX-Request", "true")
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK || service.deleted != 8 || recorder.Body.Len() != 0 {
		t.Fatalf("delete fragment = %d %q, deleted=%d", recorder.Code, recorder.Body.String(), service.deleted)
	}
}

func TestKitchenHandlerGroceryPageAndMutationFragments(t *testing.T) {
	service := &fakeKitchenService{groceries: domain.KitchenGroceryView{
		State: domain.KitchenReady, Lists: []domain.KitchenShoppingList{{ID: "list-id", Name: "Weekly"}},
		SelectedListID: "list-id", SelectedListName: "Weekly",
		ActiveGroups: []domain.KitchenGroceryGroup{{Label: "Other", Items: []domain.KitchenGroceryItem{{ID: "item-id", ShoppingListID: "list-id", Display: "Milk", Quantity: 1}}}},
	}}
	handler := NewKitchenHandler(service, discardLogger(), time.UTC)
	recorder := httptest.NewRecorder()
	handler.Groceries(recorder, httptest.NewRequest(http.MethodGet, "/kitchen/groceries?list=list-id", nil))
	if recorder.Code != http.StatusOK || !strings.Contains(recorder.Body.String(), "Weekly") || !strings.Contains(recorder.Body.String(), "Milk") {
		t.Fatalf("grocery page = %d %s", recorder.Code, recorder.Body.String())
	}

	router := chi.NewRouter()
	router.Post("/kitchen/groceries/items", handler.GroceryCreate)
	router.Delete("/kitchen/groceries/items/{itemID}", handler.GroceryDelete)
	recorder = httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/kitchen/groceries/items", strings.NewReader("list=list-id&display=Eggs&quantity=1"))
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.Header.Set("HX-Request", "true")
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK || !strings.Contains(recorder.Body.String(), `id="grocery-list-content"`) {
		t.Fatalf("grocery create fragment = %d %s", recorder.Code, recorder.Body.String())
	}

	recorder = httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodDelete, "/kitchen/groceries/items/item-id?list=list-id", nil)
	request.Header.Set("HX-Request", "true")
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK || service.deletedGrocery != "item-id" || service.groceryList != "list-id" {
		t.Fatalf("grocery delete = %d, deleted=%q, list=%q", recorder.Code, service.deletedGrocery, service.groceryList)
	}
}

func TestKitchenHandlerGroceryWriteErrorStaysInline(t *testing.T) {
	service := &fakeKitchenService{err: mealie.ErrValidation}
	handler := NewKitchenHandler(service, discardLogger(), time.UTC)
	router := chi.NewRouter()
	router.Put("/kitchen/groceries/items/{itemID}", handler.GroceryUpdate)
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPut, "/kitchen/groceries/items/item-id", strings.NewReader("list=list-id&checked=true"))
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.Header.Set("HX-Request", "true")
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK || recorder.Header().Get("HX-Retarget") != "#grocery-item-error-item-id" || !strings.Contains(recorder.Body.String(), "rejected") {
		t.Fatalf("grocery error = %d, target=%q, body=%s", recorder.Code, recorder.Header().Get("HX-Retarget"), recorder.Body.String())
	}
}
