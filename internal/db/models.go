package db

type User struct {
	ID           int64  `json:"id"`
	Username     string `json:"username"`
	PasswordHash string `json:"-"`
	Role         string `json:"role"`
	DisplayName  string `json:"display_name"`
	Email        string `json:"email"`
	Status       int    `json:"status"`
	CreatedAt    string `json:"created_at"`
	UpdatedAt    string `json:"updated_at"`
}

type Form struct {
	ID                   int64  `json:"id"`
	OwnerUserID          int64  `json:"owner_user_id"`
	ChannelID            int64  `json:"channel_id"`
	Name                 string `json:"name"`
	Token                string `json:"token"`
	RecipientEmail       string `json:"recipient_email"`
	EmailVerified        bool   `json:"email_verified"`
	VerifyToken          string `json:"verify_token"`
	SuccessRedirect      string `json:"success_redirect"`
	SuccessMessage       string `json:"success_message"`
	SuccessTheme         string `json:"success_theme"`
	AllowedOrigins       string `json:"allowed_origins"`
	AutoReplyEnabled     bool   `json:"auto_reply_enabled"`
	AutoReplySubject     string `json:"auto_reply_subject"`
	AutoReplyBody        string `json:"auto_reply_body"`
	EmailSubjectTemplate string `json:"email_subject_template"`
	EmailBodyTemplate    string `json:"email_body_template"`
	HoneypotField        string `json:"honeypot_field"`
	WebhookURL           string `json:"webhook_url"`
	WebhookSecret        string `json:"webhook_secret"`
	FieldsSchema         string `json:"fields_schema"`
	Active               bool   `json:"active"`
	CreatedAt            string `json:"created_at"`
	UpdatedAt            string `json:"updated_at"`
}

type Channel struct {
	ID                      int64  `json:"id"`
	OwnerUserID             int64  `json:"owner_user_id"`
	Name                    string `json:"name"`
	Type                    string `json:"type"`
	Provider                string `json:"provider"`
	Protocol                string `json:"protocol"`
	Host                    string `json:"host"`
	Port                    int    `json:"port"`
	Username                string `json:"username"`
	Password                string `json:"password,omitempty"`
	PasswordEnc             string `json:"-"`
	FromEmail               string `json:"from_email"`
	UseTLS                  bool   `json:"use_tls"`
	Priority                int    `json:"priority"`
	Enabled                 bool   `json:"enabled"`
	ShareEnabled            bool   `json:"share_enabled"`
	ShareMaxBindingsPerUser int    `json:"share_max_bindings_per_user"`
	ShareMaxTotalBindings   int    `json:"share_max_total_bindings"`
	CreatedAt               string `json:"created_at"`
	UpdatedAt               string `json:"updated_at"`
}

type Submission struct {
	ID        int64             `json:"id"`
	FormID    int64             `json:"form_id"`
	IP        string            `json:"ip"`
	UserAgent string            `json:"user_agent"`
	Data      map[string]string `json:"data"`
	IsSpam    bool              `json:"is_spam"`
	EmailSent bool              `json:"email_sent"`
	SendError string            `json:"send_error"`
	CreatedAt string            `json:"created_at"`
}

type APIKey struct {
	ID         int64  `json:"id"`
	UserID     int64  `json:"user_id"`
	Name       string `json:"name"`
	KeyPrefix  string `json:"key_prefix"`
	ChannelID  int64  `json:"channel_id"`
	CreatedAt  string `json:"created_at"`
	LastUsedAt string `json:"last_used_at,omitempty"`
	Enabled    bool   `json:"enabled"`
}

type APILog struct {
	ID       int64  `json:"id"`
	APIKeyID int64  `json:"api_key_id"`
	UserID   int64  `json:"user_id"`
	To       string `json:"to"`
	Subject  string `json:"subject"`
	Status   string `json:"status"`
	Error    string `json:"error,omitempty"`
	CreatedAt string `json:"created_at"`
}
