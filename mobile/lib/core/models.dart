class AppUser {
  const AppUser({required this.id, required this.username, required this.name, required this.avatarUrl, required this.role});

  final int id;
  final String username;
  final String name;
  final String avatarUrl;
  final String role;

  factory AppUser.fromJson(Map<String, dynamic> json) => AppUser(
        id: (json['id'] as num?)?.toInt() ?? 0,
        username: json['username'] as String? ?? '',
        name: json['name'] as String? ?? '',
        avatarUrl: json['avatar_url'] as String? ?? '',
        role: json['role'] as String? ?? 'user',
      );
}

class PullRequestSummary {
  const PullRequestSummary({
    required this.number,
    required this.title,
    required this.author,
    required this.createdAt,
    required this.htmlUrl,
    required this.headRef,
    required this.baseRef,
    required this.mergeableState,
    required this.buildStatus,
  });

  final int number;
  final String title;
  final String author;
  final String createdAt;
  final String htmlUrl;
  final String headRef;
  final String baseRef;
  final String mergeableState;
  final String buildStatus;

  factory PullRequestSummary.fromJson(Map<String, dynamic> json) => PullRequestSummary(
        number: (json['n'] as num?)?.toInt() ?? 0,
        title: json['t'] as String? ?? '',
        author: json['a'] as String? ?? '',
        createdAt: json['c'] as String? ?? '',
        htmlUrl: json['h'] as String? ?? '',
        headRef: json['hr'] as String? ?? '',
        baseRef: json['br'] as String? ?? '',
        mergeableState: json['ms'] as String? ?? '',
        buildStatus: json['bs'] as String? ?? '',
      );
}

class Repository {
  const Repository({
    required this.id,
    required this.owner,
    required this.name,
    required this.fullName,
    required this.description,
    required this.htmlUrl,
    required this.language,
    required this.provider,
    required this.workflowStatus,
    required this.latestCommitMessage,
    required this.latestReleaseTag,
    required this.releaseCount,
    required this.featCount,
    required this.fixCount,
    required this.docsCount,
    required this.choreCount,
    required this.otherCommitCount,
    required this.openPrCount,
    required this.pullRequests,
    required this.syncedAt,
    required this.backfillStatus,
  });

  final int id;
  final String owner;
  final String name;
  final String fullName;
  final String description;
  final String htmlUrl;
  final String language;
  final String provider;
  final String workflowStatus;
  final String latestCommitMessage;
  final String latestReleaseTag;
  final int releaseCount;
  final int featCount;
  final int fixCount;
  final int docsCount;
  final int choreCount;
  final int otherCommitCount;
  final int openPrCount;
  final List<PullRequestSummary> pullRequests;
  final DateTime? syncedAt;
  final String backfillStatus;

  int get totalCommits => featCount + fixCount + docsCount + choreCount + otherCommitCount;

  factory Repository.fromJson(Map<String, dynamic> json) {
    final rawPrs = json['pull_requests'];
    return Repository(
      id: (json['id'] as num?)?.toInt() ?? 0,
      owner: json['owner'] as String? ?? '',
      name: json['name'] as String? ?? '',
      fullName: json['full_name'] as String? ?? '',
      description: json['description'] as String? ?? '',
      htmlUrl: json['html_url'] as String? ?? '',
      language: json['language'] as String? ?? '',
      provider: json['provider'] as String? ?? '',
      workflowStatus: json['workflow_status'] as String? ?? '',
      latestCommitMessage: json['latest_commit_message'] as String? ?? '',
      latestReleaseTag: json['latest_release_tag'] as String? ?? '',
      releaseCount: (json['release_count'] as num?)?.toInt() ?? 0,
      featCount: (json['feat_count'] as num?)?.toInt() ?? 0,
      fixCount: (json['fix_count'] as num?)?.toInt() ?? 0,
      docsCount: (json['docs_count'] as num?)?.toInt() ?? 0,
      choreCount: (json['chore_count'] as num?)?.toInt() ?? 0,
      otherCommitCount: (json['other_commit_count'] as num?)?.toInt() ?? 0,
      openPrCount: (json['open_pr_count'] as num?)?.toInt() ?? 0,
      pullRequests: rawPrs is List
          ? rawPrs.whereType<Map>().map((e) => PullRequestSummary.fromJson(e.cast<String, dynamic>())).toList()
          : const [],
      syncedAt: DateTime.tryParse(json['synced_at'] as String? ?? ''),
      backfillStatus: json['backfill_status'] as String? ?? '',
    );
  }
}

class RepoOverview {
  const RepoOverview({required this.totalRepos, required this.openPrs, required this.failingRepos, required this.totalReleases, required this.lastSyncTime});

  final int totalRepos;
  final int openPrs;
  final int failingRepos;
  final int totalReleases;
  final String lastSyncTime;

  factory RepoOverview.fromJson(Map<String, dynamic> json) => RepoOverview(
        totalRepos: (json['total_repos'] as num?)?.toInt() ?? 0,
        openPrs: (json['open_prs'] as num?)?.toInt() ?? 0,
        failingRepos: (json['failing_repos'] as num?)?.toInt() ?? 0,
        totalReleases: (json['total_releases'] as num?)?.toInt() ?? 0,
        lastSyncTime: json['last_sync_time'] as String? ?? '',
      );
}

