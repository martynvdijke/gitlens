import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

import 'core/api_client.dart';
import 'core/auth.dart';
import 'features/feed/feed_screen.dart';
import 'features/metrics/metrics_screen.dart';
import 'features/onboarding/onboarding_screen.dart';
import 'features/prs/prs_screen.dart';
import 'features/repos/repos_screen.dart';
import 'ui/brand.dart';
import 'ui/theme.dart';
import 'ui/widgets.dart';

class GitLensApp extends ConsumerWidget {
  const GitLensApp({super.key});

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final auth = ref.watch(authProvider);
    return MaterialApp(
      title: 'GitLens',
      debugShowCheckedModeBanner: false,
      theme: buildGitLensTheme(),
      home: auth.loading
          ? const _LaunchScreen()
          : auth.signedIn
              ? const HomeShell()
              : const OnboardingScreen(),
    );
  }
}

class _LaunchScreen extends StatelessWidget {
  const _LaunchScreen();

  @override
  Widget build(BuildContext context) {
    return const Scaffold(
      body: Center(
        child: Column(
          mainAxisSize: MainAxisSize.min,
          children: [
            GitLensMark(size: 76),
            SizedBox(height: 22),
            SizedBox(width: 22, height: 22, child: CircularProgressIndicator(strokeWidth: 2)),
          ],
        ),
      ),
    );
  }
}

class HomeShell extends ConsumerStatefulWidget {
  const HomeShell({super.key});

  @override
  ConsumerState<HomeShell> createState() => _HomeShellState();
}

class _HomeShellState extends ConsumerState<HomeShell> {
  int _index = 0;

  static const _titles = ['Repositories', 'Pull requests', 'Activity', 'Metrics'];

  @override
  Widget build(BuildContext context) {
    final pages = const [ReposScreen(), PrsScreen(), FeedScreen(), MetricsScreen()];
    return Scaffold(
      appBar: AppBar(
        title: Row(
          children: [
            const GitLensMark(size: 28),
            const SizedBox(width: 10),
            Text(_titles[_index]),
          ],
        ),
        actions: [
          IconButton(
            tooltip: 'Settings',
            onPressed: () => _showSettings(context, ref),
            icon: const Icon(Icons.account_circle_outlined),
          ),
        ],
      ),
      body: BrandBackdrop(child: IndexedStack(index: _index, children: pages)),
      bottomNavigationBar: NavigationBar(
        selectedIndex: _index,
        onDestinationSelected: (value) => setState(() => _index = value),
        destinations: const [
          NavigationDestination(icon: Icon(Icons.folder_outlined), selectedIcon: Icon(Icons.folder), label: 'Repos'),
          NavigationDestination(icon: Icon(Icons.merge_type_outlined), selectedIcon: Icon(Icons.merge_type), label: 'PRs'),
          NavigationDestination(icon: Icon(Icons.bolt_outlined), selectedIcon: Icon(Icons.bolt), label: 'Activity'),
          NavigationDestination(icon: Icon(Icons.insights_outlined), selectedIcon: Icon(Icons.insights), label: 'Metrics'),
        ],
      ),
    );
  }

