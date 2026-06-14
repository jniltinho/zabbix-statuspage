#!/usr/bin/env python3
"""Import hosts from a Zabbix 3.2 JSON export into Zabbix 7 LTS.

Reads the JSON file produced by export_zabbix_hosts.py and recreates each host
in a Zabbix 7 instance, mapping legacy template names to their modern equivalents.
Supports --dry-run to preview the payload without touching the destination.
"""

import argparse
import json
import sys
import requests


# Maps Zabbix 3.2 template names to their Zabbix 7 equivalents.
TEMPLATE_MAP = {
    "Template OS Linux": "Linux by Zabbix agent",
    "Template ICMP Ping": "ICMP Ping",
    "Template App Zabbix Server": "Zabbix server health",
}


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
            "id": self.req_id,
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

    def get_host(self, hostname):
        """Return host records matching the exact technical name."""
        return self.call("host.get", {
            "output": ["hostid", "host"],
            "filter": {
                "host": [hostname]
            }
        })

    def get_group(self, name):
        """Return the host group record for the given name, or None."""
        result = self.call("hostgroup.get", {
            "output": ["groupid", "name"],
            "filter": {
                "name": [name]
            }
        })
        return result[0] if result else None

    def create_group(self, name):
        """Create a new host group and return its ID."""
        result = self.call("hostgroup.create", {
            "name": name
        })
        return result["groupids"][0]

    def get_or_create_group(self, name):
        """Return the group ID, creating the group if it does not exist."""
        group = self.get_group(name)

        if group:
            return group["groupid"]

        print(f"Creating group: {name}")
        return self.create_group(name)

    def get_template(self, name):
        """Return the template ID after applying TEMPLATE_MAP, or None if not found."""
        mapped_name = TEMPLATE_MAP.get(name, name)

        result = self.call("template.get", {
            "output": ["templateid", "host", "name"],
            "search": {
                "host": mapped_name
            }
        })

        if result:
            return result[0]["templateid"]

        result = self.call("template.get", {
            "output": ["templateid", "host", "name"],
            "search": {
                "name": mapped_name
            }
        })

        if result:
            return result[0]["templateid"]

        return None

    def create_host(self, payload):
        """Create a host with the given payload and return the API result."""
        return self.call("host.create", payload)


def build_interface(iface):
    """Convert a legacy interface dict to the Zabbix 7 interface format."""
    iface_type = int(iface.get("type", 1))

    new_iface = {
        "type": iface_type,
        "main": int(iface.get("main", 1)),
        "useip": int(iface.get("useip", 1)),
        "ip": iface.get("ip") or "127.0.0.1",
        "dns": iface.get("dns") or "",
        "port": str(iface.get("port") or ("161" if iface_type == 2 else "10050"))
    }

    if iface_type == 2:
        new_iface["details"] = {
            "version": 2,
            "bulk": 1,
            "community": "{$SNMP_COMMUNITY}"
        }

    return new_iface


def build_host_payload(zbx, old_host, default_group, snmp_community, dry_run=False):
    """Build the host.create payload from a legacy host dict."""
    hostname = old_host.get("host")
    visible_name = old_host.get("name") or hostname
    status = int(old_host.get("status", 0))

    groups = []

    for group in old_host.get("groups", []):
        group_name = group.get("name")

        if not group_name:
            continue

        if dry_run:
            groups.append({"groupid": "DRY-RUN"})
        else:
            groupid = zbx.get_or_create_group(group_name)
            groups.append({"groupid": groupid})

    if not groups:
        if dry_run:
            groups.append({"groupid": "DRY-RUN"})
        else:
            groupid = zbx.get_or_create_group(default_group)
            groups.append({"groupid": groupid})

    interfaces = []

    for iface in old_host.get("interfaces", []):
        interfaces.append(build_interface(iface))

    if not interfaces:
        interfaces.append({
            "type": 1,
            "main": 1,
            "useip": 1,
            "ip": "127.0.0.1",
            "dns": "",
            "port": "10050"
        })

    templates = []

    for template in old_host.get("parentTemplates", []):
        template_name = template.get("host") or template.get("name")

        if not template_name:
            continue

        mapped_name = TEMPLATE_MAP.get(template_name, template_name)

        if dry_run:
            templates.append({"templateid": "DRY-RUN"})
            continue

        templateid = zbx.get_template(template_name)

        if templateid:
            templates.append({"templateid": templateid})
        else:
            print(f"Warning: template not found in Zabbix 7: {template_name} -> {mapped_name}")

    payload = {
        "host": hostname,
        "name": visible_name,
        "status": status,
        "groups": groups,
        "interfaces": interfaces
    }

    if templates:
        payload["templates"] = templates

    has_snmp = any(i.get("type") == 2 for i in interfaces)

    if has_snmp:
        payload["macros"] = [
            {
                "macro": "{$SNMP_COMMUNITY}",
                "value": snmp_community
            }
        ]

    return payload


def main():
    parser = argparse.ArgumentParser(
        description="Import hosts from Zabbix 3.2 to Zabbix 7 LTS"
    )

    parser.add_argument("--url", required=True, help="Zabbix URL, e.g.: http://zabbix/zabbix")
    parser.add_argument("--user", help="Zabbix username")
    parser.add_argument("--password", help="Zabbix password")
    parser.add_argument("--api-token", help="Zabbix API token (alternative to --user/--password)")
    parser.add_argument("--file", default="zabbix_hosts.json", help="JSON file with exported hosts")
    parser.add_argument("--default-group", default="Migrated/Zabbix-3.2", help="Fallback group for hosts without a group")
    parser.add_argument("--snmp-community", default="public", help="SNMP community string")
    parser.add_argument("--skip-existing", action="store_true", help="Skip hosts that already exist")
    parser.add_argument("--dry-run", action="store_true", help="Simulate import without creating anything")

    args = parser.parse_args()

    if not args.dry_run and not args.api_token and not (args.user and args.password):
        print("[ERROR] Provide --api-token or both --user and --password")
        sys.exit(1)

    try:
        with open(args.file, "r", encoding="utf-8") as f:
            hosts = json.load(f)
    except Exception as e:
        print(f"Error reading JSON file: {e}")
        sys.exit(1)

    zbx = ZabbixAPI(args.url)

    if not args.dry_run:
        if args.api_token:
            zbx.set_token(args.api_token)
        else:
            zbx.login(args.user, args.password)

    created = 0
    skipped = 0
    errors = 0

    print(f"Total hosts in file: {len(hosts)}")

    for old_host in hosts:
        hostname = old_host.get("host")

        if not hostname:
            continue

        try:
            if not args.dry_run:
                exists = zbx.get_host(hostname)

                if exists:
                    if args.skip_existing:
                        print(f"[SKIP] Host already exists: {hostname}")
                        skipped += 1
                        continue

                    print(f"[ERROR] Host already exists: {hostname}")
                    errors += 1
                    continue

            payload = build_host_payload(
                zbx=zbx,
                old_host=old_host,
                default_group=args.default_group,
                snmp_community=args.snmp_community,
                dry_run=args.dry_run
            )

            if args.dry_run:
                print(json.dumps(payload, indent=2, ensure_ascii=False))
                continue

            result = zbx.create_host(payload)

            print(f"[OK] Host created: {hostname} ID={result['hostids'][0]}")
            created += 1

        except Exception as e:
            print(f"[ERROR] {hostname}: {e}")
            errors += 1

    print("")
    print("Summary:")
    print(f"  Created: {created}")
    print(f"  Skipped: {skipped}")
    print(f"  Errors:  {errors}")


if __name__ == "__main__":
    main()
