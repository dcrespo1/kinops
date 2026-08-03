package mealie

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestClientLogsOnlySafeUpstreamMetadata(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		fmt.Fprint(w, `{"detail":"private upstream body"}`)
	}))
	defer server.Close()
	var logs bytes.Buffer
	client, err := NewClient(Options{
		BaseURL: server.URL, Token: "private-token", HTTPClient: server.Client(),
		Logger:    slog.New(slog.NewJSONHandler(&logs, nil)),
		RequestID: func(context.Context) string { return "request-123" },
	})
	if err != nil {
		t.Fatal(err)
	}
	_, _ = client.Recipes(context.Background(), RecipeQuery{})
	output := logs.String()
	for _, want := range []string{"recipes.get", "4xx", "request-123", "duration"} {
		if !strings.Contains(output, want) {
			t.Errorf("log missing %q: %s", want, output)
		}
	}
	for _, forbidden := range []string{"private-token", "private upstream body", "Authorization", server.URL} {
		if strings.Contains(output, forbidden) {
			t.Errorf("log leaked %q: %s", forbidden, output)
		}
	}
}

func TestClientReadsVerifiedMealieResources(t *testing.T) {
	var requests []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer secret-token" {
			t.Errorf("Authorization = %q", got)
		}
		if got := r.Header.Get("User-Agent"); got != "KinOps/1.0" {
			t.Errorf("User-Agent = %q", got)
		}
		requests = append(requests, r.URL.RequestURI())
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/app/about":
			fmt.Fprint(w, `{"version":"v3.22.0","production":true,"allowSignup":false,"allowPasswordLogin":true}`)
		case "/api/recipes":
			fmt.Fprint(w, `{"page":2,"per_page":10,"total":1,"total_pages":1,"items":[{"id":"recipe-id","name":"Soup","slug":"soup","image":"image.webp","recipeCategory":[{"id":"category-id","name":"Dinner","slug":"dinner"}],"rating":4.5}]}`)
		case "/api/households/mealplans":
			fmt.Fprint(w, `{"page":1,"per_page":50,"total":1,"total_pages":1,"items":[{"id":7,"date":"2026-08-01","entryType":"dinner","title":"Soup night","recipeId":"recipe-id","recipe":{"id":"recipe-id","name":"Soup","slug":"soup"}}]}`)
		case "/api/households/shopping/lists":
			fmt.Fprint(w, `{"page":1,"per_page":50,"total":1,"total_pages":1,"items":[{"id":"list-id","name":"Groceries"}]}`)
		case "/api/households/shopping/items":
			fmt.Fprint(w, `{"page":1,"per_page":50,"total":1,"total_pages":1,"items":[{"id":"item-id","shoppingListId":"list-id","quantity":2,"display":"2 cans tomatoes","checked":false,"position":1,"food":{"id":"food-id","name":"Tomato"},"unit":{"id":"unit-id","name":"can","abbreviation":"can"},"label":{"id":"label-id","name":"Pantry","color":"#123456"}}]}`)
		case "/api/users/self/favorites":
			fmt.Fprint(w, `{"ratings":[{"recipeId":"recipe-id","rating":5,"isFavorite":true}]}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	client, err := NewClient(Options{BaseURL: server.URL, Token: "secret-token", HTTPClient: server.Client()})
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	about, err := client.About(ctx)
	if err != nil || about.Version != "v3.22.0" {
		t.Fatalf("About() = %#v, %v", about, err)
	}
	recipes, err := client.Recipes(ctx, RecipeQuery{Search: " soup ", Pagination: Pagination{Page: 2, PerPage: 10}})
	if err != nil || len(recipes.Items) != 1 || recipes.Items[0].Name != "Soup" || recipes.Items[0].Rating == nil || *recipes.Items[0].Rating != 4.5 {
		t.Fatalf("Recipes() = %#v, %v", recipes, err)
	}
	start := time.Date(2026, time.August, 1, 0, 0, 0, 0, time.UTC)
	plans, err := client.MealPlans(ctx, start, start.AddDate(0, 0, 6), Pagination{})
	if err != nil || len(plans.Items) != 1 || plans.Items[0].EntryType != PlanEntryDinner || plans.Items[0].Recipe == nil {
		t.Fatalf("MealPlans() = %#v, %v", plans, err)
	}
	lists, err := client.ShoppingLists(ctx, Pagination{})
	if err != nil || len(lists.Items) != 1 || lists.Items[0].ID != "list-id" {
		t.Fatalf("ShoppingLists() = %#v, %v", lists, err)
	}
	items, err := client.ShoppingItems(ctx, `list-"id`, Pagination{})
	if err != nil || len(items.Items) != 1 || items.Items[0].Food == nil || items.Items[0].Food.Name != "Tomato" {
		t.Fatalf("ShoppingItems() = %#v, %v", items, err)
	}
	favorites, err := client.Favorites(ctx)
	if err != nil || len(favorites) != 1 || !favorites[0].Favorite {
		t.Fatalf("Favorites() = %#v, %v", favorites, err)
	}

	joined := strings.Join(requests, "\n")
	for _, want := range []string{
		"/api/recipes?page=2&perPage=10&search=soup",
		"/api/households/mealplans?end_date=2026-08-07&orderBy=date&orderDirection=asc&page=1&perPage=50&start_date=2026-08-01",
		"/api/households/shopping/lists?orderBy=name&orderDirection=asc&page=1&perPage=50",
		"queryFilter=shoppingListId+%3D+%22list-%5C%22id%22",
	} {
		if !strings.Contains(joined, want) {
			t.Errorf("requests do not contain %q:\n%s", want, joined)
		}
	}
}

