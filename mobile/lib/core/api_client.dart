import 'dart:convert';

import 'package:dio/dio.dart';

import 'models.dart';
import 'storage.dart';

class ApiException implements Exception {
  ApiException(this.message, {this.statusCode});

  final String message;
  final int? statusCode;

  @override
  String toString() => message;
}

/// JSON client for the GitLens server.
///
/// Sends `Accept: application/json` so the Go handlers return data instead of
/// HTMX partials, and authenticates with either the captured OIDC session
/// cookie or a personal API token. `Origin` is included for cookie-authenticated
/// mutations because the server rejects cross-site requests without it.
class ApiClient {
  ApiClient({required String baseUrl, required this.storage})
      : _baseUrl = baseUrl,
        _dio = Dio(BaseOptions(
          baseUrl: baseUrl,
          connectTimeout: const Duration(seconds: 12),
          receiveTimeout: const Duration(seconds: 25),
          sendTimeout: const Duration(seconds: 15),
          headers: const {'Accept': 'application/json'},
          validateStatus: (_) => true,
        ));

  final String _baseUrl;
  final StorageService storage;
  final Dio _dio;

  String get baseUrl => _baseUrl;

  Future<Options> _options({bool form = false}) async {
    final session = await storage.getOidcSession();
    final token = await storage.getApiToken();
    final headers = <String, String>{
      'Accept': 'application/json',
      'Origin': _baseUrl,
    };
    if (session != null && session.isNotEmpty) {
      headers['Cookie'] = 'gitlens_session_oidc=$session';
    }
    if (token != null && token.isNotEmpty) {
      headers['Authorization'] = 'Bearer $token';
    }
    return Options(
      headers: headers,
      contentType: form ? Headers.formUrlEncodedContentType : Headers.jsonContentType,
      validateStatus: (_) => true,
    );
  }

  Future<dynamic> _get(String path, {Map<String, dynamic>? query}) async {
    try {
      final response = await _dio.get<dynamic>(path, queryParameters: query, options: await _options());
      return _decode(response);
    } on DioException catch (e) {
      throw ApiException(_networkMessage(e), statusCode: e.response?.statusCode);
    }
  }

  Future<dynamic> _post(String path, {Map<String, dynamic>? body, Map<String, dynamic>? fields, Map<String, dynamic>? query}) async {
    try {
      final data = fields != null
          ? fields.entries.expand((e) {
              if (e.value is List) {
                return (e.value as List).map((v) => '${Uri.encodeQueryComponent(e.key)}=${Uri.encodeQueryComponent('$v')}');
              }
              return ['${Uri.encodeQueryComponent(e.key)}=${Uri.encodeQueryComponent('${e.value}')}'];
            }).join('&')
          : body;
      final response = await _dio.post<dynamic>(
        path,
        data: data,
        queryParameters: query,
        options: await _options(form: fields != null),
      );
      return _decode(response);
    } on DioException catch (e) {
      throw ApiException(_networkMessage(e), statusCode: e.response?.statusCode);
    }
  }

  dynamic _decode(Response<dynamic> response) {
    final code = response.statusCode ?? 0;
    final data = response.data;
    if (code == 401) {
      throw ApiException('Your session has expired. Sign in again.', statusCode: code);
    }
    if (code >= 400) {
      final message = data is Map && data['error'] is String
          ? data['error'] as String
          : 'The server returned HTTP $code.';
      throw ApiException(message, statusCode: code);
    }
    return data;
  }

  String _networkMessage(DioException e) {
    switch (e.type) {
      case DioExceptionType.connectionTimeout:
      case DioExceptionType.sendTimeout:
      case DioExceptionType.receiveTimeout:
        return 'The server took too long to respond.';
      case DioExceptionType.connectionError:
        return 'Could not reach the server. Check the address and your connection.';
      default:
        return 'The request failed. Pull to refresh and try again.';
    }
  }

  static Future<bool> oidcEnabledAt(String baseUrl) async {
    final dio = Dio(BaseOptions(
      baseUrl: baseUrl,
      connectTimeout: const Duration(seconds: 8),
      receiveTimeout: const Duration(seconds: 8),
      headers: const {'Accept': 'application/json'},
      validateStatus: (_) => true,
    ));
    try {
      final response = await dio.get<dynamic>('/api/auth/oidc/status');
      final data = response.data;
      if (response.statusCode == 200 && data is Map) {
        return data['enabled'] == true;
      }
    } catch (_) {
      // Unreachable server or non-JSON response: OIDC isn't usable yet.
    }
    return false;
  }

  static Future<bool> testConnection(String baseUrl) async {
    final dio = Dio(BaseOptions(
      baseUrl: baseUrl,
      connectTimeout: const Duration(seconds: 8),
      receiveTimeout: const Duration(seconds: 8),
      headers: const {'Accept': 'application/json'},
      validateStatus: (_) => true,
    ));
    try {
      final response = await dio.get<dynamic>('/api/auth/oidc/status');
      return response.statusCode != null && response.statusCode! < 500;
    } catch (_) {
      return false;
    }
  }

