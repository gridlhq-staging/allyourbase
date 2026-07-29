#!/usr/bin/env python3
"""Validate the inert first-lane SDK publish-readiness workflow."""

from __future__ import annotations

import copy
import json
import os
import re
import subprocess
import sys
from pathlib import Path
from typing import Any, Callable

import yaml

try:
    import tomllib
except ModuleNotFoundError:  # Python 3.9, the SDK's supported floor.
    import tomli as tomllib


ROOT = Path(__file__).resolve().parents[2]
WORKFLOW_PATH = ROOT / ".github" / "workflows" / "sdk_publish.yml"
NPM_MANIFESTS = {
    "sdk": ROOT / "sdk" / "package.json",
    "sdk_react": ROOT / "sdk_react" / "package.json",
    "sdk_ssr": ROOT / "sdk_ssr" / "package.json",
}
NPM_VERSION_INPUTS = {
    package_dir: f"{package_dir}_version" for package_dir in NPM_MANIFESTS
}
NPM_CONFIRMATION_INPUTS = {
    package_dir: f"{package_dir}_confirmation" for package_dir in NPM_MANIFESTS
}
PYTHON_MANIFEST = ROOT / "sdk_python" / "pyproject.toml"
PYTHON_VERSION_INPUT = "sdk_python_version"
PYTHON_CONFIRMATION_INPUT = "sdk_python_confirmation"
GO_VERSION_INPUT = "sdk_go_version"
GO_TAG_PREFIX = "sdk_go/v"
PYPI_PUBLISH_ACTION = (
    "pypa/gh-action-pypi-publish@ba38be9e461d3875417946c167d0b5f3d385a247"
)
GO_SEMVER_CASES = {
    "0.0.0": True,
    "1.2.3": True,
    "1.2.3-alpha.1": True,
    "1.2.3-alpha.1+build.7": True,
    "1.2.3+build.7": True,
    "v1.2.3": False,
    "1.2.3foo": False,
    "1.2.3-": False,
    "1.2.3.4": False,
}


def require(condition: bool, message: str) -> None:
    if not condition:
        raise AssertionError(message)


def load_workflow() -> dict[str, Any]:
    with WORKFLOW_PATH.open(encoding="utf-8") as workflow_file:
        return yaml.safe_load(workflow_file)


def load_triggers(workflow: dict[str, Any]) -> dict[str, Any]:
    # PyYAML decodes bare `on:` as Python bool True, not the string "on".
    return workflow.get("on") or workflow.get(True) or {}


def npm_package_names() -> dict[str, str]:
    return {
        package_dir: json.loads(path.read_text(encoding="utf-8"))["name"]
        for package_dir, path in NPM_MANIFESTS.items()
    }


def npm_package_versions() -> dict[str, str]:
    return {
        package_dir: json.loads(path.read_text(encoding="utf-8"))["version"]
        for package_dir, path in NPM_MANIFESTS.items()
    }


def python_distribution_name() -> str:
    return tomllib.loads(PYTHON_MANIFEST.read_text(encoding="utf-8"))["project"]["name"]


def python_distribution_version() -> str:
    return tomllib.loads(PYTHON_MANIFEST.read_text(encoding="utf-8"))["project"]["version"]


def assert_manual_dry_run_trigger_only(workflow: dict[str, Any]) -> None:
    triggers = load_triggers(workflow)
    require(set(triggers) == {"workflow_dispatch"}, f"unexpected triggers: {sorted(triggers)}")
    inputs = triggers["workflow_dispatch"].get("inputs", {})
    require("dry_run" in inputs, "dry_run input missing")
    require(inputs["dry_run"].get("default") is True, "dry_run default must be true")
    require("push" not in triggers, "push trigger must not be present")
    require("schedule" not in triggers, "schedule trigger must not be present")