func TestClientUsesVerifiedMealAndFavoriteWriteContracts(t *testing.T) {
	var update UpdatePlanEntry
	var favoriteMethods []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/users/self":
			fmt.Fprint(w, `{"id":"user-id","groupId":"group-id","groupSlug":"kinops","householdId":"household-id","householdSlug":"family"}`)
		case r.Method == http.MethodPost && r.URL.Path == "/api/households/mealplans":
			w.WriteHeader(http.StatusCreated)
			fmt.Fprint(w, `{"id":8,"date":"2026-08-03","entryType":"dinner","groupId":"group-id","userId":"user-id","householdId":"household-id"}`)
		case r.Method == http.MethodGet && r.URL.Path == "/api/households/mealplans/8":
			fmt.Fprint(w, `{"id":8,"date":"2026-08-03","entryType":"dinner","groupId":"group-id","userId":"user-id","householdId":"household-id"}`)
		case r.Method == http.MethodPut && r.URL.Path == "/api/households/mealplans/8":
			if err := json.NewDecoder(r.Body).Decode(&update); err != nil {
				t.Fatal(err)
			}
			fmt.Fprint(w, `{"id":8,"date":"2026-08-04","entryType":"lunch","groupId":"group-id","userId":"user-id","householdId":"household-id"}`)
		case r.Method == http.MethodDelete && r.URL.Path == "/api/households/mealplans/8":
			fmt.Fprint(w, `{"id":8,"date":"2026-08-04","entryType":"lunch","groupId":"group-id","userId":"user-id","householdId":"household-id"}`)
		case strings.HasPrefix(r.URL.Path, "/api/users/user-id/favorites/"):
			favoriteMethods = append(favoriteMethods, r.Method)
			fmt.Fprint(w, `{}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	client, err := NewClient(Options{BaseURL: server.URL, Token: "secret-token", HTTPClient: server.Client()})
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	user, err := client.CurrentUser(ctx)
	if err != nil || user.ID != "user-id" || user.GroupSlug != "kinops" {
		t.Fatalf("CurrentUser() = %#v, %v", user, err)
	}
	created, err := client.CreateMealPlan(ctx, CreatePlanEntry{Date: "2026-08-03", EntryType: PlanEntryDinner})
	if err != nil || created.ID != 8 {
		t.Fatalf("CreateMealPlan() = %#v, %v", created, err)
	}
	current, err := client.MealPlan(ctx, 8)
	if err != nil || current.GroupID != "group-id" {
		t.Fatalf("MealPlan() = %#v, %v", current, err)
	}
	_, err = client.UpdateMealPlan(ctx, 8, UpdatePlanEntry{CreatePlanEntry: CreatePlanEntry{Date: "2026-08-04", EntryType: PlanEntryLunch}, ID: 8, GroupID: current.GroupID, UserID: current.UserID})
	if err != nil || update.GroupID != "group-id" || update.UserID != "user-id" {
		t.Fatalf("UpdateMealPlan() payload = %#v, %v", update, err)
	}
	if _, err := client.DeleteMealPlan(ctx, 8); err != nil {
		t.Fatal(err)
	}
	if err := client.SetFavorite(ctx, "user-id", "soup", true); err != nil {
		t.Fatal(err)
	}
	if err := client.SetFavorite(ctx, "user-id", "soup", false); err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(favoriteMethods, ","); got != "POST,DELETE" {
		t.Errorf("favorite methods = %s", got)
	}
}

func TestClientUsesVerifiedShoppingItemWriteContracts(t *testing.T) {
	var writes []ShoppingItemWrite
	var methods []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		methods = append(methods, r.Method+" "+r.URL.Path)
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/foods":
			fmt.Fprint(w, `{"page":1,"per_page":100,"total":1,"total_pages":1,"items":[{"id":"food-id","name":"butter"}]}`)
		case r.Method == http.MethodPost && r.URL.Path == "/api/units":
			fmt.Fprint(w, `{"id":"unit-id","name":"box"}`)
		case r.Method == http.MethodGet && r.URL.Path == "/api/households/shopping/items/item-id":
			fmt.Fprint(w, `{"id":"item-id","shoppingListId":"list-id","quantity":1,"display":"Milk","checked":false,"position":2,"groupId":"group-id","householdId":"household-id"}`)
		case r.Method == http.MethodPost && r.URL.Path == "/api/households/shopping/items":
			var input ShoppingItemWrite
			if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
				t.Fatal(err)
			}
			writes = append(writes, input)
			w.WriteHeader(http.StatusCreated)
			fmt.Fprint(w, `{"createdItems":[{"id":"item-id","shoppingListId":"list-id","quantity":1,"display":"Milk","groupId":"group-id","householdId":"household-id"}],"updatedItems":[],"deletedItems":[]}`)
		case r.Method == http.MethodPut && r.URL.Path == "/api/households/shopping/items/item-id":
			var input ShoppingItemWrite
			if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
				t.Fatal(err)
			}
			writes = append(writes, input)
			fmt.Fprint(w, `{"createdItems":[],"updatedItems":[{"id":"item-id","shoppingListId":"list-id","quantity":2,"display":"Milk","checked":true,"groupId":"group-id","householdId":"household-id"}],"deletedItems":[]}`)
		case r.Method == http.MethodDelete && r.URL.Path == "/api/households/shopping/items/item-id":
			fmt.Fprint(w, `{"message":"success"}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	client, err := NewClient(Options{BaseURL: server.URL, Token: "secret-token", HTTPClient: server.Client()})
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	foods, err := client.Foods(ctx, "butter", Pagination{PerPage: 100})
	if err != nil || len(foods.Items) != 1 || foods.Items[0].ID != "food-id" {
		t.Fatalf("Foods() = %#v, %v", foods, err)
	}
	unit, err := client.CreateUnit(ctx, "box")
	if err != nil || unit.ID != "unit-id" {
		t.Fatalf("CreateUnit() = %#v, %v", unit, err)
	}
	current, err := client.ShoppingItem(ctx, "item-id")
	if err != nil || current.Position != 2 {
		t.Fatalf("ShoppingItem() = %#v, %v", current, err)
	}
	created, err := client.CreateShoppingItem(ctx, ShoppingItemWrite{ShoppingListID: "list-id", Display: "Milk", Quantity: 1})
	if err != nil || created.ID != "item-id" {
		t.Fatalf("CreateShoppingItem() = %#v, %v", created, err)
	}
	updated, err := client.UpdateShoppingItem(ctx, "item-id", ShoppingItemWrite{ShoppingListID: "list-id", Display: "Milk", Quantity: 2, Checked: true})
	if err != nil || !updated.Checked {
		t.Fatalf("UpdateShoppingItem() = %#v, %v", updated, err)
	}
	if err := client.DeleteShoppingItem(ctx, "item-id"); err != nil {
		t.Fatal(err)
	}
	if len(writes) != 2 || writes[1].Quantity != 2 || !writes[1].Checked {
		t.Errorf("shopping writes = %#v", writes)
	}
	wantMethods := "GET /api/foods,POST /api/units,GET /api/households/shopping/items/item-id,POST /api/households/shopping/items,PUT /api/households/shopping/items/item-id,DELETE /api/households/shopping/items/item-id"
	if got := strings.Join(methods, ","); got != wantMethods {
		t.Errorf("methods = %s", got)
	}
}

func TestClientMapsStatusesWithoutLeakingTokenOrBody(t *testing.T) {
	tests := []struct {
		status int
		kind   error
	}{
		{http.StatusBadRequest, ErrValidation},
		{http.StatusUnauthorized, ErrUnauthorized},
		{http.StatusForbidden, ErrForbidden},
		{http.StatusNotFound, ErrNotFound},
		{http.StatusUnprocessableEntity, ErrValidation},
		{http.StatusTooManyRequests, ErrRateLimited},
		{http.StatusBadGateway, ErrUnavailable},
		{http.StatusServiceUnavailable, ErrUnavailable},
		{http.StatusGatewayTimeout, ErrUnavailable},
		{http.StatusInternalServerError, ErrUnavailable},
		{http.StatusTeapot, ErrUnexpectedStatus},
	}
	for _, tt := range tests {
		t.Run(http.StatusText(tt.status), func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(tt.status)
				fmt.Fprint(w, `{"detail":"secret upstream body"}`)
			}))
			defer server.Close()
			client, err := NewClient(Options{BaseURL: server.URL, Token: "never-log-this", HTTPClient: server.Client()})
			if err != nil {
				t.Fatal(err)
			}
			_, err = client.Recipes(context.Background(), RecipeQuery{})
			if !errors.Is(err, tt.kind) {
				t.Fatalf("error = %v, want %v", err, tt.kind)
			}
			if strings.Contains(err.Error(), "never-log-this") || strings.Contains(err.Error(), "secret upstream body") {
				t.Errorf("error leaked sensitive content: %v", err)
			}
		})
	}
}

