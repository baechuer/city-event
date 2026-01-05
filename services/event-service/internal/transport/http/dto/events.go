package dto

import "time"

type CreateEventReq struct {
	Title         string    `json:"title"`
	Description   string    `json:"description"`
	City          string    `json:"city"`
	Category      string    `json:"category"`
	StartTime     time.Time `json:"start_time"`
	EndTime       time.Time `json:"end_time"`
	Capacity      int       `json:"capacity"`
	CoverImageIDs []string  `json:"cover_image_ids"`
}

type UpdateEventReq struct {
	Title         *string    `json:"title,omitempty"`
	Description   *string    `json:"description,omitempty"`
	City          *string    `json:"city,omitempty"`
	Category      *string    `json:"category,omitempty"`
	StartTime     *time.Time `json:"start_time,omitempty"`
	EndTime       *time.Time `json:"end_time,omitempty"`
	Capacity      *int       `json:"capacity,omitempty"`
	CoverImageIDs *[]string  `json:"cover_image_ids,omitempty"`
}

type CancelEventReq struct {
	Reason string `json:"reason"`
}

type UnpublishEventReq struct {
	Reason string `json:"reason"`
}
