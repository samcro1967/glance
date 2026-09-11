#!/usr/bin/env python3
"""Collect Tdarr failure metrics for Glance monitoring."""
import json, os
from datetime import datetime
from pathlib import Path
import requests
BASE=os.getenv("TDARR_URL","http://localhost:8265").rstrip("/"); TIMEOUT=float(os.getenv("REQUEST_TIMEOUT","15")); OUT=Path(os.getenv("OUTPUT_FILE","monitoring.json"))
def crud(collection,mode,doc_id=None):
    data={"collection":collection,"mode":mode}
    if doc_id is not None: data["docID"]=doc_id
    r=requests.post(f"{BASE}/api/v2/cruddb",json={"data":data},timeout=TIMEOUT); r.raise_for_status(); return r.json()
stats=crud("StatisticsJSONDB","getById","statistics"); transcode=int(stats.get("table3Count") or 0); health=int(stats.get("table6Count") or 0); total=transcode+health
items=[]
if total:
    records=crud("FileJSONDB","getAll")
    for x in records if isinstance(records,list) else []:
        name=str(x.get("file") or x.get("_id") or "Unknown file")
        if x.get("TranscodeDecisionMaker")=="Transcode error" or x.get("transcodeError"): items.append({"label":name,"value":"Transcode Error","state":"error"})
        if x.get("HealthCheck")=="Error" or x.get("healthCheckError"): items.append({"label":name,"value":"Healthcheck Error","state":"error"})
message=f"{total} Tdarr failure{'s' if total != 1 else ''} require attention" if total else "Tdarr healthy"
payload={"icon":"🎬","updated":datetime.now().astimezone().strftime("%Y-%m-%d %H:%M"),"state":"error" if total else "ok","message":message,"metrics":[{"label":"Transcode Errors","value":str(transcode),"state":"error" if transcode else "neutral"},{"label":"Healthcheck Errors","value":str(health),"state":"error" if health else "neutral"},{"label":"Total Errors","value":str(total),"state":"error" if total else "ok"}],"details":[],"expandable_label":"Failed Items","expandable_items":items}
tmp=OUT.with_suffix(OUT.suffix+".tmp"); tmp.write_text(json.dumps(payload,indent=2,ensure_ascii=False)+"\n"); tmp.replace(OUT)
