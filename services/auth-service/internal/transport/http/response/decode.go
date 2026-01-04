package response

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"github.com/baechuer/real-time-ressys/services/auth-service/internal/domain"
)

// DecodeJSON decodes a JSON request body into dst.
// It rejects unknown fields and multiple JSON values.
func DecodeJSON(r *http.Request, dst any) error {
	dec := json.NewDecoder(r.Body)
	// dec.DisallowUnknownFields()

	if err := dec.Decode(dst); err != nil {
		return domain.ErrInvalidJSON(err)
	}

	// Disallow trailing data: {}{}
	// Decode one more time; it must be EOF.
	if err := dec.Decode(&struct{}{}); err != nil {
		if errors.Is(err, io.EOF) {
			return nil
		}
		return domain.ErrInvalidJSON(err)
	}

	return domain.ErrInvalidJSON(errors.New("multiple JSON values"))
}
