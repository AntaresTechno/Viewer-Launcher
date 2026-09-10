# Viewer Launcher notices

This launcher downloads and executes the Viewer backend from
[AntaresTechno/Viewer](https://github.com/AntaresTechno/Viewer). Viewer is
licensed under GNU GPL v3 or later. Each generated Windows runtime contains a
verbatim `LICENSES/Viewer-GPL-3.0-or-later.txt`, the upstream commit identifier,
and third-party Python notices.

The launcher build itself is released under GPL-3.0-or-later so that a
distributed combined package can be conveyed under terms compatible with the
upstream application. Before publishing, run the `Build Viewer runtime`
workflow; it rejects an unrecognised Python dependency licence and records the
resolved package set under `runtime/licenses/`.
