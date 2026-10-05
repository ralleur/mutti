# Mutti identity

Owner reference: **03 / Kompakte Bögen**, supplied 5 October 2026.
The outlined vectors reconstruct the compact twin arches, curled tail and two
yellow rays. The lowercase wordmark is independent of installed fonts.

- Graphite `#1F1F1F`, Ivory `#FAF8F1`, Electric Yellow `#FFE600`.
- `mark-light.svg` / `mark-dark.svg`: transparent standalone marks.
- `wordmark-light.svg` / `wordmark-dark.svg`: outlined lowercase wordmarks.
- `symbol.svg` / `symbol-light.svg`: dark and light app icons.
- Sora: Regular 400, SemiBold 600, Bold 700, bundled locally under SIL OFL.
  The reference does not name a font; Sora continues the shared kurtz foundation.

The import journey follows the dark reference. Yellow controls have graphite
text; fine borders and ivory typography provide hierarchy. At desktop widths,
the seven steps form a vertical left column and Kurt occupies the right column.
Below 650 px, the list stays vertical and Kurt follows below, preserving zoom
and small-screen readability. Status, timing and connection feedback stay below.

The Web library retains its existing light/dark preference. Light-theme links
use accessible `#625800`; yellow stays decorative on ivory. Native macOS controls
remain standard; the app icon, wordmark and launch heading use the same identity.
Kurt's drawing and full travel area are preserved at their original aspect ratio.

Tokens are in `tokens.json`. Canonical source assets live in `assets/`; their
copies in Import, Connect and Mutti Web must match. Touch icons and `Mutti.icns`
are rasterized from `symbol.svg`; native wordmark PNG is from its SVG at 3×.
Use `packaging/render-brand.cjs` with an installed `@resvg/resvg-js` package to
regenerate raster assets, followed by `iconutil` for the generated iconset.
Existing attribution and third-party licenses remain unchanged.

Reference filename: `Codex-Bild 5. Okt. 2026, 14_37_46.png`.
Reference SHA-256: `98af480286a071a509a0aa55b1f76acf1729adfadeb9cc83b348ece6ed960a80`.
