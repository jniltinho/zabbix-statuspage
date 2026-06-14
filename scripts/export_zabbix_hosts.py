#!/usr/bin/env python3
# -*- coding: utf-8 -*-

import requests
import csv
import json
import argparse
import sys


class ZabbixAPI:
    def __init__(self, url, user, password):
        self.url = url.rstrip("/") + "/api_jsonrpc.php"
        self.user = user
        self.password = password
        self.auth_token = None
        self.request_id = 1

    def call(self, method, params=None):
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
            print(f"Erro de conexão com o Zabbix: {e}")
            sys.exit(1)

        data = response.json()

        if "error" in data:
            print(f"Erro da API Zabbix: {data['error']}")
            sys.exit(1)

        return data.get("result")

    def login(self):
        self.auth_token = self.call("user.login", {
            "user": self.user,
            "password": self.password
        })

        print("Login realizado com sucesso.")

    def get_hosts(self):
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
    return "Ativo" if str(status) == "0" else "Desativado"


def available_text(available):
    mapping = {
        "0": "Desconhecido",
        "1": "Disponível",
        "2": "Indisponível"
    }
    return mapping.get(str(available), "Desconhecido")


def export_csv(hosts, filename):
    with open(filename, "w", newline="", encoding="utf-8") as csvfile:
        writer = csv.writer(csvfile, delimiter=";")

        writer.writerow([
            "HostID",
            "Host",
            "Nome Visível",
            "Status",
            "Disponibilidade",
            "IP",
            "DNS",
            "Porta",
            "Grupos",
            "Templates",
            "Erro"
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

    print(f"Arquivo CSV gerado: {filename}")


def export_json(hosts, filename):
    with open(filename, "w", encoding="utf-8") as jsonfile:
        json.dump(hosts, jsonfile, indent=4, ensure_ascii=False)

    print(f"Arquivo JSON gerado: {filename}")


def main():
    parser = argparse.ArgumentParser(
        description="Exporta todos os hosts cadastrados no Zabbix 3.2"
    )

    parser.add_argument("--url", required=True, help="URL do Zabbix, ex: http://zabbix.local/zabbix")
    parser.add_argument("--user", required=True, help="Usuário do Zabbix")
    parser.add_argument("--password", required=True, help="Senha do Zabbix")
    parser.add_argument("--csv", default="zabbix_hosts.csv", help="Arquivo CSV de saída")
    parser.add_argument("--json", default="zabbix_hosts.json", help="Arquivo JSON de saída")

    args = parser.parse_args()

    zbx = ZabbixAPI(args.url, args.user, args.password)
    zbx.login()

    hosts = zbx.get_hosts()

    print(f"Total de hosts encontrados: {len(hosts)}")

    export_csv(hosts, args.csv)
    export_json(hosts, args.json)


if __name__ == "__main__":
    main()
