import 'dart:convert';
import 'dart:typed_data';

import 'package:dio/dio.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:gitlens/src/telemetry/telemetry.dart';

class _FakeAdapter implements HttpClientAdapter {
  _FakeAdapter(this.handler);

  final Future<ResponseBody> Function(RequestOptions options) handler;

  @override
  Future<ResponseBody> fetch(
    RequestOptions options,
    Stream<Uint8List>? requestStream,
    Future<void>? cancelFuture,
  ) =>
      handler(options);

  @override
  void close({bool force = false}) {}
}

ResponseBody _jsonBody(Object data, int statusCode) => ResponseBody.fromString(
      jsonEncode(data),
      statusCode,
      headers: {
        Headers.contentTypeHeader: [Headers.jsonContentType],
      },
    );

void main() {
  group('normalizeEndpoint', () {
    test('appends the OTLP traces path', () {
      expect(
        Telemetry.normalizeEndpoint('http://localhost:4318/'),
        'http://localhost:4318/v1/traces',
      );
    });

    test('keeps an existing traces path', () {
      expect(
        Telemetry.normalizeEndpoint('https://otel.example.com/v1/traces'),
        'https://otel.example.com/v1/traces',
      );
    });

    test('returns empty for an empty endpoint', () {
      expect(Telemetry.normalizeEndpoint('   '), '');
    });
  });

  group('parseOtelHeaders', () {
    test('parses and URL-decodes header pairs', () {
      expect(
        Telemetry.parseOtelHeaders(
          'authorization=Bearer%20abc, x-tenant = foo ,broken,=empty',
        ),
        {'authorization': 'Bearer abc', 'x-tenant': 'foo'},
      );
    });

    test('returns empty map for empty input', () {
      expect(Telemetry.parseOtelHeaders(''), isEmpty);
    });
  });

  group('buildOtlpPayload', () {
    test('builds the resource, scope and span shape', () {
      final span = TraceSpan(
        name: 'http.request',
        attributes: const {},
        traceId: '0123456789abcdef0123456789abcdef',
        spanId: '0123456789abcdef',
        startNanos: 1000,
      );

      final payload = buildOtlpPayload(
        serviceName: 'gitlens-android',
        serviceVersion: '1.2.3',
        span: span,
        attributes: const {'http.request.method': 'GET', 'attempt': 2},
        failed: false,
        endNanos: 2000,
      );

      final resourceSpans = (payload['resourceSpans'] as List).single as Map;
      final resource = resourceSpans['resource'] as Map;
      final resourceAttrs = (resource['attributes'] as List).cast<Map>();
      final resourceValues = {
        for (final attr in resourceAttrs) attr['key'] as String: attr['value'] as Map,
      };
      expect(
        resourceValues['service.name'],
        {'stringValue': 'gitlens-android'},
      );
      expect(
        resourceValues['service.version'],
        {'stringValue': '1.2.3'},
      );
      expect(
        resourceValues['telemetry.sdk.name'],
        {'stringValue': 'gitlens-mobile'},
      );

      final scopeSpans = (resourceSpans['scopeSpans'] as List).single as Map;
      expect((scopeSpans['scope'] as Map)['name'], 'gitlens.mobile');

      final spanJson = (scopeSpans['spans'] as List).single as Map;
      expect(spanJson['traceId'], '0123456789abcdef0123456789abcdef');
      expect(spanJson['spanId'], '0123456789abcdef');
      expect(spanJson['name'], 'http.request');
      expect(spanJson['kind'], 1);
      expect(spanJson['startTimeUnixNano'], '1000');
      expect(spanJson['endTimeUnixNano'], '2000');
      expect((spanJson['status'] as Map)['code'], 1);

      final attrs = (spanJson['attributes'] as List).cast<Map>();
      final values = {
        for (final attr in attrs) attr['key'] as String: attr['value'] as Map,
      };
      expect(values['http.request.method'], {'stringValue': 'GET'});
      expect(values['attempt'], {'intValue': '2'});
    });

    test('marks failed spans with error status', () {
      final span = TraceSpan(
        name: 'http.request',
        attributes: const {},
        traceId: 'trace',
        spanId: 'span',
        startNanos: 1,
      );
      final payload = buildOtlpPayload(
        serviceName: 'gitlens-android',
        serviceVersion: '1.0.0',
        span: span,
        attributes: const {'error.message': 'boom'},
        failed: true,
        endNanos: 2,
      );
      final resourceSpans = (payload['resourceSpans'] as List).single as Map;
      final scopeSpans = (resourceSpans['scopeSpans'] as List).single as Map;
      final spanJson = (scopeSpans['spans'] as List).single as Map;
      expect((spanJson['status'] as Map)['code'], 2);
    });
  });

  group('OtelSettings.fromJson', () {
    test('accepts admin-shaped keys and defaults traces to enabled', () {
      final settings = OtelSettings.fromJson({
        'otel_endpoint': ' http://collector:4318/ ',
        'otel_enabled': 'true',
      });
      expect(settings.endpoint, 'http://collector:4318/');
      expect(settings.enabled, isTrue);
      expect(settings.tracesEnabled, isTrue);
    });

    test('honours an explicit traces_enabled flag', () {
      final settings = OtelSettings.fromJson({
        'endpoint': 'http://collector:4318',
        'enabled': true,
        'traces_enabled': false,
        'metrics_enabled': 1,
        'logs_enabled': 'yes',
      });
      expect(settings.enabled, isTrue);
      expect(settings.tracesEnabled, isFalse);
      expect(settings.metricsEnabled, isTrue);
      expect(settings.logsEnabled, isTrue);
    });

    test('disables everything when the endpoint is empty', () {
      final settings = OtelSettings.fromJson({
        'endpoint': '',
        'enabled': true,
        'traces_enabled': true,
      });
      expect(settings.enabled, isFalse);
      expect(settings.tracesEnabled, isFalse);
    });

    test('tolerates missing keys', () {
      final settings = OtelSettings.fromJson(const {});
      expect(settings.endpoint, '');
      expect(settings.enabled, isFalse);
      expect(settings.tracesEnabled, isFalse);
      expect(settings.metricsEnabled, isFalse);
      expect(settings.logsEnabled, isFalse);
    });
  });

  group('fetchOtelSettings', () {
    test('returns parsed settings on 200', () async {
      final dio = Dio();
      RequestOptions? seen;
      dio.httpClientAdapter = _FakeAdapter((options) async {
        seen = options;
        return _jsonBody({
          'endpoint': 'http://collector:4318',
          'enabled': true,
          'traces_enabled': true,
          'metrics_enabled': false,
          'logs_enabled': false,
        }, 200);
      });

      final settings = await fetchOtelSettings(
        'https://gitlens.example/',
        dio: dio,
      );

      expect(seen, isNotNull);
      expect(seen!.uri.path, '/api/otel-settings');
      expect(settings, isNotNull);
      expect(settings!.endpoint, 'http://collector:4318');
      expect(settings.enabled, isTrue);
      expect(settings.tracesEnabled, isTrue);
    });

    test('returns null on a non-200 response', () async {
      final dio = Dio();
      dio.httpClientAdapter = _FakeAdapter(
        (options) async => _jsonBody({'error': 'nope'}, 500),
      );

      final settings = await fetchOtelSettings(
        'https://gitlens.example',
        dio: dio,
      );

      expect(settings, isNull);
    });

    test('returns null when the network throws', () async {
      final dio = Dio();
      dio.httpClientAdapter = _FakeAdapter(
        (options) async => throw DioException(
          requestOptions: options,
          message: 'offline',
        ),
      );

      final settings = await fetchOtelSettings(
        'https://gitlens.example',
        dio: dio,
      );

      expect(settings, isNull);
    });

    test('returns null for an empty base URL without a request', () async {
      final dio = Dio();
      var called = false;
      dio.httpClientAdapter = _FakeAdapter((options) async {
        called = true;
        return _jsonBody({}, 200);
      });

      final settings = await fetchOtelSettings('', dio: dio);

      expect(settings, isNull);
      expect(called, isFalse);
    });
  });
}
