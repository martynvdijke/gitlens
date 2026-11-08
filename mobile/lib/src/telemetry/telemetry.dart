import 'dart:async';
import 'dart:convert';
import 'dart:math';

import 'package:dio/dio.dart';
import 'package:flutter/foundation.dart';

/// Configuration is compile-time so secrets never live in app storage.
abstract final class TelemetryConfig {
  static const otelEndpoint = String.fromEnvironment(
    'OTEL_EXPORTER_OTLP_ENDPOINT',
  );
  static const otelHeaders = String.fromEnvironment(
    'OTEL_EXPORTER_OTLP_HEADERS',
  );
  static const serviceName = String.fromEnvironment(
    'OTEL_SERVICE_NAME',
    defaultValue: 'gitlens-android',
  );
  static const serviceVersion = String.fromEnvironment(
    'OTEL_SERVICE_VERSION',
    defaultValue: '1.0.0',
  );
  static const umamiUrl = String.fromEnvironment('UMAMI_URL');
  static const umamiWebsiteId = String.fromEnvironment('UMAMI_WEBSITE_ID');

  static bool get otelEnabled => otelEndpoint.trim().isNotEmpty && !kIsWeb;
  static bool get umamiEnabled =>
      umamiUrl.trim().isNotEmpty && umamiWebsiteId.trim().isNotEmpty;
}

/// Server-provided OTEL settings. Parsing is deliberately tolerant: the public
/// API may return either the mobile-shaped keys or the admin `/api/settings`
/// keys, and booleans may arrive as JSON bools, numbers, or strings.
class OtelSettings {
  const OtelSettings({
    this.endpoint = '',
    this.enabled = false,
    this.tracesEnabled = false,
    this.metricsEnabled = false,
    this.logsEnabled = false,
  });

  final String endpoint;
  final bool enabled;
  final bool tracesEnabled;
  final bool metricsEnabled;
  final bool logsEnabled;

  factory OtelSettings.fromJson(Map<String, dynamic> json) {
    final endpoint = _asString(json['endpoint'] ?? json['otel_endpoint']);
    final enabled = _asBool(json['enabled'] ?? json['otel_enabled']);
    final traces = json.containsKey('traces_enabled')
        ? _asBool(json['traces_enabled'])
        : enabled;
    final metrics = _asBool(json['metrics_enabled']);
    final logs = _asBool(json['logs_enabled']);
    final usable = endpoint.isNotEmpty;
    return OtelSettings(
      endpoint: endpoint,
      enabled: enabled && usable,
      tracesEnabled: traces && usable,
      metricsEnabled: metrics && usable,
      logsEnabled: logs && usable,
    );
  }

  static String _asString(Object? value) {
    if (value is String) return value.trim();
    if (value == null) return '';
    return value.toString().trim();
  }

  static bool _asBool(Object? value) {
    if (value is bool) return value;
    if (value is num) return value != 0;
    if (value is String) {
      final normalized = value.trim().toLowerCase();
      return normalized == 'true' || normalized == '1' || normalized == 'yes';
    }
    return false;
  }
}

/// Fetches the public OTEL bootstrap settings from [baseUrl].
///
/// Never throws: an offline device, timeout, non-200 response, or malformed
/// body all resolve to `null`.
Future<OtelSettings?> fetchOtelSettings(
  String baseUrl, {
  Dio? dio,
  Duration timeout = const Duration(seconds: 3),
}) async {
  final base = baseUrl.trim().replaceAll(RegExp(r'/+$'), '');
  if (base.isEmpty) return null;

  final client = dio ?? Dio();
  final ownsClient = dio == null;
  try {
    final response = await client
        .get<dynamic>(
          '$base/api/otel-settings',
          options: Options(
            responseType: ResponseType.json,
            receiveTimeout: timeout,
            sendTimeout: timeout,
          ),
        )
        .timeout(timeout);
    if (response.statusCode != 200) return null;
    final data = response.data;
    if (data is Map) return OtelSettings.fromJson(Map<String, dynamic>.from(data));
    if (data is String && data.isNotEmpty) {
      final decoded = jsonDecode(data);
      if (decoded is Map) return OtelSettings.fromJson(Map<String, dynamic>.from(decoded));
    }
    return null;
  } catch (_) {
    return null;
  } finally {
    if (ownsClient) client.close(force: true);
  }
}

/// Allowlisted analytics events. Keys and values are enforced at the boundary,
/// so repository names, PR titles, usernames, and searches can never be sent.
const Map<String, Set<String>> umamiEventAllowlist = {
  'screen_view': {'screen'},
  'sync_started': {},
  'theme_changed': {'theme'},
};

const Set<String> _screens = {
  'repos',
  'prs',
  'feed',
  'metrics',
  'settings',
  'onboarding',
};
const Set<String> _themes = {'system', 'light', 'dark'};

