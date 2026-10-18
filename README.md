# zotero-pcloud-webdav-proxy
WebDAV proxy sitting between Zotero and pCloud

## Configuration

All settings are read from environment variables; the proxy refuses to start if any required one is missing.

| Variable      | Description                                                                      |
|---------------|----------------------------------------------------------------------------------|
| `PCLOUD_REGION` | Region your pCloud account was created in: `us` (`webdav.pcloud.com`) or `eu` (`ewebdav.pcloud.com`) |
| `PCLOUD_URL`  | Optional. Full upstream URL to use instead of `PCLOUD_REGION`, e.g. a local WebDAV server for testing. Scheme and host only; set only one of the two. |
| `PCLOUD_ROOT` | pCloud folder Zotero is confined to, e.g. `/Zotero`. The proxy's `/` maps to this folder and requests cannot escape it. The drive root is not allowed. |
| `PCLOUD_USER` | pCloud account username                                                          |
| `PCLOUD_PASS` | pCloud account password                                                          |
| `PROXY_USER`  | Username Zotero uses to authenticate to this proxy                               |
| `PROXY_PASS`  | Password Zotero uses to authenticate to this proxy                               |
