#!/usr/bin/env python3
"""Add one or more tags to all Zabbix hosts that do not already have them."""

import argparse
import requests
import sys


def api_call(url, auth, method, params):
    """Send a JSON-RPC request with an existing auth token and return the result."""
    payload = {
        "jsonrpc": "2.0",
        "method": method,
        "params": params,
        "auth": auth,
        "id": 1
    }

    r = requests.post(
        url,
        json=payload,
        headers={"Content-Type": "application/json-rpc"}
    )

    data = r.json()

    if "error" in data:
        raise Exception(data["error"])

    return data["result"]


def login(url, user, password):
    """Authenticate against the Zabbix API and return the session token."""
    payload = {
        "jsonrpc": "2.0",
        "method": "user.login",
        "params": {
            "username": user,
            "password": password
        },
        "id": 1
    }

    r = requests.post(url, json=payload)
    data = r.json()

    if "error" in data:
        raise Exception(data["error"])

    return data["result"]


def parse_tags(tags_str):
    """Parse a comma-separated 'key:value' string into a list of tag dicts."""
    tags = []
    for item in tags_str.split(","):
        item = item.strip()
        if not item:
            continue
        tag, _, value = item.partition(":")
        tags.append({"tag": tag.strip(), "value": value.strip()})
    return tags


def main():
    parser = argparse.ArgumentParser(
        description="Add one or more tags to all Zabbix hosts that do not already have them"
    )

    parser.add_argument("--url", required=True, help="Zabbix URL, e.g.: http://zabbix/zabbix")
    parser.add_argument("--user", help="Zabbix username")
    parser.add_argument("--password", help="Zabbix password")
    parser.add_argument("--api-token", help="Zabbix API token (alternative to --user/--password)")
    parser.add_argument(
        "--tags",
        default="output:statuspage",
        help="Tags in key:value format, separated by comma (e.g. output:statuspage,env:prod)"
    )

    args = parser.parse_args()

    if not args.api_token and not (args.user and args.password):
        print("[ERROR] Provide --api-token or both --user and --password")
        sys.exit(1)

    new_tags = parse_tags(args.tags)

    if not new_tags:
        print("[ERROR] No valid tags provided.")
        sys.exit(1)

    api_url = args.url.rstrip("/") + "/api_jsonrpc.php"

    if args.api_token:
        auth = args.api_token
    else:
        auth = login(api_url, args.user, args.password)

    hosts = api_call(url=api_url, auth=auth, method="host.get", params={
        "output": ["hostid", "host"],
        "selectTags": "extend"
    })

    print(f"Hosts found: {len(hosts)}")

    for host in hosts:
        hostid   = host["hostid"]
        hostname = host["host"]
        existing = host.get("tags", [])

        existing_set = {(t["tag"], t["value"]) for t in existing}
        to_add = [t for t in new_tags if (t["tag"], t["value"]) not in existing_set]

        if not to_add:
            print(f"[SKIP] {hostname}")
            continue

        updated_tags = existing + to_add

        try:
            api_call(url=api_url, auth=auth, method="host.update", params={
                "hostid": hostid,
                "tags": updated_tags
            })

            print(f"[OK] {hostname}")

        except Exception as e:
            print(f"[ERROR] {hostname}: {e}")

    print("Done.")


if __name__ == "__main__":
    main()