  Future<AppUser> me() async {
    final data = await _get('/api/user/me');
    return AppUser.fromJson((data as Map)['user'] as Map<String, dynamic>);
  }

  Future<ReposPayload> repos({String? query, String? sort}) async {
    final data = await _get('/repos', query: {
      if (query != null && query.isNotEmpty) 'q': query,
      if (sort != null && sort.isNotEmpty) 'sort': sort,
    });
    return ReposPayload.fromJson(data as Map<String, dynamic>);
  }

  Future<DashboardPayload> dashboard() async {
    final data = await _get('/dashboard');
    return DashboardPayload.fromJson(data as Map<String, dynamic>);
  }

  Future<PullRequestsPayload> pullRequests({String? repo}) async {
    final data = await _get('/prs', query: {if (repo != null && repo.isNotEmpty) 'repo': repo});
    return PullRequestsPayload.fromJson(data as Map<String, dynamic>);
  }

  Future<MetricsPayload> metrics() async {
    final data = await _get('/metrics');
    return MetricsPayload.fromJson(data as Map<String, dynamic>);
  }

  Future<List<MetricsSnapshot>> metricsHistory() async {
    final data = await _get('/metrics/history');
    final raw = (data as Map)['snapshots'];
    if (raw is! List) return const [];
    return raw.whereType<Map>().map((e) => MetricsSnapshot.fromJson(e.cast<String, dynamic>())).toList();
  }

  Future<YearStats?> yearStats({int? year}) async {
    final data = await _get('/year-overview/stats', query: {if (year != null) 'year': '$year'});
    if (data is! Map || data['year'] == null) return null;
    return YearStats.fromJson(data.cast<String, dynamic>());
  }

  Future<FeedPayload> feed({String since = '7d', String types = 'all'}) async {
    final data = await _get('/feed', query: {'since': since, 'types': types});
    return FeedPayload.fromJson(data as Map<String, dynamic>);
  }

  Future<ActionMessage> merge({required int repoId, required int prNumber}) async {
    final data = await _post('/prs/merge', body: {'repo_id': repoId, 'pr_number': prNumber});
    return ActionMessage.fromJson(data as Map<String, dynamic>);
  }

  Future<ActionMessage> close({required int repoId, required int prNumber}) async {
    final data = await _post('/prs/close', body: {'repo_id': repoId, 'pr_number': prNumber});
    return ActionMessage.fromJson(data as Map<String, dynamic>);
  }

  Future<ActionMessage> rerun({required int repoId, required int prNumber}) async {
    final data = await _post('/prs/rerun', body: {'repo_id': repoId, 'pr_number': prNumber});
    return ActionMessage.fromJson(data as Map<String, dynamic>);
  }

  Future<ActionMessage> batchMerge(List<String> ids, {String? repo}) async {
    final data = await _post('/prs/batch-merge', fields: {'pr_ids': ids}, query: {
      if (repo != null && repo.isNotEmpty) 'repo': repo,
    });
    return ActionMessage.fromJson(data as Map<String, dynamic>);
  }

  Future<ActionMessage> batchClose(List<String> ids, {String? repo}) async {
    final data = await _post('/prs/batch-close', fields: {'pr_ids': ids}, query: {
      if (repo != null && repo.isNotEmpty) 'repo': repo,
    });
    return ActionMessage.fromJson(data as Map<String, dynamic>);
  }

  Future<ActionMessage> mergePassing({String? repo}) async {
    final data = await _post('/prs/batch-merge-passing', query: {if (repo != null && repo.isNotEmpty) 'repo': repo});
    return ActionMessage.fromJson(data as Map<String, dynamic>);
  }

  Future<void> syncRepo(int id) async {
    await _post('/repos/$id/sync');
  }

  Future<ActionMessage> renovate(int id) async {
    final data = await _post('/repos/$id/renovate/rebase-all');
    if (data is Map) return ActionMessage.fromJson(data.cast<String, dynamic>());
    return ActionMessage(ok: true, message: '$data');
  }

  Future<({String url, Map<String, String> headers})> websocket() async {
    final session = await storage.getOidcSession();
    final token = await storage.getApiToken();
    final uri = Uri.parse(_baseUrl.replaceFirst(RegExp(r'^http'), 'ws'));
    final headers = <String, String>{
      if (session != null && session.isNotEmpty) 'Cookie': 'gitlens_session_oidc=$session',
      if (token != null && token.isNotEmpty) 'Authorization': 'Bearer $token',
    };
    return (url: uri.resolve('/ws').toString(), headers: headers);
  }

  /// API tokens are created as JSON and the one-time plaintext is shown to the
  /// user by the settings screen.
  Future<String> createApiToken(String name) async {
    final data = await _post('/api/tokens', body: {'name': name});
    return (data as Map)['token'] as String;
  }

  static Map<String, dynamic> decodeJson(String raw) =>
      jsonDecode(raw) as Map<String, dynamic>;
}
