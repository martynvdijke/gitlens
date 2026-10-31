import 'dart:async';

import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../../core/auth.dart';
import '../../core/config.dart';
import '../../core/server_url.dart';
import '../../ui/brand.dart';
import '../../ui/theme.dart';
import 'oidc_webview_screen.dart';

class OnboardingScreen extends ConsumerStatefulWidget {
  const OnboardingScreen({super.key});

  @override
  ConsumerState<OnboardingScreen> createState() => _OnboardingScreenState();
}

class _OnboardingScreenState extends ConsumerState<OnboardingScreen> {
  final _urlController = TextEditingController();
  final _tokenController = TextEditingController();

  bool _urlValid = false;
  bool _checking = false;
  bool _testing = false;
  bool _submitting = false;
  bool _obscureToken = true;
  bool _oidcEnabled = false;
  String? _urlError;
  String? _formError;
  Timer? _debounce;
  int _oidcGeneration = 0;
  String? _oidcForUrl;

  @override
  void initState() {
    super.initState();
    _loadSavedAddress();
  }

  Future<void> _loadSavedAddress() async {
    // A previously saved address always wins; otherwise fall back to the
    // hosted default instance on first run.
    final saved = await ref.read(storageProvider).getBaseUrl();
    final address = saved != null && saved.isNotEmpty ? saved : defaultServerUrl;
    if (!mounted) return;
    final valid = serverUrlError(address) == null;
    setState(() {
      _urlController.text = address;
      _urlValid = valid;
    });
    if (valid) await _checkOidc(normalizeServerUrl(address));
  }

  void _onUrlChanged(String value) {
    _debounce?.cancel();
    _oidcGeneration++;
    final normalized = normalizeServerUrl(value);
    setState(() {
      _urlValid = false;
      _urlError = null;
      _formError = null;
      _checking = false;
      if (_oidcForUrl != normalized) {
        _oidcEnabled = false;
        _oidcForUrl = null;
      }
    });
    if (serverUrlError(value) != null) return;
    _debounce = Timer(const Duration(milliseconds: 500), () => _checkOidc(normalized));
  }

  Future<void> _checkOidc(String baseUrl) async {
    final generation = ++_oidcGeneration;
    if (mounted) setState(() => _checking = true);
    final enabled = await ApiClientOidcProbe.oidcEnabled(ref, baseUrl);
    if (!mounted || generation != _oidcGeneration) return;
    setState(() {
      _oidcEnabled = enabled;
      _oidcForUrl = baseUrl;
      _checking = false;
    });
  }

  String? _validateUrl() {
    final error = serverUrlError(_urlController.text);
    if (error != null) setState(() => _urlError = error);
    return error;
  }

  Future<void> _testConnection() async {
    if (_validateUrl() != null) return;
    final baseUrl = normalizeServerUrl(_urlController.text);
    setState(() {
      _testing = true;
      _urlError = null;
    });
    final ok = await ref.read(authProvider.notifier).testConnection(baseUrl);
    if (!mounted) return;
    setState(() {
      _testing = false;
      _urlValid = ok;
      _urlError = ok ? null : 'Could not reach that GitLens server.';
    });
    if (ok) _checkOidc(baseUrl);
  }

  Future<void> _tokenLogin() async {
    if (_validateUrl() != null) return;
    if (_tokenController.text.trim().isEmpty) {
      setState(() => _formError = 'Paste an API token from your GitLens settings.');
      return;
    }
    setState(() {
      _submitting = true;
      _formError = null;
    });
    await ref.read(authProvider.notifier).loginWithToken(
          normalizeServerUrl(_urlController.text),
          _tokenController.text.trim(),
        );
    if (mounted) setState(() => _submitting = false);
  }

