#!/usr/bin/env python3
"""Add one or more hosts to Zabbix 7 via CLI arguments or a CSV file."""

import argparse
import csv
import requests
import sys


class ZabbixAPI:
    """Thin wrapper around the Zabbix JSON-RPC API."""

    def __init__(self, url):
        """Initialize the API client with the Zabbix base URL."""
        self.url = url.rstrip("/") + "/api_jsonrpc.php"
        self.auth = None
        self.req_id = 1

    def call(self, method, params=None):
        """Send a JSON-RPC request and return the result field."""
        payload = {
            "jsonrpc": "2.0",
            "method": method,
            "params": params or {},
            "id": self.req_id
        }

        if self.auth:
            payload["auth"] = self.auth

        self.req_id += 1

        try:
            r = requests.post(
                self.url,
                json=payload,
                headers={"Content-Type": "application/json-rpc"},
                timeout=30
            )
            r.raise_for_status()
            data = r.json()
        except Exception as e:
            print(f"[ERROR] HTTP/API failure: {e}")
            sys.exit(1)

        if "error" in data:
            raise Exception(data["error"])

        return data["result"]

    def login(self, user, password):
        """Authenticate with username/password and store the session token."""
        self.auth = self.call("user.login", {
            "username": user,
            "password": password
        })

    def set_token(self, token):
        """Use a pre-generated API token instead of username/password login."""
        self.auth = token

    def get_host(self, hostname):
        """Return host records matching the exact technical name."""
        return self.call("host.get", {
            "output": ["hostid", "host"],
            "filter": {"host": [hostname]}
        })

    def get_group(self, group_name):
        """Return the host group record for the given name, or None."""
        result = self.call("hostgroup.get", {
            "output": ["groupid", "name"],
            "filter": {"name": [group_name]}
        })
        return result[0] if result else None

    def create_group(self, group_name):
        """Create a new host group and return its ID."""
        result = self.call("hostgroup.create", {
            "name": group_name
        })
        return result["groupids"][0]

    def get_or_create_group(self, group_name):
        """Return the group ID, creating the group if it does not exist."""
        group = self.get_group(group_name)

        if group:
            return group["groupid"]

        print(f"[INFO] Creating group: {group_name}")
        return self.create_group(group_name)

    def get_template(self, template_name):
        """Return the template ID by technical name or display name, or None."""
        result = self.call("template.get", {
            "output": ["templateid", "host", "name"],
            "filter": {"host": [template_name]}
        })

        if result:
            return result[0]["templateid"]

        result = self.call("template.get", {
            "output": ["templateid", "host", "name"],
            "filter": {"name": [template_name]}
        })

        if result:
            return result[0]["templateid"]

        return None

    def create_host(self, payload):
        """Create a host with the given payload and return the API result."""
        return self.call("host.create", payload)


def parse_csv_list(value):
    """Split a comma-separated string into a stripped list of non-empty items."""
    if not value:
        return []

    return [
        item.strip()
        for item in value.split(",")
        if item.strip()
    ]


def parse_tags(tags_str):
    """Convert a comma-separated 'key:value' string into a Zabbix tags list."""
    tags = []

    for item in parse_csv_list(tags_str):
        if ":" in item:
            tag, value = item.split(":", 1)
        else:
            tag = item
            value = ""

        tag = tag.strip()
        value = value.strip()

        if tag:
            tags.append({
                "tag": tag,
                "value": value
            })

    return tags


def build_interface(ip, port, interface_type, snmp_community):
    """Build the interface dict and macros list for the given type (agent or snmp)."""
    interface_type = interface_type or "agent"

    if interface_type == "agent":
        return {
            "type": 1,
            "main": 1,
            "useip": 1,
            "ip": ip,
            "dns": "",
            "port": str(port or "10050")
        }, []

    if interface_type == "snmp":
        return {
            "type": 2,
            "main": 1,
            "useip": 1,
            "ip": ip,
            "dns": "",
            "port": str(port or "161"),
            "details": {
                "version": 2,
                "bulk": 1,
                "community": "{$SNMP_COMMUNITY}"
            }
        }, [
            {
                "macro": "{$SNMP_COMMUNITY}",
                "value": snmp_community or "public"
            }
        ]

    raise ValueError(f"Invalid interface type: {interface_type}")


