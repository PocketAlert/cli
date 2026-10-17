package api

import "encoding/json"

type Action struct {
	Type  string `json:"type"`
	Label string `json:"label"`
	Value string `json:"value"`
}

type Attachment struct {
	TID         string `json:"tid"`
	FileName    string `json:"file_name"`
	ContentType string `json:"content_type"`
	FileSize    int64  `json:"file_size"`
	Link        string `json:"link"`
}

type Profile struct {
	ID                 int64  `json:"id"`
	Username           string `json:"username"`
	Email              string `json:"email"`
	Timezone           string `json:"timezone"`
	EmailVerified      bool   `json:"email_verified"`
	SubscriptionStatus string `json:"subscription_status"`
}

type MessageCreate struct {
	Title         string          `json:"title"`
	Message       string          `json:"message"`
	ApplicationID string          `json:"application_id,omitempty"`
	DeviceID      string          `json:"device_id,omitempty"`
	Level         json.RawMessage `json:"level,omitempty"`
	Actions       []Action        `json:"actions,omitempty"`
	SendAt        string          `json:"send_at,omitempty"`
	Delay         string          `json:"delay,omitempty"`
	Image         string          `json:"image,omitempty"`
}

// MessageCreated covers both answers of POST /v1/messages: an immediately sent
// message and one stored for later delivery (Scheduled == true).
type MessageCreated struct {
	TID          string      `json:"tid"`
	Title        string      `json:"title"`
	Message      string      `json:"message"`
	Priority     int         `json:"priority"`
	Application  string      `json:"application"`
	Device       string      `json:"device"`
	Actions      []Action    `json:"actions"`
	Attachment   *Attachment `json:"attachment"`
	CreatedAt    string      `json:"created_at"`
	Scheduled    bool        `json:"scheduled"`
	Status       string      `json:"status"`
	DeliverAt    string      `json:"deliver_at"`
	DeliverAtUTC string      `json:"deliver_at_utc"`
	TZ           string      `json:"tz"`
}

type Message struct {
	TID          string      `json:"tid"`
	Title        string      `json:"title"`
	Message      string      `json:"message"`
	Application  string      `json:"application"`
	Device       string      `json:"device"`
	IsRead       bool        `json:"is_read"`
	Priority     int         `json:"priority"`
	WasScheduled bool        `json:"was_scheduled"`
	Actions      []Action    `json:"actions"`
	Attachment   *Attachment `json:"attachment"`
	CreatedAt    string      `json:"created_at"`
}

type MessagePage struct {
	Data []Message `json:"data"`
	Meta struct {
		Total      int  `json:"total"`
		HasMore    bool `json:"has_more"`
		NextOffset int  `json:"next_offset"`
	} `json:"meta"`
}

type ScheduledMessage struct {
	TID          string   `json:"tid"`
	Title        string   `json:"title"`
	Message      string   `json:"message"`
	Priority     int      `json:"priority"`
	Status       string   `json:"status"`
	DeliverAt    string   `json:"deliver_at"`
	DeliverAtUTC string   `json:"deliver_at_utc"`
	TZ           string   `json:"tz"`
	Application  string   `json:"application"`
	Device       string   `json:"device"`
	Actions      []Action `json:"actions"`
	CreatedAt    string   `json:"created_at"`
}

type MessageUsage struct {
	Used      int    `json:"used"`
	Limit     *int   `json:"limit"`
	Unlimited bool   `json:"unlimited"`
	Rejected  int    `json:"rejected"`
	ResetsAt  string `json:"resets_at"`
}

type MessageStat struct {
	Date  string `json:"date"`
	Count int    `json:"count"`
}

type Application struct {
	TID          string `json:"tid"`
	Name         string `json:"name"`
	Description  string `json:"description"`
	Color        string `json:"color"`
	Icon         string `json:"icon"`
	DefaultLevel *int   `json:"default_level"`
	IsActive     bool   `json:"is_active"`
	CreatedAt    string `json:"created_at"`
}

type Device struct {
	TID          string `json:"tid"`
	SerialNumber string `json:"serial_number"`
	Name         string `json:"name"`
	IsActive     bool   `json:"is_active"`
	CreatedAt    string `json:"created_at"`
}

type Webhook struct {
	TID            string `json:"tid"`
	Name           string `json:"name"`
	URL            string `json:"url"`
	ApplicationTID string `json:"application_tid"`
	DeviceTID      string `json:"device_tid"`
	Message        string `json:"message"`
	Actions        string `json:"actions"`
	DefaultLevel   *int   `json:"default_level"`
	IsActive       *bool  `json:"is_active"`
	CreatedAt      string `json:"created_at"`
}

type WebhookRequest struct {
	ID              int64  `json:"id"`
	Method          string `json:"method"`
	SourceIP        string `json:"source_ip"`
	ContentType     string `json:"content_type"`
	Body            string `json:"body"`
	Size            int64  `json:"size"`
	Status          string `json:"status"`
	ErrorCode       string `json:"error_code"`
	RenderedMessage string `json:"rendered_message"`
	MessageTID      string `json:"message_tid"`
	Priority        *int   `json:"priority"`
	DurationMS      int64  `json:"duration_ms"`
	CreatedAt       string `json:"created_at"`
}

type WebhookHistory struct {
	Requests      []WebhookRequest `json:"requests"`
	Total         int              `json:"total"`
	Page          int              `json:"page"`
	Per           int              `json:"per"`
	RetentionDays int              `json:"retention_days"`
}

type Form struct {
	TID              string `json:"tid"`
	Name             string `json:"name"`
	IsActive         bool   `json:"is_active"`
	Closed           bool   `json:"closed"`
	SubmissionsCount int    `json:"submissions_count"`
	LastSubmissionAt string `json:"last_submission_at"`
	CreatedAt        string `json:"created_at"`
}

type APIKey struct {
	TID       string `json:"tid"`
	Name      string `json:"name"`
	Token     string `json:"token"`
	IsActive  bool   `json:"is_active"`
	CreatedAt string `json:"created_at"`
}