def assert_first_lane_identities(workflow: dict[str, Any], text: str) -> None:
    jobs = workflow.get("jobs", {})
    npm_entries = (
        jobs["npm"].get("strategy", {}).get("matrix", {}).get("include", [])
    )
    npm_dirs = [entry["package_dir"] for entry in npm_entries]
    require(set(npm_dirs) == set(NPM_MANIFESTS), f"unexpected npm package dirs: {npm_dirs}")

    consumed_identities = {npm_package_names()[package_dir] for package_dir in npm_dirs}
    python_job = jobs.get("python", {})
    require(python_job.get("defaults", {}).get("run", {}).get("working-directory") == "sdk_python",
            "python job must operate from sdk_python")
    consumed_identities.add(python_distribution_name())

    go_job_text = str(jobs.get("go", {}))
    require(GO_TAG_PREFIX in go_job_text, "Go job must validate sdk_go/v tag prefix")
    consumed_identities.add(GO_TAG_PREFIX)

    expected_identities = {
        "@allyourbase/js",
        "@allyourbase/react",
        "@allyourbase/ssr",
        "allyourbase-sdk",
        GO_TAG_PREFIX,
    }
    require(consumed_identities == expected_identities,
            f"first-lane identity mismatch: {consumed_identities}")
    require("@allyourbase/" not in text, "workflow must not duplicate npm package-name literals")

    forbidden = ["sdk_dart", "sdk_kotlin", "sdk_swift", "dart", "kotlin", "swift"]
    lower_text = text.lower()
    for token in forbidden:
        require(token not in lower_text, f"workflow must not include {token} publish lane")
    require("all sdks" not in lower_text and "all SDKs" not in text,
            "workflow must not claim all SDKs")


def assert_publish_steps_are_gated(workflow: dict[str, Any]) -> None:
    confirmation_gates = {
        "npm_sdk_publish": (
            "github.event.inputs.sdk_confirmation == github.event.inputs.sdk_version"
        ),
        "npm_react_publish": (
            "github.event.inputs.sdk_react_confirmation == "
            "github.event.inputs.sdk_react_version"
        ),
        "npm_ssr_publish": (
            "github.event.inputs.sdk_ssr_confirmation == github.event.inputs.sdk_ssr_version"
        ),
        "python_publish": (
            "github.event.inputs.sdk_python_confirmation == "
            "github.event.inputs.sdk_python_version"
        ),
    }
    publish_jobs = set()
    for job_name, job in workflow.get("jobs", {}).items():
        job_permissions = job.get("permissions", {})
        job_has_publish_step = False
        for step in job.get("steps", []):
            if _step_can_publish_or_create_tag(step):
                publish_jobs.add(job_name)
                job_has_publish_step = True
                condition = "\n".join((str(job.get("if", "")), str(step.get("if", ""))))
                require("github.event.inputs.dry_run == 'false'" in condition,
                        f"{job_name}:{step.get('name')} lacks non-dry-run gate")
                require(
                    job_name in confirmation_gates,
                    f"{job_name}:{step.get('name')} has no confirmation/version contract",
                )
                require(
                    confirmation_gates[job_name] in condition,
                    f"{job_name}:{step.get('name')} lacks confirmation/version gate",
                )
        if job_has_publish_step:
            require(
                job_permissions.get("id-token") == "write",
                f"{job_name} must reserve OIDC for publish-only jobs",
            )
        else:
            require(
                job_permissions.get("id-token") != "write",
                f"{job_name} must not expose OIDC outside publish jobs",
            )
    require(
        publish_jobs == set(confirmation_gates),
        f"unexpected publish jobs: {sorted(publish_jobs)}",
    )


def _step_can_publish_or_create_tag(step: dict[str, Any]) -> bool:
    haystack = "\n".join(str(step.get(field, "")) for field in ("name", "uses", "run"))
    if "npm publish" in haystack and "--dry-run" not in haystack:
        return True
    if "pypa/gh-action-pypi-publish" in haystack:
        return True
    return bool(re.search(r"\bgit\s+(tag|push)\b", haystack))


