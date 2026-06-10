package identity

import "strings"

const (
	HeaderUserID   = "X-User-ID"
	HeaderUserRole = "X-User-Role"

	RoleUser      Role = "USER"
	RoleOrganizer Role = "ORGANIZER"
	RoleAdmin     Role = "ADMIN"
)

type Role string

func NormalizeRole(value string) Role {
	role := Role(strings.ToUpper(strings.TrimSpace(value)))
	if role == "" {
		return RoleUser
	}
	return role
}

func ValidRole(role Role) bool {
	role = NormalizeRole(string(role))
	switch role {
	case RoleUser, RoleOrganizer, RoleAdmin:
		return true
	default:
		return false
	}
}

func CanPublishEvents(role Role) bool {
	role = NormalizeRole(string(role))
	return role == RoleOrganizer || role == RoleAdmin
}

func CanAdmin(role Role) bool {
	role = NormalizeRole(string(role))
	return role == RoleAdmin
}
