package mealie

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const (
	defaultMaxResponseBytes int64 = 4 << 20
	defaultPerPage                = 50
	maxPerPage                    = 100
)

var (
	ErrUnauthorized      = errors.New("mealie authentication failed")
	ErrForbidden         = errors.New("mealie operation forbidden")
	ErrNotFound          = errors.New("mealie resource not found")
	ErrValidation        = errors.New("mealie rejected the request")
	ErrRateLimited       = errors.New("mealie rate limit exceeded")
	ErrUnavailable       = errors.New("mealie unavailable")
	ErrMalformedResponse = errors.New("mealie returned a malformed response")
	ErrResponseTooLarge  = errors.New("mealie response exceeded the size limit")
	ErrUnexpectedStatus  = errors.New("mealie returned an unexpected status")
)

type ResponseError struct {
	StatusCode int
	Kind       error
}

func (e *ResponseError) Error() string {
	if e.StatusCode == 0 {
		return e.Kind.Error()
	}
	return fmt.Sprintf("%s (status %d)", e.Kind, e.StatusCode)
}

func (e *ResponseError) Unwrap() error {
	return e.Kind
}

type Options struct {
	BaseURL          string
	Token            string
	HTTPClient       *http.Client
	MaxResponseBytes int64
	Logger           *slog.Logger
	RequestID        func(context.Context) string
}

type Client struct {
	baseURL          *url.URL
	token            string
	httpClient       *http.Client
	maxResponseBytes int64
	logger           *slog.Logger
	requestID        func(context.Context) string
}

func NewClient(options Options) (*Client, error) {
	baseURL, err := url.Parse(strings.TrimRight(options.BaseURL, "/"))
	if err != nil || baseURL.Host == "" || (baseURL.Scheme != "http" && baseURL.Scheme != "https") {
		return nil, errors.New("Mealie base URL must be an absolute HTTP or HTTPS URL")
	}
	if baseURL.User != nil || baseURL.RawQuery != "" || baseURL.Fragment != "" {
		return nil, errors.New("Mealie base URL must not contain credentials, a query, or a fragment")
	}
	if strings.TrimSpace(options.Token) == "" {
		return nil, errors.New("Mealie API token is required")
	}
	httpClient := options.HTTPClient
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 5 * time.Second}
	}
	maxResponseBytes := options.MaxResponseBytes
	if maxResponseBytes == 0 {
		maxResponseBytes = defaultMaxResponseBytes
	}
	if maxResponseBytes < 1 {
		return nil, errors.New("Mealie response size limit must be positive")
	}
	return &Client{
		baseURL:          baseURL,
		token:            strings.TrimSpace(options.Token),
		httpClient:       httpClient,
		maxResponseBytes: maxResponseBytes,
		logger:           options.Logger,
		requestID:        options.RequestID,
	}, nil
}

type Pagination struct {
	Page    int
	PerPage int
}

type RecipeQuery struct {
	Search     string
	Pagination Pagination
}

func (c *Client) About(ctx context.Context) (About, error) {
	var result About
	err := c.get(ctx, "/api/app/about", nil, &result)
	return result, err
}

func (c *Client) CurrentUser(ctx context.Context) (User, error) {
	var result User
	err := c.get(ctx, "/api/users/self", nil, &result)
	return result, err
}

func (c *Client) Recipes(ctx context.Context, query RecipeQuery) (Page[RecipeSummary], error) {
	values, err := paginationValues(query.Pagination)
	if err != nil {
		return Page[RecipeSummary]{}, err
	}
	if search := strings.TrimSpace(query.Search); search != "" {
		values.Set("search", search)
	}
	var result Page[RecipeSummary]
	err = c.get(ctx, "/api/recipes", values, &result)
	return result, err
}

func (c *Client) MealPlans(ctx context.Context, start, end time.Time, pagination Pagination) (Page[PlanEntry], error) {
	if start.IsZero() || end.IsZero() || end.Before(start) {
		return Page[PlanEntry]{}, errors.New("Mealie meal-plan range is invalid")
	}
	values, err := paginationValues(pagination)
	if err != nil {
		return Page[PlanEntry]{}, err
	}
	values.Set("start_date", start.Format("2006-01-02"))
	values.Set("end_date", end.Format("2006-01-02"))
	values.Set("orderBy", "date")
	values.Set("orderDirection", "asc")
	var result Page[PlanEntry]
	err = c.get(ctx, "/api/households/mealplans", values, &result)
	return result, err
}

