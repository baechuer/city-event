package httpapi

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
)

var (
	ErrRequestBodyTooLarge = errors.New("request body too large")
	ErrInvalidJSON         = errors.New("invalid json request")
)

func DecodeJSONLimited(w http.ResponseWriter, r *http.Request, target any, maxBytes int64) error {
	defer r.Body.Close()
	if maxBytes <= 0 {
		return fmt.Errorf("max bytes must be positive")
	}

	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBytes))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		if errors.As(err, new(*http.MaxBytesError)) {
			return ErrRequestBodyTooLarge
		}
		return ErrInvalidJSON
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		if errors.As(err, new(*http.MaxBytesError)) {
			return ErrRequestBodyTooLarge
		}
		return ErrInvalidJSON
	}
	return nil
}