def assert_unknown_publish_job_is_rejected(workflow: dict[str, Any]) -> None:
    mutated = copy.deepcopy(workflow)
    mutated["jobs"]["go"]["steps"].append(
        {
            "name": "Create Go module tag",
            "if": "${{ github.event.inputs.dry_run == 'false' }}",
            "run": "git tag sdk_go/v0.0.0",
        }
    )
    _require_assertion(
        lambda: assert_publish_steps_are_gated(mutated),
        "a tag-creating job without a confirmation contract was accepted",
    )


def assert_python_validation_cannot_upload(workflow: dict[str, Any]) -> None:
    python_job = workflow["jobs"]["python"]
    python_steps = python_job["steps"]
    validation_step = _named_step(python_steps, "Validate Python distributions")
    require("uses" not in validation_step,
            "Python dry-run validation must not invoke an upload action")
    validation_command = validation_step.get("run", "")
    require("python3 -m twine check dist/*" in validation_command,
            "Python distributions must be validated with twine check")
    require("twine upload" not in validation_command and "pypi-publish" not in validation_command,
            "Python validation command must not contain an upload path")

    require(
        python_job.get("permissions", {}).get("id-token") != "write",
        "Python validation job must not receive OIDC publish credentials",
    )

    publish_job = workflow["jobs"]["python_publish"]
    require(
        publish_job.get("permissions", {}).get("id-token") == "write",
        "Python publish job must receive OIDC publish credentials",
    )
    publish_step = _named_step(publish_job["steps"], "Publish PyPI package")
    require(publish_step.get("uses") == PYPI_PUBLISH_ACTION,
            "the pinned PyPI action must be reserved for the gated publish step")
    require(
        publish_job.get("needs") == "python",
        "Python publish job must depend on the validation job",
    )
    download_step = _named_step(publish_job["steps"], "Download Python distributions")
    require(
        download_step.get("with", {}).get("name")
        == "python-sdk-${{ github.event.inputs.sdk_python_version }}",
        "Python publish job must download the version-bound validation artifact",
    )

    unsupported_dry_run = copy.deepcopy(validation_step)
    unsupported_dry_run.pop("run", None)
    unsupported_dry_run["uses"] = PYPI_PUBLISH_ACTION
    unsupported_dry_run["with"] = {"dry-run": True}
    require(_step_can_publish_or_create_tag(unsupported_dry_run),
            "a dry-run input must not classify the PyPI upload action as inert")


