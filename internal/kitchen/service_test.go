package kitchen

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/dcrespo1/kinops/internal/domain"
	"github.com/dcrespo1/kinops/internal/mealie"
)

type flakyMealPlans struct {
	page  mealie.Page[mealie.PlanEntry]
	err   error
	calls int
}

type statusClient struct {
	flakyMealPlans
	about mealie.About
	err   error
}

func (c *statusClient) About(context.Context) (mealie.About, error) { return c.about, c.err }

func TestMealieStatusReportsConnectivityWithoutReturningAnError(t *testing.T) {
	now := time.Date(2026, 8, 2, 12, 0, 0, 0, time.UTC)
	client := &statusClient{flakyMealPlans: flakyMealPlans{page: mealie.Page[mealie.PlanEntry]{TotalPages: 1}}, about: mealie.About{Version: "v3.22.0"}}
	service := NewWithOptions(client, time.UTC, "", "", Options{Now: func() time.Time { return now }})
	connected := service.MealieStatus(context.Background())
	if !connected.Enabled || !connected.Connected || connected.Version != "v3.22.0" || !connected.CheckedAt.Equal(now) {
		t.Fatalf("connected status = %#v", connected)
	}
	client.err = mealie.ErrUnavailable
	unavailable := service.MealieStatus(context.Background())
	if !unavailable.Enabled || unavailable.Connected || unavailable.Message == "" {
		t.Fatalf("unavailable status = %#v", unavailable)
	}
}

func (f *flakyMealPlans) MealPlans(context.Context, time.Time, time.Time, mealie.Pagination) (mealie.Page[mealie.PlanEntry], error) {
	f.calls++
	return f.page, f.err
}

func TestMealCacheFreshStaleExpiryAndRecovery(t *testing.T) {
	now := time.Date(2026, 8, 2, 12, 0, 0, 0, time.UTC)
	client := &flakyMealPlans{page: mealie.Page[mealie.PlanEntry]{TotalPages: 1, Items: []mealie.PlanEntry{{ID: 1, Date: "2026-08-02", EntryType: mealie.PlanEntryDinner, Title: stringPointer("Soup")}}}}
	service := NewWithOptions(client, time.UTC, "", "", Options{Now: func() time.Time { return now }, MaxEntries: 2, MealFreshFor: time.Minute, StaleFor: time.Hour})
	first, err := service.DailyView(context.Background(), now)
	if err != nil || first.Stale || client.calls != 1 {
		t.Fatalf("first view = %#v, err=%v calls=%d", first, err, client.calls)
	}
	if _, err := service.DailyView(context.Background(), now); err != nil || client.calls != 1 {
		t.Fatalf("fresh cache err=%v calls=%d", err, client.calls)
	}

	now = now.Add(2 * time.Minute)
	client.err = mealie.ErrUnavailable
	stale, err := service.DailyView(context.Background(), now.Add(-2*time.Minute))
	if err != nil || !stale.Stale || stale.StaleMessage == "" || client.calls != 2 {
		t.Fatalf("stale view = %#v, err=%v calls=%d", stale, err, client.calls)
	}

	now = now.Add(2 * time.Hour)
	if _, err := service.DailyView(context.Background(), now.Add(-2*time.Hour-2*time.Minute)); !errors.Is(err, mealie.ErrUnavailable) {
		t.Fatalf("expired cache error = %v", err)
	}
	client.err = nil
	recovered, err := service.DailyView(context.Background(), now.Add(-2*time.Hour-2*time.Minute))
	if err != nil || recovered.Stale {
		t.Fatalf("recovered view = %#v, err=%v", recovered, err)
	}
}

type cacheMutationClient struct {
	flakyMealPlans
	created mealie.PlanEntry
}

type recipeCacheClient struct {
	flakyMealPlans
	page        mealie.Page[mealie.RecipeSummary]
	recipeErr   error
	recipeCalls int
	favorite    bool
}

func (c *recipeCacheClient) Recipes(context.Context, mealie.RecipeQuery) (mealie.Page[mealie.RecipeSummary], error) {
	c.recipeCalls++
	return c.page, c.recipeErr
}
func (c *recipeCacheClient) Favorites(context.Context) ([]mealie.Rating, error) { return nil, nil }
func (c *recipeCacheClient) CurrentUser(context.Context) (mealie.User, error) {
	return mealie.User{ID: "user-id"}, nil
}
func (c *recipeCacheClient) SetFavorite(_ context.Context, _, _ string, favorite bool) error {
	c.favorite = favorite
	return nil
}

