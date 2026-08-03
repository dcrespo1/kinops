package mealie_test

import (
	"context"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/dcrespo1/kinops/internal/config"
	"github.com/dcrespo1/kinops/internal/mealie"
)

func TestLiveMealieReadContract(t *testing.T) {
	if os.Getenv("KINOPS_MEALIE_SMOKE") != "1" {
		t.Skip("set KINOPS_MEALIE_SMOKE=1 to exercise the configured Mealie instance")
	}
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.MealieEnabled() {
		t.Fatal("Mealie integration is not configured")
	}
	client, err := mealie.NewClient(mealie.Options{
		BaseURL:    cfg.MealieBaseURL,
		Token:      cfg.MealieAPIToken,
		HTTPClient: &http.Client{Timeout: cfg.MealieTimeout},
	})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	about, err := client.About(ctx)
	if err != nil {
		t.Fatalf("read Mealie version: %v", err)
	}
	recipes, err := client.Recipes(ctx, mealie.RecipeQuery{Pagination: mealie.Pagination{PerPage: 10}})
	if err != nil {
		t.Fatalf("read recipes: %v", err)
	}
	today := time.Now().In(cfg.Location)
	plans, err := client.MealPlans(ctx, today.AddDate(0, 0, -30), today.AddDate(0, 0, 60), mealie.Pagination{PerPage: 100})
	if err != nil {
		t.Fatalf("read meal plans: %v", err)
	}
	lists, err := client.ShoppingLists(ctx, mealie.Pagination{PerPage: 100})
	if err != nil {
		t.Fatalf("read shopping lists: %v", err)
	}
	items, err := client.ShoppingItems(ctx, cfg.MealieDefaultList, mealie.Pagination{PerPage: 100})
	if err != nil {
		t.Fatalf("read shopping items: %v", err)
	}
	favorites, err := client.Favorites(ctx)
	if err != nil {
		t.Fatalf("read favorites: %v", err)
	}

	t.Logf("Mealie %s: recipes=%d meal-plans=%d shopping-lists=%d shopping-items=%d favorites=%d", about.Version, recipes.Total, plans.Total, lists.Total, items.Total, len(favorites))
}
