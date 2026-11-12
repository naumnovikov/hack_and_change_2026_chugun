import json
from pathlib import Path
from main import app


def test_checked_in_openapi_matches_runtime():
    document = Path(__file__).resolve().parents[1] / "openapi.yaml"
    assert json.loads(document.read_text(encoding="utf-8")) == app.openapi()
