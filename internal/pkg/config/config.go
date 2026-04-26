// Package config loads application settings with Viper: defaults, YAML file, environment, then CLI flags.
package config

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/spf13/pflag"
	"github.com/spf13/viper"

	"govatars/internal/pkg/apperr"
)

// App aggregates service configuration.
type App struct {
	Logging  Logging  `mapstructure:"logging"`
	HTTP     HTTP     `mapstructure:"http"`
	Postgres Postgres `mapstructure:"postgres"`
	S3       S3       `mapstructure:"s3"`
	RabbitMQ RabbitMQ `mapstructure:"rabbitmq"`
	Avatars  Avatars  `mapstructure:"avatars"`
}

// Logging configures process-wide slog output (level only for now).
type Logging struct {
	// Level is one of: debug, info, warn, error (YAML/env: logging.level, GOVATARS_LOGGING_LEVEL).
	Level string `mapstructure:"level"`
}

// HTTP server options.
type HTTP struct {
	Address          string    `mapstructure:"address"`
	PublicBaseURL    string    `mapstructure:"public_base_url"`
	PlaceholderPath  string    `mapstructure:"placeholder_path"`   // default avatar when user has none (empty = none / 404)
	StaticDir        string    `mapstructure:"static_dir"`         // filesystem root served at /web
	CORSAllowOrigins []string  `mapstructure:"cors_allow_origins"` // empty = disabled; ["*"] for any
	RateLimit        RateLimit `mapstructure:"rate_limit"`
}

// RateLimit configures echo rate limiter (disabled when RequestsPerSecond <= 0).
type RateLimit struct {
	RequestsPerSecond float64 `mapstructure:"requests_per_second"`
	Burst             int     `mapstructure:"burst"`
}

// Postgres connection. If DSN is non-empty it is used as-is; otherwise Host, User, Database (and optional fields) build a URL.
// Pool fields apply to [pgxpool.Pool] (ignored when using raw DSN-only clients outside this repo).
type Postgres struct {
	DSN      string `mapstructure:"dsn"`
	Host     string `mapstructure:"host"`
	Port     int    `mapstructure:"port"`
	User     string `mapstructure:"user"`
	Password string `mapstructure:"password"`
	Database string `mapstructure:"database"`
	SSLMode  string `mapstructure:"sslmode"`

	PoolMaxConns              int32         `mapstructure:"pool_max_conns"`
	PoolMinConns              int32         `mapstructure:"pool_min_conns"`
	PoolMaxConnLifetime       time.Duration `mapstructure:"pool_max_conn_lifetime"`
	PoolMaxConnIdleTime       time.Duration `mapstructure:"pool_max_conn_idle_time"`
	PoolHealthCheckPeriod     time.Duration `mapstructure:"pool_health_check_period"`
	PoolMaxConnLifetimeJitter time.Duration `mapstructure:"pool_max_conn_lifetime_jitter"`
}

// ResolveDSN returns the connection string from DSN or composed fields.
func (p Postgres) ResolveDSN() (string, error) {
	if strings.TrimSpace(p.DSN) != "" {
		return p.DSN, nil
	}
	if p.Host == "" || p.User == "" || p.Database == "" {
		return "", errors.New("postgres: set postgres.dsn or postgres.host, user, and database")
	}
	port := p.Port
	if port == 0 {
		port = 5432
	}
	ssl := p.SSLMode
	if ssl == "" {
		ssl = "disable"
	}
	u := &url.URL{
		Scheme: "postgres",
		User:   url.UserPassword(p.User, p.Password),
		Host:   fmt.Sprintf("%s:%d", p.Host, port),
		Path:   "/" + strings.TrimPrefix(p.Database, "/"),
	}
	q := url.Values{}
	q.Set("sslmode", ssl)
	u.RawQuery = q.Encode()
	return u.String(), nil
}

// S3-compatible object storage (MinIO, AWS S3).
type S3 struct {
	Endpoint  string `mapstructure:"endpoint"`
	AccessKey string `mapstructure:"access_key"`
	SecretKey string `mapstructure:"secret_key"`
	Bucket    string `mapstructure:"bucket"`
	UseSSL    bool   `mapstructure:"use_ssl"`
	Region    string `mapstructure:"region"`
}

