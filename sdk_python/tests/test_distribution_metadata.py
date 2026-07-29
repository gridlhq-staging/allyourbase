"""Distribution metadata contract for the Python SDK."""

from __future__ import annotations

import os
import subprocess
import sys
import tarfile
import zipfile
from email.parser import Parser
from pathlib import Path


PROJECT_ROOT = Path(__file__).resolve().parents[1]
EXPECTED_DISTRIBUTION_NAME = "allyourbase-sdk"
IMPORT_PACKAGE_NAME = "allyourbase"


def test_distribution_name_and_import_package_are_distinct(tmp_path: Path) -> None:
    dist_dir = tmp_path / "dist"
    subprocess.run(
        [
            sys.executable,
            "-m",
            "build",
            "--wheel",
            "--sdist",
            "--outdir",
            str(dist_dir),
            ".",
        ],
        cwd=PROJECT_ROOT,
        check=True,
    )

    wheel_path = _single_artifact(dist_dir, "*.whl")
    sdist_path = _single_artifact(dist_dir, "*.tar.gz")

    wheel_metadata = _read_wheel_metadata(wheel_path)
    sdist_metadata = _read_sdist_metadata(sdist_path)
    assert wheel_metadata["Name"] == EXPECTED_DISTRIBUTION_NAME
    assert sdist_metadata["Name"] == EXPECTED_DISTRIBUTION_NAME
    assert wheel_metadata["Name"] != IMPORT_PACKAGE_NAME
    assert sdist_metadata["Name"] != IMPORT_PACKAGE_NAME

    with zipfile.ZipFile(wheel_path) as wheel:
        wheel_files = set(wheel.namelist())
    assert f"{IMPORT_PACKAGE_NAME}/__init__.py" in wheel_files
    assert any(name.startswith(f"{IMPORT_PACKAGE_NAME}/") for name in wheel_files)

    _assert_import_package_available(tmp_path, wheel_path)


def _single_artifact(dist_dir: Path, pattern: str) -> Path:
    matches = sorted(dist_dir.glob(pattern))
    assert len(matches) == 1
    return matches[0]


def _read_wheel_metadata(wheel_path: Path) -> dict[str, str]:
    with zipfile.ZipFile(wheel_path) as wheel:
        metadata_paths = [
            name for name in wheel.namelist() if name.endswith(".dist-info/METADATA")
        ]
        assert len(metadata_paths) == 1
        return dict(Parser().parsestr(wheel.read(metadata_paths[0]).decode("utf-8")))


def _read_sdist_metadata(sdist_path: Path) -> dict[str, str]:
    with tarfile.open(sdist_path) as sdist:
        metadata_paths = [
            member for member in sdist.getmembers() if member.name.endswith("/PKG-INFO")
        ]
        assert len(metadata_paths) == 1
        metadata_file = sdist.extractfile(metadata_paths[0])
        assert metadata_file is not None
        return dict(Parser().parsestr(metadata_file.read().decode("utf-8")))


def _assert_import_package_available(tmp_path: Path, wheel_path: Path) -> None:
    env = os.environ.copy()
    env.pop("PYTHONPATH", None)
    result = subprocess.run(
        [
            sys.executable,
            "-c",
            (
                "import sys; "
                "sys.path.insert(0, sys.argv[1]); "
                "import allyourbase; "
                "print(allyourbase.__file__)"
            ),
            str(wheel_path),
        ],
        cwd=tmp_path,
        check=True,
        env=env,
        stdout=subprocess.PIPE,
        text=True,
    )
    assert result.stdout.strip().startswith(str(wheel_path))
