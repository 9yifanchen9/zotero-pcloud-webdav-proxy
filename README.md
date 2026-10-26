# zotero-pcloud-webdav-proxy

A small WebDAV proxy that sits between Zotero and pCloud, so Zotero can sync attachment files to pCloud without being given your pCloud account.

## Why

Zotero's WebDAV file sync needs a URL, a username and a password. Pointing it straight at pCloud means:

- **Your pCloud password is stored in Zotero.** Anyone who gets it has your whole pCloud account.
- **Zotero can reach your whole drive.** pCloud's WebDAV access isn't limited to a folder.

The proxy fixes both:

- Zotero signs in to the proxy with its own username and password (`PROXY_USER` / `PROXY_PASS`). The proxy swaps in your pCloud credentials on the way upstream, so they never leave the machine running the proxy.
- Every request is confined to one pCloud folder (`PCLOUD_ROOT`). The proxy's `/` is that folder; paths that try to climb out of it (`..`, encoded separators and the like) are rejected before they reach pCloud.

## How it works

```
Zotero ──(PROXY_USER/PROXY_PASS)──▶ proxy :8080 ──(PCLOUD_USER/PCLOUD_PASS)──▶ pCloud WebDAV
           <url>/zotero/KEY.zip                        <PCLOUD_ROOT>/zotero/KEY.zip
```

- Requests without valid proxy credentials get `401` and never reach pCloud.
- Paths are mapped under `PCLOUD_ROOT`. WebDAV URLs outside the request line are mapped too: the `Destination` header on `MOVE`/`COPY`, the `Location` header, and the `href`s in `PROPFIND` (multistatus) responses. So Zotero only ever sees paths relative to the proxy.
- `GET /healthz` answers `200 ok` without credentials and without contacting pCloud, for container health checks. Every other path needs the proxy credentials.
- Each request gets an ID, returned in `X-Request-ID` and included in every log line for that request. Logs contain the method, path and status, never credentials.

## Configuration

All settings are read from environment variables; the proxy refuses to start if any required one is missing.

| Variable | Description |
| --- | --- |
| `PCLOUD_REGION` | Region your pCloud account was created in: `us` (`webdav.pcloud.com`) or `eu` (`ewebdav.pcloud.com`) |
| `PCLOUD_URL` | Optional. Full upstream URL to use instead of `PCLOUD_REGION`, e.g. a local WebDAV server for testing. Scheme and host only; set only one of the two. |
| `PCLOUD_ROOT` | pCloud folder Zotero is confined to, e.g. `/Zotero`. The proxy's `/` maps to this folder and requests cannot escape it. The drive root is not allowed. |
| `PCLOUD_USER` | pCloud account username |
| `PCLOUD_PASS` | pCloud account password |
| `PROXY_USER` | Username Zotero uses to authenticate to this proxy |
| `PROXY_PASS` | Password Zotero uses to authenticate to this proxy |
| `LISTEN_ADDR` | Optional. Address to listen on, as `host:port`. Defaults to `127.0.0.1:8080`, reachable only from the same machine. Use `0.0.0.0:8080` to listen on all interfaces, e.g. in a container. |

## Running

Requires Go 1.27 or later.

```sh
go build -o zotero-pcloud-webdav-proxy .

PCLOUD_REGION=eu \
PCLOUD_USER=you@example.com \
PCLOUD_PASS='your-pcloud-password' \
PCLOUD_ROOT=/Zotero \
PROXY_USER=zotero \
PROXY_PASS='a-long-random-password' \
./zotero-pcloud-webdav-proxy
```

Create the `PCLOUD_ROOT` folder in pCloud first. Zotero can create the `zotero` folder inside it for you.

On `SIGINT` or `SIGTERM` the proxy stops accepting connections and gives in-flight transfers up to 30 seconds to finish.

## Configuring Zotero

In Zotero, open **Settings → Sync → File Syncing** and choose **WebDAV**:

- **URL:** the proxy's address, e.g. `http://127.0.0.1:8080`. Zotero adds `/zotero/` itself, which lands in `<PCLOUD_ROOT>/zotero/` on pCloud.
- **Username / Password:** `PROXY_USER` and `PROXY_PASS`, not your pCloud credentials.

Click **Verify Server** to check the setup.

## Security notes

The proxy speaks plain HTTP, and Basic Auth sends the proxy password with every request. By default it only listens on `127.0.0.1`, so run it on the same machine as Zotero, or put it behind a TLS-terminating reverse proxy such as Caddy. Don't set `LISTEN_ADDR` to an address reachable from a network you don't trust.

## Development

```sh
gofmt -l .
go vet ./...
go test -race ./...
```
