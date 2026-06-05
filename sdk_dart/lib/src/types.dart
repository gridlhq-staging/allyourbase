typedef JsonMap = Map<String, Object?>;

/// Query parameters for listing records with pagination, sorting, and filtering.
class ListParams {
  const ListParams({
    this.page,
    this.perPage,
    this.sort,
    this.filter,
    this.search,
    this.fields,
    this.expand,
    this.skipTotal,
    this.fuzzy,
    this.typoThreshold,
    this.highlight,
    this.facets,
    this.semantic,
    this.semanticQuery,
  });

  final int? page;
  final int? perPage;
  final String? sort;
  final String? filter;
  final String? search;
  final String? fields;
  final String? expand;
  final bool? skipTotal;
  final bool? fuzzy;
  final num? typoThreshold;
  final bool? highlight;
  final List<String>? facets;
  final bool? semantic;
  final String? semanticQuery;

  /// Converts non-null parameters to a string map suitable for URI query params.
  Map<String, String> toQueryMap() {
    final map = <String, String>{};
    if (page != null) map['page'] = page.toString();
    if (perPage != null) map['perPage'] = perPage.toString();
    if (sort != null) map['sort'] = sort!;
    if (filter != null) map['filter'] = filter!;
    if (search != null) map['search'] = search!;
    if (fields != null) map['fields'] = fields!;
    if (expand != null) map['expand'] = expand!;
    if (skipTotal == true) map['skipTotal'] = 'true';
    if (fuzzy == true) map['fuzzy'] = 'true';
    if (typoThreshold != null) map['typo_threshold'] = typoThreshold.toString();
    if (highlight == true) map['highlight'] = 'true';
    if (facets != null && facets!.isNotEmpty) map['facets'] = facets!.join(',');
    if (semantic == true) map['semantic'] = 'true';
    if (semanticQuery != null) map['semantic_query'] = semanticQuery!;
    return map;
  }
}

/// Query parameters for fetching a single record.
class GetParams {
  const GetParams({
    this.fields,
    this.expand,
  });

  final String? fields;
  final String? expand;

  /// Converts non-null parameters to a string map suitable for URI query params.
  Map<String, String> toQueryMap() {
    final map = <String, String>{};
    if (fields != null) map['fields'] = fields!;
    if (expand != null) map['expand'] = expand!;
    return map;
  }
}

class AuthResponse {
  const AuthResponse({
    required this.token,
    required this.refreshToken,
    required this.user,
  });

  final String token;
  final String refreshToken;
  final User user;

  factory AuthResponse.fromJson(JsonMap json) {
    return AuthResponse(
      token: _requireString(json, 'token'),
      refreshToken: _requireString(json, 'refreshToken'),
      user: User.fromJson(_requireJsonMap(json, 'user')),
    );
  }

  JsonMap toJson() {
    return {
      'token': token,
      'refreshToken': refreshToken,
      'user': user.toJson(),
    };
  }
}

class MagicLinkRequestResponse {
  const MagicLinkRequestResponse({
    required this.message,
  });

  final String message;

  factory MagicLinkRequestResponse.fromJson(JsonMap json) {
    return MagicLinkRequestResponse(
      message: _requireString(json, 'message'),
    );
  }
}

class MagicLinkConfirmResponse {
  const MagicLinkConfirmResponse._({
    this.auth,
    this.mfaToken,
  });

  final AuthResponse? auth;
  final String? mfaToken;

  bool get isPendingMFA => auth == null && mfaToken != null;

  factory MagicLinkConfirmResponse.authenticated(AuthResponse auth) {
    return MagicLinkConfirmResponse._(auth: auth);
  }

  factory MagicLinkConfirmResponse.pending(String mfaToken) {
    return MagicLinkConfirmResponse._(mfaToken: mfaToken);
  }

  factory MagicLinkConfirmResponse.fromJson(JsonMap json) {
    final mfaPending = _optionalBool(json, 'mfaPending') ??
        _optionalBool(json, 'mfa_pending') ??
        false;
    if (mfaPending) {
      return MagicLinkConfirmResponse.pending(
        _optionalString(json, 'mfaToken') ?? _requireString(json, 'mfa_token'),
      );
    }
    return MagicLinkConfirmResponse.authenticated(
      AuthResponse.fromJson(json),
    );
  }
}

class User {
  const User({
    required this.id,
    required this.email,
    this.isAnonymous,
    this.linkedAt,
    this.emailVerified,
    this.createdAt,
    this.updatedAt,
  });