Map<String, Object>? sanitizeUmamiEvent(
  String event, [
  Map<String, Object?> props = const {},
]) {
  final allowed = umamiEventAllowlist[event];
  if (allowed == null) return null;
  final clean = <String, Object>{};
  for (final entry in props.entries) {
    if (!allowed.contains(entry.key)) continue;
    final value = entry.value;
    if (value is bool || value is num) {
      clean[entry.key] = value as Object;
      continue;
    }
    if (value is! String || value.length > 64) continue;
    if (entry.key == 'screen' && !_screens.contains(value)) continue;
    if (entry.key == 'theme' && !_themes.contains(value)) continue;
    clean[entry.key] = value;
  }
  if (allowed.isNotEmpty && clean.isEmpty) return null;
  return clean;
}

String randomHex(int bytes) {
  final random = Random.secure();
  final buffer = StringBuffer();
  for (var i = 0; i < bytes; i++) {
    buffer.write(random.nextInt(256).toRadixString(16).padLeft(2, '0'));
  }
  return buffer.toString();
}

String buildTraceparent(String traceId, String spanId) =>
    '00-$traceId-$spanId-01';

class TraceSpan {
  TraceSpan({
    required this.name,
    required this.attributes,
    required this.traceId,
    required this.spanId,
    required this.startNanos,
    this.kind = 1,
  });

  final String name;
  final Map<String, Object> attributes;
  final String traceId;
  final String spanId;
  final int startNanos;
  final int kind;
  bool _ended = false;
  int? statusCode;
  String error = '';

  String get traceparent => buildTraceparent(traceId, spanId);

  void end({
    int? httpStatus,
    Object? failure,
    Map<String, Object> extra = const {},
  }) {
    if (_ended) return;
    _ended = true;
    statusCode = httpStatus;
    if (failure != null) error = failure.toString();
    final attrs = <String, Object>{...attributes, ...extra};
    if (httpStatus != null) attrs['http.response.status_code'] = httpStatus;
    if (error.isNotEmpty) attrs['error.message'] = _short(error);
    Telemetry.instance._finish(
      this,
      attrs,
      error.isNotEmpty || (httpStatus != null && httpStatus >= 500),
    );
  }

  String _short(String value) =>
      value.length > 200 ? '${value.substring(0, 200)}…' : value;
}

Map<String, Object?> buildOtlpPayload({
  required String serviceName,
  required String serviceVersion,
  required TraceSpan span,
  required Map<String, Object> attributes,
  required bool failed,
  required int endNanos,
}) {
  List<Map<String, Object?>> attrs(Map<String, Object> values) => values.entries
      .map(
        (entry) => {
          'key': entry.key,
          'value': switch (entry.value) {
            final bool value => {'boolValue': value},
            final int value => {'intValue': value.toString()},
            final double value => {'doubleValue': value},
            _ => {'stringValue': entry.value.toString()},
          },
        },
      )
      .toList();

  return {
    'resourceSpans': [
      {
        'resource': {
          'attributes': attrs({
            'service.name': serviceName,
            'service.version': serviceVersion,
            'telemetry.sdk.name': 'gitlens-mobile',
            'telemetry.sdk.language': 'dart',
          }),
        },
        'scopeSpans': [
          {
            'scope': {'name': 'gitlens.mobile', 'version': serviceVersion},
            'spans': [
              {
                'traceId': span.traceId,
                'spanId': span.spanId,
                'name': span.name,
                'kind': span.kind,
                'startTimeUnixNano': span.startNanos.toString(),
                'endTimeUnixNano': endNanos.toString(),
                'attributes': attrs(attributes),
                'status': {'code': failed ? 2 : 1},
              },
            ],
          },
        ],
      },
    ],
  };
}

class Telemetry {
  Telemetry._();

  static final Telemetry instance = Telemetry._();

  Dio? _dio;
  Timer? _timer;
  final List<Map<String, Object?>> _queue = [];
  bool _started = false;
  String _tracesEndpoint = '';
  Map<String, String> _headers = const {};

  bool get otelEnabled => _tracesEndpoint.isNotEmpty;
  bool get umamiEnabled => TelemetryConfig.umamiEnabled;

  /// Starts the exporter. When [endpoint] is omitted the compile-time
  /// `OTEL_EXPORTER_OTLP_ENDPOINT` fallback is used. Calling this with no
  /// usable endpoint leaves telemetry disabled.
  Future<void> start({String? endpoint, Map<String, String>? headers}) async {
    if (_started) return;
    _started = true;

    final raw = (endpoint ?? TelemetryConfig.otelEndpoint).trim();
    if (raw.isEmpty || kIsWeb) return;

    _dio = Dio();
    _tracesEndpoint = normalizeEndpoint(raw);
    _headers = headers ?? parseOtelHeaders(TelemetryConfig.otelHeaders);
    _timer = Timer.periodic(const Duration(seconds: 5), (_) => _flush());

    final span = beginSpan('app.start');
    span.end();
  }

