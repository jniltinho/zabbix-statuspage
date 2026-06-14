# Zabbix Management Scripts

## Dependencies

```bash
pip install requests
```

> `fetch_triggers.py` uses only the Python standard library (`urllib`) — no extra packages needed.

---

## Authentication

All scripts support two mutually exclusive authentication methods:

| Method | Arguments |
|---|---|
| Username / Password | `--user <user> --password '<pass>'` |
| API Token | `--api-token '<token>'` |

To generate an API token: **Administration → Users → [user] → API tokens → Create API token**.

---

## Overview

| Script                    | Description                                            |
|---------------------------|--------------------------------------------------------|
| `add_zabbix_host.py`      | Add host(s) via CLI arguments or CSV file              |
| `delete_zabbix_host.py`   | Remove a host by name or ID                            |
| `list_zabbix_hosts.py`    | List all hosts in tabular format                       |
| `export_zabbix_hosts.py`  | Export all hosts to CSV and JSON                       |
| `import_zabbix7_hosts.py` | Import hosts from a Zabbix 3.2 JSON into Zabbix 7     |
| `add_tag_hosts.py`        | Add one or more tags to all hosts                      |
| `fetch_triggers.py`       | List all triggers with priority and status             |

---

## add_zabbix_host.py

Adds hosts to Zabbix 7 via CLI arguments or a CSV file.

### CSV mode

**CSV format (`hosts_exemple.csv`):**

```csv
hostname,ip,port,group,template,tags,interface_type,snmp_community
srv-web01,10.0.0.10,10050,Linux servers,Linux by Zabbix agent,output:statuspage,agent,
sw-core01,10.0.0.1,161,Switches,ICMP Ping,"output:statuspage,tipo:switch",snmp,public
```

| Column           | Required | Default             | Description                                      |
|------------------|----------|---------------------|--------------------------------------------------|
| `hostname`       | Yes      | —                   | Technical hostname in Zabbix                     |
| `ip`             | Yes      | —                   | Host IP address                                  |
| `port`           | No       | `10050` / `161`     | Interface port (depends on interface type)       |
| `group`          | Yes      | —                   | Group(s) separated by comma                      |
| `template`       | Yes      | —                   | Template(s) separated by comma                   |
| `tags`           | No       | `output:statuspage` | Tags in `key:value` format, separated by comma   |
| `interface_type` | No       | `agent`             | `agent` or `snmp`                                |
| `snmp_community` | No       | `public`            | SNMP community string                            |

```bash
python3 add_zabbix_host.py \
  --url http://zabbix7/zabbix \
  --api-token 'TOKEN' \
  --csv hosts_exemple.csv
```

### Single host mode

**Example — Zabbix agent host:**

```bash
python3 add_zabbix_host.py \
  --url http://zabbix7/zabbix \
  --api-token 'TOKEN' \
  --hostname srv-web01 \
  --ip 10.0.0.10 \
  --group "Linux servers" \
  --template "Linux by Zabbix agent" \
  --tags "output:statuspage"
```

**Example — SNMP switch:**

```bash
python3 add_zabbix_host.py \
  --url http://zabbix7/zabbix \
  --api-token 'TOKEN' \
  --hostname sw-core01 \
  --ip 10.0.0.1 \
  --port 161 \
  --group "Switches" \
  --template "ICMP Ping" \
  --tags "output:statuspage,tipo:switch" \
  --interface-type snmp \
  --snmp-community public
```

**Behavior:** existing hosts are skipped (`[SKIP]`); missing groups are created automatically; missing templates produce an error and the host is skipped.

---

## delete_zabbix_host.py

Removes a specific host from Zabbix by technical name or ID.

```bash
# By name (interactive confirmation)
python3 delete_zabbix_host.py \
  --url http://zabbix7/zabbix \
  --api-token 'TOKEN' \
  --host srv-web01

# By ID, skipping confirmation
python3 delete_zabbix_host.py \
  --url http://zabbix7/zabbix \
  --api-token 'TOKEN' \
  --hostid 10423 \
  --yes
```

| Argument   | Description                                          |
|------------|------------------------------------------------------|
| `--host`   | Technical hostname (use `--host` or `--hostid`)      |
| `--hostid` | Numeric host ID                                      |
| `--yes`    | Skip interactive confirmation (`Type DELETE`)        |

---

## list_zabbix_hosts.py

Lists all registered Zabbix hosts in tabular format (`HOSTID`, `HOST`, `IP`, `STATUS`).

```bash
python3 list_zabbix_hosts.py \
  --url http://zabbix7/zabbix \
  --api-token 'TOKEN'
```