func TestClientRejectsMalformedOversizedAndUnavailableResponses(t *testing.T) {
	t.Run("malformed", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { fmt.Fprint(w, `{`) }))
		defer server.Close()
		client, _ := NewClient(Options{BaseURL: server.URL, Token: "token", HTTPClient: server.Client()})
		_, err := client.Recipes(context.Background(), RecipeQuery{})
		if !errors.Is(err, ErrMalformedResponse) {
			t.Fatalf("error = %v", err)
		}
	})

	t.Run("oversized", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { fmt.Fprint(w, `{"items":[]}`) }))
		defer server.Close()
		client, _ := NewClient(Options{BaseURL: server.URL, Token: "token", HTTPClient: server.Client(), MaxResponseBytes: 4})
		_, err := client.Recipes(context.Background(), RecipeQuery{})
		if !errors.Is(err, ErrResponseTooLarge) {
			t.Fatalf("error = %v", err)
		}
	})

	t.Run("network", func(t *testing.T) {
		client, _ := NewClient(Options{BaseURL: "http://127.0.0.1:1", Token: "token", HTTPClient: &http.Client{Timeout: 100 * time.Millisecond}})
		_, err := client.Recipes(context.Background(), RecipeQuery{})
		if !errors.Is(err, ErrUnavailable) {
			t.Fatalf("error = %v", err)
		}
	})

	t.Run("canceled", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { <-r.Context().Done() }))
		defer server.Close()
		client, _ := NewClient(Options{BaseURL: server.URL, Token: "token", HTTPClient: server.Client()})
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		_, err := client.Recipes(ctx, RecipeQuery{})
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("error = %v", err)
		}
	})
}

