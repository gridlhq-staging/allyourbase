import 'package:test/test.dart';
import 'dart:io';

import 'support/integration_env.dart';

void main() {
  group('IntegrationTestEnv', () {
    test('requires AYB_TEST_URL to enable integration tests', () {
      final env = IntegrationTestEnv.fromEnvironment(const <String, String>{});

      expect(env.isConfigured, isFalse);
      expect(env.baseUrl, isNull);
      expect(env.skipReason, contains('AYB_TEST_URL'));
    });

    test('normalizes AYB_TEST_URL and parses timeout', () {
      final env = IntegrationTestEnv.fromEnvironment(const <String, String>{
        'AYB_TEST_URL': ' https://api.example.com/ ',
        'AYB_TEST_ADMIN_TOKEN': ' test-admin-token ',
        'AYB_TEST_TIMEOUT_SECONDS': '45',
      });

      expect(env.isConfigured, isTrue);
      expect(env.baseUrl, 'https://api.example.com');
      expect(env.adminToken, 'test-admin-token');
      expect(env.timeout, const Duration(seconds: 45));
      expect(env.skipReason, isNull);
    });

    test('uses default timeout when AYB_TEST_TIMEOUT_SECONDS is invalid', () {
      final env = IntegrationTestEnv.fromEnvironment(const <String, String>{
        'AYB_TEST_URL': 'https://api.example.com',
        'AYB_TEST_TIMEOUT_SECONDS': '-1',
      });

      expect(env.timeout, IntegrationTestEnv.defaultTimeout);
    });

    test('creates unique credentials for isolated integration runs', () {
      final env = IntegrationTestEnv.fromEnvironment(const <String, String>{
        'AYB_TEST_URL': 'https://api.example.com',
      });

      final first = env.newCredentials('auth flow');
      final second = env.newCredentials('auth flow');

      expect(first.email, endsWith('@example.test'));
      expect(first.email, contains('auth-flow'));
      expect(first.password, startsWith('P@ssw0rd-'));
      expect(second.email, isNot(equals(first.email)));
      expect(second.password, isNot(equals(first.password)));
    });

    test(
        'falls back to the admin token file when AYB_TEST_ADMIN_TOKEN is unset',
        () async {
      final tempDir = await Directory.systemTemp.createTemp('ayb-dart-token-');
      addTearDown(() async {
        if (await tempDir.exists()) {
          await tempDir.delete(recursive: true);
        }
      });
      final tokenDir = Directory('${tempDir.path}/.ayb');
      await tokenDir.create(recursive: true);
      await File('${tokenDir.path}/admin-token')
          .writeAsString(' file-token \n');

      final env = IntegrationTestEnv.fromEnvironment(<String, String>{
        'AYB_TEST_URL': 'https://api.example.com',
        'HOME': tempDir.path,
      });

      expect(env.adminToken, 'file-token');
    });
  });
}
