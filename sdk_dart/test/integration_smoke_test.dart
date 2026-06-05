@TestOn('!browser')
library;

import 'package:allyourbase/allyourbase.dart';
import 'package:test/test.dart';

import 'support/integration_env.dart';

final _env = IntegrationTestEnv.fromEnvironment();

void main() {
  test('rejects invalid collection identifiers before issuing admin SQL', () {
    expect(
      () => _validatedCollectionIdentifier('sdk_dart_search_posts;DROP'),
      throwsArgumentError,
    );
    expect(
      () => _validatedCollectionIdentifier('sdk-dart-search-posts'),
      throwsArgumentError,
    );
  });

  group(
      'Integration: auth smoke',
    skip: _env.skipReason,
    () {
      test(
        'registers, refreshes, logs out, logs in, and deletes account',
        () async {
          final baseUrl = _env.baseUrl;
          expect(baseUrl, isNotNull);

          final client = AYBClient(baseUrl!);
          addTearDown(client.close);

          final credentials = _env.newCredentials('auth smoke');
          var accountDeleted = false;

          try {
            final registered = await client.auth.register(
              credentials.email,
              credentials.password,
            );
            expect(registered.token, isNotEmpty);
            expect(registered.refreshToken, isNotEmpty);
            expect(registered.user.email, credentials.email);
            expect(client.token, isNotEmpty);
            expect(client.refreshToken, isNotEmpty);

            final me = await client.auth.me();
            expect(me.email, credentials.email);

            final refreshed = await client.auth.refresh();
            expect(refreshed.token, isNotEmpty);
            expect(refreshed.refreshToken, isNotEmpty);

            await client.auth.logout();
            expect(client.token, isNull);
            expect(client.refreshToken, isNull);

            final loggedIn = await client.auth.login(
              credentials.email,
              credentials.password,
            );
            expect(loggedIn.user.email, credentials.email);
            expect(client.token, isNotEmpty);
            expect(client.refreshToken, isNotEmpty);

            await client.auth.deleteAccount();
            accountDeleted = true;
            expect(client.token, isNull);
            expect(client.refreshToken, isNull);
          } finally {
            if (!accountDeleted) {
              try {
                if (client.token == null) {
                  await client.auth.login(
                    credentials.email,
                    credentials.password,
                  );
                }
                await client.auth.deleteAccount();
              } catch (_) {
                // Best-effort cleanup to avoid orphaned test users.
              }
            }
          }
        },
        timeout: Timeout(_env.timeout),
      );
    },
  );

  group(
    'Integration: records search',
    skip: _env.adminSkipReason,
    () {
      const collection = 'sdk_dart_search_posts';
      late AYBClient client;

      setUpAll(() async {
        final baseUrl = _env.baseUrl;
        expect(baseUrl, isNotNull);

        final setupClient = AYBClient(baseUrl!);
        setupClient.setApiKey(_env.adminToken!);
        try {
          await _prepareSearchFixtures(setupClient, collection);
        } finally {
          setupClient.close();
        }
      });

      tearDownAll(() async {
        final baseUrl = _env.baseUrl;
        if (baseUrl == null) {
          return;
        }

        final cleanupClient = AYBClient(baseUrl);
        cleanupClient.setApiKey(_env.adminToken!);
        try {
          await _dropSearchFixtures(cleanupClient, collection);
        } finally {
          cleanupClient.close();
        }
      });

      setUp(() {
        final baseUrl = _env.baseUrl;
        expect(baseUrl, isNotNull);
        client = AYBClient(baseUrl!);
        client.setApiKey(_env.adminToken!);
      });

      tearDown(() {
        client.close();
      });

      test(
        'returns highlighted search snippets',
        () async {
          final result = await client.records.list(
            collection,
            params: const ListParams(
              search: 'allyourbase',
              highlight: true,
            ),
          );

          final highlights = result.items
              .map((item) => item['_highlight'])
              .whereType<String>()
              .toList(growable: false);
          expect(highlights, isNotEmpty);
          expect(
            highlights
                .any((highlight) => highlight.contains('<b>allyourbase</b>')),
            isTrue,
          );
        },
        timeout: Timeout(_env.timeout),
      );

      test(
        'returns fuzzy typo matches with exact facet counts',
        () async {
          final result = await client.records.list(
            collection,
            params: const ListParams(
              search: 'alyourbase',
              fuzzy: true,
              typoThreshold: 0.2,
              facets: ['category'],
            ),
          );

          expect(
            result.items.map((item) => item['id']).toSet(),
            containsAll({'one', 'two'}),
          );
          expect(_categoryFacetCounts(result), {'docs': 2});
        },
        timeout: Timeout(_env.timeout),
      );
    },
  );
}

