#!/usr/bin/env python3
# -*- coding: utf-8 -*-

import argparse
import json
import sys
import requests


TEMPLATE_MAP = {
    "Template OS Linux": "Linux by Zabbix agent",
    "Template ICMP Ping": "ICMP Ping",
    "Template App Zabbix Server": "Zabbix server health",
}


class ZabbixAPI:
    def __init__(self, url, user, password):
        self.url = url.rstrip("/") + "/api_jsonrpc.php"
        self.user = user
        self.password = password
        self.auth = None
        self.req_id = 1

    def call(self, method, params=None):
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

    def login(self):
        self.auth = self.call("user.login", {
            "username": self.user,
            "password": self.password
        })

    def get_host(self, hostname):
        return self.call("host.get", {
            "output": ["hostid", "host"],
            "filter": {
                "host": [hostname]
            }
        })

    def get_group(self, name):
        result = self.call("hostgroup.get", {
            "output": ["groupid", "name"],
            "filter": {
                "name": [name]
            }
        })
        return result[0] if result else None

    def create_group(self, name):
        result = self.call("hostgroup.create", {
            "name": name
        })
        return result["groupids"][0]

    def get_or_create_group(self, name):
        group = self.get_group(name)

        if group:
            return group["groupid"]

        print(f"Criando grupo: {name}")
        return self.create_group(name)

    def get_template(self, name):
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
        return self.call("host.create", payload)


def build_interface(iface):
    iface_type = int(iface.get("type", 1))

    new_iface = {
        "type": iface_type,
        "main": int(iface.get("main", 1)),
        "useip": int(iface.get("useip", 1)),
        "ip": iface.get("ip") or "127.0.0.1",
        "dns": iface.get("dns") or "",
        "port": str(iface.get("port") or ("161" if iface_type == 2 else "10050"))
    }

    # SNMP interface no Zabbix 7
    if iface_type == 2:
        new_iface["details"] = {
            "version": 2,
            "bulk": 1,
            "community": "{$SNMP_COMMUNITY}"
        }

    return new_iface


def build_host_payload(zbx, old_host, default_group, snmp_community, dry_run=False):
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
            print(f"Aviso: template não encontrado no Zabbix 7: {template_name} -> {mapped_name}")

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
        description="Importa hosts do Zabbix 3.2 para Zabbix 7 LTS"
    )

    parser.add_argument("--url", required=True)
    parser.add_argument("--user", required=True)
    parser.add_argument("--password", required=True)
    parser.add_argument("--file", default="zabbix_hosts.json")
    parser.add_argument("--default-group", default="Migrados/Zabbix-3.2")
    parser.add_argument("--snmp-community", default="public")
    parser.add_argument("--skip-existing", action="store_true")
    parser.add_argument("--dry-run", action="store_true")

    args = parser.parse_args()

    try:
        with open(args.file, "r", encoding="utf-8") as f:
            hosts = json.load(f)
    except Exception as e:
        print(f"Erro lendo arquivo JSON: {e}")
        sys.exit(1)

    zbx = ZabbixAPI(args.url, args.user, args.password)

    if not args.dry_run:
        zbx.login()

    created = 0
    skipped = 0
    errors = 0

    print(f"Total de hosts no arquivo: {len(hosts)}")

    for old_host in hosts:
        hostname = old_host.get("host")

        if not hostname:
            continue

        try:
            if not args.dry_run:
                exists = zbx.get_host(hostname)

                if exists:
                    if args.skip_existing:
                        print(f"[SKIP] Host já existe: {hostname}")
                        skipped += 1
                        continue

                    print(f"[ERRO] Host já existe: {hostname}")
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

            print(f"[OK] Host criado: {hostname} ID={result['hostids'][0]}")
            created += 1

        except Exception as e:
            print(f"[ERRO] {hostname}: {e}")
            errors += 1

    print("")
    print("Resumo:")
    print(f"  Criados: {created}")
    print(f"  Ignorados: {skipped}")
    print(f"  Erros: {errors}")


if __name__ == "__main__":
    main()
