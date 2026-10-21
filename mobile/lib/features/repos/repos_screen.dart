import 'dart:async';

import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:web_socket_channel/io.dart';
import 'package:web_socket_channel/web_socket_channel.dart';

import '../../core/auth.dart';
import '../../core/models.dart';
import '../../ui/theme.dart';
import '../../ui/widgets.dart';

class ReposScreen extends ConsumerStatefulWidget {
  const ReposScreen({super.key});

  @override
  ConsumerState<ReposScreen> createState() => _ReposScreenState();
}

class _ReposScreenState extends ConsumerState<ReposScreen> {
  late Future<DashboardPayload> _future;
  final _searchController = TextEditingController();
  String _sort = 'updated';
  Timer? _debounce;
  Timer? _liveDebounce;
  WebSocketChannel? _liveChannel;

  @override
  void initState() {
    super.initState();
    _future = _load();
    unawaited(_connectLive());
  }

  Future<void> _connectLive() async {
    try {
      final ws = await ref.read(apiClientProvider).websocket();
      _liveChannel = IOWebSocketChannel.connect(Uri.parse(ws.url), headers: ws.headers);
      _liveChannel!.stream.listen(
        (_) {
          _liveDebounce?.cancel();
          _liveDebounce = Timer(const Duration(milliseconds: 800), _refresh);
        },
        onError: (_) {},
        cancelOnError: false,
      );
    } catch (_) {
      // Live updates are best-effort; pull-to-refresh remains available.
    }
  }

  Future<DashboardPayload> _load() => ref.read(apiClientProvider).dashboard();

  void _refresh() => setState(() => _future = _load());

  String _sortLabel(String value) {
    switch (value) {
      case 'build_status':
        return 'Build status';
      case 'latest_commit':
        return 'Latest commit';
      case 'latest_release':
        return 'Latest release';
      case 'name_asc':
        return 'Name A–Z';
      case 'name_desc':
        return 'Name Z–A';
      case 'synced_at':
        return 'Last synced';
      default:
        return 'Recently updated';
    }
  }

  List<Repository> _sortRepos(List<Repository> repos) {
    final copy = [...repos];
    switch (_sort) {
      case 'build_status':
        const rank = {'failure': 0, 'unknown': 1, 'success': 2, '': 3};
        copy.sort((a, b) => (rank[a.workflowStatus] ?? 3).compareTo(rank[b.workflowStatus] ?? 3));
        break;
      case 'latest_commit':
        copy.sort((a, b) => b.syncedAt?.compareTo(a.syncedAt ?? DateTime(1970)) ?? 0);
        break;
      case 'latest_release':
        copy.sort((a, b) => b.releaseCount.compareTo(a.releaseCount));
        break;
      case 'name_asc':
        copy.sort((a, b) => a.fullName.toLowerCase().compareTo(b.fullName.toLowerCase()));
        break;
      case 'name_desc':
        copy.sort((a, b) => b.fullName.toLowerCase().compareTo(a.fullName.toLowerCase()));
        break;
      case 'synced_at':
        copy.sort((a, b) => (b.syncedAt ?? DateTime(1970)).compareTo(a.syncedAt ?? DateTime(1970)));
        break;
      default:
        copy.sort((a, b) => b.totalCommits.compareTo(a.totalCommits));
    }
    return copy;
  }

