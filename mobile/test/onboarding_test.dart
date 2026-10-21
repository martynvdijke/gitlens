import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:gitlens/core/auth.dart';
import 'package:gitlens/core/storage.dart';
import 'package:gitlens/features/onboarding/onboarding_screen.dart';

class _FakeStorage extends StorageService {
  String? baseUrl;
  String? session;
  String? token;

  @override
  Future<String?> getBaseUrl() async => baseUrl;

  @override
  Future<void> setBaseUrl(String url) async => baseUrl = url;

  @override
  Future<String?> getOidcSession() async => session;

  @override
  Future<void> setOidcSession(String? value) async => session = value;

  @override
  Future<String?> getApiToken() async => token;

  @override
  Future<void> setApiToken(String? value) async => token = value;

  @override
  Future<void> clearAuth() async {
    session = null;
    token = null;
  }
}

void main() {
  testWidgets('onboarding starts with an empty server address', (tester) async {
    final storage = _FakeStorage();
    await tester.pumpWidget(ProviderScope(
      overrides: [storageProvider.overrideWithValue(storage)],
      child: const MaterialApp(home: OnboardingScreen()),
    ));
    await tester.pump();

    final field = tester.widget<TextField>(find.byType(TextField).first);
    expect(field.controller?.text, isEmpty);
    expect(find.text('Sign in with SSO'), findsNothing);
  });

}
