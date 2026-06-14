#!/usr/bin/env python3

import requests
import sys

ZABBIX_URL = "http://localhost:8080/api_jsonrpc.php"
ZABBIX_USER = "Admin"
ZABBIX_PASS = "zabbix"

TAG_NAME = "output"
TAG_VALUE = "statuspage"


def api_call(auth, method, params):
    payload = {
        "jsonrpc": "2.0",
        "method": method,
        "params": params,
        "auth": auth,
        "id": 1
    }

    r = requests.post(
        ZABBIX_URL,
        json=payload,
        headers={"Content-Type": "application/json-rpc"}
    )

    data = r.json()

    if "error" in data:
        raise Exception(data["error"])

    return data["result"]


def login():
    payload = {
        "jsonrpc": "2.0",
        "method": "user.login",
        "params": {
            "username": ZABBIX_USER,
            "password": ZABBIX_PASS
        },
        "id": 1
    }

    r = requests.post(ZABBIX_URL, json=payload)
    data = r.json()

    if "error" in data:
        raise Exception(data["error"])

    return data["result"]


auth = login()

hosts = api_call(auth, "host.get", {
    "output": ["hostid", "host"],
    "selectTags": "extend"
})

print(f"Hosts encontrados: {len(hosts)}")

for host in hosts:

    hostid = host["hostid"]
    hostname = host["host"]

    tags = host.get("tags", [])

    exists = False

    for tag in tags:
        if tag["tag"] == TAG_NAME and tag["value"] == TAG_VALUE:
            exists = True
            break

    if exists:
        print(f"[SKIP] {hostname}")
        continue

    tags.append({
        "tag": TAG_NAME,
        "value": TAG_VALUE
    })

    try:
        api_call(auth, "host.update", {
            "hostid": hostid,
            "tags": tags
        })

        print(f"[OK] {hostname}")

    except Exception as e:
        print(f"[ERRO] {hostname}: {e}")

print("Finalizado.")
