#!/usr/bin/env bash
# Splices the Devices View (devices-tab.md + images/) into the August 2026
# Teacher Guide PDF: pages 1-118 as exported from Canva, the Devices pages
# built from the markdown (numbered 119-1, 119-2, … so existing numbering
# and the table of contents stay valid), page 119 rebuilt with only its
# Analytics section, then the original pages 120-121.
# Needs: pandoc, playwright-cli, qpdf, poppler (pdfimages). Run from the repo root.
# The output (~99 MB) is NOT committed: finished copies go to cloud storage, and
# the repo keeps only a compressed copy ("… (Devices View update) compressed.pdf").
set -euo pipefail
G=docs/mission-hydrosci-teacher-guide
SRC="$G/sources/August 2026 Mission HydroSci Teacher Guide.pdf"
OUT="$G/August 2026 Mission HydroSci Teacher Guide (Devices View update).pdf"
B=$(mktemp -d); trap 'rm -rf "$B"' EXIT
cp "$G"/images/*.png "$B/"
pdfimages -f 119 -l 119 -png "$SRC" "$B/p119img"; mv "$B/p119img-001.png" "$B/analytics.png"
python3 - "$G" "$B" <<'PY'
import pathlib, subprocess, sys, re
G, B = pathlib.Path(sys.argv[1]), pathlib.Path(sys.argv[2])
md = (G/'devices-tab.md').read_text()
md = re.sub(r'^# The Devices View\n\n\*Content for the Mission HydroSci Teacher Guide.*?\n\n---\n\n', '# The Devices View\n\n', md, count=1, flags=re.S)
md = md.replace('](images/', '](')
body = subprocess.run(['pandoc', '-f', 'gfm', '-t', 'html'], input=md, capture_output=True, text=True, check=True).stdout
css = '''<style>
@page { size: letter; margin: 0.55in 0.6in 0.75in 0.6in; }
html { font-family: "Avenir Next", "Helvetica Neue", Arial, sans-serif; color: #1f2937; }
body { margin: 0; font-size: 9.6pt; line-height: 1.38; }
h1 { font-size: 15pt; font-weight: 700; text-decoration: underline; margin: 0 0 8pt; color: #111827; }
h2 { font-size: 11.5pt; font-weight: 700; text-decoration: underline; margin: 14pt 0 5pt; color: #111827; break-after: avoid; }
p { margin: 0 0 7pt; } ul, ol { margin: 0 0 7pt; padding-left: 18pt; } li { margin: 0 0 3pt; }
table { border-collapse: collapse; width: 100%; margin: 4pt 0 9pt; font-size: 8.4pt; line-height: 1.3; }
th, td { border: 0.6pt solid #cbd5e1; padding: 3.5pt 5pt; vertical-align: top; text-align: left; }
th { background: #eef2f7; font-weight: 700; } tr { break-inside: avoid; }
img { max-width: 100%; height: auto; display: block; margin: 5pt auto 8pt; border: 0.6pt solid #d1d5db; border-radius: 3pt; }
blockquote { margin: 8pt auto 10pt; width: 62%; background: #cfe6f5; border-radius: 10pt; padding: 8pt 12pt; text-align: center; font-size: 9.2pt; break-inside: avoid; }
blockquote p { margin: 0; } blockquote p:first-child strong { display: block; font-size: 10.5pt; margin-bottom: 2pt; }
</style>'''
(B/'insert.html').write_text('<!doctype html><html><head><meta charset="utf-8"><title>The Devices View</title>' + css + '</head><body>' + body + '</body></html>')
(B/'p119.html').write_text('<!doctype html><html><head><meta charset="utf-8"><title>The Analytics View</title>' + css + '''</head><body>
<h1>The Analytics View</h1>
<p>This page displays the time each student spent on each task within MHS. Items on this page include the following:</p>
<ul><li>Marker that indicates a task is in progress</li>
<li>Marker that indicates a task was completed in an expected amount of time</li>
<li>Marker that indicates a task was flagged, for completion in an unexpected amount of time.</li></ul>
<img src="analytics.png" alt="The Analytics view">
<blockquote><p><strong>Tip</strong></p><p>The analytics view can be used to identify a student that might have experienced a technical issue</p></blockquote>
</body></html>''')
PY
# A free port each run, and the readiness check fails on any HTTP error, so a
# stale server from an earlier run can never hand Chromium a 404 page to print.
PORT=$(python3 -c 'import socket; s=socket.socket(); s.bind(("127.0.0.1",0)); print(s.getsockname()[1])')
( cd "$B" && exec python3 -m http.server "$PORT" --bind 127.0.0.1 >/dev/null 2>&1 ) & HTTPD=$!
trap 'kill $HTTPD 2>/dev/null; rm -rf "$B"' EXIT
curl -sf -o /dev/null --retry 30 --retry-connrefused --retry-delay 1 "http://127.0.0.1:$PORT/insert.html"
curl -sf -o /dev/null "http://127.0.0.1:$PORT/p119.html" "http://127.0.0.1:$PORT/analytics.png"
playwright-cli open "http://127.0.0.1:$PORT/insert.html" >/dev/null
playwright-cli run-code "async page => {
  const foot = (label) => '<div style=\"font-family: Avenir Next, Helvetica Neue, Arial, sans-serif; font-size:8px; color:#374151; width:100%; text-align:right; padding-right:0.6in;\">' + label + '</div>';
  const opts = (path, label) => ({ path, format: 'Letter', printBackground: true, displayHeaderFooter: true, headerTemplate: '<div></div>', footerTemplate: foot(label), margin: { top: '0.55in', right: '0.6in', bottom: '0.75in', left: '0.6in' } });
  await page.emulateMedia({ media: 'print' });
  await page.pdf(opts('$B/insert.pdf', 'Page 119-<span class=\"pageNumber\"></span>'));
  await page.goto('http://127.0.0.1:$PORT/p119.html'); await page.emulateMedia({ media: 'print' });
  await page.pdf(opts('$B/p119.pdf', 'Page 119'));
}" >/dev/null
playwright-cli close >/dev/null
INS=$(pdfinfo "$B/insert.pdf" | awk '/^Pages/{print $2}')
if [ "${INS:-0}" -lt 3 ]; then echo "insert.pdf has $INS page(s); expected several — refusing to splice" >&2; exit 1; fi
pdftotext -f 1 -l 1 "$B/insert.pdf" - | grep -q "The Devices View" || { echo "insert.pdf does not start with the Devices View" >&2; exit 1; }
qpdf --empty --pages "$SRC" 1-118 "$B/insert.pdf" 1-z "$B/p119.pdf" 1 "$SRC" 120-121 -- "$OUT"
pdfinfo "$OUT" | grep Pages
