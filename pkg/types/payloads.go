package types

import (
	"time"
)

// WebhookPayload represents the payload for HTTP webhook delivery tasks
type WebhookPayload struct {
	URL     string            `json:"url" validate:"required,url"`
	Method  string            `json:"method" validate:"required,oneof=GET POST PUT PATCH DELETE"`
	Headers map[string]string `json:"headers,omitempty"`
	Body    interface{}       `json:"body,omitempty"`

	// Webhook-specific options
	Timeout         time.Duration `json:"timeout,omitempty"`          // Request timeout
	FollowRedirects bool          `json:"follow_redirects,omitempty"` // Follow HTTP redirects
	VerifySSL       bool          `json:"verify_ssl,omitempty"`       // Verify SSL certificates
	ExpectedStatus  []int         `json:"expected_status,omitempty"`  // Expected HTTP status codes
	Secret          string        `json:"secret,omitempty"`           // For HMAC signature
	SignatureHeader string        `json:"signature_header,omitempty"` // Header name for signature

	// Circuit breaker settings
	CircuitBreakerKey string `json:"circuit_breaker_key,omitempty"` // Group webhooks for circuit breaking
}

// EmailPayload represents the payload for email processing tasks
type EmailPayload struct {
	To      []string `json:"to" validate:"required,min=1"`
	CC      []string `json:"cc,omitempty"`
	BCC     []string `json:"bcc,omitempty"`
	From    string   `json:"from" validate:"required,email"`
	ReplyTo string   `json:"reply_to,omitempty"`
	Subject string   `json:"subject" validate:"required"`

	// Content
	TextBody     string                 `json:"text_body,omitempty"`
	HTMLBody     string                 `json:"html_body,omitempty"`
	TemplateID   string                 `json:"template_id,omitempty"`   // Email template identifier
	TemplateData map[string]interface{} `json:"template_data,omitempty"` // Data for template rendering

	// Attachments
	Attachments []EmailAttachment `json:"attachments,omitempty"`

	// Email provider settings
	Provider string            `json:"provider,omitempty"` // sendgrid, ses, smtp, etc.
	Tags     []string          `json:"tags,omitempty"`     // For categorization/analytics
	Metadata map[string]string `json:"metadata,omitempty"` // Provider-specific metadata

	// Tracking
	TrackOpens  bool `json:"track_opens,omitempty"`
	TrackClicks bool `json:"track_clicks,omitempty"`
}

// EmailAttachment represents an email attachment
type EmailAttachment struct {
	Filename    string `json:"filename" validate:"required"`
	ContentType string `json:"content_type,omitempty"`
	Content     []byte `json:"content,omitempty"`     // Base64 encoded content
	ContentURL  string `json:"content_url,omitempty"` // URL to fetch content
	Size        int64  `json:"size,omitempty"`        // Size in bytes
}

// ImageProcessPayload represents the payload for image processing tasks
type ImageProcessPayload struct {
	SourceURL  string `json:"source_url,omitempty"`  // URL to source image
	SourceData []byte `json:"source_data,omitempty"` // Raw image data
	SourcePath string `json:"source_path,omitempty"` // File system path

	Operations []ImageOperation `json:"operations" validate:"required,min=1"`
	OutputPath string           `json:"output_path,omitempty"` // Where to save result
	OutputURL  string           `json:"output_url,omitempty"`  // Where to upload result

	// Processing options
	Quality     int    `json:"quality,omitempty"`     // JPEG quality (1-100)
	Format      string `json:"format,omitempty"`      // Output format (jpg, png, webp)
	Progressive bool   `json:"progressive,omitempty"` // Progressive JPEG

	// Metadata preservation
	PreserveMetadata bool `json:"preserve_metadata,omitempty"`

	// Callback
	CallbackURL string `json:"callback_url,omitempty"` // Webhook when complete
}

// ImageOperation represents a single image processing operation
type ImageOperation struct {
	Type       ImageOperationType     `json:"type" validate:"required"`
	Parameters map[string]interface{} `json:"parameters,omitempty"`
}

// ImageOperationType defines available image operations
type ImageOperationType string

const (
	ImageOpResize    ImageOperationType = "resize"    // Resize image
	ImageOpCrop      ImageOperationType = "crop"      // Crop image
	ImageOpWatermark ImageOperationType = "watermark" // Add watermark
	ImageOpFilter    ImageOperationType = "filter"    // Apply filters (blur, sharpen, etc.)
	ImageOpRotate    ImageOperationType = "rotate"    // Rotate image
	ImageOpFlip      ImageOperationType = "flip"      // Flip horizontal/vertical
	ImageOpCompress  ImageOperationType = "compress"  // Compress/optimize
)