// RabbitMQ topology and connection.
type RabbitMQ struct {
	URL                 string `mapstructure:"url"`
	Exchange            string `mapstructure:"exchange"`
	UploadRoutingKey    string `mapstructure:"upload_routing_key"`
	DeleteRoutingKey    string `mapstructure:"delete_routing_key"`
	UploadQueue         string `mapstructure:"upload_queue"`
	DeleteQueue         string `mapstructure:"delete_queue"`
	UploadDLQQueue      string `mapstructure:"upload_dlq_queue"`
	UploadDLQRoutingKey string `mapstructure:"upload_dlq_routing_key"`
	DeleteDLQQueue      string `mapstructure:"delete_dlq_queue"`
	DeleteDLQRoutingKey string `mapstructure:"delete_dlq_routing_key"`
	UploadRetryDelaysMS []int  `mapstructure:"upload_retry_delays_ms"`
	DeleteRetryDelaysMS []int  `mapstructure:"delete_retry_delays_ms"`
	// ConsumerHandleTimeout bounds a single message handler (upload/delete processing).
	ConsumerHandleTimeout time.Duration `mapstructure:"consumer_handle_timeout"`
	// UploadConsumerTag and DeleteConsumerTag are AMQP basic.consume consumer tags (RabbitMQ management / cancel).
	UploadConsumerTag string `mapstructure:"upload_consumer_tag"`
	DeleteConsumerTag string `mapstructure:"delete_consumer_tag"`
}

// Load reads configuration: defaults → optional YAML → environment (GOVATARS_*).
func Load() (*App, error) {
	fs := pflag.NewFlagSet(os.Args[0], pflag.ContinueOnError)
	fs.SetOutput(os.Stderr)

	defaultConfig := "config/config.yaml"
	configPath := fs.StringP("config", "c", defaultConfig, "path to YAML config file")

	if err := fs.Parse(os.Args[1:]); err != nil {
		return nil, apperr.Wrap(err, "parse flags")
	}

	v := viper.New()
	v.SetConfigType("yaml")
	setDefaults(v)

	if *configPath != "" {
		v.SetConfigFile(*configPath)
		if err := v.ReadInConfig(); err != nil {
			return nil, apperr.Wrap(err, fmt.Sprintf("read config %q", *configPath))
		}
	}

	v.SetEnvPrefix("GOVATARS")
	v.AutomaticEnv()
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))

	var cfg App
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, apperr.Wrap(err, "unmarshal config")
	}

	cfg.Normalize()
	return &cfg, nil
}

func setDefaults(v *viper.Viper) {
	v.SetDefault("logging.level", "info")

	v.SetDefault("http.address", "0.0.0.0:8080")
	v.SetDefault("http.public_base_url", "http://localhost:8080")
	v.SetDefault("http.placeholder_path", "web/static/placeholder.png")
	v.SetDefault("http.static_dir", "web/static")
	v.SetDefault("http.cors_allow_origins", []string{"*"})
	v.SetDefault("http.rate_limit.requests_per_second", 50)
	v.SetDefault("http.rate_limit.burst", 100)

	v.SetDefault("postgres.dsn", "postgres://govatars:govatars@localhost:5432/govatars?sslmode=disable")
	v.SetDefault("postgres.pool_max_conns", 4)
	v.SetDefault("postgres.pool_min_conns", 0)
	v.SetDefault("postgres.pool_max_conn_lifetime", "1h")
	v.SetDefault("postgres.pool_max_conn_idle_time", "30m")
	v.SetDefault("postgres.pool_health_check_period", "1m")
	v.SetDefault("postgres.pool_max_conn_lifetime_jitter", "0s")

	v.SetDefault("avatars.max_upload_bytes", 10*1024*1024)
	v.SetDefault("avatars.image_cache_control", "max-age=86400")
	v.SetDefault("s3.endpoint", "localhost:9000")
	v.SetDefault("s3.access_key", "minioadmin")
	v.SetDefault("s3.secret_key", "minioadmin")
	v.SetDefault("s3.bucket", "avatars")
	v.SetDefault("s3.use_ssl", false)
	v.SetDefault("s3.region", "us-east-1")

	v.SetDefault("rabbitmq.url", "amqp://guest:guest@localhost:5672/")
	v.SetDefault("rabbitmq.exchange", "avatars")
	v.SetDefault("rabbitmq.upload_routing_key", "avatar.uploaded")
	v.SetDefault("rabbitmq.delete_routing_key", "avatar.deleted")
	v.SetDefault("rabbitmq.upload_queue", "avatars.upload")
	v.SetDefault("rabbitmq.delete_queue", "avatars.delete")
	v.SetDefault("rabbitmq.upload_dlq_queue", "avatars.upload.dlq")
	v.SetDefault("rabbitmq.upload_dlq_routing_key", "avatar.upload.failed")
	v.SetDefault("rabbitmq.delete_dlq_queue", "avatars.delete.dlq")
	v.SetDefault("rabbitmq.delete_dlq_routing_key", "avatar.delete.failed")
	v.SetDefault("rabbitmq.upload_retry_delays_ms", []int{1000, 5000, 15000})
	v.SetDefault("rabbitmq.delete_retry_delays_ms", []int{1000, 5000, 15000})
	v.SetDefault("rabbitmq.consumer_handle_timeout", "3m")
	v.SetDefault("rabbitmq.upload_consumer_tag", "govatars-upload")
	v.SetDefault("rabbitmq.delete_consumer_tag", "govatars-delete")
}
