#!/usr/bin/env python3
from pathlib import Path

services = Path.home() / "homelab/glance/config/services.yml"
home = Path.home() / "homelab/glance/config/home.yml"

svc = services.read_text()
if "nimbus.home.arpa" in svc:
    print("services.yml already has nimbus")
else:
    bookmark = (
        "                - title: ntfy\n"
        "                  url: https://ntfy.home.arpa\n"
        "                  icon: si:ntfy\n"
        "                - title: n8n\n"
        "                  url: https://n8n.home.arpa\n"
        "                  icon: si:n8n"
    )
    bookmark_new = (
        "                - title: ntfy\n"
        "                  url: https://ntfy.home.arpa\n"
        "                  icon: si:ntfy\n"
        "                - title: Nimbus\n"
        "                  url: https://nimbus.home.arpa\n"
        "                  icon: si:telegram\n"
        "                - title: n8n\n"
        "                  url: https://n8n.home.arpa\n"
        "                  icon: si:n8n"
    )
    if bookmark not in svc:
        raise SystemExit("services bookmark anchor not found")
    svc = svc.replace(bookmark, bookmark_new, 1)

    monitor = (
        "            - title: ntfy\n"
        "              url: https://ntfy.home.arpa\n"
        "              check-url: http://ntfy:80/v1/health\n"
        "              icon: si:ntfy\n"
        "            - title: n8n\n"
        "              url: https://n8n.home.arpa\n"
        "              check-url: http://n8n:5678/healthz\n"
        "              icon: si:n8n"
    )
    monitor_new = (
        "            - title: ntfy\n"
        "              url: https://ntfy.home.arpa\n"
        "              check-url: http://ntfy:80/v1/health\n"
        "              icon: si:ntfy\n"
        "            - title: Nimbus\n"
        "              url: https://nimbus.home.arpa\n"
        "              check-url: http://nimbus:8080/health\n"
        "              icon: si:telegram\n"
        "            - title: n8n\n"
        "              url: https://n8n.home.arpa\n"
        "              check-url: http://n8n:5678/healthz\n"
        "              icon: si:n8n"
    )
    if monitor not in svc:
        raise SystemExit("services monitor anchor not found")
    svc = svc.replace(monitor, monitor_new, 1)
    services.write_text(svc)
    print("services.yml updated")

hm = home.read_text()
if "nimbus.home.arpa" in hm:
    print("home.yml already has nimbus")
else:
    qb = (
        "                - title: Nextcloud\n"
        "                  url: https://nextcloud.home.arpa\n"
        "                  icon: si:nextcloud\n"
        "\n"
        "                - title: Forgejo\n"
        "                  url: https://git.home.arpa\n"
        "                  icon: si:forgejo"
    )
    qb_new = (
        "                - title: Nextcloud\n"
        "                  url: https://nextcloud.home.arpa\n"
        "                  icon: si:nextcloud\n"
        "\n"
        "                - title: Nimbus\n"
        "                  url: https://nimbus.home.arpa\n"
        "                  icon: si:telegram\n"
        "\n"
        "                - title: Forgejo\n"
        "                  url: https://git.home.arpa\n"
        "                  icon: si:forgejo"
    )
    if qb not in hm:
        raise SystemExit("home bookmark anchor not found")
    hm = hm.replace(qb, qb_new, 1)

    cm = (
        "            - title: Nextcloud\n"
        "              url: https://nextcloud.home.arpa\n"
        "              check-url: http://nextcloud:80/robots.txt\n"
        "              icon: si:nextcloud\n"
        "\n"
        "            - title: Forgejo\n"
        "              url: https://git.home.arpa\n"
        "              check-url: http://forgejo:3000/api/healthz\n"
        "              icon: si:forgejo"
    )
    cm_new = (
        "            - title: Nextcloud\n"
        "              url: https://nextcloud.home.arpa\n"
        "              check-url: http://nextcloud:80/robots.txt\n"
        "              icon: si:nextcloud\n"
        "\n"
        "            - title: Nimbus\n"
        "              url: https://nimbus.home.arpa\n"
        "              check-url: http://nimbus:8080/health\n"
        "              icon: si:telegram\n"
        "\n"
        "            - title: Forgejo\n"
        "              url: https://git.home.arpa\n"
        "              check-url: http://forgejo:3000/api/healthz\n"
        "              icon: si:forgejo"
    )
    if cm not in hm:
        raise SystemExit("home monitor anchor not found")
    hm = hm.replace(cm, cm_new, 1)
    home.write_text(hm)
    print("home.yml updated")
