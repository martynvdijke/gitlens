import 'dart:async';

import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

import 'app.dart';
import 'core/config.dart';
import 'core/storage.dart';
import 'src/telemetry/telemetry.dart';

void main() {
  WidgetsFlutterBinding.ensureInitialized();
  runApp(const ProviderScope(child: _TelemetryBootstrap(child: GitLensApp())));
}

/// Starts telemetry after resolving the server the app talks to.
///
/// The network call runs unawaited so it never delays the first frame, and
/// telemetry silently stays disabled when the device is offline.
class _TelemetryBootstrap extends StatefulWidget {
  const _TelemetryBootstrap({required this.child});

  final Widget child;

  @override
  State<_TelemetryBootstrap> createState() => _TelemetryBootstrapState();
}

class _TelemetryBootstrapState extends State<_TelemetryBootstrap> {
  @override
  void initState() {
    super.initState();
    unawaited(_startTelemetry());
  }

  Future<void> _startTelemetry() async {
    try {
      final stored = await StorageService().getBaseUrl();
      final baseUrl =
          (stored == null || stored.trim().isEmpty) ? defaultServerUrl : stored;
      await Telemetry.instance.startFromServer(baseUrl);
    } catch (_) {
      // Telemetry must never break app startup.
    }
  }

  @override
  Widget build(BuildContext context) => widget.child;
}