class ReposPayload {
  const ReposPayload({required this.repos});

  final List<Repository> repos;

  factory ReposPayload.fromJson(Map<String, dynamic> json) => ReposPayload(
        repos: (json['repos'] as List? ?? []).whereType<Map>().map((e) => Repository.fromJson(e.cast<String, dynamic>())).toList(),
      );
}

class DashboardPayload {
  const DashboardPayload({required this.repos, required this.overview});

  final List<Repository> repos;
  final RepoOverview overview;

  factory DashboardPayload.fromJson(Map<String, dynamic> json) => DashboardPayload(
        repos: (json['repos'] as List? ?? []).whereType<Map>().map((e) => Repository.fromJson(e.cast<String, dynamic>())).toList(),
        overview: RepoOverview.fromJson((json['overview'] as Map? ?? {}).cast<String, dynamic>()),
      );
}

class PullRequest {
  const PullRequest({
    required this.repoId,
    required this.repoFullName,
    required this.number,
    required this.title,
    required this.author,
    required this.createdAt,
    required this.htmlUrl,
    required this.headRef,
    required this.baseRef,
    required this.mergeableState,
    required this.buildStatus,
  });

  final int repoId;
  final String repoFullName;
  final int number;
  final String title;
  final String author;
  final String createdAt;
  final String htmlUrl;
  final String headRef;
  final String baseRef;
  final String mergeableState;
  final String buildStatus;

  String get key => '$repoId:$number';
  int get id => key.hashCode;

  bool get buildPassing => buildStatus == 'success';
  bool get buildFailing => buildStatus == 'failure';
  bool get mergeable => mergeableState != 'dirty' && mergeableState != 'blocked';

  factory PullRequest.fromJson(Map<String, dynamic> json) => PullRequest(
        repoId: (json['repo_id'] as num?)?.toInt() ?? 0,
        repoFullName: json['repo_full_name'] as String? ?? '',
        number: (json['number'] as num?)?.toInt() ?? 0,
        title: json['title'] as String? ?? '',
        author: json['author'] as String? ?? '',
        createdAt: json['created_at'] as String? ?? '',
        htmlUrl: json['html_url'] as String? ?? '',
        headRef: json['head_ref'] as String? ?? '',
        baseRef: json['base_ref'] as String? ?? '',
        mergeableState: json['mergeable_state'] as String? ?? '',
        buildStatus: json['build_status'] as String? ?? '',
      );
}

class PullRequestsPayload {
  const PullRequestsPayload({required this.pullRequests, required this.repos, required this.passingCount});

  final List<PullRequest> pullRequests;
  final List<Repository> repos;
  final int passingCount;

  factory PullRequestsPayload.fromJson(Map<String, dynamic> json) => PullRequestsPayload(
        pullRequests: (json['prs'] as List? ?? []).whereType<Map>().map((e) => PullRequest.fromJson(e.cast<String, dynamic>())).toList(),
        repos: (json['repos'] as List? ?? []).whereType<Map>().map((e) => Repository.fromJson(e.cast<String, dynamic>())).toList(),
        passingCount: (json['passing_count'] as num?)?.toInt() ?? 0,
      );
}

class DoraMetrics {
  const DoraMetrics({
    required this.totalRepos,
    required this.totalReleases,
    required this.totalCommits,
    required this.featCount,
    required this.fixCount,
    required this.docsCount,
    required this.choreCount,
    required this.otherCount,
    required this.workflowPassRate,
    required this.avgLeadTimeHours,
    required this.featPct,
    required this.fixPct,
    required this.docsPct,
    required this.chorePct,
    required this.releasesPerRepo,
  });

  final int totalRepos;
  final int totalReleases;
  final int totalCommits;
  final int featCount;
  final int fixCount;
  final int docsCount;
  final int choreCount;
  final int otherCount;
  final double workflowPassRate;
  final double avgLeadTimeHours;
  final double featPct;
  final double fixPct;
  final double docsPct;
  final double chorePct;
  final double releasesPerRepo;

  factory DoraMetrics.fromJson(Map<String, dynamic> json) => DoraMetrics(
        totalRepos: (json['totalRepos'] as num?)?.toInt() ?? 0,
        totalReleases: (json['totalReleases'] as num?)?.toInt() ?? 0,
        totalCommits: (json['totalCommits'] as num?)?.toInt() ?? 0,
        featCount: (json['featCount'] as num?)?.toInt() ?? 0,
        fixCount: (json['fixCount'] as num?)?.toInt() ?? 0,
        docsCount: (json['docsCount'] as num?)?.toInt() ?? 0,
        choreCount: (json['choreCount'] as num?)?.toInt() ?? 0,
        otherCount: (json['otherCount'] as num?)?.toInt() ?? 0,
        workflowPassRate: (json['workflowPassRate'] as num?)?.toDouble() ?? 0,
        avgLeadTimeHours: (json['avgLeadTimeHours'] as num?)?.toDouble() ?? 0,
        featPct: (json['featPct'] as num?)?.toDouble() ?? 0,
        fixPct: (json['fixPct'] as num?)?.toDouble() ?? 0,
        docsPct: (json['docsPct'] as num?)?.toDouble() ?? 0,
        chorePct: (json['chorePct'] as num?)?.toDouble() ?? 0,
        releasesPerRepo: (json['releasesPerRepo'] as num?)?.toDouble() ?? 0,
      );
}

