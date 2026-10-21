import 'dart:math' as math;

import 'package:flutter/material.dart';

import 'theme.dart';

/// The GitLens mark, drawn from the same geometry as `static/logo.svg` used by
/// the web app's favicon. Keeping it as a painter means the in-app header and
/// splash use the exact brand styling at any resolution.
class GitLensMark extends StatelessWidget {
  const GitLensMark({super.key, this.size = 40, this.showBackground = true});

  final double size;
  final bool showBackground;

  @override
  Widget build(BuildContext context) {
    return SizedBox(
      width: size,
      height: size,
      child: CustomPaint(painter: _GitLensMarkPainter(showBackground: showBackground)),
    );
  }
}

class _GitLensMarkPainter extends CustomPainter {
  const _GitLensMarkPainter({required this.showBackground});

  final bool showBackground;

  @override
  void paint(Canvas canvas, Size size) {
    final scale = size.width / 32;
    canvas.save();
    canvas.scale(scale);

    if (showBackground) {
      final bg = Paint()..color = gitLensSurface;
      canvas.drawCircle(const Offset(16, 16), 16, bg);
    }

    final ring = Paint()
      ..style = PaintingStyle.stroke
      ..strokeWidth = 2
      ..color = gitLensGreen;
    canvas.drawCircle(const Offset(16, 16), 14, ring);

    final inner = Paint()
      ..style = PaintingStyle.stroke
      ..strokeWidth = 1.5
      ..color = gitLensBlue;
    canvas.drawCircle(const Offset(16, 16), 6, inner);

    canvas.drawCircle(const Offset(16, 16), 2, Paint()..color = gitLensGreen);

    final rays = Paint()
      ..style = PaintingStyle.stroke
      ..strokeWidth = 1.5
      ..strokeCap = StrokeCap.round
      ..color = gitLensBlue;
    for (final offset in const [Offset(0, -6), Offset(0, 6), Offset(-6, 0), Offset(6, 0)]) {
      canvas.drawLine(const Offset(16, 16) + offset, const Offset(16, 16) + offset * 1.66, rays);
    }

    final diagonals = Paint()
      ..style = PaintingStyle.stroke
      ..strokeWidth = 1
      ..strokeCap = StrokeCap.round
      ..color = gitLensMuted.withValues(alpha: 0.5);
    for (final start in const [Offset(20, 12), Offset(12, 20), Offset(20, 20), Offset(12, 12)]) {
      final direction = Offset(start.dx < 16 ? -1 : 1, start.dy < 16 ? -1 : 1);
      canvas.drawLine(start, start + direction * 4, diagonals);
    }

    canvas.restore();
  }

  @override
  bool shouldRepaint(covariant _GitLensMarkPainter oldDelegate) =>
      oldDelegate.showBackground != showBackground;
}

/// A soft branded gradient used behind hero content.
class BrandBackdrop extends StatelessWidget {
  const BrandBackdrop({super.key, required this.child});

  final Widget child;

  @override
  Widget build(BuildContext context) {
    return Stack(
      children: [
        Positioned(
          top: -140,
          right: -100,
          child: Container(
            width: 320,
            height: 320,
            decoration: BoxDecoration(
              shape: BoxShape.circle,
              gradient: RadialGradient(colors: [
                gitLensGreen.withValues(alpha: 0.18),
                Colors.transparent,
              ]),
            ),
          ),
        ),
        Positioned(
          top: 80,
          left: -140,
          child: Transform.rotate(
            angle: math.pi / 7,
            child: Container(
              width: 280,
              height: 280,
              decoration: BoxDecoration(
                shape: BoxShape.circle,
                gradient: RadialGradient(colors: [
                  gitLensBlue.withValues(alpha: 0.12),
                  Colors.transparent,
                ]),
              ),
            ),
          ),
        ),
        child,
      ],
    );
  }
}
