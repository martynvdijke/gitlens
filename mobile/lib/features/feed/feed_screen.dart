import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../../core/auth.dart';
import '../../core/models.dart';
import '../../ui/theme.dart';
import '../../ui/widgets.dart';

class FeedScreen extends ConsumerStatefulWidget {
  const FeedScreen({super.key});

  @override
  ConsumerState<FeedScreen> createState() => _FeedScreenState();
}

class _FeedScreenState extends ConsumerState<FeedScreen> {
  late Future<FeedPayload> _future;
  String _since = '7d';
  String _types = 'all';

  @override
  void initState() {
    super.initState();
    _future = _load();
  }

  Future<FeedPayload> _load() => ref.read(apiClientProvider).feed(since: _since, types: _types);

  void _refresh() => setState(() => _future = _load());

  @override
  Widget build(BuildContext context) {
    return FutureBuilder<FeedPayload>(
      future: _future,
      builder: (context, snapshot) {
        final data = snapshot.data;
        return Column(
          children: [
            SizedBox(
              height: 50,
              child: ListView(
                scrollDirection: Axis.horizontal,
                padding: const EdgeInsets.fromLTRB(20, 8, 20, 6),
                children: [
                  for (final option in const [('24h', '24 hours'), ('7d', '7 days'), ('30d', '30 days'), ('all', 'All time')])
                    Padding(
                      padding: const EdgeInsets.only(right: 8),
                      child: ChoiceChip(
                        label: Text(option.$2),
                        selected: _since == option.$1,
                        onSelected: (_) {
                          setState(() {
                            _since = option.$1;
                            _future = _load();
                          });
                        },
                      ),
                    ),
                  const SizedBox(width: 6),
                  PopupMenuButton<String>(
                    initialValue: _types,
                    tooltip: 'Event types',
                    onSelected: (value) => setState(() {
                      _types = value;
                      _future = _load();
                    }),
                    itemBuilder: (context) => const [
                      PopupMenuItem(value: 'all', child: Text('All events')),
                      PopupMenuItem(value: 'release', child: Text('Releases')),
                      PopupMenuItem(value: 'pr_merge', child: Text('Merged PRs')),
                      PopupMenuItem(value: 'workflow_failure', child: Text('Build failures')),
                    ],
                    child: Chip(
                      avatar: const Icon(Icons.tune, size: 15, color: gitLensMuted),
                      label: Text(_types == 'all' ? 'All events' : _types.replaceAll('_', ' ')),
                    ),
                  ),
                ],
              ),
            ),
            Expanded(
              child: RefreshIndicator(
                onRefresh: () async {
                  _refresh();
                  await _future;
                },
                child: _buildBody(snapshot, data),
              ),
            ),
          ],
        );
      },
    );
  }

  Widget _buildBody(AsyncSnapshot<FeedPayload> snapshot, FeedPayload? data) {
    if (snapshot.connectionState == ConnectionState.waiting) {
      return ListView(children: List.generate(5, (_) => skeletonCard()));
    }
    if (snapshot.hasError) {
      return ErrorPane(message: '${snapshot.error}', onRetry: _refresh);
    }
    if (data == null || data.groups.isEmpty) {
      return ListView(
        physics: const AlwaysScrollableScrollPhysics(),
        children: const [
          SizedBox(height: 100),
          EmptyState(icon: Icons.bolt, title: 'No activity yet', message: 'Releases, merged PRs and failed workflows show up here.'),
        ],
      );
    }
    return ListView(
      physics: const AlwaysScrollableScrollPhysics(),
      padding: const EdgeInsets.only(bottom: 26),
      children: [
        for (final group in data.groups) ...[
          SectionHeader(title: group.date, subtitle: '${group.events.length} event${group.events.length == 1 ? '' : 's'}'),
          ...group.events.map((event) => _EventCard(event: event)),
        ],
      ],
    );
  }
}

class _EventCard extends StatelessWidget {
  const _EventCard({required this.event});

  final FeedEvent event;

  ({IconData icon, Color color, String label}) get _style {
    switch (event.type) {
      case 'release':
        return (icon: Icons.new_releases_outlined, color: const Color(0xFFD2A8FF), label: 'Release');
      case 'pr_merge':
        return (icon: Icons.merge_type, color: gitLensGreen, label: 'Merged');
      case 'workflow_failure':
        return (icon: Icons.error_outline, color: const Color(0xFFF85149), label: 'Build failed');
      default:
        return (icon: Icons.circle_outlined, color: gitLensMuted, label: event.type);
    }
  }

  @override
  Widget build(BuildContext context) {
    final style = _style;
    return Container(
      margin: const EdgeInsets.symmetric(horizontal: 20, vertical: 5),
      decoration: BoxDecoration(
        color: gitLensSurface,
        borderRadius: BorderRadius.circular(14),
        border: Border.all(color: gitLensBorder),
      ),
      child: Padding(
        padding: const EdgeInsets.all(14),
        child: Row(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            Container(
              padding: const EdgeInsets.all(8),
              decoration: BoxDecoration(color: style.color.withValues(alpha: 0.12), borderRadius: BorderRadius.circular(10)),
              child: Icon(style.icon, size: 17, color: style.color),
            ),
            const SizedBox(width: 12),
            Expanded(
              child: Column(
                crossAxisAlignment: CrossAxisAlignment.start,
                children: [
                  Row(
                    children: [
                      StatusPill(label: style.label, color: style.color),
                      const Spacer(),
                      Text(relativeTime(event.timestamp), style: const TextStyle(fontSize: 10.5, color: gitLensMuted)),
                    ],
                  ),
                  const SizedBox(height: 6),
                  Text(event.title, style: const TextStyle(fontSize: 13.5, fontWeight: FontWeight.w600, height: 1.3)),
                  if (event.repoFull.isNotEmpty) ...[
                    const SizedBox(height: 3),
                    Text(event.repoFull, style: const TextStyle(fontSize: 11, color: gitLensBlue)),
                  ],
                ],
              ),
            ),
          ],
        ),
      ),
    );
  }
}
