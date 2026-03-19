# Zonemeister

A web-based management interface for DNS zones hosted on the [Netnod Primary DNS](https://www.netnod.se/) service.

> **Proof of Concept** — This project is provided as inspiration for Netnod partners building their own management tools. It comes with no support, warranty, or guarantee of any kind. This is a personal project and is not affiliated with Netnod AB.

## Overview

Zonemeister allows a **partner** (service provider) to manage DNS zones and customers through a web interface backed by the Netnod Primary DNS API. The partner's **customers** can be given limited access to manage only their assigned zones.

The project assumes the partner uses **Netnod Primary DNS** as primary DNS and **Netnod NDS** as secondary DNS. TSIG key management and zone transfer configuration are built around this setup.

### Two-level access model

- **Partner admin** (superadmin) — Full control: create zones, manage customers, assign zones to customers, configure TSIG keys, and more.
- **Customer user** — Limited access: view and edit DNS records in zones assigned to their organization. Multiple users can belong to the same customer.

## Features

### Zone management

- Create, delete, and list DNS zones via the Netnod Primary DNS API
- Add, edit, and delete DNS records (A, AAAA, CNAME, MX, TXT, SRV, NS, PTR, SPF, ALIAS, SOA)
- Export zones in BIND format
- Send DNS NOTIFY to nameservers

### Customer and user management

- Create customer organizations with contact details
- Create multiple user accounts per customer
- Assign one or more zones to a customer
- Customers only see their own assigned zones

### TSIG key management

- Fetch available TSIG keys from the Netnod NDS API
- Associate TSIG keys with customers
- Automatically apply TSIG keys as `allow_transfer_keys` when zones are created

### DynDNS

- Enable/disable DynDNS for specific labels within a zone
- Token-based authentication for dynamic updates

### ACME DNS-01 challenges

- Enable/disable ACME DNS-01 validation for specific labels
- Supports automated certificate issuance workflows

### Security

- Password authentication with bcrypt hashing
- Optional two-factor authentication (TOTP) with QR code setup
- Session-based access with CSRF protection
- Account lockout after repeated failed login attempts
- Rate limiting on login endpoints

## Tech stack

- **Go** with [Chi](https://github.com/go-chi/chi) router
- **SQLite** or **PostgreSQL** for local data (customers, users, sessions, TSIG associations)
- **Server-side rendering** with Go `html/template`
- DNS data is stored in and served from the Netnod Primary DNS API — not locally

## Configuration

Configuration is read from environment variables. A `.env` file is loaded automatically from the working directory (existing env vars take precedence).

| Variable            | Required | Default                           | Description                                    |
| ------------------- | -------- | --------------------------------- | ---------------------------------------------- |
| `NETNOD_API_TOKEN`  | Yes      | —                                 | API token for the Netnod Primary DNS API       |
| `SESSION_SECRET`    | Yes      | —                                 | Secret for signing session cookies             |
| `NETNOD_API_URL`    | No       | `https://primarydnsapi.netnod.se` | Netnod Primary DNS API base URL                |
| `NETNOD_NDSAPI_URL` | No       | `https://dnsnodeapi.netnod.se`    | Netnod NDS API base URL (TSIG keys)            |
| `SERVER_HOST`       | No       | `localhost`                       | Listen address                                 |
| `SERVER_PORT`       | No       | `3000`                            | Listen port                                    |
| `DB_DRIVER`         | No       | `sqlite`                          | Database backend: `sqlite` or `postgres`       |
| `DB_PATH`           | No       | `data/zonemeister.db`             | Path to SQLite database file (SQLite only)     |
| `DB_URL`            | No       | —                                 | PostgreSQL connection string (PostgreSQL only) |
| `LOG_LEVEL`         | No       | `info`                            | Log level (`debug`, `info`, `warn`, `error`)   |
| `SECURE_COOKIES`    | No       | `false`                           | Set to `true` when serving over HTTPS          |
| `SMTP_HOST`         | No       | —                                 | SMTP server host (enables password reset)      |
| `SMTP_PORT`         | No       | `587`                             | SMTP server port                               |
| `SMTP_USER`         | No       | —                                 | SMTP authentication username                   |
| `SMTP_PASSWORD`     | No       | —                                 | SMTP authentication password                   |
| `SMTP_FROM`         | No       | —                                 | Sender address for outgoing emails             |
| `BASE_URL`          | No       | `http://host:port`                | Base URL for links in emails                   |

### Email (password reset)

Setting `SMTP_HOST` enables the "Forgot password?" flow on the login page. Users can request a time-limited reset link sent to their email address. The link expires after 1 hour.

```env
SMTP_HOST=smtp.example.com
SMTP_PORT=587
SMTP_USER=apikey
SMTP_PASSWORD=your-smtp-password
SMTP_FROM=noreply@example.com
BASE_URL=https://dns.example.com
```

If `SMTP_HOST` is not set, the password reset feature is hidden and disabled.

### Database

By default, the application uses **SQLite** — no setup needed. For production deployments, **PostgreSQL** is available as an alternative.

**SQLite** (default):

```env
DB_DRIVER=sqlite
DB_PATH=data/netnod.db
```

**PostgreSQL**:

```env
DB_DRIVER=postgres
DB_URL=host=localhost user=myuser password=mypass dbname=zonemeister sslmode=disable
```

(`sslmode=disable` is shown for illustration only and should NOT be used in production.)

Migrations run automatically on startup for both backends.

## Getting started

### With Docker (recommended)

A pre-built image is available from GitHub Container Registry:

```bash
docker pull ghcr.io/slideware/zonemeister:latest
```

```bash
# Start with SQLite (default)
SESSION_SECRET=change-me NETNOD_API_TOKEN=your-token docker compose up

# Or create a .env file and run
docker compose up
```

To build the image locally instead of pulling:

```bash
docker compose up --build
```

To use PostgreSQL instead of SQLite:

```bash
SESSION_SECRET=change-me \
NETNOD_API_TOKEN=your-token \
DB_DRIVER=postgres \
DB_URL="postgres://zonemeister:zonemeister@postgres:5432/zonemeister?sslmode=disable" \
docker compose --profile postgres up
```

The application is available at `http://localhost:3000`.

### Without Docker

Requires a [Go](https://go.dev/) development environment (1.25 or later).

```bash
# Build
make build

# Or run directly
make run
```

### Default credentials

On first start, a default superadmin account is created:

- **Email:** `admin@example.com`
- **Password:** `changeme`

Change these credentials immediately after first login.

## Running tests

```bash
make test
```

## License

BSD 3-Clause License — see [LICENSE](LICENSE) for details.
