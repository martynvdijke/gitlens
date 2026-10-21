import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../../core/auth.dart';
import '../../core/models.dart';
import '../../ui/theme.dart';
import '../../ui/widgets.dart';

class PrsScreen extends ConsumerStatefulWidget {
  const PrsScreen({super.key});

  @override
  ConsumerState<PrsScreen> createState() => _PrsScreenState();
}

class _PrsScreenState extends ConsumerState<PrsScreen> {
  late Future<PullRequestsPayload> _future;
  final _selected = <String>{};
  String? _filterRepo;
  bool _busy = false;

  @override
  void initState() {
    super.initState();
    _future = _load();
  }

  Future<PullRequestsPayload> _load() => ref.read(apiClientProvider).pullRequests(repo: _filterRepo);

  void _refresh() => setState(() {
        _selected.clear();
        _future = _load();
      });

  Future<void> _act(Future<ActionMessage> Function() action, {String? confirmTitle, String? confirmBody}) async {
    if (confirmTitle != null) {
      final ok = await showDialog<bool>(
        context: context,
        builder: (context) => AlertDialog(
          backgroundColor: gitLensSurface,
          title: Text(confirmTitle),
          content: Text(confirmBody ?? ''),
          actions: [
            TextButton(onPressed: () => Navigator.pop(context, false), child: const Text('Cancel')),
            FilledButton(
              onPressed: () => Navigator.pop(context, true),
              style: FilledButton.styleFrom(backgroundColor: gitLensGreen, foregroundColor: gitLensBackground),
              child: const Text('Continue'),
            ),
          ],
        ),
      );
      if (ok != true) return;
    }
    setState(() => _busy = true);
    try {
      final result = await action();
      if (!mounted) return;
      _toast(result.details == null ? result.message : '${result.message} — ${result.details}', ok: result.ok);
      _refresh();
    } catch (e) {
      if (mounted) _toast('$e', ok: false);
    } finally {
      if (mounted) setState(() => _busy = false);
    }
  }

  void _toast(String message, {required bool ok}) {
    ScaffoldMessenger.of(context).showSnackBar(SnackBar(
      content: Text(message),
      backgroundColor: ok ? gitLensSurfaceRaised : const Color(0xFF3D1518),
    ));
  }