  /// Resolves settings from the server first, then falls back to the
  /// compile-time endpoint. Never throws and never blocks on the network
  /// beyond [fetchOtelSettings]'s timeout.
  Future<void> startFromServer(String baseUrl, {Dio? dio}) async {
    if (_started) return;

    final settings = await fetchOtelSettings(baseUrl, dio: dio);
    if (settings != null &&
        settings.enabled &&
        settings.tracesEnabled &&
        settings.endpoint.isNotEmpty) {
      await start(endpoint: settings.endpoint);
      return;
    }
    if (TelemetryConfig.otelEnabled) {
      await start();
      return;
    }
    _started = true;
  }

  TraceSpan beginSpan(
    String name, {
    Map<String, Object> attributes = const {},
    int kind = 1,
  }) {
    return TraceSpan(
      name: name,
      attributes: attributes,
      traceId: randomHex(16),
      spanId: randomHex(8),
      startNanos: DateTime.now().microsecondsSinceEpoch * 1000,
      kind: kind,
    );
  }

  void screen(String name) {
    final span = beginSpan('screen $name', attributes: {'screen.name': name});
    span.end();
    track('screen_view', {'screen': name});
  }

  void track(String event, [Map<String, Object?> props = const {}]) {
    final clean = sanitizeUmamiEvent(event, props);
    if (clean == null || !TelemetryConfig.umamiEnabled) return;
    final client = _dio ??= Dio();
    final base = TelemetryConfig.umamiUrl.replaceAll(RegExp(r'/+$'), '');
    final uri = Uri.tryParse('$base/api/send');
    if (uri == null) return;
    final body = {
      'type': 'event',
      'payload': {
        'website': TelemetryConfig.umamiWebsiteId,
        'hostname': 'gitlens.app',
        'url': '/app/$event',
        'event': event,
        'data': clean,
      },
    };
    unawaited(
      client
          .post<dynamic>(
            uri.toString(),
            data: jsonEncode(body),
            options: Options(contentType: Headers.jsonContentType),
          )
          .then((_) {}, onError: (Object _) {}),
    );
  }

  void _finish(TraceSpan span, Map<String, Object> attributes, bool failed) {
    if (!otelEnabled) return;
    final payload = buildOtlpPayload(
      serviceName: TelemetryConfig.serviceName,
      serviceVersion: TelemetryConfig.serviceVersion,
      span: span,
      attributes: attributes,
      failed: failed,
      endNanos: DateTime.now().microsecondsSinceEpoch * 1000,
    );
    _queue.add(payload);
    if (_queue.length >= 20) _flush();
  }

  void _flush() {
    if (_queue.isEmpty || _dio == null || _tracesEndpoint.isEmpty) return;
    final spans = <Map<String, Object?>>[];
    for (final payload in _queue) {
      final resourceSpans = payload['resourceSpans'];
      if (resourceSpans is List && resourceSpans.isNotEmpty) {
        final resource = resourceSpans.first;
        if (resource is Map) {
          final scopeSpans = resource['scopeSpans'];
          if (scopeSpans is List && scopeSpans.isNotEmpty) {
            final scope = scopeSpans.first;
            if (scope is Map && scope['spans'] is List) {
              spans.addAll(
                (scope['spans'] as List).cast<Map<String, Object?>>(),
              );
            }
          }
        }
      }
    }
    _queue.clear();
    if (spans.isEmpty) return;
    final payload = {
      'resourceSpans': [
        {
          'resource': {
            'attributes': [
              {
                'key': 'service.name',
                'value': {'stringValue': TelemetryConfig.serviceName},
              },
              {
                'key': 'service.version',
                'value': {'stringValue': TelemetryConfig.serviceVersion},
              },
            ],
          },
          'scopeSpans': [
            {
              'scope': {'name': 'gitlens.mobile'},
              'spans': spans,
            },
          ],
        },
      ],
    };
    unawaited(
      _dio!
          .post<dynamic>(
        _tracesEndpoint,
        data: jsonEncode(payload),
        options: Options(
          contentType: Headers.jsonContentType,
          headers: _headers,
        ),
      )
          .then(
        (response) {
          final status = response.statusCode ?? 0;
          if (status >= 400 && kDebugMode) {
            debugPrint('OTLP export failed: $status');
          }
        },
        onError: (Object error) {
          if (kDebugMode) debugPrint('OTLP export failed: $error');
        },
      ),
    );
  }

  Future<void> shutdown() async {
    _timer?.cancel();
    _flush();
    await Future<void>.delayed(const Duration(milliseconds: 200));
    _dio?.close(force: true);
    _dio = null;
  }

  static String normalizeEndpoint(String raw) {
    final base = raw.trim().replaceAll(RegExp(r'/+$'), '');
    if (base.isEmpty) return '';
    if (base.endsWith('/v1/traces')) return base;
    return '$base/v1/traces';
  }

  static Map<String, String> parseOtelHeaders(String raw) {
    final out = <String, String>{};
    for (final pair in raw.split(',')) {
      final idx = pair.indexOf('=');
      if (idx <= 0) continue;
      final key = pair.substring(0, idx).trim();
      var value = pair.substring(idx + 1).trim();
      try {
        value = Uri.decodeComponent(value);
      } catch (_) {}
      if (key.isNotEmpty && value.isNotEmpty) out[key] = value;
    }
    return out;
  }
}
