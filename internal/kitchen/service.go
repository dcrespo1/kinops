package kitchen

import (
	"context"
	"errors"
	"fmt"
	"math"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/dcrespo1/kinops/internal/domain"
	"github.com/dcrespo1/kinops/internal/mealie"
	"github.com/dcrespo1/kinops/internal/scheduling"
)

const mealPageSize = 100
const recipePageSize = 12

var ErrGroceryListNotFound = errors.New("grocery list not found")

var mealTypes = []struct{ Type, Label string }{
	{"breakfast", "Breakfast"}, {"lunch", "Lunch"}, {"dinner", "Dinner"},
	{"side", "Sides"}, {"snack", "Snacks"}, {"drink", "Drinks"}, {"dessert", "Dessert"},
}

type mealPlanClient interface {
	MealPlans(context.Context, time.Time, time.Time, mealie.Pagination) (mealie.Page[mealie.PlanEntry], error)
}

type recipeClient interface {
	Recipes(context.Context, mealie.RecipeQuery) (mealie.Page[mealie.RecipeSummary], error)
	Favorites(context.Context) ([]mealie.Rating, error)
	CurrentUser(context.Context) (mealie.User, error)
	SetFavorite(context.Context, string, string, bool) error
}

type mealMutationClient interface {
	MealPlan(context.Context, int64) (mealie.PlanEntry, error)
	CreateMealPlan(context.Context, mealie.CreatePlanEntry) (mealie.PlanEntry, error)
	UpdateMealPlan(context.Context, int64, mealie.UpdatePlanEntry) (mealie.PlanEntry, error)
	DeleteMealPlan(context.Context, int64) (mealie.PlanEntry, error)
}

type groceryClient interface {
	Foods(context.Context, string, mealie.Pagination) (mealie.Page[mealie.IngredientSummary], error)
	Units(context.Context, string, mealie.Pagination) (mealie.Page[mealie.IngredientSummary], error)
	CreateFood(context.Context, string) (mealie.IngredientSummary, error)
	CreateUnit(context.Context, string) (mealie.IngredientSummary, error)
	ShoppingLists(context.Context, mealie.Pagination) (mealie.Page[mealie.ShoppingList], error)
	ShoppingItems(context.Context, string, mealie.Pagination) (mealie.Page[mealie.ShoppingItem], error)
	ShoppingItem(context.Context, string) (mealie.ShoppingItem, error)
	CreateShoppingItem(context.Context, mealie.ShoppingItemWrite) (mealie.ShoppingItem, error)
	UpdateShoppingItem(context.Context, string, mealie.ShoppingItemWrite) (mealie.ShoppingItem, error)
	DeleteShoppingItem(context.Context, string) error
}

type aboutClient interface {
	About(context.Context) (mealie.About, error)
}

type Options struct {
	Now             func() time.Time
	MaxEntries      int
	MealFreshFor    time.Duration
	RecipeFreshFor  time.Duration
	GroceryFreshFor time.Duration
	StaleFor        time.Duration
}

type Service struct {
	client          mealPlanClient
	recipes         recipeClient
	mutations       mealMutationClient
	groceries       groceryClient
	about           aboutClient
	location        *time.Location
	publicURL       string
	defaultList     string
	now             func() time.Time
	mealFreshFor    time.Duration
	recipeFreshFor  time.Duration
	groceryFreshFor time.Duration
	staleFor        time.Duration
	mealCache       *memoryCache[[]domain.KitchenMeal]
	recipeCache     *memoryCache[domain.KitchenRecipeView]
	groceryCache    *memoryCache[domain.KitchenGroceryView]
}

func New(client mealPlanClient, location *time.Location, publicURL, defaultList string) *Service {
	return NewWithOptions(client, location, publicURL, defaultList, Options{})
}