  @override
  Widget build(BuildContext context) {
    return FutureBuilder<PullRequestsPayload>(
      future: _future,
      builder: (context, snapshot) {
        if (snapshot.connectionState == ConnectionState.waiting) {
          return ListView(children: List.generate(4, (_) => skeletonCard()));
        }
        if (snapshot.hasError) {
          return ErrorPane(message: '${snapshot.error}', onRetry: _refresh);
        }
        final data = snapshot.data!;
        final repos = data.repos.map((r) => r.fullName).toList()..sort();
        final prs = data.pullRequests;
        return Column(
          children: [
            if (_busy) const LinearProgressIndicator(minHeight: 2),
            Padding(
              padding: const EdgeInsets.fromLTRB(20, 10, 20, 0),
              child: Row(
                children: [
                  Expanded(
                    child: DropdownButtonFormField<String?>(
                      initialValue: _filterRepo,
                      isExpanded: true,
                      decoration: const InputDecoration(isDense: true, prefixIcon: Icon(Icons.filter_list, size: 19)),
                      items: [
                        const DropdownMenuItem<String?>(value: null, child: Text('All repositories')),
                        ...repos.map((repo) => DropdownMenuItem<String?>(value: repo, child: Text(repo, overflow: TextOverflow.ellipsis))),
                      ],
                      onChanged: (value) {
                        setState(() {
                          _filterRepo = value;
                          _selected.clear();
                          _future = _load();
                        });
                      },
                    ),
                  ),
                ],
              ),
            ),
            SectionHeader(
              title: '${prs.length} open pull request${prs.length == 1 ? '' : 's'}',
              subtitle: '${data.passingCount} passing checks',
              trailing: TextButton.icon(
                onPressed: _busy || prs.isEmpty ? null : () => _act(
                      () => ref.read(apiClientProvider).mergePassing(repo: _filterRepo),
                      confirmTitle: 'Merge passing PRs?',
                      confirmBody: 'Merges every PR with passing checks and no merge conflicts in the current filter.',
                    ),
                icon: const Icon(Icons.rocket_launch_outlined, size: 17),
                label: const Text('Merge passing'),
              ),
            ),
            Expanded(
              child: RefreshIndicator(
                onRefresh: () async {
                  _refresh();
                  await _future;
                },
                child: prs.isEmpty
                    ? ListView(
                        physics: const AlwaysScrollableScrollPhysics(),
                        children: const [
                          SizedBox(height: 100),
                          EmptyState(
                            icon: Icons.merge_type,
                            title: 'No open pull requests',
                            message: 'When repositories have open PRs they are collected here for triage.',
                          ),
                        ],
                      )
                    : ListView.builder(
                        physics: const AlwaysScrollableScrollPhysics(),
                        padding: const EdgeInsets.only(bottom: 12),
                        itemCount: prs.length,
                        itemBuilder: (context, index) {
                          final pr = prs[index];
                          return _PrCard(
                            pr: pr,
                            selected: _selected.contains(pr.key),
                            onSelected: (value) => setState(() {
                              if (value == true) {
                                _selected.add(pr.key);
                              } else {
                                _selected.remove(pr.key);
                              }
                            }),
                            busy: _busy,
                            onMerge: () => _act(() => ref.read(apiClientProvider).merge(repoId: pr.repoId, prNumber: pr.number)),
                            onClose: () => _act(
                              () => ref.read(apiClientProvider).close(repoId: pr.repoId, prNumber: pr.number),
                              confirmTitle: 'Close #${pr.number}?',
                              confirmBody: 'This closes the pull request on ${pr.repoFullName}.',
                            ),
                            onRerun: () => _act(() => ref.read(apiClientProvider).rerun(repoId: pr.repoId, prNumber: pr.number)),
                          );
                        },
                      ),
              ),
            ),
            if (_selected.isNotEmpty)
              SafeArea(
                top: false,
                child: Container(
                  padding: const EdgeInsets.fromLTRB(20, 10, 20, 10),
                  decoration: const BoxDecoration(
                    color: gitLensSurface,
                    border: Border(top: BorderSide(color: gitLensBorder)),
                  ),
                  child: Row(
                    children: [
                      Text('${_selected.length} selected', style: const TextStyle(fontWeight: FontWeight.w700)),
                      const Spacer(),
                      TextButton(
                        onPressed: _busy
                            ? null
                            : () => _act(
                                  () => ref.read(apiClientProvider).batchClose(_selected.toList(), repo: _filterRepo),
                                  confirmTitle: 'Close ${_selected.length} PR(s)?',
                                  confirmBody: 'Selected pull requests will be closed on their repositories.',
                                ),
                        child: const Text('Close', style: TextStyle(color: Color(0xFFF85149))),
                      ),
                      const SizedBox(width: 4),
                      FilledButton(
                        onPressed: _busy
                            ? null
                            : () => _act(
                                  () => ref.read(apiClientProvider).batchMerge(_selected.toList(), repo: _filterRepo),
                                  confirmTitle: 'Merge ${_selected.length} PR(s)?',
                                  confirmBody: 'Selected pull requests will be merged immediately.',
                                ),
                        style: FilledButton.styleFrom(backgroundColor: gitLensGreen, foregroundColor: gitLensBackground),
                        child: const Text('Merge'),
                      ),
                    ],
                  ),
                ),
              ),
          ],
        );
      },
    );
  }
}

class _PrCard extends StatelessWidget {
  const _PrCard({
    required this.pr,
    required this.selected,
    required this.onSelected,
    required this.busy,
    required this.onMerge,
    required this.onClose,
    required this.onRerun,
  });

  final PullRequest pr;
  final bool selected;
  final ValueChanged<bool?> onSelected;
  final bool busy;
  final VoidCallback onMerge;
  final VoidCallback onClose;
  final VoidCallback onRerun;

