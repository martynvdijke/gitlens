import 'package:flutter_riverpod/flutter_riverpod.dart';

import 'api_client.dart';
import 'models.dart';
import 'storage.dart';

class AuthState {
  const AuthState({
    this.loading = true,
    this.user,
    this.baseUrl,
    this.error,
  });

  final bool loading;
  final AppUser? user;
  final String? baseUrl;
  final String? error;

  bool get signedIn => user != null;

  AuthState copyWith({
    bool? loading,
    AppUser? user,
    bool clearUser = false,
    String? baseUrl,
    String? error,
    bool clearError = false,
  }) =>
      AuthState(
        loading: loading ?? this.loading,
        user: clearUser ? null : (user ?? this.user),
        baseUrl: baseUrl ?? this.baseUrl,
        error: clearError ? null : (error ?? this.error),
      );
}

final storageProvider = Provider<StorageService>((ref) => StorageService());

final authProvider = NotifierProvider<AuthController, AuthState>(AuthController.new);

final apiClientProvider = Provider<ApiClient>((ref) {
  final baseUrl = ref.watch(authProvider).baseUrl ?? '';
  return ApiClient(baseUrl: baseUrl, storage: ref.watch(storageProvider));
});

class AuthController extends Notifier<AuthState> {
  StorageService get _storage => ref.read(storageProvider);

  @override
  AuthState build() {
    Future.microtask(_restore);
    return const AuthState();
  }

  Future<void> _restore() async {
    final baseUrl = await _storage.getBaseUrl();
    if (baseUrl == null || baseUrl.isEmpty) {
      state = const AuthState(loading: false);
      return;
    }
    final session = await _storage.getOidcSession();
    final token = await _storage.getApiToken();
    if ((session == null || session.isEmpty) && (token == null || token.isEmpty)) {
      state = AuthState(loading: false, baseUrl: baseUrl);
      return;
    }
    try {
      final user = await ApiClient(baseUrl: baseUrl, storage: _storage).me();
      state = AuthState(loading: false, baseUrl: baseUrl, user: user);
    } catch (_) {
      // Keep the address so onboarding can prefill it, but require a new
      // sign-in. A stale cookie must not silently keep the app signed in.
      await _storage.clearAuth();
      state = AuthState(loading: false, baseUrl: baseUrl);
    }
  }

  Future<bool> testConnection(String baseUrl) async {
    state = state.copyWith(clearError: true);
    final ok = await ApiClient.testConnection(baseUrl);
    if (!ok) state = state.copyWith(error: 'Could not reach that GitLens server.');
    return ok;
  }

  Future<bool> oidcEnabled(String baseUrl) => ApiClient.oidcEnabledAt(baseUrl);

  Future<bool> loginWithToken(String baseUrl, String token) async {
    state = state.copyWith(loading: true, clearError: true);
    await _storage.setBaseUrl(baseUrl);
    await _storage.setOidcSession(null);
    await _storage.setApiToken(token);
    try {
      final user = await ApiClient(baseUrl: baseUrl, storage: _storage).me();
      state = AuthState(loading: false, baseUrl: baseUrl, user: user);
      return true;
    } on ApiException catch (e) {
      await _storage.clearAuth();
      state = AuthState(loading: false, baseUrl: baseUrl, error: e.statusCode == 401 ? 'That API token was not accepted.' : e.message);
      return false;
    } catch (_) {
      await _storage.clearAuth();
      state = AuthState(loading: false, baseUrl: baseUrl, error: 'Sign-in failed. Check the server address and token.');
      return false;
    }
  }

  Future<bool> loginWithOidc(String baseUrl, String session) async {
    state = state.copyWith(loading: true, clearError: true);
    await _storage.setBaseUrl(baseUrl);
    await _storage.setApiToken(null);
    await _storage.setOidcSession(session);
    try {
      final user = await ApiClient(baseUrl: baseUrl, storage: _storage).me();
      state = AuthState(loading: false, baseUrl: baseUrl, user: user);
      return true;
    } on ApiException catch (e) {
      await _storage.clearAuth();
      state = AuthState(loading: false, baseUrl: baseUrl, error: e.statusCode == 401 ? 'The OIDC session was not accepted. Try again.' : e.message);
      return false;
    } catch (_) {
      await _storage.clearAuth();
      state = AuthState(loading: false, baseUrl: baseUrl, error: 'Sign-in did not complete. Try again.');
      return false;
    }
  }

  Future<void> logout() async {
    final baseUrl = state.baseUrl;
    await _storage.clearAuth();
    state = AuthState(loading: false, baseUrl: baseUrl);
  }
}
