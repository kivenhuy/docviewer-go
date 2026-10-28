package api

import (
	"net/http"
	"regexp"
	"strings"

	"github.com/kivenhuy/docviewer-go/internal/aggregator"
)

// vinPattern: 17 upper-case alphanumerics. The real VIN standard also forbids
// I, O and Q; that rule is intentionally not enforced so the mock systems'
// fault-injection prefixes (FAIL..., SLOW...) remain valid input.
var vinPattern = regexp.MustCompile(`^[A-Z0-9]{17}$`)

type Handler struct {
	agg *aggregator.Aggregator
}

func NewHandler(agg *aggregator.Aggregator) *Handler {
	return &Handler{agg: agg}
}

// Documents handles GET /api/v1/vehicles/{vin}/documents.
func (h *Handler) Documents(w http.ResponseWriter, r *http.Request) {
	vin := strings.ToUpper(strings.TrimSpace(r.PathValue("vin")))
	if vin == "" {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "vin is required"})
		return
	}
	if !vinPattern.MatchString(vin) {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "vin must be 17 alphanumeric characters"})
		return
	}

	result := h.agg.Documents(r.Context(), vin)
	writeJSON(w, http.StatusOK, result)
}