func NewWithOptions(client mealPlanClient, location *time.Location, publicURL, defaultList string, options Options) *Service {
	if options.Now == nil {
		options.Now = time.Now
	}
	if options.MaxEntries == 0 {
		options.MaxEntries = 64
	}
	if options.MealFreshFor == 0 {
		options.MealFreshFor = 30 * time.Second
	}
	if options.RecipeFreshFor == 0 {
		options.RecipeFreshFor = 2 * time.Minute
	}
	if options.GroceryFreshFor == 0 {
		options.GroceryFreshFor = 15 * time.Second
	}
	if options.StaleFor == 0 {
		options.StaleFor = 24 * time.Hour
	}
	service := &Service{
		client: client, location: location, publicURL: strings.TrimRight(publicURL, "/"), defaultList: strings.TrimSpace(defaultList),
		now: options.Now, mealFreshFor: options.MealFreshFor, recipeFreshFor: options.RecipeFreshFor,
		groceryFreshFor: options.GroceryFreshFor, staleFor: options.StaleFor,
		mealCache:    newMemoryCache[[]domain.KitchenMeal](options.MaxEntries),
		recipeCache:  newMemoryCache[domain.KitchenRecipeView](options.MaxEntries),
		groceryCache: newMemoryCache[domain.KitchenGroceryView](options.MaxEntries),
	}
	service.recipes, _ = client.(recipeClient)
	service.mutations, _ = client.(mealMutationClient)
	service.groceries, _ = client.(groceryClient)
	service.about, _ = client.(aboutClient)
	return service
}

func (s *Service) RecipeView(ctx context.Context, search string, favoritesOnly bool, page int, selectedDate time.Time, selectedType string) (domain.KitchenRecipeView, error) {
	if s.recipes == nil {
		return domain.KitchenRecipeView{}, fmt.Errorf("recipe integration: %w", mealie.ErrUnavailable)
	}
	if page < 1 {
		return domain.KitchenRecipeView{}, errors.New("recipe page must be positive")
	}
	search = strings.TrimSpace(search)
	selectedDate = scheduling.RebaseDate(selectedDate, s.location)
	selectedType = normalizeMealType(selectedType)
	key := fmt.Sprintf("%t|%d|%s", favoritesOnly, page, search)
	now := s.now()
	if cached, ok, fresh := s.recipeCache.Get(key, now, s.recipeFreshFor, s.staleFor); ok && fresh {
		cached.SelectedDate, cached.SelectedType = selectedDate, selectedType
		return cached, nil
	}
	view, err := s.loadRecipeView(ctx, search, favoritesOnly, page)
	if err != nil {
		if cached, ok, _ := s.recipeCache.Get(key, now, s.recipeFreshFor, s.staleFor); ok {
			cached.SelectedDate, cached.SelectedType = selectedDate, selectedType
			cached.Stale, cached.StaleMessage = true, "Showing previously loaded recipes while Mealie is unavailable. Changes may not be visible yet."
			return cached, nil
		}
		return domain.KitchenRecipeView{}, err
	}
	view.SelectedDate, view.SelectedType = selectedDate, selectedType
	s.recipeCache.Set(key, view, now)
	return view, nil
}

func (s *Service) loadRecipeView(ctx context.Context, search string, favoritesOnly bool, page int) (domain.KitchenRecipeView, error) {
	favorites, err := s.recipes.Favorites(ctx)
	if err != nil {
		return domain.KitchenRecipeView{}, fmt.Errorf("list Mealie favorites: %w", err)
	}
	favoriteIDs := make(map[string]bool, len(favorites))
	for _, rating := range favorites {
		if rating.Favorite {
			favoriteIDs[rating.RecipeID] = true
		}
	}
	user, err := s.recipes.CurrentUser(ctx)
	if err != nil {
		return domain.KitchenRecipeView{}, fmt.Errorf("read Mealie user: %w", err)
	}

	var response mealie.Page[mealie.RecipeSummary]
	if favoritesOnly {
		response, err = s.favoriteRecipes(ctx, search, page, favoriteIDs)
	} else {
		response, err = s.recipes.Recipes(ctx, mealie.RecipeQuery{Search: search, Pagination: mealie.Pagination{Page: page, PerPage: recipePageSize}})
	}
	if err != nil {
		return domain.KitchenRecipeView{}, fmt.Errorf("list Mealie recipes: %w", err)
	}
	view := domain.KitchenRecipeView{
		State: domain.KitchenReady, Search: search, FavoritesOnly: favoritesOnly,
		Page: response.Page, TotalPages: response.TotalPages, Total: response.Total,
	}
	if view.Page == 0 {
		view.Page = page
	}
	for _, recipe := range response.Items {
		view.Recipes = append(view.Recipes, s.mapRecipe(recipe, user.GroupSlug, favoriteIDs[recipe.ID]))
	}
	return view, nil
}