  Future<void> _oidcLogin() async {
    if (_validateUrl() != null) return;
    final baseUrl = normalizeServerUrl(_urlController.text);
    setState(() {
      _submitting = true;
      _formError = null;
    });
    String? session;
    try {
      session = await Navigator.of(context).push<String>(
        MaterialPageRoute(builder: (_) => OidcWebViewScreen(baseUrl: baseUrl)),
      );
    } finally {
      if (mounted) setState(() => _submitting = false);
    }
    if (!mounted || session == null || session.isEmpty) return;
    await ref.read(authProvider.notifier).loginWithOidc(baseUrl, session);
  }

  @override
  void dispose() {
    _debounce?.cancel();
    _urlController.dispose();
    _tokenController.dispose();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    final auth = ref.watch(authProvider);
    final error = _formError ?? auth.error;
    return Scaffold(
      body: BrandBackdrop(
        child: SafeArea(
          child: Center(
            child: SingleChildScrollView(
              padding: const EdgeInsets.fromLTRB(22, 30, 22, 26),
              child: ConstrainedBox(
                constraints: const BoxConstraints(maxWidth: 520),
                child: Column(
                  crossAxisAlignment: CrossAxisAlignment.stretch,
                  children: [
                    Row(
                      children: [
                        const GitLensMark(size: 58),
                        const SizedBox(width: 14),
                        Column(
                          crossAxisAlignment: CrossAxisAlignment.start,
                          children: const [
                            Text('GitLens', style: TextStyle(fontSize: 30, fontWeight: FontWeight.w900, letterSpacing: -0.8)),
                            Text('Your repositories, at a glance', style: TextStyle(color: gitLensMuted, fontSize: 13)),
                          ],
                        ),
                      ],
                    ),
                    const SizedBox(height: 28),
                    const Text('Connect your server', style: TextStyle(fontSize: 22, fontWeight: FontWeight.w800, letterSpacing: -0.4)),
                    const SizedBox(height: 6),
                    const Text(
                      'Use the default instance below, or enter another self-hosted GitLens server.',
                      style: TextStyle(color: gitLensMuted, height: 1.45),
                    ),
                    const SizedBox(height: 20),
                    Card(
                      child: Padding(
                        padding: const EdgeInsets.all(18),
                        child: Column(
                          crossAxisAlignment: CrossAxisAlignment.stretch,
                          children: [
                            TextField(
                              controller: _urlController,
                              keyboardType: TextInputType.url,
                              autocorrect: false,
                              textInputAction: TextInputAction.next,
                              onChanged: _onUrlChanged,
                              decoration: InputDecoration(
                                labelText: 'Server URL',
                                hintText: 'https://gitlens.example.com',
                                prefixIcon: const Icon(Icons.dns_outlined),
                                errorText: _urlError,
                                helperText: 'Prefilled with the default instance',
                                suffixIcon: _urlValid ? const Icon(Icons.check_circle, color: gitLensGreen) : null,
                              ),
                            ),
                            const SizedBox(height: 12),
                            OutlinedButton.icon(
                              onPressed: _testing ? null : _testConnection,
                              icon: _testing
                                  ? const SizedBox(width: 15, height: 15, child: CircularProgressIndicator(strokeWidth: 2))
                                  : const Icon(Icons.wifi_tethering, size: 18),
                              label: Text(_testing ? 'Testing…' : 'Test connection'),
                            ),
                            if (_checking) ...[
                              const SizedBox(height: 10),
                              const Row(
                                children: [
                                  SizedBox(width: 14, height: 14, child: CircularProgressIndicator(strokeWidth: 2)),
                                  SizedBox(width: 8),
                                  Text('Checking sign-in options…', style: TextStyle(fontSize: 12, color: gitLensMuted)),
                                ],
                              ),
                            ],
                          ],
                        ),
                      ),
                    ),
                    if (_oidcEnabled) ...[
                      const SizedBox(height: 14),
                      FilledButton.icon(
                        onPressed: _submitting ? null : _oidcLogin,
                        style: FilledButton.styleFrom(
                          backgroundColor: gitLensGreen,
                          foregroundColor: gitLensBackground,
                          padding: const EdgeInsets.symmetric(vertical: 14),
                        ),
                        icon: const Icon(Icons.shield_outlined, size: 19),
                        label: const Text('Sign in with SSO', style: TextStyle(fontWeight: FontWeight.w700)),
                      ),
                      const SizedBox(height: 6),
                      const Text('Opens your identity provider securely inside the app.',
                          textAlign: TextAlign.center, style: TextStyle(fontSize: 11.5, color: gitLensMuted)),
                    ],
                    const SizedBox(height: 18),
                    const Row(children: [
                      Expanded(child: Divider()),
                      Padding(
                        padding: EdgeInsets.symmetric(horizontal: 12),
                        child: Text('or use an API token', style: TextStyle(color: gitLensMuted, fontSize: 12)),
                      ),
                      Expanded(child: Divider()),
                    ]),
                    const SizedBox(height: 18),
                    Card(
                      child: Padding(
                        padding: const EdgeInsets.all(18),
                        child: Column(
                          crossAxisAlignment: CrossAxisAlignment.stretch,
                          children: [
                            TextField(
                              controller: _tokenController,
                              obscureText: _obscureToken,
                              autocorrect: false,
                              enableSuggestions: false,
                              decoration: InputDecoration(
                                labelText: 'Personal API token',
                                hintText: 'glt_…',
                                prefixIcon: const Icon(Icons.key_outlined),
                                suffixIcon: IconButton(
                                  onPressed: () => setState(() => _obscureToken = !_obscureToken),
                                  icon: Icon(_obscureToken ? Icons.visibility_outlined : Icons.visibility_off_outlined),
                                ),
                              ),
                              onSubmitted: (_) => _tokenLogin(),
                            ),
                            const SizedBox(height: 12),
                            FilledButton.icon(
                              onPressed: _submitting ? null : _tokenLogin,
                              style: FilledButton.styleFrom(
                                backgroundColor: gitLensSurfaceRaised,
                                foregroundColor: Colors.white,
                                padding: const EdgeInsets.symmetric(vertical: 14),
                              ),
                              icon: _submitting
                                  ? const SizedBox(width: 16, height: 16, child: CircularProgressIndicator(strokeWidth: 2))
                                  : const Icon(Icons.login, size: 18),
                              label: Text(_submitting ? 'Signing in…' : 'Sign in'),
                            ),
                            const SizedBox(height: 8),
                            const Text(
                              'Create a token in GitLens under Settings → API tokens.',
                              style: TextStyle(fontSize: 11.5, color: gitLensMuted, height: 1.35),
                            ),
                          ],
                        ),
                      ),
                    ),
                    if (error != null) ...[
                      const SizedBox(height: 14),
                      Container(
                        padding: const EdgeInsets.all(12),
                        decoration: BoxDecoration(
                          color: const Color(0xFFF85149).withValues(alpha: 0.1),
                          borderRadius: BorderRadius.circular(12),
                          border: Border.all(color: const Color(0xFFF85149).withValues(alpha: 0.35)),
                        ),
                        child: Text(error, style: const TextStyle(color: Color(0xFFFFA198), fontSize: 12.5, height: 1.35)),
                      ),
                    ],
                    const SizedBox(height: 24),
                    const Text(
                      'GitLens never sends your credentials anywhere except the server you enter.',
                      textAlign: TextAlign.center,
                      style: TextStyle(fontSize: 11, color: gitLensMuted, height: 1.4),
                    ),
                  ],
                ),
              ),
            ),
          ),
        ),
      ),
    );
  }
}

/// Small indirection keeps the API dependency out of the widget while letting
/// tests override the probe if needed.
class ApiClientOidcProbe {
  /// Tests can override the network probe so the onboarding widget stays
  /// deterministic and offline.
  static Future<bool> Function(WidgetRef ref, String baseUrl)? override;

  static Future<bool> oidcEnabled(WidgetRef ref, String baseUrl) async {
    final custom = override;
    if (custom != null) return custom(ref, baseUrl);
    return ref.read(authProvider.notifier).oidcEnabled(baseUrl);
  }
}
