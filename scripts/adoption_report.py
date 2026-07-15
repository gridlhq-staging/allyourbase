"""Validate adoption API responses and render the classified Markdown report."""

import json
import os
import re
from datetime import datetime


def fail(source, message):
    raise SystemExit(f"{source}: {message}")


def load_json(path, source):
    try:
        with open(path, encoding="utf-8") as handle:
            return json.load(handle)
    except Exception as exc:
        fail(source, f"malformed JSON: {exc}")


def require_object(value, source):
    if not isinstance(value, dict):
        fail(source, "expected object")
    return value


def require_array(value, source):
    if not isinstance(value, list):
        fail(source, "expected array")
    return value


def non_negative_int(value, source, field):
    if not isinstance(value, int) or isinstance(value, bool) or value < 0:
        fail(source, f"{field} must be a non-negative integer")
    return value


def required_string(value, source, field):
    if not isinstance(value, str) or not value:
        fail(source, f"{field} must be a non-empty string")
    return value


def utc_timestamp(value, source, field):
    timestamp = required_string(value, source, field)
    if not re.fullmatch(
        r"[0-9]{4}-[0-9]{2}-[0-9]{2}T[0-9]{2}:[0-9]{2}:[0-9]{2}Z", timestamp
    ):
        fail(source, f"{field} must be a UTC timestamp")
    try:
        datetime.strptime(timestamp, "%Y-%m-%dT%H:%M:%SZ")
    except ValueError:
        fail(source, f"{field} must be a UTC timestamp")
    return timestamp


def calendar_date(value, source, field):
    date_value = required_string(value, source, field)
    if not re.fullmatch(r"[0-9]{4}-[0-9]{2}-[0-9]{2}", date_value):
        fail(source, f"{field} must be a valid YYYY-MM-DD date")
    try:
        datetime.strptime(date_value, "%Y-%m-%d")
    except ValueError:
        fail(source, f"{field} must be a valid YYYY-MM-DD date")
    return date_value


def traffic_metric(path, source, series_field):
    payload = require_object(load_json(path, source), source)
    series = require_array(payload.get(series_field), source)
    if not series:
        fail(source, f"{series_field} must contain at least one dated metric")
    timestamps = []
    for index, metric in enumerate(series):
        metric = require_object(metric, source)
        timestamp_field = f"{series_field}[{index}].timestamp"
        timestamps.append(utc_timestamp(metric.get("timestamp"), source, timestamp_field))
        non_negative_int(metric.get("count"), source, f"{series_field}[{index}].count")
        non_negative_int(metric.get("uniques"), source, f"{series_field}[{index}].uniques")
    return (
        non_negative_int(payload.get("count"), source, "count"),
        non_negative_int(payload.get("uniques"), source, "uniques"),
        min(timestamps),
        max(timestamps),
    )


def repository_counts(path):
    source = "github repository"
    payload = require_object(load_json(path, source), source)
    return (
        non_negative_int(payload.get("stargazers_count"), source, "stargazers_count"),
        non_negative_int(payload.get("forks_count"), source, "forks_count"),
        non_negative_int(payload.get("open_issues_count"), source, "open_issues_count"),
    )


def referrer_rows(path):
    source = "github referrers"
    rows = []
    for row in require_array(load_json(path, source), source):
        row = require_object(row, source)
        rows.append(
            (
                required_string(row.get("referrer"), source, "referrer"),
                non_negative_int(row.get("count"), source, "count"),
                non_negative_int(row.get("uniques"), source, "uniques"),
            )
        )
    return rows


def release_details(path, selected_tag):
    source = "github release view"
    payload = require_object(load_json(path, source), source)
    tag = required_string(payload.get("tagName"), source, "tagName")
    if tag != selected_tag:
        fail(source, f"tagName expected {selected_tag} got {tag}")
    published_at = utc_timestamp(payload.get("publishedAt"), source, "publishedAt")
    assets = require_array(payload.get("assets"), source)
    rows = []
    total = 0
    for asset in assets:
        asset = require_object(asset, source)
        name = required_string(asset.get("name"), source, "asset.name")
        downloads = non_negative_int(asset.get("downloadCount"), source, "asset.downloadCount")
        rows.append((name, downloads))
        total += downloads
    return tag, published_at, rows, total