func (c *Client) MealPlan(ctx context.Context, id int64) (PlanEntry, error) {
	if id < 1 {
		return PlanEntry{}, errors.New("Mealie meal-plan ID must be positive")
	}
	var result PlanEntry
	err := c.get(ctx, "/api/households/mealplans/"+strconv.FormatInt(id, 10), nil, &result)
	return result, err
}

func (c *Client) CreateMealPlan(ctx context.Context, input CreatePlanEntry) (PlanEntry, error) {
	var result PlanEntry
	err := c.sendJSON(ctx, http.MethodPost, "/api/households/mealplans", input, &result)
	return result, err
}

func (c *Client) UpdateMealPlan(ctx context.Context, id int64, input UpdatePlanEntry) (PlanEntry, error) {
	if id < 1 || input.ID != id {
		return PlanEntry{}, errors.New("Mealie meal-plan update ID is invalid")
	}
	var result PlanEntry
	err := c.sendJSON(ctx, http.MethodPut, "/api/households/mealplans/"+strconv.FormatInt(id, 10), input, &result)
	return result, err
}

func (c *Client) DeleteMealPlan(ctx context.Context, id int64) (PlanEntry, error) {
	if id < 1 {
		return PlanEntry{}, errors.New("Mealie meal-plan ID must be positive")
	}
	var result PlanEntry
	err := c.sendJSON(ctx, http.MethodDelete, "/api/households/mealplans/"+strconv.FormatInt(id, 10), nil, &result)
	return result, err
}

func (c *Client) ShoppingLists(ctx context.Context, pagination Pagination) (Page[ShoppingList], error) {
	values, err := paginationValues(pagination)
	if err != nil {
		return Page[ShoppingList]{}, err
	}
	values.Set("orderBy", "name")
	values.Set("orderDirection", "asc")
	var result Page[ShoppingList]
	err = c.get(ctx, "/api/households/shopping/lists", values, &result)
	return result, err
}

func (c *Client) Foods(ctx context.Context, search string, pagination Pagination) (Page[IngredientSummary], error) {
	return c.ingredients(ctx, "/api/foods", search, pagination)
}

func (c *Client) Units(ctx context.Context, search string, pagination Pagination) (Page[IngredientSummary], error) {
	return c.ingredients(ctx, "/api/units", search, pagination)
}

func (c *Client) CreateFood(ctx context.Context, name string) (IngredientSummary, error) {
	return c.createIngredient(ctx, "/api/foods", name)
}

func (c *Client) CreateUnit(ctx context.Context, name string) (IngredientSummary, error) {
	return c.createIngredient(ctx, "/api/units", name)
}

func (c *Client) ShoppingItems(ctx context.Context, shoppingListID string, pagination Pagination) (Page[ShoppingItem], error) {
	values, err := paginationValues(pagination)
	if err != nil {
		return Page[ShoppingItem]{}, err
	}
	if shoppingListID = strings.TrimSpace(shoppingListID); shoppingListID != "" {
		values.Set("queryFilter", "shoppingListId = "+strconv.Quote(shoppingListID))
	}
	values.Set("orderBy", "position")
	values.Set("orderDirection", "asc")
	var result Page[ShoppingItem]
	err = c.get(ctx, "/api/households/shopping/items", values, &result)
	return result, err
}

func (c *Client) ShoppingItem(ctx context.Context, itemID string) (ShoppingItem, error) {
	if err := validatePathSegment("shopping item ID", itemID); err != nil {
		return ShoppingItem{}, err
	}
	var result ShoppingItem
	err := c.get(ctx, "/api/households/shopping/items/"+itemID, nil, &result)
	return result, err
}