func (s *Service) favoriteRecipes(ctx context.Context, search string, requestedPage int, favorites map[string]bool) (mealie.Page[mealie.RecipeSummary], error) {
	var filtered []mealie.RecipeSummary
	for page := 1; ; page++ {
		response, err := s.recipes.Recipes(ctx, mealie.RecipeQuery{Search: search, Pagination: mealie.Pagination{Page: page, PerPage: mealPageSize}})
		if err != nil {
			return mealie.Page[mealie.RecipeSummary]{}, err
		}
		for _, recipe := range response.Items {
			if favorites[recipe.ID] {
				filtered = append(filtered, recipe)
			}
		}
		if response.TotalPages <= page || response.TotalPages == 0 {
			break
		}
	}
	total := len(filtered)
	totalPages := int(math.Ceil(float64(total) / float64(recipePageSize)))
	start := (requestedPage - 1) * recipePageSize
	if start > total {
		start = total
	}
	end := min(start+recipePageSize, total)
	return mealie.Page[mealie.RecipeSummary]{Page: requestedPage, PerPage: recipePageSize, Total: total, TotalPages: totalPages, Items: filtered[start:end]}, nil
}

func (s *Service) mapRecipe(recipe mealie.RecipeSummary, groupSlug string, favorite bool) domain.KitchenRecipeCard {
	card := domain.KitchenRecipeCard{
		ID: recipe.ID, Slug: recipe.Slug, Name: recipe.Name, Rating: recipe.Rating,
		Favorite: favorite, PrepTime: value(recipe.PrepTime), TotalTime: value(recipe.TotalTime),
	}
	for _, category := range recipe.Categories {
		if name := strings.TrimSpace(category.Name); name != "" {
			card.Categories = append(card.Categories, name)
		}
	}
	if s.publicURL != "" && recipe.ID != "" && recipe.Image != nil && strings.TrimSpace(*recipe.Image) != "" {
		card.ImageURL = s.publicURL + "/api/media/recipes/" + url.PathEscape(recipe.ID) + "/images/min-original.webp?rnd=" + url.QueryEscape(*recipe.Image)
	}
	if s.publicURL != "" && groupSlug != "" && recipe.Slug != "" {
		card.MealieURL = s.publicURL + "/g/" + url.PathEscape(groupSlug) + "/r/" + url.PathEscape(recipe.Slug)
	}
	return card
}

func (s *Service) SetFavorite(ctx context.Context, recipeSlug string, favorite bool) error {
	if s.recipes == nil {
		return mealie.ErrUnavailable
	}
	user, err := s.recipes.CurrentUser(ctx)
	if err != nil {
		return fmt.Errorf("read Mealie user: %w", err)
	}
	if err := s.recipes.SetFavorite(ctx, user.ID, strings.TrimSpace(recipeSlug), favorite); err != nil {
		return fmt.Errorf("update Mealie favorite: %w", err)
	}
	s.recipeCache.Clear()
	return nil
}

func (s *Service) CreateMeal(ctx context.Context, input domain.KitchenMealInput) (domain.KitchenMeal, error) {
	if s.mutations == nil {
		return domain.KitchenMeal{}, mealie.ErrUnavailable
	}
	normalized, err := s.normalizeMealInput(input)
	if err != nil {
		return domain.KitchenMeal{}, err
	}
	created, err := s.mutations.CreateMealPlan(ctx, planCreate(normalized))
	if err != nil {
		return domain.KitchenMeal{}, fmt.Errorf("create Mealie meal: %w", err)
	}
	meal, err := s.mapMeal(created)
	if err != nil {
		return domain.KitchenMeal{}, err
	}
	meals := []domain.KitchenMeal{meal}
	if err := s.addRecipeLinks(ctx, meals); err != nil {
		return domain.KitchenMeal{}, err
	}
	s.mealCache.Clear()
	return meals[0], nil
}

func (s *Service) UpdateMeal(ctx context.Context, id int64, input domain.KitchenMealInput) (domain.KitchenMeal, error) {
	if s.mutations == nil {
		return domain.KitchenMeal{}, mealie.ErrUnavailable
	}
	normalized, err := s.normalizeMealInput(input)
	if err != nil {
		return domain.KitchenMeal{}, err
	}
	current, err := s.mutations.MealPlan(ctx, id)
	if err != nil {
		return domain.KitchenMeal{}, fmt.Errorf("read Mealie meal: %w", err)
	}
	updated, err := s.mutations.UpdateMealPlan(ctx, id, mealie.UpdatePlanEntry{CreatePlanEntry: planCreate(normalized), ID: id, GroupID: current.GroupID, UserID: current.UserID})
	if err != nil {
		return domain.KitchenMeal{}, fmt.Errorf("update Mealie meal: %w", err)
	}
	meal, err := s.mapMeal(updated)
	if err != nil {
		return domain.KitchenMeal{}, err
	}
	meals := []domain.KitchenMeal{meal}
	if err := s.addRecipeLinks(ctx, meals); err != nil {
		return domain.KitchenMeal{}, err
	}
	s.mealCache.Clear()
	return meals[0], nil
}

