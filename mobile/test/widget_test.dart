import 'package:flutter_test/flutter_test.dart';
import 'package:gitlens/core/server_url.dart';

void main() {
  group('server URL validation', () {
    test('has no implicit server address', () {
      expect(serverUrlError(''), isNotNull);
      expect(normalizeServerUrl(''), isEmpty);
    });

    test('normalizes trailing slashes', () {
      expect(normalizeServerUrl('https://gitlens.example.com/'), 'https://gitlens.example.com');
    });

    test('accepts http for self-hosted LAN instances', () {
      expect(serverUrlError('http://10.0.2.2:6270'), isNull);
    });

    test('rejects unsupported schemes', () {
      expect(serverUrlError('ftp://example.com'), isNotNull);
    });
  });
}
