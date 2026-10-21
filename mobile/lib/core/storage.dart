import 'package:flutter_secure_storage/flutter_secure_storage.dart';

/// Persists the self-hosted server URL and the credentials used to talk to it.
///
/// The OIDC session is an HttpOnly cookie on the server. Android's WebView
/// exposes it through a native channel, after which it is stored here so every
/// API request can present the same cookie.
class StorageService {
  StorageService([FlutterSecureStorage? storage])
      : _storage = storage ?? const FlutterSecureStorage();

  final FlutterSecureStorage _storage;

  static const _baseUrlKey = 'gitlens_base_url';
  static const _oidcSessionKey = 'gitlens_oidc_session';
  static const _apiTokenKey = 'gitlens_api_token';

  Future<String?> getBaseUrl() => _storage.read(key: _baseUrlKey);

  Future<void> setBaseUrl(String url) =>
      _storage.write(key: _baseUrlKey, value: url);

  Future<String?> getOidcSession() => _storage.read(key: _oidcSessionKey);

  Future<void> setOidcSession(String? value) async {
    if (value == null || value.isEmpty) {
      await _storage.delete(key: _oidcSessionKey);
    } else {
      await _storage.write(key: _oidcSessionKey, value: value);
    }
  }

  Future<String?> getApiToken() => _storage.read(key: _apiTokenKey);

  Future<void> setApiToken(String? value) async {
    if (value == null || value.isEmpty) {
      await _storage.delete(key: _apiTokenKey);
    } else {
      await _storage.write(key: _apiTokenKey, value: value);
    }
  }

  Future<void> clearAuth() async {
    await _storage.delete(key: _oidcSessionKey);
    await _storage.delete(key: _apiTokenKey);
  }
}
