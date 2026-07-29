"""Pytest configuration for the AllYourBase Python SDK tests.

Mirrors the JS SDK's vitest exclusion (sdk/vitest.config.ts): the live
end-to-end proofs in ``test_e2e.py`` require a running server and are collected
only when ``test_e2e.py`` is named explicitly on the command line (the
live-proof lane). The default unit lane (``python3 -m pytest``) stays hermetic
by not collecting them, while the live proofs fail closed on missing env rather
than skipping green when they do run.
"""

from __future__ import annotations

import sys

_E2E_MODULE = "test_e2e.py"


def _e2e_explicitly_selected() -> bool:
    return any(_E2E_MODULE in arg for arg in sys.argv[1:])


collect_ignore = [] if _e2e_explicitly_selected() else [_E2E_MODULE]
