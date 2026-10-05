#!/usr/bin/env python3
"""Record job duration, measured queue time and retries from Actions metadata."""
import argparse
import collections
import datetime
import json
import pathlib


def seconds(start, end):
    if not start or not end:
        return None
    return max(0, int((datetime.datetime.fromisoformat(end.replace("Z", "+00:00")) - datetime.datetime.fromisoformat(start.replace("Z", "+00:00"))).total_seconds()))


def collect(pages, root):
    jobs = [job for page in pages for job in page.get("jobs", [])]
    rows = [{"name": job["name"], "job_id": job["id"], "attempt": job.get("run_attempt"), "outcome": job.get("conclusion"), "duration_seconds": seconds(job.get("started_at"), job.get("completed_at")), "queue_seconds": seconds(job.get("created_at"), job.get("started_at"))} for job in jobs]
    recurrence = collections.Counter()
    for family in ["gate-evidence", "v0421-ha-evidence", "reference-journey-evidence", "superseded-evidence"]:
        for file in (root / family).glob("**/assertion-summary.json"):
            if file.is_symlink():
                continue
            data = json.loads(file.read_text())
            recurrence[(data["classification"], data["failure_key"])] += 1
    return {"jobs": rows, "slowest_jobs": sorted([row for row in rows if row["duration_seconds"] is not None], key=lambda row: row["duration_seconds"], reverse=True)[:10], "failure_recurrence": [{"classification": key[0], "failure_key": key[1], "count": count} for key, count in recurrence.most_common()], "queue_note": "Queue time is reported only when Actions supplies a job creation timestamp; missing values are unknown, not zero."}


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("jobs", type=pathlib.Path)
    parser.add_argument("root", type=pathlib.Path)
    parser.add_argument("output", type=pathlib.Path)
    args = parser.parse_args()
    result = collect(json.loads(args.jobs.read_text()), args.root)
    args.output.parent.mkdir(parents=True, exist_ok=True)
    args.output.write_text(json.dumps(result, indent=2) + "\n")
    print("\n### Longest CI jobs\n\n| Job | Attempt | Runtime | Queue | Result |\n| --- | --- | --- | --- | --- |")
    for row in result["slowest_jobs"]:
        name = row["name"].replace("|", "/").replace("\n", " ")
        queue = "unknown" if row["queue_seconds"] is None else f'{row["queue_seconds"]} s'
        print(f'| {name} | {row["attempt"]} | {row["duration_seconds"]} s | {queue} | {row["outcome"]} |')
    print("\n" + result["queue_note"])
    if result["failure_recurrence"]:
        print("\nRepeated failure classes in retained attempts:")
        for item in result["failure_recurrence"]:
            print(f'- {item["classification"]} / {item["failure_key"]}: {item["count"]}')


if __name__ == "__main__":
    main()