def npm_details(path, expected_package):
    source = "npm downloads"
    payload = require_object(load_json(path, source), source)
    package = required_string(payload.get("package"), source, "package")
    if package != expected_package:
        fail(source, f"package expected {expected_package} got {package}")
    downloads = non_negative_int(payload.get("downloads"), source, "downloads")
    start = calendar_date(payload.get("start"), source, "start")
    end = calendar_date(payload.get("end"), source, "end")
    if start > end:
        fail(source, "start must not be after end")
    return downloads, start, end, package


def plural(noun, count):
    return noun if count == 1 else f"{noun}s"


def append_referrers(lines, rows):
    if not rows:
        lines.append("Referrers: none")
        return
    lines.extend(["Referrers:", "", "| Source | Count | Unique |", "| --- | ---: | ---: |"])
    for source, count, uniques in rows:
        lines.append(f"| {source} | {count} | {uniques} |")


def append_assets(lines, rows):
    lines.extend(["", "| Asset | Downloads |", "| --- | ---: |"])
    for name, downloads in rows:
        lines.append(f"| {name} | {downloads} |")


def cloudflare_lines():
    if os.environ.get("CLOUDFLARE_TOKEN_PRESENT") and os.environ.get(
        "CLOUDFLARE_ACCOUNT_PRESENT"
    ):
        status = "Cloudflare credentials are configured, but Cloudflare analytics are not collected by this stage."
    else:
        status = "Cloudflare analytics were not collected because optional credentials are unavailable."
    return [
        status,
        "Endpoint: POST https://api.cloudflare.com/client/v4/graphql",
        "Required environment: CLOUDFLARE_API_TOKEN with Account > Account Analytics > Read on the relevant account",
        "Required environment: CLOUDFLARE_ACCOUNT_ID",
        "Implementation owner: scripts/adoption.sh",
    ]


def build_report():
    views, unique_views, views_start, views_end = traffic_metric(
        os.environ["VIEWS_JSON"], "github views", "views"
    )
    clones, unique_cloners, clones_start, clones_end = traffic_metric(
        os.environ["CLONES_JSON"], "github clones", "clones"
    )
    referrers = referrer_rows(os.environ["REFERRERS_JSON"])
    stars, forks, open_issues = repository_counts(os.environ["REPOSITORY_JSON"])
    tag, published_at, assets, release_total = release_details(
        os.environ["RELEASE_VIEW_JSON"], os.environ["SELECTED_RELEASE_TAG"]
    )
    npm_downloads, npm_start, npm_end, npm_package = npm_details(
        os.environ["NPM_JSON"], os.environ["ADOPTION_NPM_PACKAGE"]
    )
    lines = [
        "# Adoption Signal Baseline",
        "",
        f"Repository: {os.environ['ADOPTION_REPOSITORY']}",
        f"Summary: {views} human {plural('view', views)}, {stars} {plural('star', stars)}, {len(referrers)} {plural('referrer', len(referrers))}; {clones} clones are our own CI.",
        "",
        "## Human signals",
        "",
        f"GitHub views: total={views} unique={unique_views}",
        f"GitHub views period: {views_start} through {views_end}",
        f"Stars: {stars}",
        f"Forks: {forks}",
        f"Open issues: {open_issues}",
    ]
    append_referrers(lines, referrers)
    lines.extend(
        [
            "",
            "## CI-dominated / not adoption",
            "",
            f"GitHub clones: total={clones} unique={unique_cloners} (project CI; not human adoption)",
            f"GitHub clones period: {clones_start} through {clones_end}",
            f"Selected app release: {tag} published={published_at}",
            f"Release asset downloads total: {release_total}",
        ]
    )
    append_assets(lines, assets)
    lines.extend(
        [
            "",
            "## Ambiguous npm activity",
            "",
            f"npm {npm_package} downloads: {npm_downloads} from {npm_start} through {npm_end}",
            "Project CI installs the SDK, so these downloads are ambiguous and not counted as human adoption.",
            "",
            "## Optional Cloudflare analytics",
            "",
        ]
    )
    lines.extend(cloudflare_lines())
    lines.append("")
    return lines


def main():
    with open(os.environ["OUTPUT_TMP"], "w", encoding="utf-8") as handle:
        handle.write("\n".join(build_report()))


if __name__ == "__main__":
    main()