func (s *Service) DeleteMeal(ctx context.Context, id int64) error {
	if s.mutations == nil {
		return mealie.ErrUnavailable
	}
	if _, err := s.mutations.DeleteMealPlan(ctx, id); err != nil {
		return fmt.Errorf("delete Mealie meal: %w", err)
	}
	s.mealCache.Clear()
	return nil
}

func (s *Service) GroceryView(ctx context.Context, requestedListID string) (domain.KitchenGroceryView, error) {
	if s.groceries == nil {
		return domain.KitchenGroceryView{}, mealie.ErrUnavailable
	}
	key := strings.TrimSpace(requestedListID)
	now := s.now()
	if cached, ok, fresh := s.groceryCache.Get(key, now, s.groceryFreshFor, s.staleFor); ok && fresh {
		return cached, nil
	}
	view, err := s.loadGroceryView(ctx, requestedListID)
	if err != nil {
		if cached, ok, _ := s.groceryCache.Get(key, now, s.groceryFreshFor, s.staleFor); ok {
			cached.Stale, cached.StaleMessage = true, "Showing a previously loaded shopping list while Mealie is unavailable. Writes are never queued and appear only after Mealie confirms them."
			return cached, nil
		}
		return domain.KitchenGroceryView{}, err
	}
	s.groceryCache.Set(key, view, now)
	return view, nil
}

func (s *Service) loadGroceryView(ctx context.Context, requestedListID string) (domain.KitchenGroceryView, error) {
	lists, err := s.shoppingLists(ctx)
	if err != nil {
		return domain.KitchenGroceryView{}, err
	}
	view := domain.KitchenGroceryView{State: domain.KitchenReady}
	for _, list := range lists {
		view.Lists = append(view.Lists, domain.KitchenShoppingList{ID: list.ID, Name: groceryListName(list)})
	}
	selectedID := strings.TrimSpace(requestedListID)
	if selectedID == "" {
		selectedID = s.defaultList
	}
	if selectedID == "" && len(lists) == 1 {
		selectedID = lists[0].ID
	}
	if selectedID == "" {
		view.NeedsSelection = len(lists) > 1
		if view.NeedsSelection {
			view.Message = "Choose a shopping list to get started."
		}
		return view, nil
	}
	var selected *mealie.ShoppingList
	for index := range lists {
		if lists[index].ID == selectedID {
			selected = &lists[index]
			break
		}
	}
	if selected == nil {
		if requestedListID == "" {
			view.NeedsSelection = true
			view.Message = "The configured default shopping list was not found. Choose another list."
			return view, nil
		}
		return domain.KitchenGroceryView{}, ErrGroceryListNotFound
	}
	view.SelectedListID = selected.ID
	view.SelectedListName = groceryListName(*selected)
	items, err := s.shoppingItems(ctx, selected.ID)
	if err != nil {
		return domain.KitchenGroceryView{}, err
	}
	view.ActiveGroups, view.CheckedItems = groupGroceryItems(items, selected.LabelSettings)
	return view, nil
}

func (s *Service) CreateGroceryItem(ctx context.Context, input domain.KitchenGroceryCreate) (domain.KitchenGroceryItem, error) {
	if s.groceries == nil {
		return domain.KitchenGroceryItem{}, mealie.ErrUnavailable
	}
	input.ShoppingListID = strings.TrimSpace(input.ShoppingListID)
	input.Display = strings.TrimSpace(input.Display)
	input.Note = strings.TrimSpace(input.Note)
	input.UnitName = strings.TrimSpace(input.UnitName)
	if input.ShoppingListID == "" || input.Display == "" {
		return domain.KitchenGroceryItem{}, errors.New("shopping list and item are required")
	}
	if input.Quantity < 0 || len(input.Display) > 500 || len(input.Note) > 2000 || len(input.UnitName) > 100 {
		return domain.KitchenGroceryItem{}, errors.New("grocery item details are invalid")
	}
	if input.Quantity == 0 {
		input.Quantity = 1
	}
	foodID, err := s.resolveIngredient(ctx, input.Display, s.groceries.Foods, s.groceries.CreateFood)
	if err != nil {
		return domain.KitchenGroceryItem{}, fmt.Errorf("resolve Mealie grocery food: %w", err)
	}
	write := mealie.ShoppingItemWrite{ShoppingListID: input.ShoppingListID, Display: input.Display, Note: input.Note, Quantity: input.Quantity, FoodID: &foodID}
	if input.UnitName != "" {
		unitID, err := s.resolveIngredient(ctx, input.UnitName, s.groceries.Units, s.groceries.CreateUnit)
		if err != nil {
			return domain.KitchenGroceryItem{}, fmt.Errorf("resolve Mealie grocery unit: %w", err)
		}
		write.UnitID = &unitID
	}
	item, err := s.groceries.CreateShoppingItem(ctx, write)
	if err != nil {
		return domain.KitchenGroceryItem{}, fmt.Errorf("create Mealie grocery item: %w", err)
	}
	s.groceryCache.Clear()
	return mapGroceryItem(item), nil
}