def assert_requested_versions_are_bound(workflow: dict[str, Any]) -> None:
    npm_steps = workflow["jobs"]["npm"]["steps"]
    npm_version_step = _named_step(npm_steps, "Verify requested version matches package")
    _assert_version_step_contract(
        npm_version_step,
        step_id="package_version",
        version_expression="${{ matrix.requested_version }}",
        required_tokens=("package.json", ".version", "INPUT_VERSION", "GITHUB_OUTPUT"),
    )
    npm_publish_step = _named_step(npm_steps, "Dry-run npm publish")
    require(
        "npm publish --dry-run --access public" in str(npm_publish_step.get("run", "")),
        "npm validation job must keep publish probes dry-run only",
    )
    _assert_step_precedes_all_publish_commands(npm_steps, "Verify requested version matches package")
    npm_artifact_step = _named_step(npm_steps, "Upload package artifact")
    require(
        npm_artifact_step.get("with", {}).get("name")
        == "npm-${{ matrix.package_dir }}-${{ matrix.requested_version }}",
        "npm validation artifact must be named with package and requested version",
    )
    require(
        _step_names(npm_steps).index("Verify requested version matches package")
        < _step_names(npm_steps).index("Upload package artifact"),
        "npm version validation must precede artifact upload",
    )

    python_steps = workflow["jobs"]["python"]["steps"]
    python_version_step = _named_step(
        python_steps, "Verify requested version matches distributions"
    )
    _assert_version_step_contract(
        python_version_step,
        step_id="distribution_version",
        version_expression="${{ github.event.inputs.sdk_python_version }}",
        required_tokens=(
            "dist",
            ".whl",
            ".tar.gz",
            "METADATA",
            "PKG-INFO",
            "INPUT_VERSION",
            "GITHUB_OUTPUT",
        ),
    )
    require(
        _step_names(python_steps).index("Verify requested version matches distributions")
        < _step_names(python_steps).index("Upload Python distributions"),
        "Python version validation must precede artifact upload",
    )
    python_artifact_step = _named_step(python_steps, "Upload Python distributions")
    require(
        python_artifact_step.get("with", {}).get("name")
        == "python-sdk-${{ github.event.inputs.sdk_python_version }}",
        "Python validation artifact must be named with the requested version",
    )
    _assert_step_precedes_all_publish_commands(
        python_steps, "Verify requested version matches distributions"
    )

    for job_name, expected_needs, expected_artifact_name in (
        (
            "npm_sdk_publish",
            "npm",
            "npm-sdk-${{ github.event.inputs.sdk_version }}",
        ),
        (
            "npm_react_publish",
            "npm",
            "npm-sdk_react-${{ github.event.inputs.sdk_react_version }}",
        ),
        (
            "npm_ssr_publish",
            "npm",
            "npm-sdk_ssr-${{ github.event.inputs.sdk_ssr_version }}",
        ),
        (
            "python_publish",
            "python",
            "python-sdk-${{ github.event.inputs.sdk_python_version }}",
        ),
    ):
        publish_job = workflow["jobs"][job_name]
        require(
            publish_job.get("needs") == expected_needs,
            f"{job_name} must depend on {expected_needs} validation",
        )
        download_step_names = [
            step.get("name", "") for step in publish_job.get("steps", [])
            if "download-artifact" in str(step.get("uses", ""))
        ]
        require(len(download_step_names) == 1, f"{job_name} must download one validation artifact")
        download_step = _named_step(publish_job["steps"], download_step_names[0])
        require(
            download_step.get("with", {}).get("name") == expected_artifact_name,
            f"{job_name} must download the version-bound validation artifact",
        )

    for job_name, version_step_name in (
        ("npm", "Verify requested version matches package"),
        ("python", "Verify requested version matches distributions"),
    ):
        mutated = copy.deepcopy(workflow)
        mutated["jobs"][job_name]["steps"] = [
            step
            for step in mutated["jobs"][job_name]["steps"]
            if step.get("name") != version_step_name
        ]
        _require_assertion(
            lambda candidate=mutated: assert_requested_versions_are_bound(candidate),
            f"{job_name} version mismatch mutation was not rejected",
        )

    for job_name, step_name, artifact_name in (
        (
            "npm",
            "Upload package artifact",
            "npm-${{ matrix.package_dir }}-${{ matrix.requested_version }}",
        ),
        (
            "python",
            "Upload Python distributions",
            "python-sdk-${{ github.event.inputs.sdk_python_version }}",
        ),
    ):
        mutated = copy.deepcopy(workflow)
        artifact_step = _named_step(mutated["jobs"][job_name]["steps"], step_name)
        artifact_step["with"]["name"] = artifact_name.replace("-${{", "-artifact-${{", 1)
        _require_assertion(
            lambda candidate=mutated: assert_requested_versions_are_bound(candidate),
            f"{job_name} artifact name mutation bypassed version binding",
        )


