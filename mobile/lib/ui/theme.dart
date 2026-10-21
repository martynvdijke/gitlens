import 'package:flutter/material.dart';

const gitLensBackground = Color(0xFF0D1117);
const gitLensSurface = Color(0xFF161B22);
const gitLensSurfaceRaised = Color(0xFF1C2128);
const gitLensGreen = Color(0xFF3FB950);
const gitLensBlue = Color(0xFF58A6FF);
const gitLensMuted = Color(0xFF8B949E);
const gitLensBorder = Color(0xFF30363D);

ThemeData buildGitLensTheme() {
  final scheme = ColorScheme.fromSeed(
    seedColor: gitLensGreen,
    brightness: Brightness.dark,
  ).copyWith(
    primary: gitLensGreen,
    secondary: gitLensBlue,
    surface: gitLensSurface,
    onSurface: const Color(0xFFE6EDF3),
    onSurfaceVariant: gitLensMuted,
    outline: gitLensBorder,
    error: const Color(0xFFF85149),
  );

  return ThemeData(
    useMaterial3: true,
    colorScheme: scheme,
    scaffoldBackgroundColor: gitLensBackground,
    fontFamily: 'Roboto',
    cardTheme: CardThemeData(
      color: gitLensSurface,
      elevation: 0,
      margin: EdgeInsets.zero,
      shape: RoundedRectangleBorder(
        borderRadius: BorderRadius.circular(16),
        side: const BorderSide(color: gitLensBorder),
      ),
    ),
    appBarTheme: const AppBarTheme(
      backgroundColor: gitLensBackground,
      surfaceTintColor: Colors.transparent,
      centerTitle: false,
      titleTextStyle: TextStyle(fontSize: 20, fontWeight: FontWeight.w800, letterSpacing: -0.4),
    ),
    navigationBarTheme: NavigationBarThemeData(
      backgroundColor: gitLensSurface,
      indicatorColor: gitLensGreen.withValues(alpha: 0.16),
      labelTextStyle: WidgetStateProperty.resolveWith((states) {
        final selected = states.contains(WidgetState.selected);
        return TextStyle(
          fontSize: 11,
          fontWeight: selected ? FontWeight.w700 : FontWeight.w500,
          color: selected ? gitLensGreen : gitLensMuted,
        );
      }),
      iconTheme: WidgetStateProperty.resolveWith((states) {
        final selected = states.contains(WidgetState.selected);
        return IconThemeData(color: selected ? gitLensGreen : gitLensMuted);
      }),
    ),
    inputDecorationTheme: InputDecorationTheme(
      filled: true,
      fillColor: gitLensBackground,
      border: OutlineInputBorder(borderRadius: BorderRadius.circular(12), borderSide: const BorderSide(color: gitLensBorder)),
      enabledBorder: OutlineInputBorder(borderRadius: BorderRadius.circular(12), borderSide: const BorderSide(color: gitLensBorder)),
      focusedBorder: OutlineInputBorder(borderRadius: BorderRadius.circular(12), borderSide: const BorderSide(color: gitLensGreen, width: 1.5)),
      hintStyle: const TextStyle(color: gitLensMuted),
    ),
    snackBarTheme: const SnackBarThemeData(
      backgroundColor: gitLensSurfaceRaised,
      contentTextStyle: TextStyle(color: Color(0xFFE6EDF3)),
      behavior: SnackBarBehavior.floating,
    ),
    dividerTheme: const DividerThemeData(color: gitLensBorder, thickness: 1),
    progressIndicatorTheme: const ProgressIndicatorThemeData(color: gitLensGreen),
    chipTheme: ChipThemeData(
      backgroundColor: gitLensSurfaceRaised,
      side: const BorderSide(color: gitLensBorder),
      labelStyle: const TextStyle(fontSize: 12, color: Color(0xFFE6EDF3)),
      shape: RoundedRectangleBorder(borderRadius: BorderRadius.circular(999)),
    ),
  );
}