func (s *Service) UpdateGroceryItem(ctx context.Context, itemID string, patch domain.KitchenGroceryPatch) (domain.KitchenGroceryItem, error) {
	if s.groceries == nil {
		return domain.KitchenGroceryItem{}, mealie.ErrUnavailable
	}
	current, err := s.groceries.ShoppingItem(ctx, strings.TrimSpace(itemID))
	if err != nil {
		return domain.KitchenGroceryItem{}, fmt.Errorf("read Mealie grocery item: %w", err)
	}
	write := groceryWriteFromItem(current)
	if patch.Display != nil {
		write.Display = strings.TrimSpace(*patch.Display)
		if write.Display == "" || len(write.Display) > 500 {
			return domain.KitchenGroceryItem{}, errors.New("grocery item name is required")
		}
	}
	if patch.Note != nil {
		write.Note = strings.TrimSpace(*patch.Note)
		if len(write.Note) > 2000 {
			return domain.KitchenGroceryItem{}, errors.New("grocery item note is too long")
		}
	}
	if patch.Quantity != nil {
		if *patch.Quantity < 0 {
			return domain.KitchenGroceryItem{}, errors.New("grocery quantity cannot be negative")
		}
		write.Quantity = *patch.Quantity
	}
	if patch.UnitName != nil {
		unitName := strings.TrimSpace(*patch.UnitName)
		currentUnit := ""
		if current.Unit != nil {
			currentUnit = current.Unit.Name
		}
		if strings.EqualFold(unitName, currentUnit) {
			write.UnitID = current.UnitID
		} else {
			write.UnitID, write.Unit = nil, nil
			if unitName != "" {
				unitID, err := s.resolveIngredient(ctx, unitName, s.groceries.Units, s.groceries.CreateUnit)
				if err != nil {
					return domain.KitchenGroceryItem{}, fmt.Errorf("resolve Mealie grocery unit: %w", err)
				}
				write.UnitID = &unitID
			}
		}
	}
	if patch.Checked != nil {
		write.Checked = *patch.Checked
	}
	updated, err := s.groceries.UpdateShoppingItem(ctx, current.ID, write)
	if err != nil {
		return domain.KitchenGroceryItem{}, fmt.Errorf("update Mealie grocery item: %w", err)
	}
	s.groceryCache.Clear()
	return mapGroceryItem(updated), nil
}

func (s *Service) DeleteGroceryItem(ctx context.Context, itemID string) error {
	if s.groceries == nil {
		return mealie.ErrUnavailable
	}
	if err := s.groceries.DeleteShoppingItem(ctx, strings.TrimSpace(itemID)); err != nil {
		return fmt.Errorf("delete Mealie grocery item: %w", err)
	}
	s.groceryCache.Clear()
	return nil
}

func (s *Service) shoppingLists(ctx context.Context) ([]mealie.ShoppingList, error) {
	var result []mealie.ShoppingList
	for page := 1; ; page++ {
		response, err := s.groceries.ShoppingLists(ctx, mealie.Pagination{Page: page, PerPage: mealPageSize})
		if err != nil {
			return nil, fmt.Errorf("list Mealie shopping lists: %w", err)
		}
		result = append(result, response.Items...)
		if response.TotalPages <= page || response.TotalPages == 0 {
			return result, nil
		}
	}
}

