/// Server address handling for self-hosted GitLens.
///
/// GitLens has no hosted service and no default server address: the app must
/// never invent one. The field starts empty and only a URL the user previously
/// saved is ever prefilled.
library;

String normalizeServerUrl(String input) {
  var value = input.trim();
  if (value.isEmpty) return value;
  if (!value.contains('://')) {
    // Self-hosted GitLens is commonly served over HTTPS; users running a
    // local/LAN instance can type the scheme explicitly (http://...).
    value = 'https://$value';
  }
  while (value.endsWith('/')) {
    value = value.substring(0, value.length - 1);
  }
  return value;
}

String? serverUrlError(String input) {
  final value = input.trim();
  if (value.isEmpty) return 'Enter the address of your GitLens server';
  final uri = Uri.tryParse(normalizeServerUrl(value));
  if (uri == null || uri.host.isEmpty) return 'Enter a valid server URL';
  if (uri.scheme != 'http' && uri.scheme != 'https') {
    return 'Only http:// and https:// addresses are supported';
  }
  return null;
}
