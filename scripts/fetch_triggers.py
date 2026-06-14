#!/usr/bin/env python3
"""List all Zabbix triggers sorted by priority, showing status, host, and tags.

Supports authentication via API token (--api-token) or username/password
(--user / --password). To generate a token in Zabbix:
Administration > Users > [user] > API tokens > Create API token.
"""

import argparse
import json
import sys
import urllib.request


PRIORITY = {
    "0": "Not classified",
    "1": "Information",
    "2": "Warning",
    "3": "Average",
    "4": "High",
    "5": "Disaster",
}

VALUE = {"0": "OK", "1": "PROBLEM"}


def zabbix_request(api_url, auth, method, params):
    """Send a JSON-RPC request and return the result field."""
    payload = json.dumps({
        "jsonrpc": "2.0",
        "method":  method,
        "params":  params,
        "auth":    auth,
        "id":      1,
    }).encode()

    req = urllib.request.Request(
        api_url,
        data=payload,
        headers={"Content-Type": "application/json"},
    )

    with urllib.request.urlopen(req, timeout=10) as resp:
        data = json.loads(resp.read())

    if "error" in data:
        raise RuntimeError(data["error"])

    return data["result"]


def login(api_url, user, password):
    """Authenticate with username/password and return the session token."""
    payload = json.dumps({
        "jsonrpc": "2.0",
        "method":  "user.login",
        "params":  {"username": user, "password": password},
        "id":      1,
    }).encode()

    req = urllib.request.Request(
        api_url,
        data=payload,
        headers={"Content-Type": "application/json"},
    )

    with urllib.request.urlopen(req, timeout=10) as resp:
        data = json.loads(resp.read())

    if "error" in data:
        raise RuntimeError(data["error"])

    return data["result"]


def fetch_all_triggers(api_url, auth):
    """Return all triggers with hosts, tags, and last event, ordered by priority DESC."""
    return zabbix_request(api_url, auth, "trigger.get", {
        "output":          ["triggerid", "description", "priority", "value", "lastchange"],
        "selectTags":      "extend",
        "selectHosts":     ["hostid", "host", "name"],
        "selectLastEvent": "extend",
        "sortfield":       "priority",
        "sortorder":       "DESC",
        "limit":           0,
    })


def main():
    parser = argparse.ArgumentParser(
        description="List all Zabbix triggers sorted by priority"
    )

    parser.add_argument("--url", required=True, help="Zabbix URL, e.g.: http://zabbix/zabbix")
    parser.add_argument("--user", help="Zabbix username")
    parser.add_argument("--password", help="Zabbix password")
    parser.add_argument("--api-token", help="Zabbix API token (alternative to --user/--password)")

    args = parser.parse_args()

    if not args.api_token and not (args.user and args.password):
        print("[ERROR] Provide --api-token or both --user and --password")
        sys.exit(1)

    api_url = args.url.rstrip("/") + "/api_jsonrpc.php"

    if args.api_token:
        auth = args.api_token
    else:
        auth = login(api_url, args.user, args.password)

    triggers = fetch_all_triggers(api_url, auth)
    print(f"Total triggers: {len(triggers)}\n")

    for t in triggers:
        host = t["hosts"][0]["name"] if t["hosts"] else "N/A"
        tags = ", ".join(f"{tag['tag']}={tag['value']}" for tag in t.get("tags", []))
        print(
            f"[{VALUE.get(t['value'], '?'):7s}] "
            f"{PRIORITY.get(t['priority'], '?'):14s} | "
            f"{host:30s} | "
            f"{t['description']}"
        )
        if tags:
            print(f"           tags: {tags}")


if __name__ == "__main__":
    main()
