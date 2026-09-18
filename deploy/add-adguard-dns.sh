#!/bin/bash
set -euo pipefail
CFG=/tmp/AdGuardHome.yaml
docker cp adguardhome:/opt/adguardhome/conf/AdGuardHome.yaml "$CFG"
if grep -q 'nimbus.home.arpa' "$CFG"; then
  echo dns-already-present
else
  python3 -c '
from pathlib import Path
p = Path("/tmp/AdGuardHome.yaml")
text = p.read_text()
needle = "    - domain: hello.home.arpa\n      answer: 192.168.100.49\n      enabled: true\n"
insert = needle + "    - domain: nimbus.home.arpa\n      answer: 192.168.100.49\n      enabled: true\n"
if "nimbus.home.arpa" in text:
    print("already")
elif needle not in text:
    raise SystemExit("hello rewrite not found")
else:
    p.write_text(text.replace(needle, insert, 1))
    print("rewrite inserted")
'
  docker cp "$CFG" adguardhome:/opt/adguardhome/conf/AdGuardHome.yaml
  docker restart adguardhome
  echo adguard-updated
fi
docker exec adguardhome grep -A2 nimbus.home.arpa /opt/adguardhome/conf/AdGuardHome.yaml
