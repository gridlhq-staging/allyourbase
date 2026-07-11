import 'dart:io';

import 'package:test/test.dart';

void main() {
  group('Passkey seam purity', () {
    test('library code does not import Flutter or native passkey packages', () {
      final violations = <String>[];

      for (final file in _dartFilesUnder(Directory('lib'))) {
        final relativePath = file.path;
        final contents = file.readAsStringSync();
        for (final pattern in _forbiddenImportPatterns) {
          if (pattern.hasMatch(contents)) {
            violations.add('$relativePath imports ${pattern.pattern}');
          }
        }
      }

      expect(violations, isEmpty);
    });

    test('pubspec dependencies remain pure Dart', () {
      final dependencies = _pubspecDependencies(File('pubspec.yaml'));
      final forbiddenDependencies = dependencies.intersection(
        _forbiddenDependencyNames,
      );

      expect(forbiddenDependencies, isEmpty);
    });
  });
}

const _forbiddenDependencyNames = <String>{
  'flutter',
  'flutter_web_auth',
  'flutter_web_auth_2',
  'local_auth',
  'passkeys',
  'passkeys_android',
  'passkeys_ios',
  'passkeys_platform_interface',
  'webauthn',
};

final _forbiddenImportPatterns = <RegExp>[
  RegExp(r'''import\s+['"]dart:ui['"]'''),
  RegExp(r'''import\s+['"]package:flutter/'''),
  RegExp(r'''import\s+['"]package:flutter_web_auth/'''),
  RegExp(r'''import\s+['"]package:flutter_web_auth_2/'''),
  RegExp(r'''import\s+['"]package:local_auth/'''),
  RegExp(r'''import\s+['"]package:passkeys(?:_|/|['"])'''),
  RegExp(r'''import\s+['"]package:webauthn/'''),
];

Iterable<File> _dartFilesUnder(Directory directory) sync* {
  for (final entity in directory.listSync(recursive: true)) {
    if (entity is File && entity.path.endsWith('.dart')) {
      yield entity;
    }
  }
}

Set<String> _pubspecDependencies(File pubspec) {
  final dependencies = <String>{};
  var inDependencies = false;

  for (final line in pubspec.readAsLinesSync()) {
    if (line.trim().isEmpty || line.trimLeft().startsWith('#')) {
      continue;
    }
    if (!line.startsWith(' ') && line.endsWith(':')) {
      inDependencies = line.trim() == 'dependencies:';
      continue;
    }
    if (!inDependencies || !line.startsWith('  ') || line.startsWith('    ')) {
      continue;
    }

    final separatorIndex = line.indexOf(':');
    if (separatorIndex > 0) {
      dependencies.add(line.substring(0, separatorIndex).trim());
    }
  }

  return dependencies;
}