class MetricsPayload {
  const MetricsPayload({required this.metrics});

  final DoraMetrics metrics;

  factory MetricsPayload.fromJson(Map<String, dynamic> json) => MetricsPayload(
        metrics: DoraMetrics.fromJson((json['metrics'] as Map? ?? {}).cast<String, dynamic>()),
      );
}

class MetricsSnapshot {
  const MetricsSnapshot({required this.timestamp, required this.repoName, required this.totalCommits, required this.releaseCount, required this.avgLeadTimeHours, required this.workflowStatus});

  final DateTime? timestamp;
  final String repoName;
  final int totalCommits;
  final int releaseCount;
  final double avgLeadTimeHours;
  final String workflowStatus;

  factory MetricsSnapshot.fromJson(Map<String, dynamic> json) => MetricsSnapshot(
        timestamp: DateTime.tryParse(json['timestamp'] as String? ?? ''),
        repoName: json['repo_name'] as String? ?? '',
        totalCommits: (json['total_commits_fetched'] as num?)?.toInt() ?? 0,
        releaseCount: (json['release_count'] as num?)?.toInt() ?? 0,
        avgLeadTimeHours: (json['avg_lead_time_hours'] as num?)?.toDouble() ?? 0,
        workflowStatus: json['workflow_status'] as String? ?? '',
      );
}

class FeedEvent {
  const FeedEvent({required this.type, required this.title, required this.url, required this.timestamp, required this.repoFull, required this.metadata});

  final String type;
  final String title;
  final String url;
  final DateTime? timestamp;
  final String repoFull;
  final String metadata;

  factory FeedEvent.fromJson(Map<String, dynamic> json) => FeedEvent(
        type: json['type'] as String? ?? '',
        title: json['title'] as String? ?? '',
        url: json['url'] as String? ?? '',
        timestamp: DateTime.tryParse(json['timestamp'] as String? ?? ''),
        repoFull: json['repo_full'] as String? ?? '',
        metadata: json['metadata'] as String? ?? '',
      );
}

class FeedGroup {
  const FeedGroup({required this.date, required this.events});

  final String date;
  final List<FeedEvent> events;

  factory FeedGroup.fromJson(Map<String, dynamic> json) => FeedGroup(
        date: json['date'] as String? ?? '',
        events: (json['events'] as List? ?? []).whereType<Map>().map((e) => FeedEvent.fromJson(e.cast<String, dynamic>())).toList(),
      );
}

class FeedPayload {
  const FeedPayload({required this.groups, required this.count, required this.since, required this.types});

  final List<FeedGroup> groups;
  final int count;
  final String since;
  final String types;

  factory FeedPayload.fromJson(Map<String, dynamic> json) => FeedPayload(
        groups: (json['groups'] as List? ?? []).whereType<Map>().map((e) => FeedGroup.fromJson(e.cast<String, dynamic>())).toList(),
        count: (json['count'] as num?)?.toInt() ?? 0,
        since: json['since'] as String? ?? '7d',
        types: json['types'] as String? ?? 'all',
      );
}

class YearStats {
  const YearStats({required this.year, required this.totalCommits, required this.activeDays, required this.longestStreak, required this.currentStreak, required this.monthlyTotals});

  final int year;
  final int totalCommits;
  final int activeDays;
  final int longestStreak;
  final int currentStreak;
  final List<int> monthlyTotals;

  factory YearStats.fromJson(Map<String, dynamic> json) => YearStats(
        year: (json['year'] as num?)?.toInt() ?? 0,
        totalCommits: (json['total_commits'] as num?)?.toInt() ?? 0,
        activeDays: (json['active_days'] as num?)?.toInt() ?? 0,
        longestStreak: (json['longest_streak'] as num?)?.toInt() ?? 0,
        currentStreak: (json['current_streak'] as num?)?.toInt() ?? 0,
        monthlyTotals: (json['monthly_totals'] as List? ?? []).map((e) => (e as num).toInt()).toList(),
      );
}

class ActionMessage {
  const ActionMessage({required this.ok, required this.message, this.details, this.failed});

  final bool ok;
  final String message;
  final String? details;
  final List<String>? failed;

  factory ActionMessage.fromJson(Map<String, dynamic> json) => ActionMessage(
        ok: json['ok'] as bool? ?? json['error'] == null,
        message: json['message'] as String? ?? json['error'] as String? ?? 'Done',
        details: json['details'] as String?,
        failed: (json['failed'] as List?)?.map((e) => '$e').toList(),
      );
}
