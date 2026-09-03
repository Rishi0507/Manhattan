"""Pull every Mermaid block out of the markdown into docs/diagrams/*.mmd.

The rendered SVGs exist because a judging surface may not render Mermaid
inline. Extracting them from the documents rather than maintaining a second
copy is the same rule the numbers follow: one source, many renderings, and no
way for a copy to quietly stop matching its original.
"""

import io
import os
import re

SOURCES = {"README.md": "readme", "docs/DESIGN.md": "design", "docs/EXPLAIN.md": "explain"}

os.makedirs("docs/diagrams", exist_ok=True)
seen = set()
for path, prefix in SOURCES.items():
    if not os.path.exists(path):
        continue
    text = io.open(path, encoding="utf-8").read()
    for i, block in enumerate(re.findall(r"```mermaid\n(.*?)```", text, re.S), 1):
        out = f"docs/diagrams/{prefix}-{i}.mmd"
        io.open(out, "w", encoding="utf-8", newline="\n").write(block)
        seen.add(os.path.basename(out))

# Remove stale extractions, so a deleted diagram does not leave an SVG behind
# that no document contains any more.
for name in sorted(os.listdir("docs/diagrams")):
    if name.endswith(".mmd") and name not in seen:
        os.remove(os.path.join("docs/diagrams", name))
        svg = os.path.join("docs/diagrams", name[:-4] + ".svg")
        if os.path.exists(svg):
            os.remove(svg)

print(f"extracted {len(seen)} diagrams")