func (s *Service) resolveIngredient(
	ctx context.Context,
	name string,
	search func(context.Context, string, mealie.Pagination) (mealie.Page[mealie.IngredientSummary], error),
	create func(context.Context, string) (mealie.IngredientSummary, error),
) (string, error) {
	response, err := search(ctx, name, mealie.Pagination{Page: 1, PerPage: mealPageSize})
	if err != nil {
		return "", err
	}
	for _, ingredient := range response.Items {
		if strings.EqualFold(strings.TrimSpace(ingredient.Name), strings.TrimSpace(name)) {
			return ingredient.ID, nil
		}
	}
	ingredient, err := create(ctx, name)
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(ingredient.ID) == "" {
		return "", mealie.ErrMalformedResponse
	}
	return ingredient.ID, nil
}

func (s *Service) shoppingItems(ctx context.Context, listID string) ([]mealie.ShoppingItem, error) {
	var result []mealie.ShoppingItem
	for page := 1; ; page++ {
		response, err := s.groceries.ShoppingItems(ctx, listID, mealie.Pagination{Page: page, PerPage: mealPageSize})
		if err != nil {
			return nil, fmt.Errorf("list Mealie shopping items: %w", err)
		}
		result = append(result, response.Items...)
		if response.TotalPages <= page || response.TotalPages == 0 {
			return result, nil
		}
	}
}

func (s *Service) DailyView(ctx context.Context, date time.Time) (domain.KitchenDailyView, error) {
	date = scheduling.RebaseDate(date, s.location)
	items, stale, err := s.cachedMealsBetween(ctx, date, date)
	if err != nil {
		return domain.KitchenDailyView{}, err
	}
	view := domain.KitchenDailyView{Date: date, State: domain.KitchenReady, PublicURL: s.publicURL, Day: buildDay(date, items), Stale: stale}
	if stale {
		view.StaleMessage = "Showing a previously loaded meal plan while Mealie is unavailable. Changes may not be visible yet."
	}
	return view, nil
}

func (s *Service) WeeklyView(ctx context.Context, date time.Time) (domain.KitchenWeekView, error) {
	date = scheduling.RebaseDate(date, s.location)
	start := date.AddDate(0, 0, -(int(date.Weekday())+6)%7)
	end := start.AddDate(0, 0, 6)
	items, stale, err := s.cachedMealsBetween(ctx, start, end)
	if err != nil {
		return domain.KitchenWeekView{}, err
	}
	byDate := make(map[string][]domain.KitchenMeal)
	for _, item := range items {
		byDate[item.Date.Format(scheduling.DateLayout)] = append(byDate[item.Date.Format(scheduling.DateLayout)], item)
	}
	view := domain.KitchenWeekView{StartDate: start, EndDate: end, State: domain.KitchenReady, PublicURL: s.publicURL, Days: make([]domain.KitchenDay, 7)}
	view.Stale = stale
	if stale {
		view.StaleMessage = "Showing a previously loaded meal plan while Mealie is unavailable. Changes may not be visible yet."
	}
	for index := range view.Days {
		day := start.AddDate(0, 0, index)
		view.Days[index] = buildDay(day, byDate[day.Format(scheduling.DateLayout)])
	}
	return view, nil
}

func (s *Service) cachedMealsBetween(ctx context.Context, start, end time.Time) ([]domain.KitchenMeal, bool, error) {
	key := start.Format(scheduling.DateLayout) + "|" + end.Format(scheduling.DateLayout)
	now := s.now()
	if cached, ok, fresh := s.mealCache.Get(key, now, s.mealFreshFor, s.staleFor); ok && fresh {
		return cached, false, nil
	}
	meals, err := s.mealsBetween(ctx, start, end)
	if err != nil {
		if cached, ok, _ := s.mealCache.Get(key, now, s.mealFreshFor, s.staleFor); ok {
			return cached, true, nil
		}
		return nil, false, err
	}
	s.mealCache.Set(key, meals, now)
	return meals, false, nil
}

func (s *Service) MealieStatus(ctx context.Context) domain.MealieStatus {
	status := domain.MealieStatus{Enabled: s.about != nil, CheckedAt: s.now()}
	if s.about == nil {
		status.Message = "Mealie integration is not configured."
		return status
	}
	about, err := s.about.About(ctx)
	if err != nil {
		status.Message = "Mealie is configured but could not be reached."
		return status
	}
	status.Connected, status.Version, status.Message = true, strings.TrimSpace(about.Version), "Connected"
	return status
}