func (c *Client) CreateShoppingItem(ctx context.Context, input ShoppingItemWrite) (ShoppingItem, error) {
	var result ShoppingItemsCollection
	if err := c.sendJSON(ctx, http.MethodPost, "/api/households/shopping/items", input, &result); err != nil {
		return ShoppingItem{}, err
	}
	if len(result.CreatedItems) != 1 {
		return ShoppingItem{}, &ResponseError{Kind: ErrMalformedResponse}
	}
	return result.CreatedItems[0], nil
}

func (c *Client) UpdateShoppingItem(ctx context.Context, itemID string, input ShoppingItemWrite) (ShoppingItem, error) {
	if err := validatePathSegment("shopping item ID", itemID); err != nil {
		return ShoppingItem{}, err
	}
	var result ShoppingItemsCollection
	if err := c.sendJSON(ctx, http.MethodPut, "/api/households/shopping/items/"+itemID, input, &result); err != nil {
		return ShoppingItem{}, err
	}
	if len(result.UpdatedItems) != 1 {
		return ShoppingItem{}, &ResponseError{Kind: ErrMalformedResponse}
	}
	return result.UpdatedItems[0], nil
}

func (c *Client) DeleteShoppingItem(ctx context.Context, itemID string) error {
	if err := validatePathSegment("shopping item ID", itemID); err != nil {
		return err
	}
	return c.sendJSON(ctx, http.MethodDelete, "/api/households/shopping/items/"+itemID, nil, nil)
}

func (c *Client) ingredients(ctx context.Context, endpoint, search string, pagination Pagination) (Page[IngredientSummary], error) {
	values, err := paginationValues(pagination)
	if err != nil {
		return Page[IngredientSummary]{}, err
	}
	if search = strings.TrimSpace(search); search != "" {
		values.Set("search", search)
	}
	values.Set("orderBy", "name")
	values.Set("orderDirection", "asc")
	var result Page[IngredientSummary]
	err = c.get(ctx, endpoint, values, &result)
	return result, err
}

func (c *Client) createIngredient(ctx context.Context, endpoint, name string) (IngredientSummary, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return IngredientSummary{}, errors.New("Mealie ingredient name is required")
	}
	var result IngredientSummary
	err := c.sendJSON(ctx, http.MethodPost, endpoint, IngredientReference{Name: name}, &result)
	return result, err
}

func (c *Client) Favorites(ctx context.Context) ([]Rating, error) {
	var result ratingsResponse
	if err := c.get(ctx, "/api/users/self/favorites", nil, &result); err != nil {
		return nil, err
	}
	return result.Ratings, nil
}

func (c *Client) SetFavorite(ctx context.Context, userID, recipeSlug string, favorite bool) error {
	if err := validatePathSegment("user ID", userID); err != nil {
		return err
	}
	if err := validatePathSegment("recipe slug", recipeSlug); err != nil {
		return err
	}
	method := http.MethodDelete
	if favorite {
		method = http.MethodPost
	}
	return c.sendJSON(ctx, method, "/api/users/"+userID+"/favorites/"+recipeSlug, nil, nil)
}

func paginationValues(pagination Pagination) (url.Values, error) {
	page := pagination.Page
	if page == 0 {
		page = 1
	}
	perPage := pagination.PerPage
	if perPage == 0 {
		perPage = defaultPerPage
	}
	if page < 1 || perPage < 1 || perPage > maxPerPage {
		return nil, fmt.Errorf("Mealie pagination requires page >= 1 and per-page between 1 and %d", maxPerPage)
	}
	return url.Values{"page": {strconv.Itoa(page)}, "perPage": {strconv.Itoa(perPage)}}, nil
}

func (c *Client) get(ctx context.Context, endpoint string, query url.Values, destination any) error {
	return c.doJSON(ctx, http.MethodGet, endpoint, query, nil, destination)
}

func (c *Client) sendJSON(ctx context.Context, method, endpoint string, source, destination any) error {
	var body io.Reader
	if source != nil {
		encoded, err := json.Marshal(source)
		if err != nil {
			return fmt.Errorf("encode Mealie request: %w", err)
		}
		body = bytes.NewReader(encoded)
	}
	return c.doJSON(ctx, method, endpoint, nil, body, destination)
}

