package config

import (
	"fmt"
	"net"
	"net/url"
	"strings"
)

// validateServerConfig validates server configuration settings including TLS domain, port range, and IP allowlists for both server and admin access.
func validateServerConfig(c *Config) error {
	if c.Server.TLSDomain != "" {
		c.Server.TLSEnabled = true
	}
	if c.Server.TLSEnabled && c.Server.TLSDomain == "" {
		return fmt.Errorf("server.tls_domain is required when TLS is enabled")
	}
	if c.Server.Port < 1 || c.Server.Port > 65535 {
		return fmt.Errorf("server.port must be between 1 and 65535, got %d", c.Server.Port)
	}
	if err := validateAllowlist(c.Server.AllowedIPs, "server.allowed_ips"); err != nil {
		return err
	}
	if err := validateAllowlist(c.Admin.AllowedIPs, "admin.allowed_ips"); err != nil {
		return err
	}
	return nil
}

func validateDatabaseConfig(c *Config) error {
	if c.Database.MaxConns < 1 {
		return fmt.Errorf("database.max_conns must be at least 1, got %d", c.Database.MaxConns)
	}
	if c.Database.MinConns < 0 {
		return fmt.Errorf("database.min_conns must be non-negative, got %d", c.Database.MinConns)
	}
	if c.Database.MinConns > c.Database.MaxConns {
		return fmt.Errorf("database.min_conns (%d) cannot exceed database.max_conns (%d)", c.Database.MinConns, c.Database.MaxConns)
	}
	seenReplicaURLs := make(map[string]int, len(c.Database.Replicas))
	for i, replica := range c.Database.Replicas {
		trimmedURL := strings.TrimSpace(replica.URL)
		if trimmedURL == "" {
			return fmt.Errorf("database.replicas[%d].url must not be empty", i)
		}
		if _, err := url.Parse(trimmedURL); err != nil {
			return fmt.Errorf("database.replicas[%d].url is not a valid URL: %w", i, err)
		}
		if prev, ok := seenReplicaURLs[trimmedURL]; ok {
			return fmt.Errorf("database.replicas[%d].url is a duplicate of replicas[%d]", i, prev)
		}
		seenReplicaURLs[trimmedURL] = i
		if replica.Weight < 1 {
			return fmt.Errorf("database.replicas[%d].weight must be at least 1", i)
		}
		if replica.MaxLagBytes < 0 {
			return fmt.Errorf("database.replicas[%d].max_lag_bytes must be non-negative", i)
		}
	}
	if c.Database.URL == "" && (c.Database.EmbeddedPort < 1 || c.Database.EmbeddedPort > 65535) {
		return fmt.Errorf("database.embedded_port must be between 1 and 65535, got %d", c.Database.EmbeddedPort)
	}
	if c.Database.URL == "" && (c.ManagedPG.Port < 1 || c.ManagedPG.Port > 65535) {
		return fmt.Errorf("managed_pg.port must be between 1 and 65535, got %d", c.ManagedPG.Port)
	}
	return nil
}

func validateBillingConfig(c *Config) error {
	if c.Billing.Provider != "" && c.Billing.Provider != "stripe" {
		return fmt.Errorf("billing.provider must be empty or \"stripe\", got %q", c.Billing.Provider)
	}
	if c.Billing.UsageSyncIntervalSecs <= 0 {
		return fmt.Errorf("billing.usage_sync_interval_seconds must be positive, got %d", c.Billing.UsageSyncIntervalSecs)
	}
	return nil
}

func validateGraphQLConfig(c *Config) error {
	switch c.GraphQL.Introspection {
	case "", "open", "disabled":
	default:
		return fmt.Errorf("graphql.introspection must be one of \"\", \"open\", \"disabled\", got %q", c.GraphQL.Introspection)
	}
	return nil
}