  @override
  void dispose() {
    _debounce?.cancel();
    _liveDebounce?.cancel();
    _liveChannel?.sink.close();
    _searchController.dispose();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    return FutureBuilder<DashboardPayload>(
      future: _future,
      builder: (context, snapshot) {
        if (snapshot.connectionState == ConnectionState.waiting) {
          return ListView(children: List.generate(4, (_) => skeletonCard()));
        }
        if (snapshot.hasError) {
          return ErrorPane(message: '${snapshot.error}', onRetry: _refresh);
        }
        final data = snapshot.data!;
        final query = _searchController.text.trim().toLowerCase();
        final repos = _sortRepos(data.repos.where((r) {
          if (query.isEmpty) return true;
          return r.fullName.toLowerCase().contains(query) || r.description.toLowerCase().contains(query) || r.language.toLowerCase().contains(query);
        }).toList());

        return RefreshIndicator(
          onRefresh: () async {
            _refresh();
            await _future;
          },
          child: ListView(
            physics: const AlwaysScrollableScrollPhysics(),
            padding: const EdgeInsets.only(bottom: 28),
            children: [
              _OverviewGrid(overview: data.overview),
              Padding(
                padding: const EdgeInsets.fromLTRB(20, 14, 20, 0),
                child: TextField(
                  controller: _searchController,
                  onChanged: (_) {
                    _debounce?.cancel();
                    _debounce = Timer(const Duration(milliseconds: 180), () => setState(() {}));
                  },
                  decoration: const InputDecoration(
                    hintText: 'Search repositories',
                    prefixIcon: Icon(Icons.search),
                    isDense: true,
                  ),
                ),
              ),
              SectionHeader(
                title: repos.length == 1 ? '1 repository' : '${repos.length} repositories',
                subtitle: 'Live repository wall',
                trailing: PopupMenuButton<String>(
                  tooltip: 'Sort',
                  initialValue: _sort,
                  onSelected: (value) => setState(() => _sort = value),
                  itemBuilder: (context) => ['updated', 'build_status', 'name_asc', 'name_desc', 'synced_at']
                      .map((value) => PopupMenuItem(value: value, child: Text(_sortLabel(value))))
                      .toList(),
                  child: Container(
                    padding: const EdgeInsets.symmetric(horizontal: 10, vertical: 6),
                    decoration: BoxDecoration(
                      color: gitLensSurface,
                      borderRadius: BorderRadius.circular(999),
                      border: Border.all(color: gitLensBorder),
                    ),
                    child: Row(mainAxisSize: MainAxisSize.min, children: [
                      const Icon(Icons.sort, size: 15, color: gitLensMuted),
                      const SizedBox(width: 5),
                      Text(_sortLabel(_sort), style: const TextStyle(fontSize: 11.5, color: gitLensMuted)),
                    ]),
                  ),
                ),
              ),
              if (repos.isEmpty)
                const EmptyState(
                  icon: Icons.folder_open,
                  title: 'No repositories yet',
                  message: 'Once repos are imported into GitLens they appear here automatically.',
                )
              else
                ...repos.map((repo) => _RepoCard(repo: repo, onChanged: _refresh)),
            ],
          ),
        );
      },
    );
  }
}

class _OverviewGrid extends StatelessWidget {
  const _OverviewGrid({required this.overview});

  final RepoOverview overview;

  @override
  Widget build(BuildContext context) {
    final items = [
      ('Repos', '${overview.totalRepos}', gitLensBlue),
      ('Open PRs', '${overview.openPrs}', gitLensGreen),
      ('Failing', '${overview.failingRepos}', overview.failingRepos > 0 ? const Color(0xFFF85149) : gitLensMuted),
      ('Releases', '${overview.totalReleases}', const Color(0xFFD2A8FF)),
    ];
    return Container(
      margin: const EdgeInsets.fromLTRB(20, 12, 20, 0),
      padding: const EdgeInsets.symmetric(vertical: 14),
      decoration: BoxDecoration(
        gradient: const LinearGradient(colors: [gitLensSurface, gitLensBackground]),
        borderRadius: BorderRadius.circular(18),
        border: Border.all(color: gitLensBorder),
      ),
      child: Row(
        children: items.map((item) {
          return Expanded(
            child: Column(
              children: [
                Text(item.$2, style: TextStyle(fontSize: 21, fontWeight: FontWeight.w900, color: item.$3, letterSpacing: -0.5)),
                const SizedBox(height: 2),
                Text(item.$1, style: const TextStyle(fontSize: 10.5, color: gitLensMuted, fontWeight: FontWeight.w600)),
              ],
            ),
          );
        }).toList(),
      ),
    );
  }
}

class _RepoCard extends ConsumerWidget {
  const _RepoCard({required this.repo, required this.onChanged});

  final Repository repo;
  final VoidCallback onChanged;

  Color get _statusColor {
    switch (repo.workflowStatus) {
      case 'success':
        return gitLensGreen;
      case 'failure':
        return const Color(0xFFF85149);
      default:
        return gitLensMuted;
    }
  }

