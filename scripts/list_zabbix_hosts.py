#!/usr/bin/env python3
"""List all Zabbix hosts in a tabular format showing ID, name, IP, and status."""

import argparse
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

        r = requests.post(
            self.url,
            json=payload,
            headers={"Content-Type": "application/json-rpc"},
            timeout=30
        )

        r.raise_for_status()
        data = r.json()

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

    def get_hosts(self):
        """Return all hosts with their groups and primary interface, sorted by name."""
        return self.call("host.get", {
            "output": ["hostid", "host", "name", "status"],
            "selectGroups": ["name"],
            "selectInterfaces": ["ip", "dns", "port", "type"],
            "sortfield": "host"
        })


def main():
    parser = argparse.ArgumentParser(
        description="List all Zabbix hosts"
    )

    parser.add_argument("--url", required=True, help="Zabbix URL, e.g.: http://zabbix/zabbix")
    parser.add_argument("--user", help="Zabbix username")
    parser.add_argument("--password", help="Zabbix password")
    parser.add_argument("--api-token", help="Zabbix API token (alternative to --user/--password)")

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

    print(f"{'HOSTID':<8} {'HOST':<40} {'IP':<18} STATUS")
    print("-" * 90)

    for host in hosts:
        ip = ""

        if host.get("interfaces"):
            ip = host["interfaces"][0].get("ip", "")

        status = "Enabled" if host["status"] == "0" else "Disabled"

        print(
            f"{host['hostid']:<8} "
            f"{host['host'][:40]:<40} "
            f"{ip:<18} "
            f"{status}"
        )

    print("")
    print(f"Total hosts: {len(hosts)}")


if __name__ == "__main__":
    main()
