import 'dart:async';

import 'package:flutter/material.dart';
import 'package:flutter/services.dart';
import 'package:webview_flutter/webview_flutter.dart';

import '../../ui/brand.dart';
import '../../ui/theme.dart';
import '../../utils/cookies.dart';

/// Runs the server's OIDC authorization flow in an in-app WebView.
///
/// The resulting `gitlens_session_oidc` cookie is HttpOnly, so JavaScript
/// cannot read it. It is fetched from Android's native cookie store over a
/// MethodChannel and returned to onboarding for secure storage.
class OidcWebViewScreen extends StatefulWidget {
  const OidcWebViewScreen({
    super.key,
    required this.baseUrl,
    this.cookieChannel = const MethodChannel('xyz.vandijke.gitlens/cookies'),
  });

  final String baseUrl;
  final MethodChannel cookieChannel;

  @override
  State<OidcWebViewScreen> createState() => _OidcWebViewScreenState();
}

class _OidcWebViewScreenState extends State<OidcWebViewScreen> {
  late final WebViewController _controller;
  late final Uri _serverOrigin;

  bool _completed = false;
  bool _capturing = false;
  bool _captureQueued = false;
  bool _loading = true;
  String? _error;
  bool _lastServerPageOk = false;

  @override
  void initState() {
    super.initState();
    _serverOrigin = Uri.parse(widget.baseUrl);
    _controller = WebViewController()
      ..setJavaScriptMode(JavaScriptMode.unrestricted)
      ..setNavigationDelegate(
        NavigationDelegate(
          onPageStarted: (_) {
            if (!mounted) return;
            setState(() {
              _loading = true;
              _error = null;
              _lastServerPageOk = false;
            });
          },
          onPageFinished: (url) {
            if (!mounted) return;
            final onServer = _isServerUrl(url);
            _lastServerPageOk = onServer;
            setState(() => _loading = false);
            if (onServer) unawaited(_captureSession());
          },
          onUrlChange: (change) {
            if (!mounted) return;
            if (_isServerUrl(change.url)) {
              _lastServerPageOk = true;
              unawaited(_captureSession());
            }
          },
          onHttpError: (error) {
            final status = error.response?.statusCode;
            if (!mounted || (status != 401 && status != 403)) return;
            setState(() {
              _loading = false;
              _error = 'The sign-in provider returned HTTP $status.';
            });
          },
          onWebResourceError: (error) {
            if (!mounted || error.isForMainFrame == false) return;
            if (_lastServerPageOk) return;
            if (_ignorableWebError(error)) return;
            setState(() {
              _loading = false;
              _error = error.description.isEmpty ? 'The sign-in page could not be loaded.' : error.description;
            });
          },
          onNavigationRequest: (_) => NavigationDecision.navigate,
        ),
      );
    unawaited(_start());
  }

  bool _ignorableWebError(WebResourceError error) {
    if (error.errorType?.name == 'cancelled') return true;
    // ERR_ABORTED, ERR_UNKNOWN_URL_SCHEME.
    if (error.errorCode == -3 || error.errorCode == -10) return true;
    final text = error.description.toLowerCase();
    return text.contains('err_aborted') || text.contains('err_unknown_url_scheme') || text.contains('cancel');
  }

  Future<void> _start() async {
    await _clearCookies();
    if (!mounted || _completed) return;
    await _controller.loadRequest(Uri.parse('${widget.baseUrl}/api/auth/oidc/login'));
  }

  Future<void> _clearCookies() async {
    try {
      await widget.cookieChannel.invokeMethod<void>('clearCookies');
    } catch (_) {
      // Non-Android hosts may not provide the channel.
    }
    try {
      await WebViewCookieManager().clearCookies();
    } catch (_) {
      // The flow can still succeed with stale cookies; capture only happens
      // after returning to the configured server origin.
    }
  }

  bool _isServerUrl(String? url) {
    if (url == null || url.isEmpty) return false;
    final uri = Uri.tryParse(url);
    if (uri == null || !uri.hasScheme || uri.host.isEmpty) return false;
    return uri.scheme == _serverOrigin.scheme &&
        uri.host == _serverOrigin.host &&
        uri.port == _serverOrigin.port;
  }

  Future<void> _captureSession() async {
    if (_completed) return;
    if (_capturing) {
      _captureQueued = true;
      return;
    }
    _capturing = true;
    try {
      do {
        _captureQueued = false;
        for (var attempt = 0; attempt < 12 && !_completed; attempt++) {
          if (!mounted) return;
          final current = await _controller.currentUrl();
          if (current != null && !_isServerUrl(current)) return;
          final session = await _readSessionCookie();
          if (session != null && session.isNotEmpty) {
            if (!mounted) return;
            final current = await _controller.currentUrl();
            if (current != null && !_isServerUrl(current)) continue;
            _completed = true;
            if (!mounted) return;
            Navigator.of(context).pop(session);
            return;
          }
          if (attempt < 11) await Future<void>.delayed(const Duration(milliseconds: 250));
        }
      } while (_captureQueued && !_completed && mounted);
    } finally {
      _capturing = false;
      _captureQueued = false;
    }
  }

  Future<String?> _readSessionCookie() async {
    try {
      final raw = await widget.cookieChannel.invokeMethod<String>('getCookies', {'url': widget.baseUrl});
      final value = parseCookieHeader(raw, 'gitlens_session_oidc');
      if (value != null && value.isNotEmpty) return value;
    } on MissingPluginException {
      // Fall through to the WebView plugin.
    } on PlatformException {
      // Retry on the next poll.
    }
    try {
      final cookies = await WebViewCookieManager().getCookies(domain: _serverOrigin);
      for (final cookie in cookies) {
        if (cookie.name == 'gitlens_session_oidc' && cookie.value.isNotEmpty) return cookie.value;
      }
    } catch (_) {
      // No platform cookie manager.
    }
    return null;
  }

  @override
  Widget build(BuildContext context) {
    return Scaffold(
      appBar: AppBar(
        title: const Row(
          children: [
            GitLensMark(size: 26),
            SizedBox(width: 10),
            Text('GitLens sign-in'),
          ],
        ),
      ),
      body: Stack(
        children: [
          WebViewWidget(controller: _controller),
          if (_loading)
            const ColoredBox(
              color: gitLensBackground,
              child: Center(child: CircularProgressIndicator()),
            ),
          if (_error != null)
            Positioned(
              left: 16,
              right: 16,
              bottom: 20,
              child: Card(
                child: Padding(
                  padding: const EdgeInsets.all(16),
                  child: Row(
                    children: [
                      const Icon(Icons.error_outline, color: Color(0xFFF85149)),
                      const SizedBox(width: 12),
                      Expanded(child: Text(_error!, style: const TextStyle(height: 1.35))),
                      TextButton(
                        onPressed: () {
                          setState(() {
                            _error = null;
                            _loading = true;
                          });
                          _controller.reload();
                        },
                        child: const Text('Retry'),
                      ),
                    ],
                  ),
                ),
              ),
            ),
        ],
      ),
    );
  }
}