Sample output:

```
HOSTID   HOST                                     IP                 STATUS
------------------------------------------------------------------------------------------
10084    srv-web01                                10.0.0.10          Enabled
10085    sw-core01                                10.0.0.1           Enabled

Total hosts: 2
```

---

## export_zabbix_hosts.py

Exports all hosts to a CSV file (`;` delimiter) and a JSON file. Useful for backup or migration.

```bash
python3 export_zabbix_hosts.py \
  --url http://zabbix7/zabbix \
  --api-token 'TOKEN' \
  --csv zabbix_hosts.csv \
  --json zabbix_hosts.json
```

| Argument  | Default              | Description          |
|-----------|----------------------|----------------------|
| `--csv`   | `zabbix_hosts.csv`   | Output CSV file      |
| `--json`  | `zabbix_hosts.json`  | Output JSON file     |

**Exported fields:** HostID, Host, Visible Name, Status, Availability, IP, DNS, Port, Groups, Templates, Error.

---

## import_zabbix7_hosts.py

Imports hosts from a JSON file generated by `export_zabbix_hosts.py` (from a Zabbix 3.2 instance) into Zabbix 7. Applies automatic template name mapping.

**Template mapping:**

| Zabbix 3.2 Template              | Zabbix 7 Template              |
|----------------------------------|--------------------------------|
| `Template OS Linux`              | `Linux by Zabbix agent`        |
| `Template ICMP Ping`             | `ICMP Ping`                    |
| `Template App Zabbix Server`     | `Zabbix server health`         |

```bash
# Dry-run: prints the payload without creating anything (no auth required)
python3 import_zabbix7_hosts.py \
  --url http://zabbix7/zabbix \
  --file zabbix_hosts.json \
  --dry-run

# Real import, skipping hosts that already exist
python3 import_zabbix7_hosts.py \
  --url http://zabbix7/zabbix \
  --api-token 'TOKEN' \
  --file zabbix_hosts.json \
  --skip-existing \
  --snmp-community public
```

| Argument           | Default               | Description                                              |
|--------------------|-----------------------|----------------------------------------------------------|
| `--file`           | `zabbix_hosts.json`   | JSON file with exported hosts                            |
| `--default-group`  | `Migrated/Zabbix-3.2` | Fallback group for hosts with no group defined           |
| `--snmp-community` | `public`              | SNMP community string for SNMP interfaces                |
| `--skip-existing`  | —                     | Skip hosts that already exist in the destination         |
| `--dry-run`        | —                     | Simulate import without creating anything in Zabbix      |

**Recommended migration workflow:**

```bash
# 1. Export from Zabbix 3.2
python3 export_zabbix_hosts.py --url http://zabbix32/zabbix --api-token 'TOKEN_32'

# 2. Validate with dry-run (no auth required)
python3 import_zabbix7_hosts.py --url http://zabbix7/zabbix --dry-run

# 3. Import into Zabbix 7
python3 import_zabbix7_hosts.py --url http://zabbix7/zabbix --api-token 'TOKEN_7' --skip-existing
```

---

## add_tag_hosts.py

Adds one or more tags to **all** Zabbix hosts that do not already have them. Hosts where all specified tags already exist are skipped (`[SKIP]`).

```bash
# Add default tag (output:statuspage)
python3 add_tag_hosts.py \
  --url http://zabbix7/zabbix \
  --api-token 'TOKEN'

# Add multiple tags at once
python3 add_tag_hosts.py \
  --url http://zabbix7/zabbix \
  --api-token 'TOKEN' \
  --tags "output:statuspage,ambiente:producao,cliente:acme"
```

| Argument   | Default              | Description                                              |
|------------|----------------------|----------------------------------------------------------|
| `--tags`   | `output:statuspage`  | Tags in `key:value` format, separated by comma           |

---

## fetch_triggers.py

Lists all Zabbix triggers sorted by priority (Disaster → Not classified), showing status (`OK` / `PROBLEM`), priority, host, description, and tags.

```bash
python3 fetch_triggers.py \
  --url http://zabbix7/zabbix \
  --api-token 'TOKEN'

python3 fetch_triggers.py \
  --url http://zabbix7/zabbix \
  --user Admin --password 'PASSWORD'
```

Sample output:

```
Total triggers: 3

[PROBLEM ] Disaster        | srv-web01                       | /: Disk space is critically low
           tags: output=statuspage
[OK      ] Warning         | sw-core01                       | ICMP: High ICMP ping loss
[OK      ] Information     | zabbix-server                   | Zabbix agent is not available
```
