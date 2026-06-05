import 'dart:io';

class IntegrationCredentials {
  const IntegrationCredentials({
    required this.email,
    required this.password,
  });

  final String email;
  final String password;
}

class IntegrationTestEnv {
  IntegrationTestEnv._({
    required this.baseUrl,
    required this.adminToken,
    required this.timeout,
  });

  static const Duration defaultTimeout = Duration(seconds: 30);

  final String? baseUrl;
  final String? adminToken;
  final Duration timeout;
  int _credentialCounter = 0;

  bool get isConfigured => baseUrl != null;

  String? get skipReason {
    if (isConfigured) {
      return null;
    }
    return 'Set AYB_TEST_URL to run integration tests.';
  }

  String? get adminSkipReason {
    if (!isConfigured) {
      return 'Set AYB_TEST_URL to run integration tests.';
    }
    if (adminToken == null) {
      return 'Set AYB_TEST_ADMIN_TOKEN/AYB_ADMIN_TOKEN or provide ~/.ayb/admin-token to run admin-authenticated tests.';
    }
    return null;
  }

  static IntegrationTestEnv fromEnvironment(
      [Map<String, String>? environment]) {
    final env = environment ?? Platform.environment;
    final rawBaseUrl = env['AYB_TEST_URL']?.trim();
    final normalizedBaseUrl = _normalizeUrl(rawBaseUrl);
    final adminToken = _resolveAdminToken(env);
    final timeout = _parseTimeout(env['AYB_TEST_TIMEOUT_SECONDS']);
    return IntegrationTestEnv._(
      baseUrl: normalizedBaseUrl,
      adminToken: adminToken,
      timeout: timeout,
    );
  }

  IntegrationCredentials newCredentials(String scenario) {
    _credentialCounter += 1;
    final scenarioSlug = _slugifyScenario(scenario);
    final timestamp = DateTime.now().microsecondsSinceEpoch.toRadixString(36);
    final suffix = '$timestamp-${_credentialCounter.toRadixString(36)}';
    return IntegrationCredentials(
      email: 'sdk-dart-$scenarioSlug-$suffix@example.test',
      password: 'P@ssw0rd-$suffix',
    );
  }

  static String? _normalizeUrl(String? rawBaseUrl) {
    final normalizedRawBaseUrl = _blankToNull(rawBaseUrl);
    if (normalizedRawBaseUrl == null) {
      return null;
    }

    var normalized = normalizedRawBaseUrl;
    while (normalized.endsWith('/')) {
      normalized = normalized.substring(0, normalized.length - 1);
    }
    return normalized.isEmpty ? null : normalized;
  }

  static String? _blankToNull(String? value) {
    if (value == null || value.isEmpty) {
      return null;
    }
    return value;
  }

  static String? _resolveAdminToken(Map<String, String> env) {
    final explicitToken = _blankToNull(env['AYB_TEST_ADMIN_TOKEN']?.trim()) ??
        _blankToNull(env['AYB_ADMIN_TOKEN']?.trim());
    if (explicitToken != null) {
      return explicitToken;
    }

    final explicitPath = _blankToNull(env['AYB_ADMIN_TOKEN_PATH']?.trim());
    final homeDir = _blankToNull(env['HOME']?.trim());
    final tokenPath =
        explicitPath ?? (homeDir == null ? null : '$homeDir/.ayb/admin-token');
    if (tokenPath == null) {
      return null;
    }

    try {
      final token = File(tokenPath).readAsStringSync().trim();
      return _blankToNull(token);
    } on FileSystemException {
      return null;
    }
  }

  static Duration _parseTimeout(String? rawTimeoutSeconds) {
    final seconds = int.tryParse(rawTimeoutSeconds ?? '');
    if (seconds == null || seconds <= 0) {
      return defaultTimeout;
    }
    return Duration(seconds: seconds);
  }

  static String _slugifyScenario(String scenario) {
    final buffer = StringBuffer();
    var previousWasHyphen = false;

    for (final rune in scenario.runes) {
      final char = String.fromCharCode(rune).toLowerCase();
      final isAlphaNum =
          (char.codeUnitAt(0) >= 48 && char.codeUnitAt(0) <= 57) ||
              (char.codeUnitAt(0) >= 97 && char.codeUnitAt(0) <= 122);

      if (isAlphaNum) {
        buffer.write(char);
        previousWasHyphen = false;
        continue;
      }

      if (!previousWasHyphen) {
        buffer.write('-');
        previousWasHyphen = true;
      }
    }

    final slug = buffer.toString().replaceAll(RegExp(r'^-+|-+$'), '');
    return slug.isEmpty ? 'scenario' : slug;
  }
}
