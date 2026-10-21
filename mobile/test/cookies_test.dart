import 'package:flutter_test/flutter_test.dart';
import 'package:gitlens/utils/cookies.dart';

void main() {
  group('OIDC cookie parsing', () {
    test('extracts the signed GitLens session cookie', () {
      const header = 'theme=dark; gitlens_session_oidc=abc123; Path=/; HttpOnly';
      expect(parseCookieHeader(header, 'gitlens_session_oidc'), 'abc123');
    });

    test('does not confuse a legacy session with the OIDC session', () {
      const header = 'gitlens_session=legacy; gitlens_session_oidc=oidc-value';
      expect(parseCookieHeader(header, 'gitlens_session_oidc'), 'oidc-value');
    });

    test('returns null for a missing cookie', () {
      expect(parseCookieHeader('theme=dark', 'gitlens_session_oidc'), isNull);
      expect(parseCookieHeader(null, 'gitlens_session_oidc'), isNull);
    });
  });
}
