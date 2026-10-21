import 'package:fl_chart/fl_chart.dart';
import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../../core/auth.dart';
import '../../core/models.dart';
import '../../ui/theme.dart';
import '../../ui/widgets.dart';

class MetricsBundle {
  const MetricsBundle({required this.metrics, required this.history, required this.year});

  final DoraMetrics metrics;
  final List<MetricsSnapshot> history;
  final YearStats? year;
}

class MetricsScreen extends ConsumerStatefulWidget {
  const MetricsScreen({super.key});

  @override
  ConsumerState<MetricsScreen> createState() => _MetricsScreenState();
}

class _MetricsScreenState extends ConsumerState<MetricsScreen> {
  late Future<MetricsBundle> _future;

  @override
  void initState() {
    super.initState();
    _future = _load();
  }

  Future<MetricsBundle> _load() async {
    final api = ref.read(apiClientProvider);
    final results = await Future.wait<Object?>([
      api.metrics(),
      api.metricsHistory(),
      api.yearStats(),
    ]);
    return MetricsBundle(
      metrics: (results[0] as MetricsPayload).metrics,
      history: results[1] as List<MetricsSnapshot>,
      year: results[2] as YearStats?,
    );
  }

  void _refresh() => setState(() => _future = _load());

  @override
  Widget build(BuildContext context) {
    return FutureBuilder<MetricsBundle>(
      future: _future,
      builder: (context, snapshot) {
        if (snapshot.connectionState == ConnectionState.waiting) {
          return ListView(children: List.generate(4, (_) => skeletonCard()));
        }
        if (snapshot.hasError) {
          return ErrorPane(message: '${snapshot.error}', onRetry: _refresh);
        }
        final bundle = snapshot.data!;
        final m = bundle.metrics;
        return RefreshIndicator(
          onRefresh: () async {
            _refresh();
            await _future;
          },
          child: ListView(
            physics: const AlwaysScrollableScrollPhysics(),
            padding: const EdgeInsets.only(bottom: 30),
            children: [
              _StatsGrid(metrics: m),
              SectionHeader(title: 'Commit mix', subtitle: '${m.totalCommits} commits across ${m.totalRepos} repositories'),
              if (m.totalCommits == 0)
                const Padding(padding: EdgeInsets.all(24), child: Text('No commit data yet.', style: TextStyle(color: gitLensMuted)))
              else
                _CommitMixChart(metrics: m),
              SectionHeader(title: 'Lead time trend', subtitle: 'Average time from commit to release'),
              _LeadTimeChart(history: bundle.history),
              if (bundle.year != null) ...[
                SectionHeader(title: '${bundle.year!.year} in review', subtitle: '${bundle.year!.totalCommits} commits · ${bundle.year!.activeDays} active days'),
                _YearCard(stats: bundle.year!),
              ],
            ],
          ),
        );
      },
    );
  }
}

class _StatsGrid extends StatelessWidget {
  const _StatsGrid({required this.metrics});

  final DoraMetrics metrics;

  @override
  Widget build(BuildContext context) {
    final cards = [
      ('Repositories', '${metrics.totalRepos}', Icons.folder_outlined, gitLensBlue),
      ('Releases', '${metrics.totalReleases}', Icons.new_releases_outlined, const Color(0xFFD2A8FF)),
      ('Commits', '${metrics.totalCommits}', Icons.commit, gitLensGreen),
      ('Pass rate', '${metrics.workflowPassRate.toStringAsFixed(0)}%', Icons.verified_outlined, gitLensGreen),
      ('Avg lead time', '${metrics.avgLeadTimeHours.toStringAsFixed(1)}h', Icons.timer_outlined, const Color(0xFFD29922)),
      ('Releases / repo', metrics.releasesPerRepo.toStringAsFixed(1), Icons.bar_chart, gitLensBlue),
    ];
    return GridView.count(
      crossAxisCount: 2,
      shrinkWrap: true,
      physics: const NeverScrollableScrollPhysics(),
      padding: const EdgeInsets.fromLTRB(20, 12, 20, 0),
      crossAxisSpacing: 10,
      mainAxisSpacing: 10,
      childAspectRatio: 2.35,
      children: cards.map((card) {
        return Container(
          padding: const EdgeInsets.all(13),
          decoration: BoxDecoration(
            color: gitLensSurface,
            borderRadius: BorderRadius.circular(14),
            border: Border.all(color: gitLensBorder),
          ),
          child: Row(
            children: [
              Container(
                padding: const EdgeInsets.all(8),
                decoration: BoxDecoration(color: card.$4.withValues(alpha: 0.12), borderRadius: BorderRadius.circular(10)),
                child: Icon(card.$3, size: 17, color: card.$4),
              ),
              const SizedBox(width: 10),
              Expanded(
                child: Column(
                  crossAxisAlignment: CrossAxisAlignment.start,
                  mainAxisAlignment: MainAxisAlignment.center,
                  children: [
                    Text(card.$2, style: TextStyle(fontSize: 19, fontWeight: FontWeight.w900, color: card.$4, letterSpacing: -0.5)),
                    Text(card.$1, maxLines: 1, overflow: TextOverflow.ellipsis, style: const TextStyle(fontSize: 10, color: gitLensMuted)),
                  ],
                ),
              ),
            ],
          ),
        );
      }).toList(),
    );
  }
}