// validateBillingStripeConfig validates Stripe-specific billing configuration when the billing provider is set to Stripe, including API keys, webhook secrets, and price IDs.
func validateBillingStripeConfig(c *Config) error {
	if c.Billing.Provider != "stripe" {
		return nil
	}
	if c.Billing.StripeSecretKey == "" {
		return fmt.Errorf("billing.stripe_secret_key is required when billing.provider = stripe")
	}
	if c.Billing.StripeWebhookSecret == "" {
		return fmt.Errorf("billing.stripe_webhook_secret is required when billing.provider = stripe")
	}
	if c.Billing.StripeStarterPriceID == "" {
		return fmt.Errorf("billing.stripe_starter_price_id is required when billing.provider = stripe")
	}
	if c.Billing.StripeProPriceID == "" {
		return fmt.Errorf("billing.stripe_pro_price_id is required when billing.provider = stripe")
	}
	if c.Billing.StripeEnterprisePriceID == "" {
		return fmt.Errorf("billing.stripe_enterprise_price_id is required when billing.provider = stripe")
	}
	if c.Billing.StripeMeterAPIRequests == "" {
		return fmt.Errorf("billing.stripe_meter_api_requests is required when billing.provider = stripe")
	}
	if c.Billing.StripeMeterStorageBytes == "" {
		return fmt.Errorf("billing.stripe_meter_storage_bytes is required when billing.provider = stripe")
	}
	if c.Billing.StripeMeterBandwidthBytes == "" {
		return fmt.Errorf("billing.stripe_meter_bandwidth_bytes is required when billing.provider = stripe")
	}
	if c.Billing.StripeMeterFunctionInvs == "" {
		return fmt.Errorf("billing.stripe_meter_function_invocations is required when billing.provider = stripe")
	}
	return nil
}

// validateEmailConfig validates email backend configuration, with different requirements for log, SMTP, and webhook backends.
func validateEmailConfig(c *Config) error {
	switch c.Email.Backend {
	case "", "log":
	case "smtp":
		if c.Email.SMTP.Host == "" {
			return fmt.Errorf("email.smtp.host is required when email backend is \"smtp\"")
		}
		if c.Email.From == "" {
			return fmt.Errorf("email.from is required when email backend is \"smtp\"")
		}
	case "webhook":
		if c.Email.Webhook.URL == "" {
			return fmt.Errorf("email.webhook.url is required when email backend is \"webhook\"")
		}
	default:
		return fmt.Errorf("email.backend must be \"log\", \"smtp\", or \"webhook\", got %q", c.Email.Backend)
	}
	return nil
}

// validateStorageConfig validates storage backend configuration, with different requirements for local filesystem and S3 backends.
func validateStorageConfig(c *Config) error {
	if !c.Storage.Enabled {
		return nil
	}
	switch c.Storage.Backend {
	case "local":
		if c.Storage.LocalPath == "" {
			return fmt.Errorf("storage.local_path is required when storage backend is \"local\"")
		}
	case "s3":
		if c.Storage.S3Endpoint == "" {
			return fmt.Errorf("storage.s3_endpoint is required when storage backend is \"s3\"")
		}
		if c.Storage.S3Bucket == "" {
			return fmt.Errorf("storage.s3_bucket is required when storage backend is \"s3\"")
		}
		if c.Storage.S3AccessKey == "" {
			return fmt.Errorf("storage.s3_access_key is required when storage backend is \"s3\"")
		}
		if c.Storage.S3SecretKey == "" {
			return fmt.Errorf("storage.s3_secret_key is required when storage backend is \"s3\"")
		}
	default:
		return fmt.Errorf("storage.backend must be \"local\" or \"s3\", got %q", c.Storage.Backend)
	}
	if err := validateStorageCDNConfig(c.Storage); err != nil {
		return err
	}
	return nil
}