func TestRecipeCacheStaleFallbackAndFavoriteInvalidation(t *testing.T) {
	now := time.Date(2026, 8, 2, 0, 0, 0, 0, time.UTC)
	client := &recipeCacheClient{
		flakyMealPlans: flakyMealPlans{page: mealie.Page[mealie.PlanEntry]{TotalPages: 1}},
		page:           mealie.Page[mealie.RecipeSummary]{Page: 1, TotalPages: 1, Total: 1, Items: []mealie.RecipeSummary{{ID: "recipe-id", Slug: "soup", Name: "Soup"}}},
	}
	service := NewWithOptions(client, time.UTC, "", "", Options{Now: func() time.Time { return now }, RecipeFreshFor: time.Minute, StaleFor: time.Hour})
	if _, err := service.RecipeView(context.Background(), "", false, 1, now, "dinner"); err != nil {
		t.Fatal(err)
	}
	if _, err := service.RecipeView(context.Background(), "", false, 1, now, "lunch"); err != nil || client.recipeCalls != 1 {
		t.Fatalf("fresh recipe cache calls=%d err=%v", client.recipeCalls, err)
	}
	now = now.Add(2 * time.Minute)
	client.recipeErr = mealie.ErrUnavailable
	stale, err := service.RecipeView(context.Background(), "", false, 1, now, "dinner")
	if err != nil || !stale.Stale || client.recipeCalls != 2 {
		t.Fatalf("stale recipes = %#v, calls=%d err=%v", stale, client.recipeCalls, err)
	}
	client.recipeErr = nil
	if err := service.SetFavorite(context.Background(), "soup", true); err != nil || !client.favorite {
		t.Fatalf("SetFavorite()=%v favorite=%v", err, client.favorite)
	}
	refreshed, err := service.RecipeView(context.Background(), "", false, 1, now, "dinner")
	if err != nil || refreshed.Stale || client.recipeCalls != 3 {
		t.Fatalf("refreshed recipes = %#v, calls=%d err=%v", refreshed, client.recipeCalls, err)
	}
}

func (c *cacheMutationClient) MealPlan(context.Context, int64) (mealie.PlanEntry, error) {
	return mealie.PlanEntry{}, nil
}
func (c *cacheMutationClient) CreateMealPlan(context.Context, mealie.CreatePlanEntry) (mealie.PlanEntry, error) {
	return c.created, nil
}
func (c *cacheMutationClient) UpdateMealPlan(context.Context, int64, mealie.UpdatePlanEntry) (mealie.PlanEntry, error) {
	return c.created, nil
}
func (c *cacheMutationClient) DeleteMealPlan(context.Context, int64) (mealie.PlanEntry, error) {
	return c.created, nil
}

func TestSuccessfulMealWriteInvalidatesReadCache(t *testing.T) {
	now := time.Date(2026, 8, 2, 0, 0, 0, 0, time.UTC)
	client := &cacheMutationClient{flakyMealPlans: flakyMealPlans{page: mealie.Page[mealie.PlanEntry]{TotalPages: 1}}, created: mealie.PlanEntry{ID: 2, Date: "2026-08-02", EntryType: mealie.PlanEntryDinner, Title: stringPointer("Dinner out")}}
	service := NewWithOptions(client, time.UTC, "", "", Options{Now: func() time.Time { return now }, MealFreshFor: time.Hour, StaleFor: 2 * time.Hour})
	if _, err := service.DailyView(context.Background(), now); err != nil {
		t.Fatal(err)
	}
	if _, err := service.DailyView(context.Background(), now); err != nil || client.calls != 1 {
		t.Fatalf("prime calls=%d err=%v", client.calls, err)
	}
	if _, err := service.CreateMeal(context.Background(), domain.KitchenMealInput{Date: now, EntryType: "dinner", Title: "Dinner out"}); err != nil {
		t.Fatal(err)
	}
	if _, err := service.DailyView(context.Background(), now); err != nil || client.calls != 2 {
		t.Fatalf("post-write calls=%d err=%v", client.calls, err)
	}
}

