package dto

type MeStatusData struct {
	UserID        string `json:"user_id"`
	Role          string `json:"role"`
	Locked        bool   `json:"locked"`
	EmailVerified bool   `json:"email_verified"`
	HasPassword   bool   `json:"has_password"`
}
