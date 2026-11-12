import 'package:flutter_test/flutter_test.dart';
import 'package:gitlens/core/models.dart';

void main() {
  group('NotificationConfig', () {
    test('parses the server payload', () {
      final config = NotificationConfig.fromJson(const {
        'enabled': true,
        'project_id': 'gitlens-prod',
        'api_key': 'AIza-key',
        'app_id': '1:2:android:abc',
        'sender_id': '1234567890',
        'storage_bucket': 'gitlens-prod.appspot.com',
      });

      expect(config.enabled, isTrue);
      expect(config.projectId, 'gitlens-prod');
      expect(config.apiKey, 'AIza-key');
      expect(config.appId, '1:2:android:abc');
      expect(config.senderId, '1234567890');
      expect(config.storageBucket, 'gitlens-prod.appspot.com');
      expect(config.isUsable, isTrue);
    });

    test('treats missing fields as empty and unusable', () {
      final config = NotificationConfig.fromJson(const {});

      expect(config.enabled, isFalse);
      expect(config.projectId, isEmpty);
      expect(config.apiKey, isEmpty);
      expect(config.isUsable, isFalse);
      expect(NotificationConfig.disabled.isUsable, isFalse);
    });

    test('requires every credential even when enabled', () {
      const config = NotificationConfig(
        enabled: true,
        projectId: 'p',
        apiKey: 'k',
        appId: 'a',
      );

      expect(config.isUsable, isFalse, reason: 'sender_id is empty');
    });

    test('round-trips through JSON', () {
      const config = NotificationConfig(
        enabled: true,
        projectId: 'p',
        apiKey: 'k',
        appId: 'a',
        senderId: 's',
        storageBucket: 'b',
      );

      final decoded = NotificationConfig.fromJson(config.toJson());

      expect(decoded.toJson(), config.toJson());
      expect(decoded.isUsable, isTrue);
    });
  });

  group('NotificationPrefs', () {
    test('parses the server payload', () {
      final prefs = NotificationPrefs.fromJson(const {
        'notifications_enabled': true,
        'notify_web': true,
        'notify_android': false,
        'notify_release': false,
        'notify_pr_merge': true,
        'notify_workflow_failure': false,
      });

      expect(prefs.notificationsEnabled, isTrue);
      expect(prefs.notifyWeb, isTrue);
      expect(prefs.notifyAndroid, isFalse);
      expect(prefs.notifyRelease, isFalse);
      expect(prefs.notifyPrMerge, isTrue);
      expect(prefs.notifyWorkflowFailure, isFalse);
    });

    test('defaults to disabled with event types on', () {
      final prefs = NotificationPrefs.fromJson(const {});

      expect(prefs.notificationsEnabled, isFalse);
      expect(prefs.notifyWeb, isFalse);
      expect(prefs.notifyAndroid, isTrue);
      expect(prefs.notifyRelease, isTrue);
      expect(prefs.notifyPrMerge, isTrue);
      expect(prefs.notifyWorkflowFailure, isTrue);
    });

    test('round-trips every field through JSON', () {
      const prefs = NotificationPrefs(
        notificationsEnabled: true,
        notifyWeb: true,
        notifyAndroid: false,
        notifyRelease: false,
        notifyPrMerge: true,
        notifyWorkflowFailure: false,
      );

      final json = prefs.toJson();
      expect(json.keys.toSet(), {
        'notifications_enabled',
        'notify_web',
        'notify_android',
        'notify_release',
        'notify_pr_merge',
        'notify_workflow_failure',
      });

      final decoded = NotificationPrefs.fromJson(json);
      expect(decoded.toJson(), json);
    });

    test('copyWith replaces only the requested fields', () {
      const prefs = NotificationPrefs(notificationsEnabled: true);

      final updated = prefs.copyWith(notifyRelease: false, notifyPrMerge: false);

      expect(updated.notificationsEnabled, isTrue);
      expect(updated.notifyAndroid, isTrue);
      expect(updated.notifyRelease, isFalse);
      expect(updated.notifyPrMerge, isFalse);
      expect(updated.notifyWorkflowFailure, isTrue);
    });
  });
}