Map<String, int> _categoryFacetCounts(ListResponse<JsonMap> result) {
  final categoryBuckets = result.facets?['category'];
  expect(categoryBuckets, isA<List<Object?>>());

  return {
    for (final bucket in categoryBuckets! as List<Object?>)
      if (bucket
          case {'value': final Object? value, 'count': final Object? count})
        value.toString(): count as int,
  };
}

Future<void> _prepareSearchFixtures(AYBClient client, String collection) async {
  final safeCollection = _validatedCollectionIdentifier(collection);
  await _adminSql(client, 'DROP TABLE IF EXISTS $safeCollection CASCADE');
  await _adminSql(
      client,
      '''
  CREATE TABLE $safeCollection (
    id text PRIMARY KEY,
    title text NOT NULL,
    category text NOT NULL
  )
  ''',
  );
  await _adminSql(
    client,
    'ALTER TABLE $safeCollection ENABLE ROW LEVEL SECURITY',
  );
  await _adminSql(
      client,
      'CREATE POLICY ${safeCollection}_all ON $safeCollection FOR ALL USING (true) WITH CHECK (true)',
  );
  await _adminSql(
      client,
      """
  INSERT INTO $safeCollection (id, title, category) VALUES
    ('one', 'allyourbase migration guide', 'docs'),
    ('two', 'allyourbase search cookbook', 'docs'),
    ('three', 'postgres indexing handbook', 'guides')
  """,
  );
  await _waitForCollection(client, safeCollection);
}

Future<void> _dropSearchFixtures(AYBClient client, String collection) {
  final safeCollection = _validatedCollectionIdentifier(collection);
  return _adminSql(client, 'DROP TABLE IF EXISTS $safeCollection CASCADE');
}

Future<JsonMap> _adminSql(AYBClient client, String query) {
  return client.request<JsonMap>(
    '/api/admin/sql',
    method: 'POST',
    body: <String, Object?>{'query': query},
    decode: (value) {
      if (value is Map<String, Object?>) {
        return value;
      }
      if (value is Map) {
        return value.cast<String, Object?>();
      }
      throw StateError('Expected JSON object from admin SQL endpoint.');
    },
  );
}

Future<void> _waitForCollection(AYBClient client, String collection) async {
  final deadline = DateTime.now().add(_env.timeout);
  while (DateTime.now().isBefore(deadline)) {
    try {
      await client.records.list(collection);
      return;
    } on AYBError catch (error) {
      if (error.status == 404 &&
          error.message == 'collection not found: $collection') {
        await Future<void>.delayed(const Duration(milliseconds: 250));
        continue;
      }
      rethrow;
    }
  }
  throw StateError(
      'Timed out waiting for collection $collection to become queryable.');
}

String _validatedCollectionIdentifier(String collection) {
  final normalized = collection.trim();
  if (!RegExp(r'^[A-Za-z0-9_]+$').hasMatch(normalized)) {
    throw ArgumentError.value(
      collection,
      'collection',
      'must contain only letters, numbers, and underscores.',
    );
  }
  return normalized;
}
