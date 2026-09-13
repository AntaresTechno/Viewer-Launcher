# Viewer Launcher notices

This launcher downloads and executes Viewer release assets built from
[AntaresTechno/Viewer](https://github.com/AntaresTechno/Viewer). Viewer is
licensed under GNU GPL v3 or later. Each generated web, backend, and platform
runtime archive records the immutable upstream commit. Runtime archives contain
a verbatim `LICENSES/Viewer-GPL-3.0-or-later.txt` and a collected third-party
Python licence inventory.

The launcher build itself is released under GPL-3.0-or-later so that a
distributed combined package can be conveyed under terms compatible with the
upstream application. The `Build preview release` workflow publishes the
resolved Python package set and any licence files provided by wheels under
`lib/licenses/`, together with a SHA-256 manifest for every release asset.
