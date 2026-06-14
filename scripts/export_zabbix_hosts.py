#!/usr/bin/env python3
"""Export all Zabbix hosts to a CSV file and a JSON file for backup or migration."""

import requests
import csv
import json
import argparse
import sys


class ZabbixAPI:
    """Thin wrapper around the Zabbix JSON-RPC API."""

    def __init__(self, url):
        """Initialize the API client with the Zabbix base URL."""
        self.url = url.rstrip("/") + "/api_jsonrpc.php"
        self.auth_token = None
        self.request_id = 1

    def call(self, method, params=None):
        """Send a JSON-RPC request and return the result field."""
        payload = {
            "jsonrpc": "2.0",
            "method": method,
            "params": params or {},
            "id": self.request_id,
            "auth": self.auth_token
        }

        self.request_id += 1

        try:
            response = requests.post(
                self.url,
                json=payload,
                headers={"Content-Type": "application/json-rpc"},
                timeout=30
            )
            response.raise_for_status()
        except requests.exceptions.RequestException as e:
            print(f"Zabbix connection error: {e}")
            sys.exit(1)

        data = response.json()

        if "error" in data:
            print(f"Zabbix API error: {data['error']}")
            sys.exit(1)

        return data.get("result")

    def login(self, user, password):
        """Authenticate with username/password and store the session token."""
        self.auth_token = self.call("user.login", {
            "user": user,
            "password": password
        })

        print("Login successful.")

    def set_token(self, token):
        """Use a pre-generated API token instead of username/password login."""
        self.auth_token = token

    def get_hosts(self):
        """Return all hosts with interfaces, groups, and templates, sorted by name."""
        return self.call("host.get", {
            "output": [
                "hostid",
                "host",
                "name",
                "status",
                "available",
                "error"
            ],
            "selectInterfaces": [
                "interfaceid",
                "ip",
                "dns",
                "port",
                "type",
                "main",
                "useip"
            ],
            "selectGroups": [
                "groupid",
                "name"
            ],
            "selectParentTemplates": [
                "templateid",
                "host",
                "name"
            ],
            "sortfield": "host"
        })


def status_text(status):
    """Convert the numeric Zabbix status field to a human-readable string."""
    return "Enabled" if str(status) == "0" else "Disabled"


def available_text(available):
    """Convert the numeric Zabbix availability field to a human-readable string."""
    mapping = {
        "0": "Unknown",
        "1": "Available",
        "2": "Unavailable"
    }
    return mapping.get(str(available), "Unknown")


def export_csv(hosts, filename):
    """Write host data to a semicolon-delimited CSV file."""
    with open(filename, "w", newline="", encoding="utf-8") as csvfile:
        writer = csv.writer(csvfile, delimiter=";")

        writer.writerow([
            "HostID",
            "Host",
            "Visible Name",
            "Status",
            "Availability",
            "IP",
            "DNS",
            "Port",
            "Groups",
            "Templates",
            "Error"
        ])

        for host in hosts:
            interfaces = host.get("interfaces", [])
            main_interface = interfaces[0] if interfaces else {}

            groups = ", ".join([
                group.get("name", "")
                for group in host.get("groups", [])
            ])

            templates = ", ".join([
                template.get("name") or template.get("host", "")
                for template in host.get("parentTemplates", [])
            ])

            writer.writerow([
                host.get("hostid", ""),
                host.get("host", ""),
                host.get("name", ""),
                status_text(host.get("status")),
                available_text(host.get("available")),
                main_interface.get("ip", ""),
                main_interface.get("dns", ""),
                main_interface.get("port", ""),
                groups,
                templates,
                host.get("error", "")
            ])

    print(f"CSV file generated: {filename}")


def export_json(hosts, filename):
    """Write the raw host list to a JSON file (used as input for import_zabbix7_hosts.py)."""
    with open(filename, "w", encoding="utf-8") as jsonfile:
        json.dump(hosts, jsonfile, indent=4, ensure_ascii=False)

    print(f"JSON file generated: {filename}")


def main():
    parser = argparse.ArgumentParser(
        description="Export all registered hosts from Zabbix to CSV and JSON"
    )

    parser.add_argument("--url", required=True, help="Zabbix URL, e.g.: http://zabbix.local/zabbix")
    parser.add_argument("--user", help="Zabbix username")
    parser.add_argument("--password", help="Zabbix password")
    parser.add_argument("--api-token", help="Zabbix API token (alternative to --user/--password)")
    parser.add_argument("--csv", default="zabbix_hosts.csv", help="Output CSV file")
    parser.add_argument("--json", default="zabbix_hosts.json", help="Output JSON file")

    args = parser.parse_args()

    if not args.api_token and not (args.user and args.password):
        print("[ERROR] Provide --api-token or both --user and --password")
        sys.exit(1)

    zbx = ZabbixAPI(args.url)

    if args.api_token:
        zbx.set_token(args.api_token)
    else:
        zbx.login(args.user, args.password)

    hosts = zbx.get_hosts()

    print(f"Total hosts found: {len(hosts)}")

    export_csv(hosts, args.csv)
    export_json(hosts, args.json)


if __name__ == "__main__":
    main()
