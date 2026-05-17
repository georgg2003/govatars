package otelpkg

// Instrumentation scope names group traces, metrics, and logs by subsystem in backends (Jaeger, Prometheus, Loki).
const (
	ScopeSlog     = "govatars"
	ScopeUsecase  = "govatars/usecase"
	ScopeS3       = "govatars/s3"
	ScopeRabbitMQ = "govatars/rabbitmq"
	ScopeWorker   = "govatars/worker"
	ScopeBusiness = "govatars/business"
)
