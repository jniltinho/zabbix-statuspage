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
            print(f"[ERRO] Falha HTTP/API: {e}")
            sys.exit(1)

        if "error" in data:
            raise Exception(data["error"])

        return data["result"]

    def login(self):
        self.auth = self.call("user.login", {
            "username": self.user,
            "password": self.password
        })

    def find_host_by_name(self, hostname):
        return self.call("host.get", {
            "output": ["hostid", "host", "name", "status"],
            "filter": {
                "host": [hostname]
            }
        })

    def find_host_by_id(self, hostid):
        return self.call("host.get", {
            "output": ["hostid", "host", "name", "status"],
            "hostids": [hostid]
        })

    def delete_host(self, hostid):
        return self.call("host.delete", [hostid])


def main():
    parser = argparse.ArgumentParser(
        description="Deleta um host específico no Zabbix 7"
    )

    parser.add_argument("--url", required=True, help="URL do Zabbix, ex: http://zabbix/zabbix")
    parser.add_argument("--user", required=True, help="Usuário do Zabbix")
    parser.add_argument("--password", required=True, help="Senha do Zabbix")

    parser.add_argument("--host", help="Nome técnico do host")
    parser.add_argument("--hostid", help="ID do host")
    parser.add_argument("--yes", action="store_true", help="Confirma exclusão sem perguntar")

    args = parser.parse_args()

    if not args.host and not args.hostid:
        print("Use --host ou --hostid")
        sys.exit(1)

    zbx = ZabbixAPI(args.url, args.user, args.password)
    zbx.login()

    if args.hostid:
        hosts = zbx.find_host_by_id(args.hostid)
    else:
        hosts = zbx.find_host_by_name(args.host)

    if not hosts:
        print("[ERRO] Host não encontrado.")
        sys.exit(1)

    host = hosts[0]

    print("Host encontrado:")
    print(f"  ID:     {host['hostid']}")
    print(f"  Host:   {host['host']}")
    print(f"  Nome:   {host['name']}")
    print(f"  Status: {'Ativo' if str(host['status']) == '0' else 'Desativado'}")
    print("")

    if not args.yes:
        confirm = input("Digite DELETE para confirmar: ")

        if confirm != "DELETE":
            print("Cancelado.")
            sys.exit(0)

    result = zbx.delete_host(host["hostid"])

    print(f"[OK] Host deletado com sucesso: {host['host']}")
    print(f"ID removido: {result['hostids'][0]}")


if __name__ == "__main__":
    main()
