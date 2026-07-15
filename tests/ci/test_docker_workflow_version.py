#!/usr/bin/env python3
"""Validate Docker workflow release version metadata contract."""

import os
import sys

import yaml

WORKFLOW_PATH = os.path.join(
    os.path.dirname(__file__),
    "..",
    "..",
    ".github",
    "workflows",
    "docker.yml",
)


def load_docker_job():
    with open(WORKFLOW_PATH, encoding="utf-8") as workflow_file:
        workflow = yaml.safe_load(workflow_file)
    jobs = workflow.get("jobs", {})
    assert "docker" in jobs, "Missing jobs.docker block"
    return jobs["docker"]


def find_step(steps, name):
    for step in steps:
        if step.get("name") == name:
            return step
    raise AssertionError(f"Missing step: {name}")


def assert_tag_version_strips_v_prefix(docker_job):
    steps = docker_job.get("steps", [])
    build_meta = find_step(steps, "Derive build metadata args")
    script = build_meta.get("run", "")

    assert 'ayb_version="${GITHUB_REF_NAME#v}"' in script, (
        "Tag-triggered Docker releases must strip the leading v prefix before "
        "passing AYB_VERSION to the binary"
    )
    assert "AYB_VERSION=${{ steps.build_meta.outputs.version }}" in str(docker_job), (
        "Docker build must consume the sanitized build_meta version output"
    )


def main():
    docker_job = load_docker_job()
    assert_tag_version_strips_v_prefix(docker_job)
    print("PASS: docker workflow version metadata contract assertions passed")


if __name__ == "__main__":
    try:
        main()
    except AssertionError as err:
        print(f"FAIL: {err}", file=sys.stderr)
        sys.exit(1)