func TestMemoryCacheIsBounded(t *testing.T) {
	cache := newMemoryCache[int](2)
	now := time.Now()
	cache.Set("one", 1, now)
	cache.Set("two", 2, now.Add(time.Second))
	cache.Set("three", 3, now.Add(2*time.Second))
	if _, ok, _ := cache.Get("one", now.Add(3*time.Second), time.Hour, time.Hour); ok {
		t.Fatal("oldest cache entry was not evicted")
	}
	if len(cache.entries) != 2 {
		t.Fatalf("cache entries = %d", len(cache.entries))
	}
}

type fakeMealPlans struct {
	pages []mealie.Page[mealie.PlanEntry]
	calls []mealie.Pagination
	start time.Time
	end   time.Time
}

func (f *fakeMealPlans) MealPlans(_ context.Context, start, end time.Time, pagination mealie.Pagination) (mealie.Page[mealie.PlanEntry], error) {
	f.start, f.end = start, end
	f.calls = append(f.calls, pagination)
	return f.pages[pagination.Page-1], nil
}

func TestDailyAndWeeklyViewsGroupMealsInHouseholdTime(t *testing.T) {
	recipeName := "Tomato soup"
	client := &fakeMealPlans{pages: []mealie.Page[mealie.PlanEntry]{{TotalPages: 2, Items: []mealie.PlanEntry{
		{ID: 2, Date: "2026-08-03", EntryType: mealie.PlanEntryDinner, Recipe: &mealie.RecipeSummary{ID: "recipe", Slug: "tomato-soup", Name: recipeName}},
		{ID: 3, Date: "2026-08-03", EntryType: mealie.PlanEntryDinner, Title: stringPointer("Dinner note")},
	}}, {TotalPages: 2, Items: []mealie.PlanEntry{
		{ID: 1, Date: "2026-08-04", EntryType: mealie.PlanEntryBreakfast, Title: stringPointer("Pancakes")},
	}}}}
	service := New(client, time.FixedZone("household", -4*60*60), "http://localhost:9925/", "")
	view, err := service.WeeklyView(context.Background(), time.Date(2026, time.August, 5, 23, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	if got := view.StartDate.Format("2006-01-02"); got != "2026-08-03" {
		t.Errorf("week start = %s", got)
	}
	if len(client.calls) != 2 || client.calls[0].Page != 1 || client.calls[1].Page != 2 {
		t.Errorf("pagination calls = %#v", client.calls)
	}
	if len(view.Days) != 7 || len(view.Days[0].Groups) != 1 || len(view.Days[0].Groups[0].Meals) != 2 {
		t.Fatalf("week days = %#v", view.Days)
	}
	if view.Days[0].Groups[0].Meals[0].ID != 2 || view.PublicURL != "http://localhost:9925" {
		t.Errorf("mapped view = %#v", view)
	}
}

func TestDailyViewUsesSingleDate(t *testing.T) {
	client := &fakeMealPlans{pages: []mealie.Page[mealie.PlanEntry]{{TotalPages: 1}}}
	service := New(client, time.UTC, "", "")
	date := time.Date(2026, time.August, 2, 12, 0, 0, 0, time.UTC)
	view, err := service.DailyView(context.Background(), date)
	if err != nil {
		t.Fatal(err)
	}
	if !client.start.Equal(client.end) || !view.Date.Equal(view.Day.Date) {
		t.Errorf("daily range = %v through %v, view = %#v", client.start, client.end, view)
	}
}

func stringPointer(value string) *string { return &value }

type fakeKitchenClient struct {
	recipePages  []mealie.Page[mealie.RecipeSummary]
	favorites    []mealie.Rating
	user         mealie.User
	current      mealie.PlanEntry
	created      mealie.PlanEntry
	updated      mealie.PlanEntry
	updateInput  mealie.UpdatePlanEntry
	favorite     bool
	deleted      int64
	lists        mealie.Page[mealie.ShoppingList]
	items        mealie.Page[mealie.ShoppingItem]
	currentItem  mealie.ShoppingItem
	createdItem  mealie.ShoppingItem
	updatedItem  mealie.ShoppingItem
	groceryWrite mealie.ShoppingItemWrite
	deletedItem  string
	foods        mealie.Page[mealie.IngredientSummary]
	units        mealie.Page[mealie.IngredientSummary]
	listCalls    int
}

func (f *fakeKitchenClient) MealPlans(context.Context, time.Time, time.Time, mealie.Pagination) (mealie.Page[mealie.PlanEntry], error) {
	return mealie.Page[mealie.PlanEntry]{TotalPages: 1}, nil
}
func (f *fakeKitchenClient) Recipes(_ context.Context, _ mealie.RecipeQuery) (mealie.Page[mealie.RecipeSummary], error) {
	return f.recipePages[0], nil
}
func (f *fakeKitchenClient) Favorites(context.Context) ([]mealie.Rating, error) {
	return f.favorites, nil
}
func (f *fakeKitchenClient) CurrentUser(context.Context) (mealie.User, error) { return f.user, nil }
func (f *fakeKitchenClient) SetFavorite(_ context.Context, _ string, _ string, favorite bool) error {
	f.favorite = favorite
	return nil
}
func (f *fakeKitchenClient) MealPlan(context.Context, int64) (mealie.PlanEntry, error) {
	return f.current, nil
}
func (f *fakeKitchenClient) CreateMealPlan(context.Context, mealie.CreatePlanEntry) (mealie.PlanEntry, error) {
	return f.created, nil
}
func (f *fakeKitchenClient) UpdateMealPlan(_ context.Context, _ int64, input mealie.UpdatePlanEntry) (mealie.PlanEntry, error) {
	f.updateInput = input
	return f.updated, nil
}
func (f *fakeKitchenClient) DeleteMealPlan(_ context.Context, id int64) (mealie.PlanEntry, error) {
	f.deleted = id
	return f.current, nil
}
func (f *fakeKitchenClient) ShoppingLists(context.Context, mealie.Pagination) (mealie.Page[mealie.ShoppingList], error) {
	f.listCalls++
	return f.lists, nil
}
func (f *fakeKitchenClient) Foods(context.Context, string, mealie.Pagination) (mealie.Page[mealie.IngredientSummary], error) {
	return f.foods, nil
}
func (f *fakeKitchenClient) Units(context.Context, string, mealie.Pagination) (mealie.Page[mealie.IngredientSummary], error) {
	return f.units, nil
}
func (f *fakeKitchenClient) CreateFood(_ context.Context, name string) (mealie.IngredientSummary, error) {
	return mealie.IngredientSummary{ID: "created-food", Name: name}, nil
}
func (f *fakeKitchenClient) CreateUnit(_ context.Context, name string) (mealie.IngredientSummary, error) {
	return mealie.IngredientSummary{ID: "created-unit", Name: name}, nil
}
func (f *fakeKitchenClient) ShoppingItems(context.Context, string, mealie.Pagination) (mealie.Page[mealie.ShoppingItem], error) {
	return f.items, nil
}
func (f *fakeKitchenClient) ShoppingItem(context.Context, string) (mealie.ShoppingItem, error) {
	return f.currentItem, nil
}
func (f *fakeKitchenClient) CreateShoppingItem(_ context.Context, input mealie.ShoppingItemWrite) (mealie.ShoppingItem, error) {
	f.groceryWrite = input
	return f.createdItem, nil
}
func (f *fakeKitchenClient) UpdateShoppingItem(_ context.Context, _ string, input mealie.ShoppingItemWrite) (mealie.ShoppingItem, error) {
	f.groceryWrite = input
	return f.updatedItem, nil
}
func (f *fakeKitchenClient) DeleteShoppingItem(_ context.Context, id string) error {
	f.deletedItem = id
	return nil
}

func TestRecipeViewMapsFavoritesImagesAndMealieLinks(t *testing.T) {
	image := "cache-key"
	rating := 4.5
	client := &fakeKitchenClient{
		recipePages: []mealie.Page[mealie.RecipeSummary]{{Page: 1, TotalPages: 1, Total: 1, Items: []mealie.RecipeSummary{{
			ID: "recipe-id", Slug: "tomato-soup", Name: "Tomato soup", Image: &image, Rating: &rating,
			Categories: []mealie.OrganizerSummary{{Name: "Dinner"}}, PrepTime: stringPointer("10 minutes"), TotalTime: stringPointer("30 minutes"),
		}}}},
		favorites: []mealie.Rating{{RecipeID: "recipe-id", Favorite: true}},
		user:      mealie.User{ID: "user-id", GroupSlug: "family-kitchen"},
	}
	service := New(client, time.UTC, "http://localhost:9925", "")
	view, err := service.RecipeView(context.Background(), " soup ", false, 1, time.Date(2026, 8, 3, 12, 0, 0, 0, time.UTC), "dinner")
	if err != nil {
		t.Fatal(err)
	}
	if len(view.Recipes) != 1 || !view.Recipes[0].Favorite || view.Recipes[0].Categories[0] != "Dinner" {
		t.Fatalf("recipe view = %#v", view)
	}
	if !strings.Contains(view.Recipes[0].ImageURL, "/api/media/recipes/recipe-id/images/min-original.webp") || view.Recipes[0].MealieURL != "http://localhost:9925/g/family-kitchen/r/tomato-soup" {
		t.Errorf("recipe URLs = %#v", view.Recipes[0])
	}
	if err := service.SetFavorite(context.Background(), "tomato-soup", false); err != nil || client.favorite {
		t.Errorf("SetFavorite() = %v, favorite=%v", err, client.favorite)
	}
}

func TestMealMutationsUseServerOwnedUpdateFields(t *testing.T) {
	recipe := &mealie.RecipeSummary{ID: "recipe-id", Slug: "soup", Name: "Soup"}
	client := &fakeKitchenClient{
		user:    mealie.User{ID: "user-id", GroupSlug: "kinops"},
		current: mealie.PlanEntry{ID: 8, GroupID: "group-id", UserID: "owner-id"},
		created: mealie.PlanEntry{ID: 9, Date: "2026-08-03", EntryType: mealie.PlanEntryDinner, Recipe: recipe},
		updated: mealie.PlanEntry{ID: 8, Date: "2026-08-04", EntryType: mealie.PlanEntryLunch, Recipe: recipe},
	}
	service := New(client, time.UTC, "http://localhost:9925", "")
	input := domain.KitchenMealInput{Date: time.Date(2026, 8, 3, 0, 0, 0, 0, time.UTC), EntryType: "dinner", RecipeID: "recipe-id"}
	created, err := service.CreateMeal(context.Background(), input)
	if err != nil || created.ID != 9 || created.Recipe.MealieURL == "" {
		t.Fatalf("CreateMeal() = %#v, %v", created, err)
	}
	input.Date = input.Date.AddDate(0, 0, 1)
	input.EntryType = "lunch"
	if _, err := service.UpdateMeal(context.Background(), 8, input); err != nil {
		t.Fatal(err)
	}
	if client.updateInput.GroupID != "group-id" || client.updateInput.UserID != "owner-id" || client.updateInput.ID != 8 {
		t.Errorf("update input = %#v", client.updateInput)
	}
	if err := service.DeleteMeal(context.Background(), 8); err != nil || client.deleted != 8 {
		t.Errorf("DeleteMeal() = %v, deleted=%d", err, client.deleted)
	}
}

func TestGroceryViewSelectionGroupingAndCheckedItems(t *testing.T) {
	listNameA, listNameB := "Bulk", "Weekly"
	quantity := 2.0
	labelColor := "#123456"
	client := &fakeKitchenClient{
		lists: mealie.Page[mealie.ShoppingList]{TotalPages: 1, Items: []mealie.ShoppingList{
			{ID: "bulk", Name: &listNameA},
			{ID: "weekly", Name: &listNameB, LabelSettings: []mealie.ShoppingLabelSetting{{LabelID: "produce", Position: 1, Label: mealie.LabelSummary{ID: "produce", Name: "Produce", Color: &labelColor}}}},
		}},
		items: mealie.Page[mealie.ShoppingItem]{TotalPages: 1, Items: []mealie.ShoppingItem{
			{ID: "active", ShoppingListID: "weekly", Display: stringPointer("Apples"), Quantity: &quantity, LabelID: stringPointer("produce"), Label: &mealie.LabelSummary{ID: "produce", Name: "Produce", Color: &labelColor}, Position: 2},
			{ID: "checked", ShoppingListID: "weekly", Display: stringPointer("Bread"), Quantity: &quantity, Checked: true, Position: 1},
		}},
	}
	service := New(client, time.UTC, "", "")
	view, err := service.GroceryView(context.Background(), "")
	if err != nil || !view.NeedsSelection || view.SelectedListID != "" {
		t.Fatalf("unselected GroceryView() = %#v, %v", view, err)
	}
	defaultView, err := New(client, time.UTC, "", "weekly").GroceryView(context.Background(), "")
	if err != nil || defaultView.SelectedListID != "weekly" {
		t.Fatalf("default GroceryView() = %#v, %v", defaultView, err)
	}
	view, err = service.GroceryView(context.Background(), "weekly")
	if err != nil || view.SelectedListName != "Weekly" || len(view.ActiveGroups) != 1 || view.ActiveGroups[0].Label != "Produce" || len(view.CheckedItems) != 1 {
		t.Fatalf("selected GroceryView() = %#v, %v", view, err)
	}
}

func TestGroceryMutationsPreserveMealieOwnedFields(t *testing.T) {
	quantity := 1.0
	foodID, unitID, labelID := "food-id", "unit-id", "label-id"
	client := &fakeKitchenClient{
		currentItem: mealie.ShoppingItem{ID: "item-id", ShoppingListID: "list-id", Display: stringPointer("Milk"), Quantity: &quantity, Position: 7, FoodID: &foodID, UnitID: &unitID, LabelID: &labelID, Unit: &mealie.IngredientSummary{ID: unitID, Name: "carton"}},
		createdItem: mealie.ShoppingItem{ID: "new-id", ShoppingListID: "list-id", Display: stringPointer("Eggs"), Quantity: &quantity},
		updatedItem: mealie.ShoppingItem{ID: "item-id", ShoppingListID: "list-id", Display: stringPointer("Milk"), Quantity: &quantity},
	}
	service := New(client, time.UTC, "", "")
	if _, err := service.CreateGroceryItem(context.Background(), domain.KitchenGroceryCreate{ShoppingListID: "list-id", Display: "Eggs", Quantity: 1}); err != nil {
		t.Fatal(err)
	}
	if client.groceryWrite.FoodID == nil || *client.groceryWrite.FoodID != "created-food" {
		t.Errorf("grocery create payload = %#v", client.groceryWrite)
	}
	unit := "bottle"
	checked := true
	if _, err := service.UpdateGroceryItem(context.Background(), "item-id", domain.KitchenGroceryPatch{UnitName: &unit, Checked: &checked}); err != nil {
		t.Fatal(err)
	}
	if client.groceryWrite.ShoppingListID != "list-id" || client.groceryWrite.Position != 7 || client.groceryWrite.FoodID == nil || *client.groceryWrite.FoodID != foodID || client.groceryWrite.LabelID == nil || *client.groceryWrite.LabelID != labelID || client.groceryWrite.UnitID == nil || *client.groceryWrite.UnitID != "created-unit" || !client.groceryWrite.Checked {
		t.Errorf("grocery update payload = %#v", client.groceryWrite)
	}
	if err := service.DeleteGroceryItem(context.Background(), "item-id"); err != nil || client.deletedItem != "item-id" {
		t.Errorf("DeleteGroceryItem() = %v, deleted=%q", err, client.deletedItem)
	}
}

func TestSuccessfulGroceryWriteInvalidatesReadCache(t *testing.T) {
	listName := "Weekly"
	quantity := 1.0
	client := &fakeKitchenClient{
		lists:       mealie.Page[mealie.ShoppingList]{TotalPages: 1, Items: []mealie.ShoppingList{{ID: "list-id", Name: &listName}}},
		items:       mealie.Page[mealie.ShoppingItem]{TotalPages: 1},
		createdItem: mealie.ShoppingItem{ID: "new-id", ShoppingListID: "list-id", Display: stringPointer("Eggs"), Quantity: &quantity},
	}
	service := New(client, time.UTC, "", "list-id")
	if _, err := service.GroceryView(context.Background(), ""); err != nil {
		t.Fatal(err)
	}
	if _, err := service.GroceryView(context.Background(), ""); err != nil || client.listCalls != 1 {
		t.Fatalf("fresh grocery cache calls=%d err=%v", client.listCalls, err)
	}
	if _, err := service.CreateGroceryItem(context.Background(), domain.KitchenGroceryCreate{ShoppingListID: "list-id", Display: "Eggs", Quantity: 1}); err != nil {
		t.Fatal(err)
	}
	if _, err := service.GroceryView(context.Background(), ""); err != nil || client.listCalls != 2 {
		t.Fatalf("post-write grocery calls=%d err=%v", client.listCalls, err)
	}
}