def assert_dry_run_dispatch_is_satisfiable(workflow: dict[str, Any]) -> None:
    inputs = load_triggers(workflow)["workflow_dispatch"]["inputs"]
    expected_inputs = {
        "dry_run",
        *NPM_VERSION_INPUTS.values(),
        *NPM_CONFIRMATION_INPUTS.values(),
        PYTHON_VERSION_INPUT,
        PYTHON_CONFIRMATION_INPUT,
        GO_VERSION_INPUT,
    }
    require(set(inputs) == expected_inputs, f"unexpected dispatch inputs: {sorted(inputs)}")

    npm_matrix = workflow["jobs"]["npm"]["strategy"]["matrix"]
    matrix_entries = {
        entry["package_dir"]: entry for entry in npm_matrix.get("include", [])
    }
    require(
        set(matrix_entries) == set(NPM_MANIFESTS),
        f"unexpected npm matrix package dirs: {sorted(matrix_entries)}",
    )

    manifest_versions = npm_package_versions()
    dry_run_dispatch = {
        input_name: manifest_versions[package_dir]
        for package_dir, input_name in NPM_VERSION_INPUTS.items()
    }
    dry_run_dispatch[PYTHON_VERSION_INPUT] = python_distribution_version()
    dry_run_dispatch[GO_VERSION_INPUT] = manifest_versions["sdk"]
    dry_run_dispatch["dry_run"] = True

    for package_dir, entry in matrix_entries.items():
        version_input = NPM_VERSION_INPUTS[package_dir]
        confirmation_input = NPM_CONFIRMATION_INPUTS[package_dir]
        require(
            entry.get("requested_version")
            == f"${{{{ github.event.inputs.{version_input} }}}}",
            f"{package_dir} must consume {version_input}",
        )
        require(
            entry.get("confirmation")
            == f"${{{{ github.event.inputs.{confirmation_input} }}}}",
            f"{package_dir} must consume {confirmation_input}",
        )
        require(
            dry_run_dispatch[version_input] == manifest_versions[package_dir],
            f"{package_dir} dry-run version must match its manifest",
        )
        require(inputs[version_input].get("required") is True,
                f"{version_input} must be required")
        require(inputs[confirmation_input].get("required") is False,
                f"{confirmation_input} must be optional for dry runs")
        require(inputs[confirmation_input].get("default") == "",
                f"{confirmation_input} must default to empty")

    python_version_step = _named_step(
        workflow["jobs"]["python"]["steps"],
        "Verify requested version matches distributions",
    )
    require(
        python_version_step.get("env", {}).get("INPUT_VERSION")
        == f"${{{{ github.event.inputs.{PYTHON_VERSION_INPUT} }}}}",
        f"Python version gate must consume {PYTHON_VERSION_INPUT}",
    )
    go_version_step = _named_step(
        workflow["jobs"]["go"]["steps"], "Validate Go module tag format"
    )
    require(
        go_version_step.get("env", {}).get("INPUT_VERSION")
        == f"${{{{ github.event.inputs.{GO_VERSION_INPUT} }}}}",
        f"Go tag gate must consume {GO_VERSION_INPUT}",
    )
    require(dry_run_dispatch["dry_run"] is True, "satisfiable dispatch must be a dry run")
    for job in workflow["jobs"].values():
        for step in job.get("steps", []):
            if _step_can_publish_or_create_tag(step):
                condition = "\n".join((str(job.get("if", "")), str(step.get("if", ""))))
                require(
                    "github.event.inputs.dry_run == 'false'" in condition,
                    "satisfiable dry-run dispatch must not reach publish steps",
                )


def _assert_version_step_contract(
    step: dict[str, Any],
    *,
    step_id: str,
    version_expression: str,
    required_tokens: tuple[str, ...],
) -> None:
    require(step.get("id") == step_id, f"{step.get('name')} must expose {step_id} outputs")
    require(
        step.get("env", {}).get("INPUT_VERSION") == version_expression,
        f"{step.get('name')} must consume the requested version",
    )
    command = step.get("run", "")
    for token in required_tokens:
        require(token in command, f"{step.get('name')} must inspect {token}")


def _assert_step_precedes_all_publish_commands(
    steps: list[dict[str, Any]], version_step_name: str
) -> None:
    version_index = _step_names(steps).index(version_step_name)
    for index, step in enumerate(steps):
        step_text = "\n".join(
            str(step.get(key, "")) for key in ("name", "uses", "run")
        ).lower()
        if "publish" in step_text:
            require(version_index < index, f"{version_step_name} must precede {step.get('name')}")


def _require_assertion(check: Callable[[], None], message: str) -> None:
    try:
        check()
    except (AssertionError, ValueError):
        return
    raise AssertionError(message)