class _CommitMixChart extends StatelessWidget {
  const _CommitMixChart({required this.metrics});

  final DoraMetrics metrics;

  @override
  Widget build(BuildContext context) {
    final sections = [
      ('Features', metrics.featCount, gitLensGreen),
      ('Fixes', metrics.fixCount, const Color(0xFFF85149)),
      ('Docs', metrics.docsCount, gitLensBlue),
      ('Chores', metrics.choreCount, const Color(0xFFD29922)),
      ('Other', metrics.otherCount, gitLensMuted),
    ].where((e) => e.$2 > 0).toList();

    return Card(
      margin: const EdgeInsets.symmetric(horizontal: 20),
      child: Padding(
        padding: const EdgeInsets.all(18),
        child: Row(
          children: [
            SizedBox(
              width: 132,
              height: 132,
              child: PieChart(
                PieChartData(
                  sectionsSpace: 3,
                  centerSpaceRadius: 30,
                  startDegreeOffset: -90,
                  sections: sections
                      .map((s) => PieChartSectionData(
                            value: s.$2.toDouble(),
                            color: s.$3,
                            radius: 24,
                            showTitle: false,
                          ))
                      .toList(),
                ),
              ),
            ),
            const SizedBox(width: 18),
            Expanded(
              child: Column(
                mainAxisSize: MainAxisSize.min,
                children: sections.map((s) {
                  final pct = metrics.totalCommits == 0 ? 0 : s.$2 / metrics.totalCommits * 100;
                  return Padding(
                    padding: const EdgeInsets.symmetric(vertical: 4),
                    child: Row(
                      children: [
                        Container(width: 8, height: 8, decoration: BoxDecoration(color: s.$3, borderRadius: BorderRadius.circular(2))),
                        const SizedBox(width: 8),
                        Expanded(child: Text(s.$1, style: const TextStyle(fontSize: 12, color: gitLensMuted))),
                        Text('${pct.toStringAsFixed(0)}%', style: const TextStyle(fontSize: 12, fontWeight: FontWeight.w700)),
                        const SizedBox(width: 6),
                        SizedBox(width: 34, child: Text('${s.$2}', textAlign: TextAlign.right, style: const TextStyle(fontSize: 11, color: gitLensMuted))),
                      ],
                    ),
                  );
                }).toList(),
              ),
            ),
          ],
        ),
      ),
    );
  }
}

class _LeadTimeChart extends StatelessWidget {
  const _LeadTimeChart({required this.history});

  final List<MetricsSnapshot> history;

