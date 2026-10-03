#!/usr/bin/env python3
"""Point the Homebrew formula at a new release.

    update_homebrew_formula.py FORMULA VERSION SHA256SUMS

Rewrites every release download URL in FORMULA to VERSION and sets the sha256 under
each URL to the checksum SHA256SUMS lists for that file. The checksums come from the
SHA256SUMS published with the release, not from anything computed here, so the
formula pins exactly the files people download.

Fails, changing nothing, if an artefact the formula names is missing from
SHA256SUMS: a formula pointing at a file that does not exist installs nothing, and the
failure would only surface on someone else's machine.
"""

import re
import sys

URL_SHA = re.compile(
    r'(url "https://github\.com/theazz/awless-ro/releases/download/)v[^/"]+/([^"]+)"'
    r'(\s+sha256 ")[0-9a-f]{64}"'
)


def main(formula_path, version, sums_path):
    if not re.fullmatch(r"\d+\.\d+\.\d+", version):
        sys.exit(f"version must be MAJOR.MINOR.PATCH without the v, got {version!r}")

    sums = {}
    with open(sums_path) as f:
        for line in f:
            if line.strip():
                digest, name = line.split()
                sums[name.lstrip("*")] = digest

    with open(formula_path) as f:
        formula = f.read()

    missing = []

    def replace(m):
        name = m.group(2)
        if name not in sums:
            missing.append(name)
            return m.group(0)
        return f'{m.group(1)}v{version}/{name}"{m.group(3)}{sums[name]}"'

    updated, count = URL_SHA.subn(replace, formula)
    if count == 0:
        sys.exit(f"no release download URL with a sha256 found in {formula_path}")
    if missing:
        sys.exit(f"not in {sums_path}: {', '.join(missing)}")

    with open(formula_path, "w") as f:
        f.write(updated)
    print(f"{formula_path}: {count} artefacts now at v{version}")


if __name__ == "__main__":
    if len(sys.argv) != 4:
        sys.exit(__doc__)
    main(*sys.argv[1:])