// DataProcessPayload represents the payload for data processing tasks
type DataProcessPayload struct {
	SourceType   DataSourceType   `json:"source_type" validate:"required"`
	SourceConfig DataSourceConfig `json:"source_config" validate:"required"`

	TargetType   DataTargetType   `json:"target_type" validate:"required"`
	TargetConfig DataTargetConfig `json:"target_config" validate:"required"`

	Operations []DataOperation `json:"operations,omitempty"`

	// Processing options
	BatchSize int `json:"batch_size,omitempty"` // Records per batch
	MaxErrors int `json:"max_errors,omitempty"` // Max errors before failing

	// Progress tracking
	CallbackURL string `json:"callback_url,omitempty"` // Progress updates
}

// DataSourceType defines supported data sources
type DataSourceType string

const (
	DataSourceCSV      DataSourceType = "csv"
	DataSourceJSON     DataSourceType = "json"
	DataSourceDatabase DataSourceType = "database"
	DataSourceS3       DataSourceType = "s3"
	DataSourceAPI      DataSourceType = "api"
)

// DataTargetType defines supported data targets
type DataTargetType string

const (
	DataTargetCSV      DataTargetType = "csv"
	DataTargetJSON     DataTargetType = "json"
	DataTargetDatabase DataTargetType = "database"
	DataTargetS3       DataTargetType = "s3"
	DataTargetAPI      DataTargetType = "api"
)

// DataSourceConfig contains source-specific configuration
type DataSourceConfig struct {
	URL        string                 `json:"url,omitempty"`
	Path       string                 `json:"path,omitempty"`
	Query      string                 `json:"query,omitempty"`      // SQL query for database
	Headers    map[string]string      `json:"headers,omitempty"`    // HTTP headers for API
	Auth       AuthConfig             `json:"auth,omitempty"`       // Authentication config
	Parameters map[string]interface{} `json:"parameters,omitempty"` // Source-specific params
}

// DataTargetConfig contains target-specific configuration
type DataTargetConfig struct {
	URL        string                 `json:"url,omitempty"`
	Path       string                 `json:"path,omitempty"`
	Table      string                 `json:"table,omitempty"`      // Database table
	Headers    map[string]string      `json:"headers,omitempty"`    // HTTP headers for API
	Auth       AuthConfig             `json:"auth,omitempty"`       // Authentication config
	Parameters map[string]interface{} `json:"parameters,omitempty"` // Target-specific params
}

// DataOperation represents a data transformation operation
type DataOperation struct {
	Type       DataOperationType      `json:"type" validate:"required"`
	Parameters map[string]interface{} `json:"parameters,omitempty"`
}

// DataOperationType defines available data operations
type DataOperationType string

const (
	DataOpFilter    DataOperationType = "filter"    // Filter rows
	DataOpTransform DataOperationType = "transform" // Transform columns
	DataOpAggregate DataOperationType = "aggregate" // Group and aggregate
	DataOpJoin      DataOperationType = "join"      // Join with other data
	DataOpSort      DataOperationType = "sort"      // Sort data
	DataOpDedupe    DataOperationType = "dedupe"    // Remove duplicates
	DataOpValidate  DataOperationType = "validate"  // Validate data quality
)

// ScheduledPayload represents the payload for scheduled/cron tasks
type ScheduledPayload struct {
	CronExpression string     `json:"cron_expression,omitempty"` // Cron expression for recurring
	Timezone       string     `json:"timezone,omitempty"`        // Timezone for execution
	MaxRuns        int        `json:"max_runs,omitempty"`        // Limit number of executions
	StartDate      *time.Time `json:"start_date,omitempty"`      // When to start schedule
	EndDate        *time.Time `json:"end_date,omitempty"`        // When to end schedule

	// The actual task to execute
	TaskType    TaskType    `json:"task_type" validate:"required"`
	TaskPayload interface{} `json:"task_payload" validate:"required"`
	TaskOptions TaskOptions `json:"task_options,omitempty"`
}

// BatchPayload represents the payload for batch operations
type BatchPayload struct {
	Tasks       []BatchTask `json:"tasks" validate:"required,min=1"`
	Sequential  bool        `json:"sequential,omitempty"`    // Process tasks in sequence
	StopOnError bool        `json:"stop_on_error,omitempty"` // Stop batch if any task fails

	// Progress tracking
	CallbackURL     string            `json:"callback_url,omitempty"` // Progress webhook
	CallbackHeaders map[string]string `json:"callback_headers,omitempty"`
}

// BatchTask represents a single task within a batch
type BatchTask struct {
	ID       string      `json:"id"` // Unique ID within batch
	Type     TaskType    `json:"type" validate:"required"`
	Payload  interface{} `json:"payload" validate:"required"`
	Priority Priority    `json:"priority,omitempty"`
	Options  TaskOptions `json:"options,omitempty"`
}
