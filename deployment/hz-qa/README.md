# HZ linkage QA

This Compose project runs the existing COMKUN-AI backend, frontend, HZ adapter,
event inbox, mirror engine, and Token ledger against an isolated SQLite file.
It does not mount or copy the production database.

Runtime secrets belong in `/root/.config/comkun-hz-qa/runtime.env` with mode
`0600`; QA data belongs in `/root/.local/share/comkun-hz-qa/data`.

The B event destination is:

`http://<A-host>:13000/api/integrations/hz/v1/master-events`

The reconciliation credential uses `HZ_MASTER_POLL_API_URL`,
`HZ_MASTER_POLL_API_KEY`, `HZ_MASTER_POLL_API_SECRET`, and
`HZ_MASTER_POLL_ACCOUNT_ID`. Each follower binding additionally needs the HZ
API base URL, API key, API secret, and account name. The existing binding flow
verifies AI scope, read/trade permission, MARKET-only orders, and denied
withdraw/transfer/security permissions before enabling an account.

Bootstrap an empty QA database before starting the stack:

```sh
HZ_QA_DB_PATH=/root/.local/share/comkun-hz-qa/data/data.db \
HZ_QA_ADMIN_EMAIL=... HZ_QA_ADMIN_PASSWORD=... \
go run ./cmd/hz_qa_bootstrap
```