def assert_builds_precede_dry_run_publish(workflow: dict[str, Any]) -> None:
    jobs = workflow.get("jobs", {})
    npm_steps = jobs["npm"]["steps"]
    npm_order = _step_names(npm_steps)
    require(npm_order.index("Build package") < npm_order.index("Pack package"),
            "npm build must precede pack")
    require(npm_order.index("Pack package") < npm_order.index("Dry-run npm publish"),
            "npm pack must precede dry-run publish")

    python_steps = jobs["python"]["steps"]
    python_order = _step_names(python_steps)
    require(python_order.index("Test package") < python_order.index("Build wheel and sdist"),
            "Python tests must precede package build")
    require(
        python_order.index("Build wheel and sdist")
        < python_order.index("Validate Python distributions"),
        "Python build must precede distribution validation",
    )

    go_steps = "\n".join(step.get("run", "") for step in jobs["go"]["steps"])
    require("go test ./..." in go_steps, "Go job must run go test ./...")
    require(f'tag=\"{GO_TAG_PREFIX}${{INPUT_VERSION}}\"' in go_steps,
            "Go job must validate sdk_go/v<version> tag format")
    require("git tag" not in go_steps and "git push" not in go_steps,
            "Go job must not create or push tags")


def assert_go_tag_uses_semver_validation(workflow: dict[str, Any]) -> None:
    go_steps = workflow["jobs"]["go"]["steps"]
    validation_step = _named_step(go_steps, "Validate Go module tag format")
    validation_script = validation_step.get("run", "")
    require('npx --yes semver@7.7.3 "$INPUT_VERSION"' in validation_script,
            "Go tag validation must use the pinned semver validator")

    for version, expected_valid in GO_SEMVER_CASES.items():
        env = os.environ.copy()
        env["INPUT_VERSION"] = version
        result = subprocess.run(
            ["bash", "-euo", "pipefail", "-c", validation_script],
            cwd=ROOT,
            env=env,
            stdout=subprocess.DEVNULL,
            stderr=subprocess.DEVNULL,
            check=False,
        )
        require(
            (result.returncode == 0) is expected_valid,
            f"Go tag semver validation result for {version!r} was {result.returncode}",
        )


def _step_names(steps: list[dict[str, Any]]) -> list[str]:
    return [step.get("name", "") for step in steps]


def _named_step(steps: list[dict[str, Any]], name: str) -> dict[str, Any]:
    matches = [step for step in steps if step.get("name") == name]
    require(len(matches) == 1, f"expected exactly one {name!r} step")
    return matches[0]


def main() -> None:
    text = WORKFLOW_PATH.read_text(encoding="utf-8")
    workflow = load_workflow()
    checks = (
        ("manual trigger", lambda: assert_manual_dry_run_trigger_only(workflow)),
        ("first-lane identities", lambda: assert_first_lane_identities(workflow, text)),
        ("publish gates", lambda: assert_publish_steps_are_gated(workflow)),
        (
            "unknown publish job gate",
            lambda: assert_unknown_publish_job_is_rejected(workflow),
        ),
        (
            "non-uploading Python validation",
            lambda: assert_python_validation_cannot_upload(workflow),
        ),
        ("artifact version binding", lambda: assert_requested_versions_are_bound(workflow)),
        (
            "satisfiable dry-run dispatch",
            lambda: assert_dry_run_dispatch_is_satisfiable(workflow),
        ),
        ("build ordering", lambda: assert_builds_precede_dry_run_publish(workflow)),
        ("Go semantic version validation", lambda: assert_go_tag_uses_semver_validation(workflow)),
    )
    failures = []
    for check_name, check in checks:
        try:
            check()
        except (AssertionError, ValueError) as err:
            failures.append(f"{check_name}: {err}")
    require(not failures, "\n".join(failures))
    print("PASS: sdk_publish.yml publish-readiness contract assertions passed")


if __name__ == "__main__":
    try:
        main()
    except AssertionError as err:
        print(f"FAIL: {err}", file=sys.stderr)
        sys.exit(1)
