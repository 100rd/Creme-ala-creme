package v1alpha1

import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

// CloudflareOperatorConfigSpec defines the desired configuration for the operator.
type CloudflareOperatorConfigSpec struct {
	// Kafka holds connection and topic configuration.
	Kafka KafkaConfig `json:"kafka,omitempty"`
	// Features holds feature flag configuration.
	Features FeatureFlags `json:"features,omitempty"`
	// Reconciliation holds reconciliation loop tuning.
	Reconciliation ReconciliationConfig `json:"reconciliation,omitempty"`
}

// KafkaConfig defines Kafka connection and topic settings.
type KafkaConfig struct {
	// BootstrapServers is a comma-separated list of broker addresses.
	BootstrapServers string `json:"bootstrapServers,omitempty"`
	// TLSEnabled enables TLS for Kafka connections.
	TLSEnabled bool `json:"tlsEnabled,omitempty"`
	// Topics holds topic name configuration.
	Topics KafkaTopics `json:"topics,omitempty"`
}

// KafkaTopics defines the Kafka topic names used by the operator.
type KafkaTopics struct {
	// Sessions is the topic name for session events.
	Sessions string `json:"sessions,omitempty"`
	// IDs is the topic name for ID events.
	IDs string `json:"ids,omitempty"`
}

// FeatureFlags controls operator behavior at runtime.
type FeatureFlags struct {
	// TracingEnabled enables distributed tracing via OTEL.
	TracingEnabled bool `json:"tracingEnabled,omitempty"`
	// MetricsEnabled enables Prometheus metrics endpoint.
	MetricsEnabled bool `json:"metricsEnabled,omitempty"`
	// CloudflareAPIEnabled enables live Cloudflare API calls (false = dry-run mode).
	CloudflareAPIEnabled bool `json:"cloudflareAPIEnabled,omitempty"`
	// DryRunMode when true logs actions but does not apply them.
	DryRunMode bool `json:"dryRunMode,omitempty"`
}

// ReconciliationConfig tunes the reconciliation loop behavior.
type ReconciliationConfig struct {
	// RequeueDuration is how long to wait before requeuing a failed reconciliation (default: 30s).
	RequeueDuration metav1.Duration `json:"requeueDuration,omitempty"`
	// MaxConcurrentReconciles controls parallelism (default: 1).
	MaxConcurrentReconciles int `json:"maxConcurrentReconciles,omitempty"`
}

// CloudflareOperatorConfigStatus reflects the last-applied configuration generation.
type CloudflareOperatorConfigStatus struct {
	// ObservedGeneration is the generation of the spec last processed.
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`
	// Conditions reflect the current state.
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status
//+kubebuilder:resource:scope=Cluster,shortName=cfoc
//+kubebuilder:printcolumn:name="Kafka Brokers",type=string,JSONPath=`.spec.kafka.bootstrapServers`
//+kubebuilder:printcolumn:name="Tracing",type=boolean,JSONPath=`.spec.features.tracingEnabled`
//+kubebuilder:printcolumn:name="DryRun",type=boolean,JSONPath=`.spec.features.dryRunMode`
//+kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// CloudflareOperatorConfig is the cluster-scoped configuration resource for the operator.
// Exactly one instance should exist (by convention named "default").
// Managed via ArgoCD or applied directly with kubectl.
type CloudflareOperatorConfig struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   CloudflareOperatorConfigSpec   `json:"spec,omitempty"`
	Status CloudflareOperatorConfigStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// CloudflareOperatorConfigList contains a list of CloudflareOperatorConfig.
type CloudflareOperatorConfigList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []CloudflareOperatorConfig `json:"items"`
}

func init() {
	SchemeBuilder.Register(&CloudflareOperatorConfig{}, &CloudflareOperatorConfigList{})
}