// validateStorageCDNConfig validates CDN cache-invalidation provider settings (cloudflare, cloudfront, or webhook) when a provider is configured, ensuring required credentials and endpoints are present.
func validateStorageCDNConfig(storage StorageConfig) error {
	provider := storage.CDN.NormalizedProvider()
	switch provider {
	case "":
		return nil
	case "cloudflare":
		if strings.TrimSpace(storage.CDNURL) == "" {
			return fmt.Errorf("storage.cdn_url is required when storage.cdn.provider is configured")
		}
		if strings.TrimSpace(storage.CDN.Cloudflare.ZoneID) == "" {
			return fmt.Errorf("storage.cdn.cloudflare.zone_id is required when storage.cdn.provider is \"cloudflare\"")
		}
		if strings.TrimSpace(storage.CDN.Cloudflare.APIToken) == "" {
			return fmt.Errorf("storage.cdn.cloudflare.api_token is required when storage.cdn.provider is \"cloudflare\"")
		}
		return nil
	case "cloudfront":
		if strings.TrimSpace(storage.CDNURL) == "" {
			return fmt.Errorf("storage.cdn_url is required when storage.cdn.provider is configured")
		}
		if strings.TrimSpace(storage.CDN.CloudFront.DistributionID) == "" {
			return fmt.Errorf("storage.cdn.cloudfront.distribution_id is required when storage.cdn.provider is \"cloudfront\"")
		}
		return nil
	case "webhook":
		if strings.TrimSpace(storage.CDNURL) == "" {
			return fmt.Errorf("storage.cdn_url is required when storage.cdn.provider is configured")
		}
		if strings.TrimSpace(storage.CDN.Webhook.Endpoint) == "" {
			return fmt.Errorf("storage.cdn.webhook.endpoint is required when storage.cdn.provider is \"webhook\"")
		}
		if strings.TrimSpace(storage.CDN.Webhook.SigningSecret) == "" {
			return fmt.Errorf("storage.cdn.webhook.signing_secret is required when storage.cdn.provider is \"webhook\"")
		}
		return nil
	default:
		return fmt.Errorf("storage.cdn.provider must be one of: cloudflare, cloudfront, webhook, or empty; got %q", storage.CDN.Provider)
	}
}

// validateEdgeFuncConfig validates edge function settings including pool size, timeouts, memory limits, and concurrency bounds.
func validateEdgeFuncConfig(c *Config) error {
	if c.EdgeFunctions.PoolSize < 1 {
		return fmt.Errorf("edge_functions.pool_size must be at least 1, got %d", c.EdgeFunctions.PoolSize)
	}
	if c.EdgeFunctions.DefaultTimeoutMs < 1 {
		return fmt.Errorf("edge_functions.default_timeout_ms must be at least 1, got %d", c.EdgeFunctions.DefaultTimeoutMs)
	}
	if c.EdgeFunctions.MaxRequestBodyBytes < 1 {
		return fmt.Errorf("edge_functions.max_request_body_bytes must be at least 1, got %d", c.EdgeFunctions.MaxRequestBodyBytes)
	}
	if c.EdgeFunctions.MemoryLimitMB < 1 {
		return fmt.Errorf("edge_functions.memory_limit_mb must be at least 1, got %d", c.EdgeFunctions.MemoryLimitMB)
	}
	if c.EdgeFunctions.MaxConcurrentInvocations < 1 {
		return fmt.Errorf("edge_functions.max_concurrent_invocations must be at least 1, got %d", c.EdgeFunctions.MaxConcurrentInvocations)
	}
	if c.EdgeFunctions.CodeCacheSize < 1 {
		return fmt.Errorf("edge_functions.code_cache_size must be at least 1, got %d", c.EdgeFunctions.CodeCacheSize)
	}
	if c.EdgeFunctions.MaxConcurrentInvocations < c.EdgeFunctions.PoolSize {
		return fmt.Errorf(
			"edge_functions.max_concurrent_invocations must be at least edge_functions.pool_size (%d), got %d",
			c.EdgeFunctions.PoolSize,
			c.EdgeFunctions.MaxConcurrentInvocations,
		)
	}
	return nil
}

// validateAllowlist validates a list of allowed IP addresses or CIDR ranges for the given configuration section.
func validateAllowlist(entries []string, section string) error {
	for _, entry := range entries {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			continue
		}
		if strings.Contains(entry, "/") {
			if _, _, err := net.ParseCIDR(entry); err != nil {
				return fmt.Errorf("invalid %s entry %q: %w", section, entry, err)
			}
			continue
		}
		if net.ParseIP(entry) == nil {
			return fmt.Errorf("invalid %s entry %q: not an IP or CIDR", section, entry)
		}
	}
	return nil
}