def create_one_host(zbx, item):
    """Create a single host from a dict of fields; return True on success."""
    hostname = item.get("hostname")
    ip = item.get("ip")
    port = item.get("port")
    group = item.get("group")
    template = item.get("template")
    tags = item.get("tags") or "output:statuspage"
    interface_type = item.get("interface_type") or "agent"
    snmp_community = item.get("snmp_community") or "public"

    if not hostname or not ip or not group or not template:
        print(f"[ERROR] Missing required fields: {item}")
        return False

    exists = zbx.get_host(hostname)

    if exists:
        print(f"[SKIP] Host already exists: {hostname}")
        return False

    groups = []

    for group_name in parse_csv_list(group):
        groupid = zbx.get_or_create_group(group_name)
        groups.append({"groupid": groupid})

    templates = []

    for template_name in parse_csv_list(template):
        templateid = zbx.get_template(template_name)

        if not templateid:
            print(f"[ERROR] Template not found: {template_name} / host={hostname}")
            return False

        templates.append({"templateid": templateid})

    interface, macros = build_interface(
        ip=ip,
        port=port,
        interface_type=interface_type,
        snmp_community=snmp_community
    )

    payload = {
        "host": hostname,
        "groups": groups,
        "interfaces": [interface],
        "templates": templates,
        "tags": parse_tags(tags)
    }

    if macros:
        payload["macros"] = macros

    try:
        result = zbx.create_host(payload)
        print(f"[OK] Host created: {hostname} ID={result['hostids'][0]}")
        return True

    except Exception as e:
        print(f"[ERROR] Failed to create host {hostname}: {e}")
        return False


def load_hosts_from_csv(filename):
    """Read a CSV file and return a list of host dicts."""
    hosts = []

    with open(filename, "r", encoding="utf-8-sig", newline="") as f:
        reader = csv.DictReader(f)

        for row in reader:
            hosts.append({
                "hostname": row.get("hostname", "").strip(),
                "ip": row.get("ip", "").strip(),
                "port": row.get("port", "").strip(),
                "group": row.get("group", "").strip(),
                "template": row.get("template", "").strip(),
                "tags": row.get("tags", "").strip(),
                "interface_type": row.get("interface_type", "").strip().lower(),
                "snmp_community": row.get("snmp_community", "").strip()
            })

    return hosts


def main():
    parser = argparse.ArgumentParser(
        description="Add host(s) to Zabbix 7 via arguments or CSV"
    )

    parser.add_argument("--url", required=True, help="Zabbix URL, e.g.: http://zabbix/zabbix")
    parser.add_argument("--user", help="Zabbix username")
    parser.add_argument("--password", help="Zabbix password")
    parser.add_argument("--api-token", help="Zabbix API token (alternative to --user/--password)")

    parser.add_argument("--csv", help="CSV file with hosts")

    parser.add_argument("--hostname", help="Technical hostname")
    parser.add_argument("--ip", help="Host IP address")
    parser.add_argument("--port", default="10050", help="Interface port")

    parser.add_argument("--group", help="Group or groups separated by comma")
    parser.add_argument("--template", help="Template or templates separated by comma")
    parser.add_argument("--tags", default="output:statuspage", help="Tags in key:value format")

    parser.add_argument(
        "--interface-type",
        choices=["agent", "snmp"],
        default="agent",
        help="Interface type: agent or snmp"
    )

    parser.add_argument("--snmp-community", default="public", help="SNMP community string")

    args = parser.parse_args()

    if not args.api_token and not (args.user and args.password):
        print("[ERROR] Provide --api-token or both --user and --password")
        sys.exit(1)

    zbx = ZabbixAPI(args.url)

    if args.api_token:
        zbx.set_token(args.api_token)
    else:
        zbx.login(args.user, args.password)

    if args.csv:
        hosts = load_hosts_from_csv(args.csv)
    else:
        hosts = [
            {
                "hostname": args.hostname,
                "ip": args.ip,
                "port": args.port,
                "group": args.group,
                "template": args.template,
                "tags": args.tags,
                "interface_type": args.interface_type,
                "snmp_community": args.snmp_community
            }
        ]

    total = len(hosts)
    created = 0
    skipped_or_error = 0

    for item in hosts:
        if create_one_host(zbx, item):
            created += 1
        else:
            skipped_or_error += 1

    print("")
    print("Summary:")
    print(f"  Total:          {total}")
    print(f"  Created:        {created}")
    print(f"  Skipped/Errors: {skipped_or_error}")


if __name__ == "__main__":
    main()
