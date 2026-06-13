#!/usr/bin/env python3
import json
import urllib.request

API_URL   = "http://localhost:8080/api_jsonrpc.php"
API_TOKEN = "373cc076b8f4c1e3b6ed7d653e49f07e"


def zabbix_request(method: str, params: dict) -> dict:
    payload = json.dumps({
        "jsonrpc": "2.0",
        "method":  method,
        "params":  params,
        "id":      1,
    }).encode()

    req = urllib.request.Request(
        API_URL,
        data=payload,
        headers={
            "Content-Type":  "application/json",
            "Authorization": f"Bearer {API_TOKEN}",
        },
    )

    with urllib.request.urlopen(req, timeout=10) as resp:
        data = json.loads(resp.read())

    if "error" in data:
        raise RuntimeError(data["error"])

    return data["result"]


def fetch_all_triggers() -> list:
    return zabbix_request("trigger.get", {
        "output":          ["triggerid", "description", "priority", "value", "lastchange"],
        "selectTags":      "extend",
        "selectHosts":     ["hostid", "host", "name"],
        "selectLastEvent": "extend",
        "sortfield":       "priority",
        "sortorder":       "DESC",
        "limit":           0,
    })


PRIORITY = {
    "0": "Not classified",
    "1": "Information",
    "2": "Warning",
    "3": "Average",
    "4": "High",
    "5": "Disaster",
}

VALUE = {"0": "OK", "1": "PROBLEM"}


def main():
    triggers = fetch_all_triggers()
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
