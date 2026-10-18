# zotero-pcloud-webdav-proxy
WebDAV proxy sitting between Zotero and pCloud

## Configuration

All settings are read from environment variables; the proxy refuses to start if any are missing.

| Variable      | Description                                                                      |
|---------------|----------------------------------------------------------------------------------|
| `PCLOUD_URL`  | pCloud WebDAV endpoint, e.g. `https://webdav.pcloud.com` (US) or `https://ewebdav.pcloud.com` (EU) |
| `PCLOUD_USER` | pCloud account username                                                          |
| `PCLOUD_PASS` | pCloud account password                                                          |
| `PROXY_USER`  | Username Zotero uses to authenticate to this proxy                               |
| `PROXY_PASS`  | Password Zotero uses to authenticate to this proxy                               |