  Future<void> _showSettings(BuildContext context, WidgetRef ref) async {
    final auth = ref.read(authProvider);
    final nameController = TextEditingController(text: 'Android app');
    final confirmController = TextEditingController();
    final username = auth.user?.username ?? '';
    const danger = Color(0xFFF85149);
    String? createdToken;
    String? deleteError;
    var deleting = false;

    Future<void> confirmDelete(BuildContext sheetContext, StateSetter setSheetState) async {
      final messenger = ScaffoldMessenger.of(sheetContext);
      final confirm = confirmController.text.trim();
      if (confirm.isEmpty || confirm != username) {
        setSheetState(() => deleteError = 'Type your username exactly to confirm.');
        return;
      }
      final approved = await showDialog<bool>(
        context: sheetContext,
        builder: (dialogContext) => AlertDialog(
          backgroundColor: gitLensSurface,
          title: const Text('Delete account permanently?'),
          content: const Text('This cannot be undone.'),
          actions: [
            TextButton(
              onPressed: () => Navigator.of(dialogContext).pop(false),
              child: const Text('Cancel'),
            ),
            FilledButton(
              style: FilledButton.styleFrom(backgroundColor: danger, foregroundColor: Colors.white),
              onPressed: () => Navigator.of(dialogContext).pop(true),
              child: const Text('Delete'),
            ),
          ],
        ),
      );
      if (approved != true || !sheetContext.mounted) return;
      setSheetState(() {
        deleting = true;
        deleteError = null;
      });
      try {
        await ref.read(apiClientProvider).deleteAccount(confirm);
        if (sheetContext.mounted) Navigator.of(sheetContext).pop();
        await ref.read(authProvider.notifier).logout();
        messenger.showSnackBar(const SnackBar(content: Text('Account deleted.')));
      } on ApiException catch (e) {
        if (!sheetContext.mounted) return;
        setSheetState(() {
          deleting = false;
          deleteError = e.message;
        });
      } catch (_) {
        if (!sheetContext.mounted) return;
        setSheetState(() {
          deleting = false;
          deleteError = 'Could not delete the account. Try again.';
        });
      }
    }

    await showModalBottomSheet<void>(
      context: context,
      isScrollControlled: true,
      backgroundColor: gitLensSurface,
      shape: const RoundedRectangleBorder(borderRadius: BorderRadius.vertical(top: Radius.circular(24))),
      builder: (sheetContext) => StatefulBuilder(
        builder: (context, setSheetState) => SingleChildScrollView(
          keyboardDismissBehavior: ScrollViewKeyboardDismissBehavior.onDrag,
          padding: EdgeInsets.fromLTRB(22, 18, 22, 24 + MediaQuery.of(context).viewInsets.bottom),
          child: Column(
            mainAxisSize: MainAxisSize.min,
            crossAxisAlignment: CrossAxisAlignment.stretch,
            children: [
              Center(
                child: Container(
                  width: 38,
                  height: 4,
                  decoration: BoxDecoration(color: gitLensBorder, borderRadius: BorderRadius.circular(99)),
                ),
              ),
              const SizedBox(height: 18),
              Row(
                children: [
                  const GitLensMark(size: 42),
                  const SizedBox(width: 12),
                  Expanded(
                    child: Column(
                      crossAxisAlignment: CrossAxisAlignment.start,
                      children: [
                        Text(auth.user?.name.isNotEmpty == true ? auth.user!.name : auth.user?.username ?? 'GitLens',
                            style: const TextStyle(fontSize: 18, fontWeight: FontWeight.w800)),
                        Text(auth.baseUrl ?? '', style: const TextStyle(fontSize: 12, color: gitLensMuted)),
                      ],
                    ),
                  ),
                  StatusPill(label: auth.user?.role ?? 'user', color: gitLensBlue),
                ],
              ),
              const Divider(height: 30),
              if (createdToken != null) ...[
                const Text('API token created', style: TextStyle(fontWeight: FontWeight.w700)),
                const SizedBox(height: 6),
                const Text('Copy it now — the server only shows it once.', style: TextStyle(fontSize: 12, color: gitLensMuted)),
                const SizedBox(height: 8),
                SelectableText(createdToken!, style: const TextStyle(fontFamily: 'monospace', fontSize: 12, color: gitLensGreen)),
                const Divider(height: 30),
              ],
              TextField(
                controller: nameController,
                decoration: const InputDecoration(labelText: 'New API token name', prefixIcon: Icon(Icons.key)),
              ),
              const SizedBox(height: 10),
              OutlinedButton.icon(
                onPressed: () async {
                  try {
                    final token = await ref.read(apiClientProvider).createApiToken(nameController.text.trim().isEmpty ? 'Android app' : nameController.text.trim());
                    setSheetState(() => createdToken = token);
                  } catch (e) {
                    if (context.mounted) {
                      ScaffoldMessenger.of(context).showSnackBar(SnackBar(content: Text('$e')));
                    }
                  }
                },
                icon: const Icon(Icons.add, size: 18),
                label: const Text('Create API token'),
              ),
              const Divider(height: 30),
              const Text('Delete account', style: TextStyle(fontSize: 15, fontWeight: FontWeight.w800, color: danger)),
              const SizedBox(height: 6),
              const Text(
                'Removes your profile, tokens, sessions, tracked repositories and derived history. This is immediate and cannot be undone. The only administrator cannot delete the last admin account.',
                style: TextStyle(fontSize: 12, color: gitLensMuted, height: 1.45),
              ),
              const SizedBox(height: 12),
              TextField(
                controller: confirmController,
                autocorrect: false,
                enableSuggestions: false,
                decoration: InputDecoration(
                  labelText: 'Type ${username.isEmpty ? 'your username' : username} to confirm',
                  prefixIcon: const Icon(Icons.badge_outlined),
                  errorText: deleteError,
                ),
              ),
              const SizedBox(height: 10),
              FilledButton.icon(
                style: FilledButton.styleFrom(backgroundColor: danger, foregroundColor: Colors.white),
                onPressed: deleting ? null : () => confirmDelete(sheetContext, setSheetState),
                icon: deleting
                    ? const SizedBox(width: 16, height: 16, child: CircularProgressIndicator(strokeWidth: 2))
                    : const Icon(Icons.delete_forever_outlined, size: 18),
                label: Text(deleting ? 'Deleting…' : 'Delete account permanently'),
              ),
              const SizedBox(height: 10),
              SelectableText(
                'Privacy policy: ${auth.baseUrl ?? ''}/privacy\nAccount deletion: ${auth.baseUrl ?? ''}/account/delete',
                style: const TextStyle(fontSize: 11, color: gitLensMuted, height: 1.4),
              ),
              const SizedBox(height: 16),
              FilledButton.icon(
                style: FilledButton.styleFrom(backgroundColor: danger, foregroundColor: Colors.white),
                onPressed: () async {
                  Navigator.of(sheetContext).pop();
                  await ref.read(authProvider.notifier).logout();
                },
                icon: const Icon(Icons.logout, size: 18),
                label: const Text('Sign out'),
              ),
            ],
          ),
        ),
      ),
    );
    nameController.dispose();
    confirmController.dispose();
  }
}
