package handlers

import (
	"net/http"

	"github.com/dcrespo1/kinops/internal/views/pages"
)

type HomeHandler struct{}

func NewHomeHandler() *HomeHandler {
	return &HomeHandler{}
}

func (h *HomeHandler) Get(w http.ResponseWriter, r *http.Request) {
	if err := pages.Home().Render(r.Context(), w); err != nil {
		http.Error(
			w,
			http.StatusText(http.StatusInternalServerError),
			http.StatusInternalServerError,
		)
	}
}
