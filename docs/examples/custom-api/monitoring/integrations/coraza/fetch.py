#!/usr/bin/env python3
"""Summarize Coraza/OWASP CRS log events for Glance monitoring."""
import json, os, re
from collections import Counter
from datetime import datetime
from pathlib import Path
OUT=Path(os.getenv("OUTPUT_FILE","monitoring.json"))
LOG=Path(os.getenv("CORAZA_LOG_FILE","/var/log/caddy/caddy.log"))
KNOWN={x.strip() for x in os.getenv("CORAZA_KNOWN_HOSTS","").split(",") if x.strip()}
rr=re.compile(r'\[id "(\d+)"\]'); hr=re.compile(r'"host":"([^"]+)"'); ir=re.compile(r'"remote_ip":"([^"]+)"'); sr=re.compile(r'"status":(\d{3})')
events=[]
for line in LOG.read_text(encoding="utf-8",errors="replace").splitlines():
    if "coraza" not in line.lower() and "owasp" not in line.lower(): continue
    rule=rr.search(line)
    if not rule or rule.group(1)=="949110": continue
    h=hr.search(line); i=ir.search(line); s=sr.search(line)
    events.append({"host":h.group(1) if h else "unknown","ip":i.group(1) if i else "unknown","rule":rule.group(1),"blocked":bool(s and s.group(1)=="403")})
dedup={}
for e in events: dedup[(e["host"],e["ip"],e["blocked"])]=e
events=list(dedup.values()); detected=len(events); blocked=sum(e["blocked"] for e in events)
known=sum(e["host"] in KNOWN for e in events) if KNOWN else 0; unknown=detected-known
clients={e["ip"] for e in events}; rules=Counter(e["rule"] for e in events)
if not detected: state,message="ok","No WAF activity detected"
elif blocked: state,message="ok",f"{blocked} malicious request{'s' if blocked != 1 else ''} blocked"
else: state,message="warning",f"{detected} WAF detection{'s' if detected != 1 else ''} not blocked"
payload={"icon":"🛡️","updated":datetime.now().astimezone().strftime("%Y-%m-%d %H:%M"),"state":state,"message":message,
"metrics":[{"label":"Detected","value":str(detected),"state":"warning" if detected else "neutral"},{"label":"Blocked","value":str(blocked),"state":"ok" if blocked else "neutral"},{"label":"Known Hosts","value":str(known),"state":"neutral"},{"label":"Unknown Hosts","value":str(unknown),"state":"warning" if unknown else "neutral"},{"label":"Clients","value":str(len(clients)),"state":"neutral"},{"label":"Rules","value":str(len(rules)),"state":"neutral"}],"details":[],"expandable_label":"Recent WAF Events","expandable_items":[{"label":f"{e['host']} · {e['ip']}","value":f"{'Blocked' if e['blocked'] else 'Detected'} · Rule {e['rule']}","state":"ok" if e["blocked"] else "warning"} for e in events[-20:]]}
tmp=OUT.with_suffix(OUT.suffix+".tmp"); tmp.write_text(json.dumps(payload,indent=2,ensure_ascii=False)+"\n"); tmp.replace(OUT)