func TestClientValidatesOptionsAndQueries(t *testing.T) {
	for _, options := range []Options{
		{},
		{BaseURL: "file:///tmp/mealie", Token: "token"},
		{BaseURL: "http://user:password@mealie", Token: "token"},
		{BaseURL: "http://mealie?token=secret", Token: "token"},
		{BaseURL: "http://mealie", Token: ""},
		{BaseURL: "http://mealie", Token: "token", MaxResponseBytes: -1},
	} {
		if _, err := NewClient(options); err == nil {
			t.Errorf("NewClient(%#v) returned nil error", options)
		}
	}

	client, err := NewClient(Options{BaseURL: "http://mealie", Token: "token"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := client.Recipes(context.Background(), RecipeQuery{Pagination: Pagination{Page: -1}}); err == nil {
		t.Error("negative page was accepted")
	}
	if _, err := client.Recipes(context.Background(), RecipeQuery{Pagination: Pagination{PerPage: 101}}); err == nil {
		t.Error("excessive per-page was accepted")
	}
	start := time.Date(2026, time.August, 2, 0, 0, 0, 0, time.UTC)
	if _, err := client.MealPlans(context.Background(), start, start.AddDate(0, 0, -1), Pagination{}); err == nil {
		t.Error("backwards meal-plan range was accepted")
	}
}
