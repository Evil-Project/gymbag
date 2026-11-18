import sys
from pathlib import Path

src = Path(__file__).resolve().parents[1] / "src"
src_path = str(src)
if src_path not in sys.path:
    sys.path.insert(0, src_path)
