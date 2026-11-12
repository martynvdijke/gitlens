import 'dart:async';

import 'package:firebase_core/firebase_core.dart';
import 'package:firebase_messaging/firebase_messaging.dart';
import 'package:flutter/foundation.dart';

import '../../core/api_client.dart';

/// Owns Firebase Cloud Messaging setup for the app.
///
/// The app intentionally ships without `google-services.json` and without the
/// Google Services Gradle plugin. Firebase is initialized at runtime with the
/// options the GitLens server advertises at `/api/v1/notifications/config`.
///
/// Every Firebase and network failure is swallowed: push is a best-effort
/// enhancement and must never prevent the app from starting.
class PushService {
  PushService._();

  static final PushService instance = PushService._();

  /// Emits the latest message received while the app is in the foreground.
  ///
  /// The UI listens to this and shows an in-app banner. Background and
  /// terminated messages are handled by the operating system.
  final ValueNotifier<RemoteMessage?> foregroundMessage = ValueNotifier<RemoteMessage?>(null);

  ApiClient? _api;
  Future<bool>? _initializing;
  String? _lastToken;
  StreamSubscription<RemoteMessage>? _messageSub;
  StreamSubscription<String>? _refreshSub;
  bool _enabled = false;

  /// Whether Firebase was initialized and a token flow was started.
  bool get enabled => _enabled;

  /// Initializes push notifications. Safe to call repeatedly; only the first
  /// call does work. Returns true when push is configured and ready.
  Future<bool> initialize(ApiClient api) {
    _api = api;
    final pending = _initializing;
    if (pending != null) return pending;

    final future = _start(api);
    _initializing = future;
    // Allow a later retry (e.g. after a transient network failure) while
    // keeping the idempotent fast path once we are up.
    future.then<void>(
      (ok) {
        if (!ok && identical(_initializing, future)) _initializing = null;
      },
      onError: (_) {
        if (identical(_initializing, future)) _initializing = null;
      },
    );
    return future;
  }

  Future<bool> _start(ApiClient api) async {
    try {
      final config = await api.notificationConfig();
      if (!config.isUsable) return false;

      if (Firebase.apps.isEmpty) {
        await Firebase.initializeApp(
          options: FirebaseOptions(
            apiKey: config.apiKey,
            appId: config.appId,
            messagingSenderId: config.senderId,
            projectId: config.projectId,
            storageBucket: config.storageBucket.isEmpty ? null : config.storageBucket,
          ),
        );
      }

      final messaging = FirebaseMessaging.instance;

      try {
        await messaging.requestPermission(alert: true, badge: true, sound: true);
      } catch (_) {
        // Denied or unavailable: foreground messages simply will not arrive.
      }

      _messageSub ??= FirebaseMessaging.onMessage.listen(_onForegroundMessage);
      // firebase_messaging 17 deprecates the FCM token APIs in favour of the
      // Firebase Installation ID, but the GitLens backend registers and sends
      // to FCM registration tokens, so keep using them here.
      // ignore: deprecated_member_use
      _refreshSub ??= messaging.onTokenRefresh.listen((token) => _register(token, api: api));

      // ignore: deprecated_member_use
      final token = await messaging.getToken();
      if (token != null && token.isNotEmpty) {
        await _register(token, api: api);
      }

      _enabled = true;
      return true;
    } catch (_) {
      _enabled = false;
      return false;
    }
  }

  void _onForegroundMessage(RemoteMessage message) {
    foregroundMessage.value = message;
  }

  Future<void> _register(String token, {ApiClient? api}) async {
    final client = api ?? _api;
    if (client == null || token.isEmpty) return;
    // The server upserts by token, but skipping duplicates keeps the API quiet
    // when initialize runs again for the same signed-in user.
    if (token == _lastToken) return;
    try {
      await client.registerDeviceToken(token);
      _lastToken = token;
    } catch (_) {
      // Network hiccup: leave _lastToken unset so the next attempt retries.
    }
  }

  /// Detaches this device from the signed-in user. Called on sign-out.
  Future<void> unregister() async {
    final client = _api;
    var token = _lastToken;
    if (token == null) {
      try {
        // ignore: deprecated_member_use
        token = await FirebaseMessaging.instance.getToken();
      } catch (_) {
        token = null;
      }
    }
    if (client != null && token != null && token.isNotEmpty) {
      try {
        await client.unregisterDeviceToken(token);
      } catch (_) {
        // A failed unregister must not block sign-out.
      }
    }
    _lastToken = null;
    _enabled = false;
    _initializing = null;
  }

  /// Releases listeners. Mainly useful for tests and hot restart hygiene.
  Future<void> dispose() async {
    await _messageSub?.cancel();
    await _refreshSub?.cancel();
    _messageSub = null;
    _refreshSub = null;
  }
}