  String get _statusLabel {
    switch (repo.workflowStatus) {
      case 'success':
        return 'Passing';
      case 'failure':
        return 'Failing';
      default:
        return 'No build';
    }
  }

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    return Container(
      margin: const EdgeInsets.symmetric(horizontal: 20, vertical: 6),
      decoration: BoxDecoration(
        color: gitLensSurface,
        borderRadius: BorderRadius.circular(16),
        border: Border.all(color: gitLensBorder),
      ),
      child: InkWell(
        borderRadius: BorderRadius.circular(16),
        onTap: () => _showDetails(context, ref),
        child: Padding(
          padding: const EdgeInsets.all(16),
          child: Column(
            crossAxisAlignment: CrossAxisAlignment.start,
            children: [
              Row(
                children: [
                  Icon(Icons.folder_rounded, size: 17, color: _statusColor),
                  const SizedBox(width: 8),
                  Expanded(
                    child: Text(repo.fullName,
                        maxLines: 1,
                        overflow: TextOverflow.ellipsis,
                        style: const TextStyle(fontSize: 15, fontWeight: FontWeight.w800, letterSpacing: -0.2)),
                  ),
                  StatusPill(label: _statusLabel, color: _statusColor),
                  const SizedBox(width: 2),
                  IconButton(
                    visualDensity: VisualDensity.compact,
                    tooltip: 'Repository actions',
                    onPressed: () => _showActions(context, ref),
                    icon: const Icon(Icons.more_vert, size: 18, color: gitLensMuted),
                  ),
                ],
              ),
              if (repo.description.isNotEmpty) ...[
                const SizedBox(height: 8),
                Text(repo.description, maxLines: 2, overflow: TextOverflow.ellipsis, style: const TextStyle(fontSize: 12.5, color: gitLensMuted, height: 1.35)),
              ],
              const SizedBox(height: 12),
              Wrap(
                spacing: 6,
                runSpacing: 6,
                children: [
                  if (repo.language.isNotEmpty) StatusPill(label: repo.language, color: gitLensBlue),
                  StatusPill(label: '${repo.openPrCount} PR${repo.openPrCount == 1 ? '' : 's'}', color: gitLensGreen),
                  StatusPill(label: '${repo.releaseCount} releases', color: const Color(0xFFD2A8FF)),
                  if (repo.backfillStatus == 'running') const StatusPill(label: 'Backfilling', color: Color(0xFFD29922)),
                ],
              ),
              const SizedBox(height: 12),
              Row(
                children: [
                  Expanded(child: _CommitBar(repo: repo)),
                  const SizedBox(width: 12),
                  Text(repo.syncedAt == null ? 'Not synced' : 'Synced ${relativeTime(repo.syncedAt)}',
                      style: const TextStyle(fontSize: 10.5, color: gitLensMuted)),
                ],
              ),
            ],
          ),
        ),
      ),
    );
  }

  Future<void> _showActions(BuildContext context, WidgetRef ref) async {
    await showModalBottomSheet<void>(
      context: context,
      backgroundColor: gitLensSurface,
      shape: const RoundedRectangleBorder(borderRadius: BorderRadius.vertical(top: Radius.circular(22))),
      builder: (sheetContext) => SafeArea(
        child: Column(
          mainAxisSize: MainAxisSize.min,
          children: [
            const SizedBox(height: 10),
            ListTile(
              leading: const Icon(Icons.sync, color: gitLensBlue),
              title: const Text('Sync repository now'),
              onTap: () async {
                Navigator.pop(sheetContext);
                await _run(context, () => ref.read(apiClientProvider).syncRepo(repo.id), 'Synced ${repo.fullName}');
              },
            ),
            ListTile(
              leading: const Icon(Icons.auto_fix_high, color: gitLensGreen),
              title: const Text('Renovate: rebase all PRs'),
              subtitle: const Text('Ask Renovate to rebase every open dependency PR'),
              onTap: () async {
                Navigator.pop(sheetContext);
                final result = await _runMessage(context, () => ref.read(apiClientProvider).renovate(repo.id));
                if (result != null && context.mounted) _toast(context, result.message, ok: result.ok);
              },
            ),
            const SizedBox(height: 8),
          ],
        ),
      ),
    );
  }

  Future<void> _run(BuildContext context, Future<void> Function() action, String success) async {
    try {
      await action();
      onChanged();
      if (context.mounted) _toast(context, success, ok: true);
    } catch (e) {
      if (context.mounted) _toast(context, '$e', ok: false);
    }
  }

  Future<ActionMessage?> _runMessage(BuildContext context, Future<ActionMessage> Function() action) async {
    try {
      return await action();
    } catch (e) {
      if (context.mounted) _toast(context, '$e', ok: false);
      return null;
    }
  }

  void _toast(BuildContext context, String message, {required bool ok}) {
    ScaffoldMessenger.of(context).showSnackBar(SnackBar(
      content: Text(message),
      backgroundColor: ok ? gitLensSurfaceRaised : const Color(0xFF3D1518),
    ));
  }

  Future<void> _showDetails(BuildContext context, WidgetRef ref) async {
    await showModalBottomSheet<void>(
      context: context,
      isScrollControlled: true,
      backgroundColor: gitLensSurface,
      shape: const RoundedRectangleBorder(borderRadius: BorderRadius.vertical(top: Radius.circular(24))),
      builder: (context) => DraggableScrollableSheet(
        expand: false,
        initialChildSize: 0.6,
        minChildSize: 0.35,
        maxChildSize: 0.92,
        builder: (context, controller) => ListView(
          controller: controller,
          padding: const EdgeInsets.fromLTRB(22, 14, 22, 30),
          children: [
            Center(child: Container(width: 38, height: 4, decoration: BoxDecoration(color: gitLensBorder, borderRadius: BorderRadius.circular(99)))),
            const SizedBox(height: 18),
            Text(repo.fullName, style: const TextStyle(fontSize: 20, fontWeight: FontWeight.w900, letterSpacing: -0.4)),
            if (repo.description.isNotEmpty) ...[
              const SizedBox(height: 6),
              Text(repo.description, style: const TextStyle(color: gitLensMuted, height: 1.4)),
            ],
            const SizedBox(height: 16),
            Row(children: [
              Expanded(child: _MiniStat(label: 'Commits', value: '${repo.totalCommits}')),
              Expanded(child: _MiniStat(label: 'Releases', value: '${repo.releaseCount}')),
              Expanded(child: _MiniStat(label: 'Open PRs', value: '${repo.openPrCount}')),
            ]),
            const Divider(height: 28),
            const Text('Commit mix', style: TextStyle(fontWeight: FontWeight.w700)),
            const SizedBox(height: 10),
            ..._commitRows(repo),
            const Divider(height: 28),
            const Text('Open pull requests', style: TextStyle(fontWeight: FontWeight.w700)),
            const SizedBox(height: 8),
            if (repo.pullRequests.isEmpty)
              const Text('No open pull requests.', style: TextStyle(color: gitLensMuted, fontSize: 12.5))
            else
              ...repo.pullRequests.map((pr) => ListTile(
                    contentPadding: EdgeInsets.zero,
                    leading: Icon(
                      pr.buildStatus == 'success' ? Icons.check_circle : pr.buildStatus == 'failure' ? Icons.error : Icons.circle_outlined,
                      color: pr.buildStatus == 'success' ? gitLensGreen : pr.buildStatus == 'failure' ? const Color(0xFFF85149) : gitLensMuted,
                      size: 19,
                    ),
                    title: Text('#${pr.number} ${pr.title}', maxLines: 2, overflow: TextOverflow.ellipsis, style: const TextStyle(fontSize: 13.5)),
                    subtitle: Text(pr.author, style: const TextStyle(fontSize: 11.5, color: gitLensMuted)),
                  )),
          ],
        ),
      ),
    );
  }

  List<Widget> _commitRows(Repository repo) {
    final total = repo.totalCommits;
    if (total == 0) return [const Text('No commit data yet.', style: TextStyle(color: gitLensMuted, fontSize: 12.5))];
    final rows = [
      ('Features', repo.featCount, gitLensGreen),
      ('Fixes', repo.fixCount, const Color(0xFFF85149)),
      ('Docs', repo.docsCount, gitLensBlue),
      ('Chores', repo.choreCount, const Color(0xFFD29922)),
      ('Other', repo.otherCommitCount, gitLensMuted),
    ];
    return rows.map((row) {
      final pct = row.$2 / total;
      return Padding(
        padding: const EdgeInsets.only(bottom: 8),
        child: Row(children: [
          SizedBox(width: 70, child: Text(row.$1, style: const TextStyle(fontSize: 11.5, color: gitLensMuted))),
          Expanded(
            child: ClipRRect(
              borderRadius: BorderRadius.circular(99),
              child: LinearProgressIndicator(value: pct, minHeight: 6, backgroundColor: gitLensBackground, color: row.$3),
            ),
          ),
          const SizedBox(width: 8),
          SizedBox(width: 32, child: Text('${row.$2}', textAlign: TextAlign.right, style: const TextStyle(fontSize: 11.5))),
        ]),
      );
    }).toList();
  }
}

