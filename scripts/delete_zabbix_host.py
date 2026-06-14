#!/usr/bin/env python3
"""Delete a specific host from Zabbix 7 by technical name or host ID."""

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

    def find_host_by_name(self, hostname):
        """Return host records matching the exact technical name."""
        return self.call("host.get", {
            "output": ["hostid", "host", "name", "status"],
            "filter": {
                "host": [hostname]
            }
        })

    def find_host_by_id(self, hostid):
        """Return host records matching the given host ID."""
        return self.call("host.get", {
            "output": ["hostid", "host", "name", "status"],
            "hostids": [hostid]
        })

    def delete_host(self, hostid):
        """Permanently delete the host with the given ID."""
        return self.call("host.delete", [hostid])


def main():
    parser = argparse.ArgumentParser(
        description="Delete a specific host from Zabbix 7"
    )

    parser.add_argument("--url", required=True, help="Zabbix URL, e.g.: http://zabbix/zabbix")
    parser.add_argument("--user", help="Zabbix username")
    parser.add_argument("--password", help="Zabbix password")
    parser.add_argument("--api-token", help="Zabbix API token (alternative to --user/--password)")

    parser.add_argument("--host", help="Technical hostname")
    parser.add_argument("--hostid", help="Host ID")
    parser.add_argument("--yes", action="store_true", help="Skip confirmation prompt")

    args = parser.parse_args()

    if not args.api_token and not (args.user and args.password):
        print("[ERROR] Provide --api-token or both --user and --password")
        sys.exit(1)

    if not args.host and not args.hostid:
        print("[ERROR] Provide --host or --hostid")
        sys.exit(1)

    zbx = ZabbixAPI(args.url)

    if args.api_token:
        zbx.set_token(args.api_token)
    else:
        zbx.login(args.user, args.password)

    if args.hostid:
        hosts = zbx.find_host_by_id(args.hostid)
    else:
        hosts = zbx.find_host_by_name(args.host)

    if not hosts:
        print("[ERROR] Host not found.")
        sys.exit(1)

    host = hosts[0]

    print("Host found:")
    print(f"  ID:     {host['hostid']}")
    print(f"  Host:   {host['host']}")
    print(f"  Name:   {host['name']}")
    print(f"  Status: {'Enabled' if str(host['status']) == '0' else 'Disabled'}")
    print("")

    if not args.yes:
        confirm = input("Type DELETE to confirm: ")

        if confirm != "DELETE":
            print("Cancelled.")
            sys.exit(0)

    result = zbx.delete_host(host["hostid"])

    print(f"[OK] Host successfully deleted: {host['host']}")
    print(f"Removed ID: {result['hostids'][0]}")


if __name__ == "__main__":
    main()