func (s *Service) mealsBetween(ctx context.Context, start, end time.Time) ([]domain.KitchenMeal, error) {
	var result []domain.KitchenMeal
	for page := 1; ; page++ {
		response, err := s.client.MealPlans(ctx, start, end, mealie.Pagination{Page: page, PerPage: mealPageSize})
		if err != nil {
			return nil, fmt.Errorf("list Mealie meal plans: %w", err)
		}
		for _, item := range response.Items {
			meal, err := s.mapMeal(item)
			if err != nil {
				return nil, err
			}
			result = append(result, meal)
		}
		if response.TotalPages <= page || response.TotalPages == 0 {
			break
		}
	}
	if err := s.addRecipeLinks(ctx, result); err != nil {
		return nil, err
	}
	return result, nil
}

func (s *Service) addRecipeLinks(ctx context.Context, meals []domain.KitchenMeal) error {
	if s.recipes == nil || s.publicURL == "" {
		return nil
	}
	hasRecipe := false
	for _, meal := range meals {
		if meal.Recipe != nil {
			hasRecipe = true
			break
		}
	}
	if !hasRecipe {
		return nil
	}
	user, err := s.recipes.CurrentUser(ctx)
	if err != nil {
		return fmt.Errorf("read Mealie user for recipe links: %w", err)
	}
	for index := range meals {
		if meals[index].Recipe != nil && meals[index].Recipe.Slug != "" {
			meals[index].Recipe.MealieURL = s.publicURL + "/g/" + url.PathEscape(user.GroupSlug) + "/r/" + url.PathEscape(meals[index].Recipe.Slug)
		}
	}
	return nil
}

func (s *Service) mapMeal(item mealie.PlanEntry) (domain.KitchenMeal, error) {
	date, err := scheduling.ParseDate(item.Date, s.location)
	if err != nil {
		return domain.KitchenMeal{}, fmt.Errorf("parse Mealie meal date: %w", err)
	}
	meal := domain.KitchenMeal{ID: item.ID, Date: date, EntryType: string(item.EntryType), Title: value(item.Title), Text: value(item.Text)}
	if item.Recipe != nil {
		meal.Recipe = &domain.KitchenRecipe{ID: item.Recipe.ID, Slug: item.Recipe.Slug, Name: item.Recipe.Name}
	}
	return meal, nil
}

func (s *Service) normalizeMealInput(input domain.KitchenMealInput) (domain.KitchenMealInput, error) {
	if input.Date.IsZero() {
		return domain.KitchenMealInput{}, errors.New("meal date is required")
	}
	input.Date = scheduling.RebaseDate(input.Date, s.location)
	input.EntryType = strings.TrimSpace(input.EntryType)
	input.Title = strings.TrimSpace(input.Title)
	input.Text = strings.TrimSpace(input.Text)
	input.RecipeID = strings.TrimSpace(input.RecipeID)
	if !isMealType(input.EntryType) {
		return domain.KitchenMealInput{}, errors.New("meal type is invalid")
	}
	if input.RecipeID == "" && input.Title == "" && input.Text == "" {
		return domain.KitchenMealInput{}, errors.New("choose a recipe or enter a meal note")
	}
	if len(input.Title) > 200 || len(input.Text) > 4000 || len(input.RecipeID) > 100 {
		return domain.KitchenMealInput{}, errors.New("meal details are too long")
	}
	return input, nil
}

func planCreate(input domain.KitchenMealInput) mealie.CreatePlanEntry {
	var recipeID *string
	if input.RecipeID != "" {
		recipeID = &input.RecipeID
	}
	return mealie.CreatePlanEntry{
		Date: input.Date.Format(scheduling.DateLayout), EntryType: mealie.PlanEntryType(input.EntryType),
		Title: input.Title, Text: input.Text, RecipeID: recipeID,
	}
}

func normalizeMealType(value string) string {
	value = strings.TrimSpace(value)
	if isMealType(value) {
		return value
	}
	return string(mealie.PlanEntryDinner)
}

func isMealType(value string) bool {
	for _, definition := range mealTypes {
		if value == definition.Type {
			return true
		}
	}
	return false
}