class _MiniStat extends StatelessWidget {
  const _MiniStat({required this.label, required this.value});

  final String label;
  final String value;

  @override
  Widget build(BuildContext context) {
    return Column(children: [
      Text(value, style: const TextStyle(fontSize: 22, fontWeight: FontWeight.w900, color: gitLensGreen)),
      Text(label, style: const TextStyle(fontSize: 11, color: gitLensMuted)),
    ]);
  }
}

class _CommitBar extends StatelessWidget {
  const _CommitBar({required this.repo});

  final Repository repo;

  @override
  Widget build(BuildContext context) {
    final total = repo.totalCommits;
    if (total == 0) {
      return const Text('No commit data', style: TextStyle(fontSize: 10.5, color: gitLensMuted));
    }
    return ClipRRect(
      borderRadius: BorderRadius.circular(99),
      child: SizedBox(
        height: 6,
        child: Row(
          children: [
            if (repo.featCount > 0) Expanded(flex: repo.featCount, child: const ColoredBox(color: gitLensGreen)),
            if (repo.fixCount > 0) Expanded(flex: repo.fixCount, child: const ColoredBox(color: Color(0xFFF85149))),
            if (repo.docsCount > 0) Expanded(flex: repo.docsCount, child: const ColoredBox(color: gitLensBlue)),
            if (repo.choreCount > 0) Expanded(flex: repo.choreCount, child: const ColoredBox(color: Color(0xFFD29922))),
            if (repo.otherCommitCount > 0) Expanded(flex: repo.otherCommitCount, child: const ColoredBox(color: gitLensMuted)),
          ],
        ),
      ),
    );
  }
}