  @override
  Widget build(BuildContext context) {
    final points = history.where((s) => s.avgLeadTimeHours > 0).toList();
    // Keep the most recent snapshots and show them oldest -> newest.
    final recent = points.length > 20 ? points.sublist(points.length - 20) : points;
    if (recent.isEmpty) {
      return const Card(
        margin: EdgeInsets.symmetric(horizontal: 20),
        child: Padding(
          padding: EdgeInsets.all(24),
          child: Text('Lead time snapshots appear after GitLens has synced releases.', style: TextStyle(color: gitLensMuted, height: 1.4)),
        ),
      );
    }
    final maxValue = recent.map((e) => e.avgLeadTimeHours).reduce((a, b) => a > b ? a : b);
    return Card(
      margin: const EdgeInsets.symmetric(horizontal: 20),
      child: Padding(
        padding: const EdgeInsets.fromLTRB(12, 20, 20, 12),
        child: SizedBox(
          height: 170,
          child: LineChart(
            LineChartData(
              minX: 0,
              maxX: (recent.length - 1).toDouble(),
              minY: 0,
              maxY: (maxValue * 1.2).ceilToDouble(),
              gridData: FlGridData(
                show: true,
                drawVerticalLine: false,
                getDrawingHorizontalLine: (value) => const FlLine(color: gitLensBorder, strokeWidth: 1),
              ),
              borderData: FlBorderData(show: false),
              titlesData: FlTitlesData(
                topTitles: const AxisTitles(sideTitles: SideTitles(showTitles: false)),
                rightTitles: const AxisTitles(sideTitles: SideTitles(showTitles: false)),
                bottomTitles: const AxisTitles(sideTitles: SideTitles(showTitles: false)),
                leftTitles: AxisTitles(
                  sideTitles: SideTitles(
                    showTitles: true,
                    reservedSize: 34,
                    getTitlesWidget: (value, meta) => Text('${value.toInt()}h', style: const TextStyle(fontSize: 9, color: gitLensMuted)),
                  ),
                ),
              ),
              lineBarsData: [
                LineChartBarData(
                  spots: [
                    for (var i = 0; i < recent.length; i++) FlSpot(i.toDouble(), recent[i].avgLeadTimeHours),
                  ],
                  isCurved: true,
                  curveSmoothness: 0.25,
                  color: gitLensGreen,
                  barWidth: 2.5,
                  isStrokeCapRound: true,
                  dotData: const FlDotData(show: false),
                  belowBarData: BarAreaData(show: true, gradient: LinearGradient(begin: Alignment.topCenter, end: Alignment.bottomCenter, colors: [gitLensGreen.withValues(alpha: 0.22), Colors.transparent])),
                ),
              ],
            ),
          ),
        ),
      ),
    );
  }
}

class _YearCard extends StatelessWidget {
  const _YearCard({required this.stats});

  final YearStats stats;

  @override
  Widget build(BuildContext context) {
    final months = ['J', 'F', 'M', 'A', 'M', 'J', 'J', 'A', 'S', 'O', 'N', 'D'];
    final totals = stats.monthlyTotals.length == 12 ? stats.monthlyTotals : List<int>.filled(12, 0);
    final max = totals.fold<int>(1, (a, b) => b > a ? b : a);
    return Card(
      margin: const EdgeInsets.symmetric(horizontal: 20),
      child: Padding(
        padding: const EdgeInsets.all(18),
        child: Column(
          children: [
            Row(
              children: [
                Expanded(child: _YearStat(label: 'Commits', value: '${stats.totalCommits}')),
                Expanded(child: _YearStat(label: 'Active days', value: '${stats.activeDays}')),
                Expanded(child: _YearStat(label: 'Longest streak', value: '${stats.longestStreak}')),
              ],
            ),
            const Divider(height: 26),
            SizedBox(
              height: 86,
              child: Row(
                crossAxisAlignment: CrossAxisAlignment.end,
                children: List.generate(12, (index) {
                  final height = 8 + (totals[index] / max * 58);
                  return Expanded(
                    child: Padding(
                      padding: const EdgeInsets.symmetric(horizontal: 2),
                      child: Column(
                        mainAxisAlignment: MainAxisAlignment.end,
                        children: [
                          Container(
                            height: height,
                            decoration: BoxDecoration(
                              color: totals[index] == 0 ? gitLensBorder : gitLensGreen.withValues(alpha: 0.35 + (totals[index] / max) * 0.65),
                              borderRadius: BorderRadius.circular(3),
                            ),
                          ),
                          const SizedBox(height: 5),
                          Text(months[index], style: const TextStyle(fontSize: 8.5, color: gitLensMuted)),
                        ],
                      ),
                    ),
                  );
                }),
              ),
            ),
          ],
        ),
      ),
    );
  }
}

class _YearStat extends StatelessWidget {
  const _YearStat({required this.label, required this.value});

  final String label;
  final String value;

  @override
  Widget build(BuildContext context) {
    return Column(children: [
      Text(value, style: const TextStyle(fontSize: 18, fontWeight: FontWeight.w900, color: gitLensBlue)),
      Text(label, style: const TextStyle(fontSize: 10, color: gitLensMuted)),
    ]);
  }
}
