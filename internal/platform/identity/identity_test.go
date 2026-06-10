package identity

import "testing"

func TestRoleNormalizationAndPermissions(t *testing.T) {
	if got := NormalizeRole(" organizer "); got != RoleOrganizer {
		t.Fatalf("NormalizeRole() = %q, want ORGANIZER", got)
	}
	if got := NormalizeRole(""); got != RoleUser {
		t.Fatalf("empty NormalizeRole() = %q, want USER", got)
	}
	if !CanPublishEvents(Role("admin")) {
		t.Fatal("admin should be allowed to publish events")
	}
	if CanPublishEvents(RoleUser) {
		t.Fatal("plain user should not publish events")
	}
	if !CanAdmin(Role(" admin ")) {
		t.Fatal("admin role should allow admin operations")
	}
}
