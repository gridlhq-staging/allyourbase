#!/usr/bin/env python3
"""Validate build-postgres workflow architecture execution contract."""

import os
import sys

import yaml

WORKFLOW_PATH = os.path.join(
    os.path.dirname(__file__),
    "..",
    "..",
    ".github",
    "workflows",
    "build-postgres-binaries.yml",
)


def load_build_job():
    with open(WORKFLOW_PATH, encoding="utf-8") as workflow_file:
        workflow = yaml.safe_load(workflow_file)
    jobs = workflow.get("jobs", {})
    assert "build" in jobs, "Missing jobs.build block"
    return workflow, jobs["build"]


def load_build_matrix_rows(build_job):
    include_rows = (
        build_job.get("strategy", {}).get("matrix", {}).get("include", [])
    )
    assert include_rows, "Build matrix include rows are missing"
    return include_rows


def find_platform_row(matrix_rows, platform):
    for row in matrix_rows:
        if row.get("platform") == platform:
            return row
    raise AssertionError(f"Missing matrix row for platform={platform}")


def runner_labels_contain_arm64(runs_on):
    if isinstance(runs_on, str):
        labels = [runs_on]
    elif isinstance(runs_on, list):
        labels = [str(label) for label in runs_on]
    else:
        return False
    normalized_labels = [label.lower() for label in labels]
    return any(
        "arm64" in label or label.endswith("-arm") for label in normalized_labels
    )


def runner_labels_contain_amd64(runs_on):
    if isinstance(runs_on, str):
        labels = [runs_on]
    elif isinstance(runs_on, list):
        labels = [str(label) for label in runs_on]
    else:
        return False
    normalized_labels = [label.lower() for label in labels]
    return any(
        "amd64" in label or "x64" in label or "intel" in label
        for label in normalized_labels
    )


def assert_arm64_execution_path(matrix_rows):
    linux_amd64 = find_platform_row(matrix_rows, "linux-amd64")
    linux_arm64 = find_platform_row(matrix_rows, "linux-arm64")

    amd64_runs_on = linux_amd64.get("runs_on")
    arm64_runs_on = linux_arm64.get("runs_on")
    assert arm64_runs_on is not None, "linux-arm64 must declare per-row runs_on metadata"

    arm64_has_native_runner = runner_labels_contain_arm64(arm64_runs_on)
    arm64_has_qemu_fallback = (
        linux_arm64.get("needs_qemu") is True
        and linux_arm64.get("container_platform") == "linux/arm64"
    )

    if arm64_runs_on == amd64_runs_on:
        assert arm64_has_qemu_fallback, (
            "linux-arm64 cannot share linux-amd64 host-only runner path unless "
            "the matrix row carries an explicit QEMU fallback tuple"
        )
    else:
        assert arm64_has_native_runner or arm64_has_qemu_fallback, (
            "linux-arm64 must use an arm64 runner label or explicit QEMU fallback tuple"
        )


def assert_darwin_amd64_execution_path(matrix_rows):
    darwin_amd64 = find_platform_row(matrix_rows, "darwin-amd64")
    darwin_runs_on = darwin_amd64.get("runs_on")
    assert darwin_runs_on is not None, "darwin-amd64 must declare per-row runs_on metadata"

    if isinstance(darwin_runs_on, str):
        normalized_labels = [darwin_runs_on.lower()]
    elif isinstance(darwin_runs_on, list):
        normalized_labels = [str(label).lower() for label in darwin_runs_on]
    else:
        raise AssertionError("darwin-amd64 runs_on metadata must be a string or list")

    assert all(label != "macos-latest" for label in normalized_labels), (
        "darwin-amd64 cannot run on macos-latest; it must use an explicit amd64/intel path"
    )
    assert runner_labels_contain_amd64(darwin_runs_on), (
        "darwin-amd64 must use an explicit amd64/intel runner label"
    )


def assert_build_job_uses_per_row_runner_metadata(build_job):
    runs_on = build_job.get("runs-on")
    assert runs_on == "${{ matrix.runs_on }}", (
        "jobs.build.runs-on must consume per-row matrix.runs_on metadata"
    )


def assert_post_build_arch_verification_step(build_job):
    steps = build_job.get("steps", [])
    verify_step = next(
        (step for step in steps if step.get("name") == "Verify built postgres architecture"),
        None,
    )
    assert verify_step is not None, "Missing Verify built postgres architecture step"

    verify_script = verify_step.get("run", "")
    expected_binary_path = (
        "dist/pg-binaries/ayb-postgres-${{ steps.version.outputs.pg_version }}/bin/postgres"
    )
    assert expected_binary_path in verify_script, "Verify step must inspect built postgres binary path"
    assert "${{ matrix.goarch }}" in verify_script, "Verify step must compare with matrix.goarch"
    assert "x86_64" in verify_script and "aarch64" in verify_script, (
        "Verify step must normalize x86_64/aarch64 tokens before comparison"
    )
    assert "/arm64|arm64e/" in verify_script and 'detected_goarch="arm64"' in verify_script, (
        "Verify step must map native darwin-arm64 Mach-O token to arm64"
    )
    assert "exit 1" in verify_script, "Verify step must fail on architecture mismatch"


def assert_workflow_permissions_are_scoped(workflow, build_job):
    top_permissions = workflow.get("permissions", {})
    assert top_permissions.get("contents") == "read", (
        "Workflow default token must be contents:read so build job does not inherit write access"
    )
    assert "permissions" not in build_job, (
        "Build job should inherit read-only default permissions rather than widening the token"
    )

    release_job = workflow.get("jobs", {}).get("release", {})
    release_permissions = release_job.get("permissions", {})
    assert release_permissions.get("contents") == "write", (
        "Release job must be the only job that widens contents permission for publishing"
    )


def assert_pg_version_output_is_validated(build_job):
    steps = build_job.get("steps", [])
    version_step = next(
        (step for step in steps if step.get("name") == "Set PG version"),
        None,
    )
    assert version_step is not None, "Missing Set PG version step"

    version_script = version_step.get("run", "")
    assert 'candidate="${GITHUB_REF_NAME#pg-}"' in version_script, (
        "Push-trigger version extraction must route through a validated candidate variable"
    )
    assert "candidate=\"${{ github.event.inputs.pg_version || '16' }}\"" in version_script, (
        "workflow_dispatch version input must route through a validated candidate variable"
    )
    assert '[[ "$candidate" =~ ^[0-9]+$ ]]' in version_script, (
        "pg_version must be restricted to numeric major-version tokens before writing workflow output"
    )
    assert "printf 'pg_version=%s\\n' \"$candidate\" >> \"$GITHUB_OUTPUT\"" in version_script, (
        "Validated pg_version must be emitted with printf to avoid raw output injection"
    )


def main():
    workflow, build_job = load_build_job()
    matrix_rows = load_build_matrix_rows(build_job)
    assert_arm64_execution_path(matrix_rows)
    assert_darwin_amd64_execution_path(matrix_rows)
    assert_build_job_uses_per_row_runner_metadata(build_job)
    assert_post_build_arch_verification_step(build_job)
    assert_workflow_permissions_are_scoped(workflow, build_job)
    assert_pg_version_output_is_validated(build_job)
    print("PASS: build-postgres workflow architecture contract assertions passed")


if __name__ == "__main__":
    try:
        main()
    except AssertionError as err:
        print(f"FAIL: {err}", file=sys.stderr)
        sys.exit(1)
