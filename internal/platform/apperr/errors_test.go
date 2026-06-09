package apperr

import (
	"errors"
	"testing"
)

func TestErrorWrapsCause(t *testing.T) {
	cause := errors.New("db unavailable")
	err := Wrap(CodeInternal, "dependency failed", cause)

	if !errors.Is(err, cause) {
		t.Fatalf("expected wrapped cause to be discoverable")
	}
	if err.Error() == "" {
		t.Fatalf("expected non-empty error string")
	}
}
