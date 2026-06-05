package config

func defaultServerConfig() ServerConfig {
	return ServerConfig{
		Host:               "127.0.0.1",
		Port:               8090,
		CORSAllowedOrigins: []string{"*"},
		AllowedIPs:         []string{},
		BodyLimit:          "1MB",
		ShutdownTimeout:    10,
	}
}

func defaultDatabaseConfig() DatabaseConfig {
	return DatabaseConfig{
		MaxConns:        25,
		MinConns:        2,
		HealthCheckSecs: 30,
		EmbeddedPort:    15432,
		MigrationsDir:   "./migrations",
	}
}

func defaultManagedPGConfig() ManagedPGConfig {
	return ManagedPGConfig{
		Port:                   15432,
		PGVersion:              "16",
		Extensions:             []string{"pgvector", "pg_trgm", "pg_cron"},
		SharedPreloadLibraries: []string{"pg_stat_statements"},
	}
}

func defaultAdminConfig() AdminConfig {
	return AdminConfig{
		Enabled:        true,
		Path:           "/admin",
		LoginRateLimit: 20,
		AllowedIPs:     []string{},
	}
}

func defaultAuthConfig() AuthConfig {
	return AuthConfig{
		TokenDuration:        900,    // 15 minutes
		RefreshTokenDuration: 604800, // 7 days
		Argon2Memory:         65536,  // 64 MiB in KiB units
		Argon2Time:           3,
		Argon2Threads:        2,
		RateLimit:            10, // requests per minute per IP
		AnonymousRateLimit:   30, // anonymous sign-ins per hour per IP
		RateLimitAuth:        "10/min",
		MinPasswordLength:    8,   // NIST SP 800-63B recommended minimum
		MagicLinkDuration:    600, // 10 minutes
		WebAuthnEnabled:      true,
		SMSProvider:          "log",
		SMSCodeLength:        6,
		SMSCodeExpiry:        300, // 5 minutes
		SMSMaxAttempts:       3,
		SMSDailyLimit:        1000,
		SMSAllowedCountries:  []string{"US", "CA"},
		OAuthProviderMode: OAuthProviderModeConfig{
			AccessTokenDuration:  3600,    // 1 hour
			RefreshTokenDuration: 2592000, // 30 days
			AuthCodeDuration:     600,     // 10 minutes
		},
	}
}

func defaultBillingConfig() BillingConfig {
	return BillingConfig{
		Provider:              "",
		UsageSyncIntervalSecs: 3600,
	}
}

func defaultSupportConfig() SupportConfig {
	return SupportConfig{
		Enabled:            false,
		InboundEmailDomain: "",
		WebhookSecret:      "",
	}
}

func defaultRateLimitConfig() RateLimitConfig {
	return RateLimitConfig{
		API:          "100/min",
		APIAnonymous: "30/min",
	}
}

func defaultAPIConfig() APIConfig {
	return APIConfig{
		ImportMaxSizeMB:  50,
		ImportMaxRows:    100000,
		ExportMaxRows:    1000000,
		AggregateEnabled: true,
		TextSearchConfig: "english",
	}
}

func defaultVaultConfig() VaultConfig {
	return VaultConfig{}
}

func defaultEmailConfig() EmailConfig {
	return EmailConfig{
		Backend:  "log",
		FromName: "Allyourbase",
	}
}

func defaultStorageConfig() StorageConfig {
	return StorageConfig{
		Backend:        "local",
		LocalPath:      "./ayb_storage",
		MaxFileSize:    "10MB",
		DefaultQuotaMB: 100,
		S3Region:       "us-east-1",
		S3UseSSL:       true,
	}
}

func defaultEdgeFunctionsConfig() EdgeFuncConfig {
	return EdgeFuncConfig{
		PoolSize:                 12,
		DefaultTimeoutMs:         5000,
		MaxRequestBodyBytes:      1 << 20,
		FetchDomainAllowlist:     []string{},
		MemoryLimitMB:            128,
		MaxConcurrentInvocations: 50,
		CodeCacheSize:            256,
	}
}

func defaultLoggingConfig() LoggingConfig {
	return LoggingConfig{
		Level:                       "info",
		Format:                      "json",
		RequestLogEnabled:           true,
		RequestLogRetentionDays:     7,
		RequestLogBatchSize:         100,
		RequestLogFlushIntervalSecs: 5,
		RequestLogQueueSize:         10000,
	}
}

func defaultMetricsConfig() MetricsConfig {
	return MetricsConfig{
		Enabled: true,
		Path:    "/metrics",
	}
}

func defaultTelemetryConfig() TelemetryConfig {
	return TelemetryConfig{
		Enabled:     true,
		ServiceName: "ayb",
		SampleRate:  1.0,
	}
}

func defaultJobsConfig() JobsConfig {
	return JobsConfig{
		Enabled:              false,
		WorkerConcurrency:    4,
		PollIntervalMs:       1000,
		LeaseDurationS:       300,
		MaxRetriesDefault:    3,
		SchedulerEnabled:     true,
		SchedulerTickS:       15,
		JobRunsRetentionDays: 90,
	}
}

func defaultStatusConfig() StatusConfig {
	return StatusConfig{
		Enabled:               false,
		CheckIntervalSeconds:  30,
		HistorySize:           1000,
		PublicEndpointEnabled: true,
	}
}

func defaultPushConfig() PushConfig {
	return PushConfig{
		Enabled: false,
		APNS: PushAPNSConfig{
			Environment: "production",
		},
	}
}

func defaultAuditConfig() AuditConfig {
	return AuditConfig{
		Enabled:       false,
		RetentionDays: 90,
	}
}

func defaultAIConfig() AIConfig {
	return AIConfig{
		DefaultProvider: "openai",
		TimeoutSecs:     30,
		MaxRetries:      2,
		Breaker: AIBreakerConfig{
			FailureThreshold:   5,
			OpenSeconds:        30,
			HalfOpenProbeLimit: 1,
		},
		EmbeddingDimensions: map[string]int{},
		Providers:           map[string]ProviderConfig{},
	}
}

func defaultDashboardAIConfig() DashboardAIConfig {
	return DashboardAIConfig{
		Enabled:   false,
		RateLimit: "20/min",
	}
}

func defaultBackupConfig() BackupConfig {
	return BackupConfig{
		Enabled:        false,
		Region:         "us-east-1",
		Prefix:         "backups",
		Schedule:       "0 2 * * *",
		RetentionCount: 7,
		RetentionDays:  30,
		Encryption:     "AES256",
		UseSSL:         true,
		PITR: PITRConfig{
			Enabled:                  false,
			WALRetentionDays:         14,
			BaseBackupRetentionDays:  35,
			ComplianceSnapshotMonths: 12,
			RPOMinutes:               5,
			ShadowMode:               true,
			RetentionSchedule:        "0 4 * * *",
			StorageBudgetBytes:       0,
			VerifySchedule:           "0 */6 * * *",
			BaseBackupSchedule:       "0 3 * * *",
		},
	}
}

func defaultGraphQLConfig() GraphQLConfig {
	return GraphQLConfig{
		MaxDepth:      0,
		MaxComplexity: 0,
		Introspection: "",
	}
}

func defaultRealtimeConfig() RealtimeConfig {
	return RealtimeConfig{
		MaxConnectionsPerUser:       100,
		HeartbeatIntervalSeconds:    25,
		BroadcastRateLimitPerSecond: 100,
		BroadcastMaxMessageBytes:    262144,
		PresenceLeaveTimeoutSeconds: 10,
	}
}
