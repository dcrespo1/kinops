package mealie

import "time"

type Page[T any] struct {
	Page       int     `json:"page"`
	PerPage    int     `json:"per_page"`
	Total      int     `json:"total"`
	TotalPages int     `json:"total_pages"`
	Items      []T     `json:"items"`
	Next       *string `json:"next"`
	Previous   *string `json:"previous"`
}

type OrganizerSummary struct {
	ID   *string `json:"id"`
	Name string  `json:"name"`
	Slug string  `json:"slug"`
}

type RecipeSummary struct {
	ID             string             `json:"id"`
	Name           string             `json:"name"`
	Slug           string             `json:"slug"`
	Image          *string            `json:"image"`
	RecipeServings *float64           `json:"recipeServings"`
	RecipeYield    *string            `json:"recipeYield"`
	TotalTime      *string            `json:"totalTime"`
	PrepTime       *string            `json:"prepTime"`
	CookTime       *string            `json:"cookTime"`
	Description    *string            `json:"description"`
	Categories     []OrganizerSummary `json:"recipeCategory"`
	Tags           []OrganizerSummary `json:"tags"`
	Rating         *float64           `json:"rating"`
	OriginalURL    *string            `json:"orgURL"`
	CreatedAt      *time.Time         `json:"createdAt"`
	UpdatedAt      *time.Time         `json:"updatedAt"`
}

type User struct {
	ID            string `json:"id"`
	GroupID       string `json:"groupId"`
	GroupSlug     string `json:"groupSlug"`
	HouseholdID   string `json:"householdId"`
	HouseholdSlug string `json:"householdSlug"`
}

type PlanEntryType string

const (
	PlanEntryBreakfast PlanEntryType = "breakfast"
	PlanEntryLunch     PlanEntryType = "lunch"
	PlanEntryDinner    PlanEntryType = "dinner"
	PlanEntrySide      PlanEntryType = "side"
	PlanEntrySnack     PlanEntryType = "snack"
	PlanEntryDrink     PlanEntryType = "drink"
	PlanEntryDessert   PlanEntryType = "dessert"
)

type PlanEntry struct {
	ID          int64          `json:"id"`
	Date        string         `json:"date"`
	EntryType   PlanEntryType  `json:"entryType"`
	Title       *string        `json:"title"`
	Text        *string        `json:"text"`
	RecipeID    *string        `json:"recipeId"`
	Recipe      *RecipeSummary `json:"recipe"`
	GroupID     string         `json:"groupId"`
	UserID      string         `json:"userId"`
	HouseholdID string         `json:"householdId"`
}

type CreatePlanEntry struct {
	Date      string        `json:"date"`
	EntryType PlanEntryType `json:"entryType"`
	Title     string        `json:"title"`
	Text      string        `json:"text"`
	RecipeID  *string       `json:"recipeId"`
}

type UpdatePlanEntry struct {
	CreatePlanEntry
	ID      int64  `json:"id"`
	GroupID string `json:"groupId"`
	UserID  string `json:"userId"`
}

type ShoppingList struct {
	ID            string                 `json:"id"`
	Name          *string                `json:"name"`
	LabelSettings []ShoppingLabelSetting `json:"labelSettings"`
	CreatedAt     *time.Time             `json:"createdAt"`
	UpdatedAt     *time.Time             `json:"updatedAt"`
}

type IngredientSummary struct {
	ID           string  `json:"id"`
	Name         string  `json:"name"`
	Abbreviation *string `json:"abbreviation"`
}

type LabelSummary struct {
	ID    string  `json:"id"`
	Name  string  `json:"name"`
	Color *string `json:"color"`
}

type ShoppingLabelSetting struct {
	ID       string       `json:"id"`
	LabelID  string       `json:"labelId"`
	Position int          `json:"position"`
	Label    LabelSummary `json:"label"`
}

type ShoppingItem struct {
	ID             string             `json:"id"`
	ShoppingListID string             `json:"shoppingListId"`
	Quantity       *float64           `json:"quantity"`
	Unit           *IngredientSummary `json:"unit"`
	Food           *IngredientSummary `json:"food"`
	Note           *string            `json:"note"`
	Display        *string            `json:"display"`
	Checked        bool               `json:"checked"`
	Position       int                `json:"position"`
	FoodID         *string            `json:"foodId"`
	LabelID        *string            `json:"labelId"`
	UnitID         *string            `json:"unitId"`
	Label          *LabelSummary      `json:"label"`
	CreatedAt      *time.Time         `json:"createdAt"`
	UpdatedAt      *time.Time         `json:"updatedAt"`
}

type IngredientReference struct {
	ID   string `json:"id,omitempty"`
	Name string `json:"name"`
}

type ShoppingItemWrite struct {
	Quantity       float64              `json:"quantity"`
	Unit           *IngredientReference `json:"unit,omitempty"`
	Food           *IngredientReference `json:"food,omitempty"`
	Note           string               `json:"note"`
	Display        string               `json:"display"`
	ShoppingListID string               `json:"shoppingListId"`
	Checked        bool                 `json:"checked"`
	Position       int                  `json:"position"`
	FoodID         *string              `json:"foodId"`
	LabelID        *string              `json:"labelId"`
	UnitID         *string              `json:"unitId"`
}

type ShoppingItemsCollection struct {
	CreatedItems []ShoppingItem `json:"createdItems"`
	UpdatedItems []ShoppingItem `json:"updatedItems"`
	DeletedItems []ShoppingItem `json:"deletedItems"`
}

type Rating struct {
	RecipeID string   `json:"recipeId"`
	Rating   *float64 `json:"rating"`
	Favorite bool     `json:"isFavorite"`
}

type ratingsResponse struct {
	Ratings []Rating `json:"ratings"`
}

type About struct {
	Version            string `json:"version"`
	Production         bool   `json:"production"`
	AllowSignup        bool   `json:"allowSignup"`
	AllowPasswordLogin bool   `json:"allowPasswordLogin"`
}