func (c *Client) doJSON(ctx context.Context, method, endpoint string, query url.Values, body io.Reader, destination any) error {
	started := time.Now()
	statusClass := "request_error"
	defer func() {
		if c.logger == nil {
			return
		}
		requestID := ""
		if c.requestID != nil {
			requestID = c.requestID(ctx)
		}
		c.logger.InfoContext(ctx, "Mealie request",
			"operation", mealieOperation(method, endpoint),
			"duration", time.Since(started),
			"status_class", statusClass,
			"request_id", requestID,
		)
	}()
	requestURL := *c.baseURL
	requestURL.Path = strings.TrimRight(c.baseURL.Path, "/") + endpoint
	if query != nil {
		requestURL.RawQuery = query.Encode()
	}
	request, err := http.NewRequestWithContext(ctx, method, requestURL.String(), body)
	if err != nil {
		return fmt.Errorf("create Mealie request: %w", err)
	}
	request.Header.Set("Accept", "application/json")
	if body != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	request.Header.Set("Authorization", "Bearer "+c.token)
	request.Header.Set("User-Agent", "KinOps/1.0")

	response, err := c.httpClient.Do(request)
	if err != nil {
		if ctx.Err() != nil {
			statusClass = "canceled"
			return ctx.Err()
		}
		statusClass = "network_error"
		return &ResponseError{Kind: ErrUnavailable}
	}
	defer response.Body.Close()
	statusClass = strconv.Itoa(response.StatusCode/100) + "xx"
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 64<<10))
		return statusError(response.StatusCode)
	}
	responseBody, err := io.ReadAll(io.LimitReader(response.Body, c.maxResponseBytes+1))
	if err != nil {
		return &ResponseError{Kind: ErrMalformedResponse}
	}
	if int64(len(responseBody)) > c.maxResponseBytes {
		return &ResponseError{Kind: ErrResponseTooLarge}
	}
	if destination == nil || len(bytes.TrimSpace(responseBody)) == 0 {
		return nil
	}
	if err := json.Unmarshal(responseBody, destination); err != nil {
		return &ResponseError{Kind: ErrMalformedResponse}
	}
	return nil
}

func mealieOperation(method, endpoint string) string {
	resource := "unknown"
	switch {
	case endpoint == "/api/app/about":
		resource = "about"
	case endpoint == "/api/users/self":
		resource = "current_user"
	case endpoint == "/api/users/self/favorites":
		resource = "favorites"
	case strings.Contains(endpoint, "/favorites/"):
		resource = "favorite"
	case endpoint == "/api/recipes":
		resource = "recipes"
	case strings.HasPrefix(endpoint, "/api/households/mealplans"):
		resource = "meal_plan"
	case endpoint == "/api/households/shopping/lists":
		resource = "shopping_lists"
	case strings.HasPrefix(endpoint, "/api/households/shopping/items"):
		resource = "shopping_item"
	case endpoint == "/api/foods":
		resource = "foods"
	case endpoint == "/api/units":
		resource = "units"
	}
	action := strings.ToLower(method)
	return resource + "." + action
}

func validatePathSegment(name, value string) error {
	value = strings.TrimSpace(value)
	if value == "" || value == "." || value == ".." || strings.ContainsAny(value, "/\\") {
		return fmt.Errorf("Mealie %s is invalid", name)
	}
	return nil
}

func statusError(statusCode int) error {
	kind := ErrUnexpectedStatus
	if statusCode >= http.StatusInternalServerError && statusCode <= 599 {
		return &ResponseError{StatusCode: statusCode, Kind: ErrUnavailable}
	}
	switch statusCode {
	case http.StatusUnauthorized:
		kind = ErrUnauthorized
	case http.StatusForbidden:
		kind = ErrForbidden
	case http.StatusNotFound:
		kind = ErrNotFound
	case http.StatusBadRequest, http.StatusUnprocessableEntity:
		kind = ErrValidation
	case http.StatusTooManyRequests:
		kind = ErrRateLimited
	case http.StatusBadGateway, http.StatusServiceUnavailable, http.StatusGatewayTimeout:
		kind = ErrUnavailable
	}
	return &ResponseError{StatusCode: statusCode, Kind: kind}
}
