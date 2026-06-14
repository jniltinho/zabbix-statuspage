#!/usr/bin/env python3
# -*- coding: utf-8 -*-

import argparse
import requests
import sys


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

    def login(self):
        self.auth = self.call("user.login", {
            "username": self.user,
            "password": self.password
        })

    def get_hosts(self):
        return self.call("host.get", {
            "output": ["hostid", "host", "name", "status"],
            "selectGroups": ["name"],
            "selectInterfaces": ["ip", "dns", "port", "type"],
            "sortfield": "host"
        })


def main():
    parser = argparse.ArgumentParser(
        description="Lista todos os hosts do Zabbix"
    )

    parser.add_argument("--url", required=True)
    parser.add_argument("--user", required=True)
    parser.add_argument("--password", required=True)

    args = parser.parse_args()

    zbx = ZabbixAPI(
        args.url,
        args.user,
        args.password
    )

    zbx.login()

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
    print(f"Total de hosts: {len(hosts)}")


if __name__ == "__main__":
    main()
