/// Parses a `Cookie` header and returns the value for [name].
String? parseCookieHeader(String? raw, String name) {
  if (raw == null || raw.isEmpty) return null;
  for (final part in raw.split(';')) {
    final trimmed = part.trim();
    final eq = trimmed.indexOf('=');
    if (eq <= 0) continue;
    if (trimmed.substring(0, eq).trim() == name) {
      return trimmed.substring(eq + 1);
    }
  }
  return null;
}