  final String id;
  final String email;
  final bool? isAnonymous;
  final String? linkedAt;
  final bool? emailVerified;
  final String? createdAt;
  final String? updatedAt;

  factory User.fromJson(JsonMap json) {
    return User(
      id: _requireString(json, 'id'),
      email: _requireString(json, 'email'),
      isAnonymous: _optionalBool(json, 'isAnonymous') ??
          _optionalBool(json, 'is_anonymous'),
      linkedAt: _optionalString(json, 'linkedAt') ??
          _optionalString(json, 'linked_at'),
      emailVerified: _optionalBool(json, 'emailVerified') ??
          _optionalBool(json, 'email_verified'),
      createdAt: _optionalString(json, 'createdAt') ??
          _optionalString(json, 'created_at'),
      updatedAt: _optionalString(json, 'updatedAt') ??
          _optionalString(json, 'updated_at'),
    );
  }

  JsonMap toJson() {
    final json = <String, Object?>{
      'id': id,
      'email': email,
    };
    if (isAnonymous != null) json['isAnonymous'] = isAnonymous;
    if (linkedAt != null) json['linkedAt'] = linkedAt;
    if (emailVerified != null) json['emailVerified'] = emailVerified;
    if (createdAt != null) json['createdAt'] = createdAt;
    if (updatedAt != null) json['updatedAt'] = updatedAt;
    return json;
  }
}

class ListResponse<T> {
  const ListResponse({
    required this.items,
    required this.page,
    required this.perPage,
    required this.totalItems,
    required this.totalPages,
    this.facets,
  });

  final List<T> items;
  final int page;
  final int perPage;
  final int totalItems;
  final int totalPages;
  final JsonMap? facets;

  factory ListResponse.fromJson(
    JsonMap json, {
    required T Function(Object? value) decodeItem,
  }) {
    final rawItems = _requireList(json, 'items');
    return ListResponse<T>(
      items: rawItems.map(decodeItem).toList(growable: false),
      page: _requireInt(json, 'page'),
      perPage: _requireInt(json, 'perPage'),
      totalItems: _requireInt(json, 'totalItems'),
      totalPages: _requireInt(json, 'totalPages'),
      facets: _optionalJsonMap(json, 'facets'),
    );
  }
}

class RealtimeEvent {
  const RealtimeEvent({
    required this.action,
    required this.table,
    required this.record,
    this.oldRecord,
  });

  final String action;
  final String table;
  final JsonMap record;
  final JsonMap? oldRecord;

  factory RealtimeEvent.fromJson(JsonMap json) {
    return RealtimeEvent(
      action: _requireString(json, 'action'),
      table: _requireString(json, 'table'),
      record: _requireJsonMap(json, 'record'),
      oldRecord: _optionalJsonMap(json, 'oldRecord') ??
          _optionalJsonMap(json, 'old_record'),
    );
  }

  JsonMap toJson() {
    return {
      'action': action,
      'table': table,
      'record': record,
      'oldRecord': oldRecord,
    };
  }
}

class StorageObject {
  const StorageObject({
    required this.id,
    required this.bucket,
    required this.name,
    required this.size,
    required this.contentType,
    this.userId,
    required this.createdAt,
    required this.updatedAt,
  });

  final String id;
  final String bucket;
  final String name;
  final int size;
  final String contentType;
  final String? userId;
  final String createdAt;
  final String? updatedAt;

  factory StorageObject.fromJson(JsonMap json) {
    return StorageObject(
      id: _requireString(json, 'id'),
      bucket: _requireString(json, 'bucket'),
      name: _requireString(json, 'name'),
      size: _requireInt(json, 'size'),
      contentType: _optionalString(json, 'contentType') ??
          _requireString(json, 'content_type'),
      userId:
          _optionalString(json, 'userId') ?? _optionalString(json, 'user_id'),
      createdAt: _optionalString(json, 'createdAt') ??
          _requireString(json, 'created_at'),
      updatedAt: _optionalString(json, 'updatedAt') ??
          _optionalString(json, 'updated_at'),
    );
  }

  JsonMap toJson() {
    return {
      'id': id,
      'bucket': bucket,
      'name': name,
      'size': size,
      'contentType': contentType,
      'userId': userId,
      'createdAt': createdAt,
      'updatedAt': updatedAt,
    };
  }
}

class BatchOperation {
  const BatchOperation({
    required this.method,
    this.id,
    this.body,
  });

  final String method;
  final String? id;
  final JsonMap? body;