func buildDay(date time.Time, meals []domain.KitchenMeal) domain.KitchenDay {
	groups := make([]domain.KitchenMealGroup, 0, len(mealTypes)+1)
	known := make(map[string]bool, len(mealTypes))
	for _, definition := range mealTypes {
		known[definition.Type] = true
		var matching []domain.KitchenMeal
		for _, meal := range meals {
			if meal.EntryType == definition.Type {
				matching = append(matching, meal)
			}
		}
		if len(matching) > 0 {
			groups = append(groups, domain.KitchenMealGroup{Type: definition.Type, Label: definition.Label, Meals: matching})
		}
	}
	var other []domain.KitchenMeal
	for _, meal := range meals {
		if !known[meal.EntryType] {
			other = append(other, meal)
		}
	}
	if len(other) > 0 {
		groups = append(groups, domain.KitchenMealGroup{Type: "other", Label: "Other", Meals: other})
	}
	for index := range groups {
		sort.SliceStable(groups[index].Meals, func(i, j int) bool { return groups[index].Meals[i].ID < groups[index].Meals[j].ID })
	}
	return domain.KitchenDay{Date: date, Groups: groups}
}

func value(value *string) string {
	if value == nil {
		return ""
	}
	return strings.TrimSpace(*value)
}

func groceryListName(list mealie.ShoppingList) string {
	if name := value(list.Name); name != "" {
		return name
	}
	return "Shopping list"
}

func groupGroceryItems(items []mealie.ShoppingItem, settings []mealie.ShoppingLabelSetting) ([]domain.KitchenGroceryGroup, []domain.KitchenGroceryItem) {
	type labelDefinition struct {
		name     string
		color    string
		position int
	}
	labels := make(map[string]labelDefinition, len(settings))
	for _, setting := range settings {
		labels[setting.LabelID] = labelDefinition{name: setting.Label.Name, color: value(setting.Label.Color), position: setting.Position}
	}
	var groups []domain.KitchenGroceryGroup
	groupIndex := make(map[string]int)
	var checked []domain.KitchenGroceryItem
	for _, source := range items {
		item := mapGroceryItem(source)
		if item.Checked {
			checked = append(checked, item)
			continue
		}
		key := item.LabelID
		definition, known := labels[key]
		if !known {
			definition = labelDefinition{name: item.LabelName, color: item.LabelColor, position: 10_000}
		}
		if key == "" {
			definition.name, definition.position = "Other", 20_000
		}
		index, exists := groupIndex[key]
		if !exists {
			index = len(groups)
			groupIndex[key] = index
			groups = append(groups, domain.KitchenGroceryGroup{LabelID: key, Label: definition.name, LabelColor: definition.color, Position: definition.position})
		}
		groups[index].Items = append(groups[index].Items, item)
	}
	sort.SliceStable(groups, func(i, j int) bool {
		if groups[i].Position == groups[j].Position {
			return groups[i].Label < groups[j].Label
		}
		return groups[i].Position < groups[j].Position
	})
	for index := range groups {
		sort.SliceStable(groups[index].Items, func(i, j int) bool { return groups[index].Items[i].Position < groups[index].Items[j].Position })
	}
	sort.SliceStable(checked, func(i, j int) bool { return checked[i].Position < checked[j].Position })
	return groups, checked
}

func mapGroceryItem(item mealie.ShoppingItem) domain.KitchenGroceryItem {
	quantity := 1.0
	if item.Quantity != nil {
		quantity = *item.Quantity
	}
	mapped := domain.KitchenGroceryItem{
		ID: item.ID, ShoppingListID: item.ShoppingListID, Display: value(item.Display), Note: value(item.Note),
		Quantity: quantity, Checked: item.Checked, Position: item.Position, LabelID: value(item.LabelID),
	}
	if mapped.Display == "" && item.Food != nil {
		mapped.Display = item.Food.Name
	}
	if item.Unit != nil {
		mapped.UnitName = item.Unit.Name
	}
	if item.Label != nil {
		mapped.LabelName = item.Label.Name
		mapped.LabelColor = value(item.Label.Color)
	}
	return mapped
}

func groceryWriteFromItem(item mealie.ShoppingItem) mealie.ShoppingItemWrite {
	quantity := 1.0
	if item.Quantity != nil {
		quantity = *item.Quantity
	}
	write := mealie.ShoppingItemWrite{
		Quantity: quantity, Note: value(item.Note), Display: value(item.Display), ShoppingListID: item.ShoppingListID,
		Checked: item.Checked, Position: item.Position, FoodID: item.FoodID, LabelID: item.LabelID, UnitID: item.UnitID,
	}
	if write.FoodID == nil && item.Food != nil {
		write.Food = &mealie.IngredientReference{ID: item.Food.ID, Name: item.Food.Name}
	}
	if write.UnitID == nil && item.Unit != nil {
		write.Unit = &mealie.IngredientReference{ID: item.Unit.ID, Name: item.Unit.Name}
	}
	return write
}