  Color get _buildColor {
    if (pr.buildPassing) return gitLensGreen;
    if (pr.buildFailing) return const Color(0xFFF85149);
    if (pr.buildStatus == 'in_progress' || pr.buildStatus == 'pending') return const Color(0xFFD29922);
    return gitLensMuted;
  }

  String get _buildLabel {
    if (pr.buildPassing) return 'Passing';
    if (pr.buildFailing) return 'Failing';
    if (pr.buildStatus == 'in_progress' || pr.buildStatus == 'pending') return 'Running';
    return 'No checks';
  }

  @override
  Widget build(BuildContext context) {
    final borderColor = selected ? gitLensGreen.withValues(alpha: 0.65) : gitLensBorder;
    return Container(
      margin: const EdgeInsets.symmetric(horizontal: 20, vertical: 5),
      decoration: BoxDecoration(
        color: gitLensSurface,
        borderRadius: BorderRadius.circular(16),
        border: Border.all(color: borderColor),
      ),
      child: Padding(
        padding: const EdgeInsets.fromLTRB(8, 12, 12, 10),
        child: Row(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            Checkbox(
              value: selected,
              onChanged: busy ? null : onSelected,
              activeColor: gitLensGreen,
              checkColor: gitLensBackground,
              side: const BorderSide(color: gitLensMuted),
            ),
            Expanded(
              child: Column(
                crossAxisAlignment: CrossAxisAlignment.start,
                children: [
                  Row(
                    children: [
                      Expanded(
                        child: Text(pr.repoFullName,
                            maxLines: 1,
                            overflow: TextOverflow.ellipsis,
                            style: const TextStyle(fontSize: 11.5, fontWeight: FontWeight.w700, color: gitLensBlue)),
                      ),
                      Text(relativeTime(DateTime.tryParse(pr.createdAt)),
                          style: const TextStyle(fontSize: 10.5, color: gitLensMuted)),
                    ],
                  ),
                  const SizedBox(height: 4),
                  Text('#${pr.number} ${pr.title}',
                      maxLines: 2,
                      overflow: TextOverflow.ellipsis,
                      style: const TextStyle(fontSize: 14.5, fontWeight: FontWeight.w700, height: 1.3, letterSpacing: -0.15)),
                  const SizedBox(height: 5),
                  Text('by ${pr.author} · ${pr.headRef} → ${pr.baseRef}',
                      maxLines: 1,
                      overflow: TextOverflow.ellipsis,
                      style: const TextStyle(fontSize: 10.5, color: gitLensMuted)),
                  const SizedBox(height: 9),
                  Wrap(
                    spacing: 6,
                    runSpacing: 6,
                    children: [
                      StatusPill(
                        label: _buildLabel,
                        color: _buildColor,
                        icon: pr.buildPassing ? Icons.check_circle : pr.buildFailing ? Icons.error : Icons.pending_outlined,
                      ),
                      if (pr.mergeableState.isNotEmpty)
                        StatusPill(
                          label: pr.mergeableState == 'clean' ? 'Mergeable' : pr.mergeableState,
                          color: pr.mergeable ? gitLensGreen : const Color(0xFFD29922),
                        ),
                    ],
                  ),
                ],
              ),
            ),
            PopupMenuButton<String>(
              enabled: !busy,
              tooltip: 'PR actions',
              color: gitLensSurfaceRaised,
              onSelected: (value) {
                switch (value) {
                  case 'merge':
                    onMerge();
                    break;
                  case 'rerun':
                    onRerun();
                    break;
                  case 'close':
                    onClose();
                    break;
                }
              },
              itemBuilder: (context) => const [
                PopupMenuItem(value: 'merge', child: ListTile(leading: Icon(Icons.merge_type, color: gitLensGreen), title: Text('Merge'))),
                PopupMenuItem(value: 'rerun', child: ListTile(leading: Icon(Icons.replay, color: gitLensBlue), title: Text('Re-run failed builds'))),
                PopupMenuItem(value: 'close', child: ListTile(leading: Icon(Icons.close, color: Color(0xFFF85149)), title: Text('Close PR'))),
              ],
              icon: const Icon(Icons.more_vert, size: 18, color: gitLensMuted),
            ),
          ],
        ),
      ),
    );
  }
}
