package httpapi

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5/middleware"
	"github.com/oapi-codegen/nullable"

	"github.com/Overland-East-Bay/trip-planner-api/internal/adapters/httpapi/oas"
)

func writeOASError(w http.ResponseWriter, r *http.Request, status int, code string, message string, details map[string]any) {
	var er oas.ErrorResponse
	er.Error.Code = code
	er.Error.Message = message
	if details != nil {
		er.Error.Details = nullable.NewNullableWithValue(map[string]any(details))
	}
	if rid := middleware.GetReqID(r.Context()); rid != "" {
		er.Error.RequestId = nullable.NewNullableWithValue(rid)
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(er)
}
