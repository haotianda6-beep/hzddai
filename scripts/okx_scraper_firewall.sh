#!/usr/bin/env bash
set -euo pipefail

PORTS="18765,18766,18767,18768,18769,18770,18771,18772"

delete_rule() {
  iptables -D INPUT "$@" 2>/dev/null || true
}

delete_rule -p tcp -m multiport --dports "$PORTS" -m comment --comment okx_scraper_allow_loopback -j ACCEPT
delete_rule -p tcp -s 172.17.0.0/16 -m multiport --dports "$PORTS" -m comment --comment okx_scraper_allow_docker0 -j ACCEPT
delete_rule -p tcp -s 172.18.0.0/16 -m multiport --dports "$PORTS" -m comment --comment okx_scraper_allow_hzddai_net -j ACCEPT
delete_rule -p tcp -m multiport --dports "$PORTS" -m comment --comment okx_scraper_drop_public -j DROP

iptables -I INPUT 1 -p tcp -m multiport --dports "$PORTS" -m comment --comment okx_scraper_drop_public -j DROP
iptables -I INPUT 1 -p tcp -s 172.18.0.0/16 -m multiport --dports "$PORTS" -m comment --comment okx_scraper_allow_hzddai_net -j ACCEPT
iptables -I INPUT 1 -p tcp -s 172.17.0.0/16 -m multiport --dports "$PORTS" -m comment --comment okx_scraper_allow_docker0 -j ACCEPT
iptables -I INPUT 1 -p tcp -i lo -m multiport --dports "$PORTS" -m comment --comment okx_scraper_allow_loopback -j ACCEPT