  factory BatchOperation.fromJson(JsonMap json) {
    return BatchOperation(
      method: _requireString(json, 'method'),
      id: _optionalString(json, 'id'),
      body: _optionalJsonMap(json, 'body'),
    );
  }

  JsonMap toJson() {
    final json = <String, Object?>{'method': method};
    if (id != null) json['id'] = id;
    if (body != null) json['body'] = body;
    return json;
  }
}

class BatchResult<T> {
  const BatchResult({
    required this.index,
    required this.status,
    this.body,
  });

  final int index;
  final int status;
  final T? body;

  factory BatchResult.fromJson(
    JsonMap json, {
    T Function(Object? value)? decodeBody,
  }) {
    final Object? rawBody = json['body'];
    T? decodedBody;

    if (rawBody != null) {
      if (decodeBody != null) {
        decodedBody = decodeBody(rawBody);
      } else {
        if (rawBody is! T) {
          throw FormatException(
            'BatchResult body has type ${rawBody.runtimeType}; expected $T',
          );
        }
        decodedBody = rawBody as T;
      }
    }

    return BatchResult<T>(
      index: _requireInt(json, 'index'),
      status: _requireInt(json, 'status'),
      body: decodedBody,
    );
  }
}

class DeviceToken {
  const DeviceToken({
    required this.id,
    required this.provider,
    required this.platform,
    required this.token,
    this.deviceName,
    required this.isActive,
    this.lastRefreshedAt,
    required this.createdAt,
  });

  final String id;
  final String provider;
  final String platform;
  final String token;
  final String? deviceName;
  final bool isActive;
  final String? lastRefreshedAt;
  final String createdAt;

  factory DeviceToken.fromJson(JsonMap json) {
    return DeviceToken(
      id: _requireString(json, 'id'),
      provider: _requireString(json, 'provider'),
      platform: _requireString(json, 'platform'),
      token: _requireString(json, 'token'),
      deviceName: _optionalString(json, 'device_name'),
      isActive: _requireBool(json, 'is_active'),
      lastRefreshedAt: _optionalString(json, 'last_refreshed_at'),
      createdAt: _requireString(json, 'created_at'),
    );
  }

  JsonMap toJson() {
    return {
      'id': id,
      'provider': provider,
      'platform': platform,
      'token': token,
      'device_name': deviceName,
      'is_active': isActive,
      'last_refreshed_at': lastRefreshedAt,
      'created_at': createdAt,
    };
  }
}

String _requireString(JsonMap json, String key) {
  final value = json[key];
  if (value is String) {
    return value;
  }
  throw FormatException('Missing or invalid String for key "$key".');
}

String? _optionalString(JsonMap json, String key) {
  final value = json[key];
  if (value == null) {
    return null;
  }
  if (value is String) {
    return value;
  }
  throw FormatException('Invalid String for key "$key".');
}

bool _requireBool(JsonMap json, String key) {
  final value = json[key];
  if (value is bool) {
    return value;
  }
  throw FormatException('Missing or invalid bool for key "$key".');
}

bool? _optionalBool(JsonMap json, String key) {
  final value = json[key];
  if (value == null) {
    return null;
  }
  if (value is bool) {
    return value;
  }
  throw FormatException('Invalid bool for key "$key".');
}

int _requireInt(JsonMap json, String key) {
  final value = json[key];
  if (value is int) {
    return value;
  }
  if (value is num && value.isFinite && value % 1 == 0) {
    return value.toInt();
  }
  throw FormatException('Missing or invalid int for key "$key".');
}

List<Object?> _requireList(JsonMap json, String key) {
  final value = json[key];
  if (value is List<Object?>) {
    return value;
  }
  if (value is List) {
    return value.cast<Object?>();
  }
  throw FormatException('Missing or invalid List for key "$key".');
}

JsonMap _requireJsonMap(JsonMap json, String key) {
  final value = json[key];
  return _asJsonMap(value, key);
}

JsonMap? _optionalJsonMap(JsonMap json, String key) {
  final value = json[key];
  if (value == null) {
    return null;
  }
  return _asJsonMap(value, key);
}

JsonMap _asJsonMap(Object? value, String key) {
  if (value is Map<String, Object?>) {
    return value;
  }
  if (value is Map) {
    final map = <String, Object?>{};
    for (final entry in value.entries) {
      final mapKey = entry.key;
      if (mapKey is! String) {
        throw FormatException(
          'Missing or invalid object for key "$key": non-string map key.',
        );
      }
      map[mapKey] = entry.value;
    }
    return map;
  }
  throw FormatException('Missing or invalid object for key "$key".');
}
